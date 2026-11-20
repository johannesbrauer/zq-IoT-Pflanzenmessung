# Pflanzenmessung: code documentation

Plant monitor REST API on a Seeed XIAO ESP32-C3, built with TinyGo 0.44
(Go 1.26): a BME280 (I2C) and a soil moisture sensor (ADC), served as JSON at
`http://<ip>:8080/api/readings`. The finished
code is `pflanzenmessung.go` next to this file; flash with
`tinygo flash -target=xiao-esp32c3 -monitor .`.

### [Go Docu](https://go.dev/doc/)

### [TinyGo Docu](https://tinygo.org/docs/reference/)

![Plant monitoring system](.images/plant_monitoring_system.svg)

## 1. Libraries (pinned in go.mod) and their documentation

| Import | Docs | What it gives us |
|---|---|---|
| `machine` | [xiao-esp32c3 machine package](https://tinygo.org/docs/reference/microcontrollers/machine/xiao-esp32c3) | Board pins, `I2C0`, `ADC`. |
| `tinygo.org/x/drivers` v0.37.0 | [bme280](https://pkg.go.dev/tinygo.org/x/drivers@v0.37.0/bme280) · [netdev](https://pkg.go.dev/tinygo.org/x/drivers@v0.37.0/netdev) · [netlink](https://pkg.go.dev/tinygo.org/x/drivers@v0.37.0/netlink) | `bme280` driver ([BME280 datasheet](https://cdn-shop.adafruit.com/datasheets/BST-BME280_DS001-10.pdf)); `netdev`/`netlink` plug a network device into TinyGo's network stack. |
| `tinygo.org/x/espradio` v0.5.0 | [pkg.go.dev](https://pkg.go.dev/tinygo.org/x/espradio@v0.5.0) · [GitHub](https://github.com/tinygo-org/espradio) | WiFi on Espressif chips: drives Espressif's tested binary blobs via CGo and adds a pure-Go lneto TCP/IP stack with DHCP. Our `main()` mirrors the official `http-hello` example. |
| `github.com/soypat/lneto/http/httphi` v0.4.0 | [pkg.go.dev](https://pkg.go.dev/github.com/soypat/lneto@v0.4.0/http/httphi) · [GitHub](https://github.com/soypat/lneto) | "httphi": heapless HTTP/1.1 router — buffers and worker allocated once, nothing per request. |

You can add these imports and run `go mod tidy` to fetch exactly these
pinned versions.

**Why not Go's `net/http`:** per the [espradio examples README](https://github.com/tinygo-org/espradio/blob/main/examples/README.md), every `net/http` connection allocates ~10 kB on the Go heap; TinyGo's GC handles the resulting fragmentation poorly and the program eventually crashes - we tried that in a old version's and got `API timeouts / 404's`. httphi allocates everything once at `router.Configure` and refuses extra connections instead of growing memory.

## 2. How to rebuild it

The steps follow the real `main()` in order. Each snippet is part of the actual code.

### Step 1 - Settings and wiring

```go
// Package level (above main): so we see them directly, and can edit them quick.
const (
	sdaPin  = machine.D1
	sclPin  = machine.D0 
	soilPin = machine.D3 
)

// Inside main(): settings
ssid, password := "YourWiFi", ""             // empty password = open WiFi
soilDry, soilWet := 50000, 20000             // soil ADC raw value in air / water
useBME, useSoil := true, false               // which sensors are attached
fallbackTemp, fallbackHum := -23.4, -51.2    // used when a sensor fails
fallbackPres, fallbackRaw := -1013.0, -35000
```

Wire BME280 SDA→`D1`, SCL→`D0`; soil sensor→`D3`.

### Step 2 - I2C and the BME280

```go
i2c := machine.I2C0                 // we're assigning machine.I2C0 to a shorter variable
i2c.Configure(machine.I2CConfig{SDA: sdaPin, SCL: sclPin})
bme := bme280.New(i2c)             // creates a new BME280 connection
bme.Configure()                    // sets up the device for communication and reads the calibration coefficients
```

The default configuration is the Indoor Navigation settings from the BME280 datasheet.

### Step 3 - ADC for the soil sensor

```go
machine.InitADC()
soil := machine.ADC{Pin: soilPin}   // plain value struct
soil.Configure(machine.ADCConfig{})
```

### Step 4 - The route: a handler closure

```go
var routes httphi.MuxSlice
routes.Handle("/api/readings", func(exch *httphi.Exchange) { /* Step 5+6 */ })
```

`Handle` only fills the route table. The handler is a **closure**: a function
value that captures `bme`, `soil`, the flags, and the fallbacks from `main` -
no globals needed. It runs much later, on a router worker goroutine, whenever
a request matches the path.

`exch` must be a pointer: the handler writes its response into the
router-owned exchange (the fixed buffers inside it), which a copy couldn't.
It is valid only during the call - never store it for later.

### Step 5 - Reading sensors: Go's error idiom

```go
temp, hum, pres := fallbackTemp, fallbackHum, fallbackPres
if useBME {
	if t, err := bme.ReadTemperature(); err == nil {
		temp = float64(t) / 1000 // driver returns int32 milli-°C
	}
	// same pattern: ReadHumidity() / 100, ReadPressure() / 100000 (mPa → hPa)
}
raw := fallbackRaw
if useSoil {
	raw = int(soil.Get())
}
```

`if t, err := f(); err == nil` declares both variables scoped to the `if`;
`err == nil` means success. On failure the pre-set fallback simply stays, so
the API always answers, just with stand-in values. `float64(t)` is a type
conversion, not a function call.

### Step 6 - Building the JSON without allocations

```go
var buf [192]byte                   // fixed local array
b := buf[:0]                        // slice of buf: len 0, cap 192, no copy
b = append(b, `{"temperature_c":`...)
b = strconv.AppendFloat(b, temp, 'f', 1, 64)
// ... humidity_pct, pressure_hpa, soil_moisture_pct, soil_raw ...
b = strconv.AppendBool(b, pct < 30)  // needs_water
b = append(b, '}')
exch.Respond(httphi.StatusOK, "application/json", b)
```

- A slice is a (pointer, length, capacity) triple (basically a expandable array or list or however it's called in other langauges). `buf[:0]` borrows the
  array's memory - nothing is copied or heap-allocated.
- `append` returns the extended slice, hence `b = append(b, ...)`. Within
  capacity it just moves the length; past 192 it would allocate a bigger
  array (the longest real response is ~129 bytes, so that never happens).
- `strconv.AppendFloat(b, v, 'f', 1, 64)` writes digits straight into `b`:
  fixed-point, one decimal, float64. `AppendInt`/`AppendBool` likewise. No
  temporary strings, no garbage.
- `Respond` stages `Content-Type`, `Content-Length: len(b)` (the used length,
  not the 192 capacity) and `Connection: close`, then writes the body from
  `b` directly.

### Step 7 - Router with fixed memory

```go
var router httphi.Router
cfg := httphi.DefaultRouterConfig(1, 2048, routes.MaxPathValues())
if err := router.Configure(&routes, cfg); err != nil {
	println("router config failed:", err.Error())
	return
}
```

`DefaultRouterConfig(workers, bytesPerConnection, pathWildcards)`: 1 worker,
2048-byte buffer budget, 0 wildcards in `/api/readings`. `Configure`
allocates all buffers and the worker goroutine **once**. `&routes` is a
pointer because the router keeps a reference to the route table for lookups.
While a request runs, new connections are refused instead of allocated for;
with one worker, only one handler touches the shared I2C bus at a time.

### Step 8 — WiFi and serving

```go
radio := link.Esplink{ArenaPoolSize: 48 * 1024} // 48 KiB arena for the net stack
netdev.UseNetdev(&radio)                        // register as THE network device

for radio.NetConnect(&nl.ConnectParams{Ssid: ssid, Passphrase: password}) != nil {
	println("WiFi failed, retrying in 5s...")
	time.Sleep(5 * time.Second)
}

ip, _ := radio.Addr() // _ = ignore the error value
for !ip.IsValid() || ip.IsUnspecified() { // wait for a real DHCP IP
	time.Sleep(1 * time.Second)
	ip, _ = radio.Addr()
}

go func() { // goroutine: runs in the background and prints the ip to the serial console
	for {
		println("Open: http://" + ip.String() + ":8080/api/readings")
		time.Sleep(5 * time.Second)
	}
}()

err := radio.ListenAndServe(&router, 8080) // creates a new variable 'err' to store the failure reason 
println("HTTP server stopped:", err.Error())
```

- `&nl.ConnectParams{...}` builds a struct literal and passes its address in
  one expression; `&radio` / `&router` are pointers because the net stack and
  server call methods on the originals - a copy would be useless.
- The retry loop uses the error itself as the condition; `nil` = connected.
- `ListenAndServe` never returns in practice - it is what keeps the program
  alive.

## 3. Settings reference

- Fallback values are negative on purpose, so "no real data" is obvious.
- Soil: `pct = (soilDry - raw) * 100 / (soilDry - soilWet)`, clamped to
  0..100; `soilDry` must not equal `soilWet`; integer division rounds down.
- `needs_water` is true below 30 % moisture.

## 4. Expected response

```json
{"temperature_c":-23.4,"humidity_pct":-51.2,"pressure_hpa":-1013.0,"soil_moisture_pct":100,"soil_raw":-35000,"needs_water":false}
```

With a working BME280 the first three values are real measurements; soil
values become real once `useSoil=true` and `soilDry`/`soilWet` are calibrated.
