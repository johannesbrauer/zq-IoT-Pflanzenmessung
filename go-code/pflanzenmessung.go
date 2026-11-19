// Plant monitor REST API for the XIAO ESP32-C3 (TinyGo).
// Flash: tinygo flash -target esp32c3-generic -monitor .
// Then open http://<IP-from-monitor>:8080/api/readings
package main

import (
	"io"
	"machine"
	"net/http"
	"strconv"
	"time"

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
	fallbackTemp := -40.4   // °C
	fallbackHum := -51.2    // %
	fallbackPres := -1013.0 // hPa
	fallbackRaw := -35000   // soil ADC value (0-65535)

	// BME280 on I2C: D4=GPIO6 (SDA), D5=GPIO7 (SCL).
	i2c := machine.I2C0
	i2c.Configure(machine.I2CConfig{SDA: machine.D1, SCL: machine.D0})

	bme := bme280.New(i2c)
	bme.Configure()

	// Soil sensor on D0=GPIO2 (analog).
	machine.InitADC()
	soil := machine.ADC{Pin: machine.D3}
	soil.Configure(machine.ADCConfig{})

	http.HandleFunc("/api/readings", func(w http.ResponseWriter, r *http.Request) {
		// Use real sensors when enabled and working, otherwise use the fallbacks.
		// Units are converted from the driver: milli-degrees -> °C,
		// hundredths of % -> %, milli-pascal -> hPa.
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

		// Clamp to 0-100 because readings can be slightly out of range.
		if pct < 0 {
			pct = 0
		}
		if pct > 100 {
			pct = 100
		}

		// Helper: number -> text with 1 decimal (23.4 -> "23.4").
		f := func(x float64) string { return strconv.FormatFloat(x, 'f', 1, 64) }

		// Send JSON. "needs_water" is true below 30 % soil moisture
		// (change 30 to adjust).
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"temperature_c":`+f(temp)+
			`,"humidity_pct":`+f(hum)+
			`,"pressure_hpa":`+f(pres)+
			`,"soil_moisture_pct":`+strconv.Itoa(pct)+
			`,"soil_raw":`+strconv.Itoa(raw)+
			`,"needs_water":`+strconv.FormatBool(pct < 30)+`}`)
	})

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

	// Run the server in the background so the loop below can print the IP.
	go http.ListenAndServe(ip.String()+":8080", nil)

	// Print the URL every 5 s; this loop also keeps the program alive.
	for {
		println("Open: http://" + ip.String() + ":8080/api/readings")
		time.Sleep(5 * time.Second)
	}
}
