// Plant monitor REST API for the XIAO ESP32-C3 (TinyGo).
// Flash: tinygo flash -target=xiao-esp32c3 -monitor .
// Then open http://<IP-from-monitor>:8080/api/readings
package main

import (
	"machine"
	"strconv"
	"time"

	"github.com/soypat/lneto/http/httphi"
	"tinygo.org/x/drivers/bme280"
	"tinygo.org/x/drivers/netdev"
	nl "tinygo.org/x/drivers/netlink"
	link "tinygo.org/x/espradio/netlink"
)

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

	var router httphi.Router
	cfg := httphi.DefaultRouterConfig(1, 2048, routes.MaxPathValues())
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
			println("Open: http://" + ip.String() + ":8080/api/readings")
			time.Sleep(5 * time.Second)
		}
	}()

	// Serve until the listener fails, then report the error.
	err := radio.ListenAndServe(&router, 8080)
	println("HTTP server stopped:", err.Error())
}
