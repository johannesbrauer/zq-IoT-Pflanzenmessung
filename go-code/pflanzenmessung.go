// Plant monitor REST API for the XIAO ESP32-C3 (TinyGo).
// Flash: tinygo flash -target=xiao-esp32c3 -monitor .
// Then open http://<IP-from-monitor>:8080/api/readings
package main

import (
	"context"
	"errors"
	"machine"
	"net"
	"net/netip"
	"strconv"
	"time"

	"github.com/soypat/lneto"
	"github.com/soypat/lneto/http/httphi"
	"github.com/soypat/lneto/x/xnet"
	"tinygo.org/x/drivers/bme280"
	"tinygo.org/x/drivers/netdev"
	nl "tinygo.org/x/drivers/netlink"
	link "tinygo.org/x/espradio/netlink"
)

// httpConnDeadline fails a peer that opens a connection and then stalls,
// instead of it holding a router worker forever.
const httpConnDeadline = 10 * time.Second

func main() {
	// Give the serial monitor time to connect before printing.
	time.Sleep(2 * time.Second)

	// --- Settings: change these ---
	ssid := "MikroTik-AF2E92"
	password := "" // empty = open WiFi

	// Soil calibration: raw ADC value in air (dry) and in water (wet).
	// Read "soil_raw" once, then update these if the sensor changes.
	soilDry := 50000
	soilWet := 20000

	// Set to false for sensors we don't have yet; the fallback values are used.
	useBME := true
	useSoil := false

	// Fallback readings used when a sensor is missing or fails.
	fallbackTemp := -23.4   // °C
	fallbackHum := -51.2    // %
	fallbackPres := -1013.0 // hPa
	fallbackRaw := -35000   // soil ADC value (0-65535)

	// BME280 on I2C: SDA=D1, SCL=D0.
	i2c := machine.I2C0
	i2c.Configure(machine.I2CConfig{SDA: machine.D1, SCL: machine.D0})

	bme := bme280.New(i2c)
	bme.Configure()

	// Soil sensor on D3 (analog).
	machine.InitADC()
	soil := machine.ADC{Pin: machine.D3}
	soil.Configure(machine.ADCConfig{})

	// HTTP route. httphi builds the response without net/http allocations.
	//TODO: Read httphi docu
	var routes httphi.MuxSlice
	routes.Handle("/api/readings", func(exch *httphi.Exchange) {
		// Use real sensors when enabled and working, otherwise the fallbacks.
		temp, hum, pres := fallbackTemp, fallbackHum, fallbackPres
		if useBME {
			if t, err := bme.ReadTemperature(); err == nil {
				temp = float64(t) / 1000
			}
			if h, err := bme.ReadHumidity(); err == nil {
				hum = float64(h) / 100
			}
			if p, err := bme.ReadPressure(); err == nil {
				pres = float64(p) / 100000
			}
		}

		raw := fallbackRaw
		if useSoil {
			raw = int(soil.Get())
		}

		// Map raw to 0-100 %: soilDry -> 0 %, soilWet -> 100 %.
		pct := (soilDry - raw) * 100 / (soilDry - soilWet)
		if pct < 0 {
			pct = 0
		}
		if pct > 100 {
			pct = 100
		}

		// Build JSON in a small fixed buffer (no heap allocation).
		var buf [192]byte
		b := buf[:0]
		b = append(b, `{"temperature_c":`...)
		b = strconv.AppendFloat(b, temp, 'f', 1, 64)
		b = append(b, `,"humidity_pct":`...)
		b = strconv.AppendFloat(b, hum, 'f', 1, 64)
		b = append(b, `,"pressure_hpa":`...)
		b = strconv.AppendFloat(b, pres, 'f', 1, 64)
		b = append(b, `,"soil_moisture_pct":`...)
		b = strconv.AppendInt(b, int64(pct), 10)
		b = append(b, `,"soil_raw":`...)
		b = strconv.AppendInt(b, int64(raw), 10)
		b = append(b, `,"needs_water":`...)
		b = strconv.AppendBool(b, pct < 30) // true below 30 % moisture
		b = append(b, '}')
		exch.Respond(httphi.StatusOK, "application/json", b)
	})
	// Serve the web UI (webui.go) for any other path.
	routes.Handle("/{$}", serveWebUI)

	var router httphi.Router
	cfg := httphi.DefaultRouterConfig(4, 2048, routes.MaxPathValues())
	if err := router.Configure(&routes, cfg); err != nil {
		println("router config failed:", err.Error())
		return
	}

	// WiFi setup. ArenaPoolSize is the memory the network stack may use.
	radio := link.Esplink{ArenaPoolSize: 48 * 1024}
	netdev.UseNetdev(&radio)

	// Retry forever until WiFi connects.
	for radio.NetConnect(&nl.ConnectParams{Ssid: ssid, Passphrase: password}) != nil {
		println("WiFi failed, retrying in 5s...")
		time.Sleep(5 * time.Second)
	}

	// Wait for DHCP to hand out a real IP.
	ip, _ := radio.Addr()
	for !ip.IsValid() || ip.IsUnspecified() {
		time.Sleep(1 * time.Second)
		ip, _ = radio.Addr()
	}

	// Print the URL every 5 s; this loop also keeps the program alive.
	go func() {
		for {
			println("Open: http://" + ip.String() + ":8080/  (API: /api/readings)")
			time.Sleep(5 * time.Second)
		}
	}()

	// Serve until the listener fails, then report the error.
	err := listenAndServe(&radio, &router, 8080)
	println("HTTP server stopped:", err.Error())
}

// listenAndServe is Esplink.ListenAndServe with a per-connection deadline.
// Without it an idle peer holds a router worker forever, so the browser's
// page or /api/readings request is never served.
func listenAndServe(radio *link.Esplink, router *httphi.Router, port uint16) error {
	stack := radio.StackAsync().StackGo(pollBackoff, xnet.StackGoConfig{
		ListenerPoolConfig: xnet.TCPPoolConfig{
			PoolSize:           4,
			QueueSize:          4,
			TxBufSize:          len(webUIHTML) + 128,
			RxBufSize:          1024,
			EstablishedTimeout: 5 * time.Second,
			ClosingTimeout:     5 * time.Second,
			NewBackoff:         func() lneto.BackoffStrategy { return pollBackoff },
		},
	})
	laddr := netip.AddrPortFrom(netip.Addr{}, port)
	sock, err := stack.SocketNetip(context.Background(), "tcp4", xnet.AF_INET, xnet.SOCK_STREAM, laddr, netip.AddrPort{})
	if err != nil {
		return err
	}
	listener, ok := sock.(net.Listener)
	if !ok {
		return errors.New("stack returned non-listener socket")
	}
	defer listener.Close()
	for {
		conn, err := listener.Accept()
		if err != nil {
			return err
		}
		conn.SetDeadline(time.Now().Add(httpConnDeadline))
		err = router.Handle(conn)
		if err != nil {
			conn.Close()
			if err == net.ErrClosed {
				return err
			}
			println("HTTP router dropped connection:", err.Error())
		}
	}
}

func pollBackoff(uint) time.Duration { return 5 * time.Millisecond }
