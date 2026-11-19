# Pflanzenmessung: the code explained for Go beginners

This document explains `pflanzenmessung.go` line by line. The program runs on a XIAO ESP32-C3 with TinyGo, reads a BME280
temperature/humidity/pressure sensor and a soil moisture sensor, and serves the
values as JSON at `/api/readings`.

---

## 1. How to get the IP

Flash the board with the serial monitor enabled:

```bash
cd go-code
tinygo flash -target esp32c3-generic -monitor .
```

After the board connects to WiFi, it prints this every 5 seconds:

```text
Open: http://192.168.1.123:8080/api/readings
```

The `192.168.1.123` part is the IP. Open the full URL in a browser, or run:

```bash
curl http://192.168.1.123:8080/api/readings
```

If you closed the monitor, reopen it with:

```bash
tinygo monitor
```

You can also find the board in your router's DHCP/client list. The IP is
assigned by DHCP, so it can change after a reboot unless you reserve it in the
router.

---

## 2. What the program does

1. Waits 2 seconds so the serial monitor can connect.
2. Stores your WiFi settings, sensor calibration, on/off flags, and fallback
   values.
3. Sets up the I2C bus for the BME280 and the ADC for the soil sensor.
4. Registers the `/api/readings` HTTP endpoint.
5. Connects to WiFi.
6. Waits until DHCP gives the board a real IP address.
7. Starts the web server in the background.
8. Prints the URL forever and keeps the program alive.

---

## 3. Go basics used in this file

- `package main` — this is a runnable program, not a library.
- `func main()` — where the program starts.
- `x := 5` — create a variable and assign a value; Go guesses the type.
- `x = 6` — change an existing variable (no `:`).
- `_` — "I don't need this value, throw it away".
- `nil` — "nothing" (no error, no value).
- **struct** — a bundle of named values, e.g. `machine.I2CConfig{SDA: ..., SCL: ...}`.
- **pointer** — `&x` means "the address of x". Some functions need a pointer so
  they can use or modify the original object.
- **anonymous function / closure** — a function with no name. It can use
  variables from the surrounding function, like `bme` or `soilDry` inside the
  HTTP handler.
- **error** — many Go functions return an extra `error` value. `nil` means
  success; anything else means failure.
- **goroutine** — a function started with `go` runs in the background.
- `for { }` with no condition — an infinite loop.
- Integer division — `7 / 2` gives `3`, not `3.5`, because both values are ints.

---

## 4. Line-by-line explanation

### Header and package (lines 1–4)

```go
// Plant monitor REST API for the XIAO ESP32-C3 (TinyGo).
// Flash: tinygo flash -target esp32c3-generic -monitor .
// Then open http://<IP-from-monitor>:8080/api/readings
package main
```

The first three lines are comments. Line 4 says this is a runnable `main`
package, which is required for a program that produces an executable.

### Imports (lines 6–17)

```go
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
```

- `io` — writes text to the HTTP response.
- `machine` — TinyGo hardware access: pins, I2C, ADC.
- `net/http` — the web server.
- `strconv` — converts numbers to text.
- `time` — sleeps and durations.
- `bme280` — driver for the BME280 sensor.
- `netdev` — connects the WiFi driver to Go's standard `net`/`net/http` code.
- `nl "..."` — imports a package and gives it the short name `nl`.
- `link "..."` — imports a package and gives it the short name `link`.

### Start of `main` (line 19)

```go
func main() {
```

The entry point. Execution starts here.

### Startup delay (line 21)

```go
time.Sleep(2 * time.Second)
```

Waits 2 seconds. The USB serial monitor often needs a moment to connect;
without this delay, the first printed messages can be lost.

### WiFi name and password (lines 24–25)

```go
ssid := "MikroTik-AF2E92"
password := "" // empty = open WiFi
```

- `ssid` is the WiFi network name.
- `password` is the WiFi password.
- An empty password means an open network.

Change these to match your WiFi.

### Soil calibration (lines 29–30)

```go
soilDry := 50000
soilWet := 20000
```

Calibration values for the soil sensor:

- `soilDry` — raw ADC value when the probe is in air.
- `soilWet` — raw ADC value when the probe is in water.

Measure both with the real sensor, read `soil_raw` from the API, and update
these numbers if the sensor or calibration changes.

### Sensor on/off flags (lines 33–34)

```go
useBME := true
useSoil := false
```

- `useBME = true` — try to use the real BME280. If a read fails, the fallback
  value is used.
- `useSoil = false` — do not read the soil sensor; use `fallbackRaw` instead.
  Set this to `true` when the soil sensor is connected.

### Fallback values (lines 37–40)

```go
fallbackTemp := 23.4   // °C
fallbackHum := 51.2    // %
fallbackPres := 1013.0 // hPa
fallbackRaw := 35000   // soil ADC value (0-65535)
```

These are fake values used when a sensor is missing or a read fails. They let
the API keep working while you wait for hardware.

### I2C bus setup (lines 43–44)

```go
i2c := machine.I2C0
i2c.Configure(machine.I2CConfig{SDA: machine.D1, SCL: machine.D0})
```

- `I2C0` is the first I2C bus.
- `Configure` sets the two wires used by I2C:
  - SDA (data) on pin `D1`.
  - SCL (clock) on pin `D0`.
- `machine.I2CConfig{...}` is a struct with settings filled in.

Note: the comment above this code mentions D4/D5, but the actual code uses
`D1` and `D0`. Trust the code and your wiring, and update the comment if it is
outdated.

### BME280 setup (lines 46–47)

```go
bme := bme280.New(i2c)
bme.Configure()
```

- `bme280.New(i2c)` creates the sensor object on that I2C bus.
- `Configure()` initializes the chip and reads its calibration data.

This runs even if the sensor is missing. Missing sensors are handled later by
the fallback logic.

### Soil sensor / ADC setup (lines 50–52)

```go
machine.InitADC()
soil := machine.ADC{Pin: machine.D3}
soil.Configure(machine.ADCConfig{})
```

- `InitADC()` turns on the analog-to-digital converter, which converts a
  voltage into a number.
- `ADC{Pin: machine.D3}` says the analog signal is on pin `D3`.
- `ADCConfig{}` means "use the default ADC settings".

Note: the comment above says D0/GPIO2, but the actual code uses `D3`. Again,
the code is what runs.

### Register the HTTP endpoint (line 54)

```go
http.HandleFunc("/api/readings", func(w http.ResponseWriter, r *http.Request) {
```

This registers a route. When a request for `/api/readings` arrives, the
anonymous function below runs.

- `w http.ResponseWriter` — where you write the response.
- `r *http.Request` — the incoming request (unused here, but required).
- The function is not called now; it runs later, once per request.
- It is a closure, so it can use `bme`, `soil`, `useBME`, `useSoil`,
  `soilDry`, `soilWet`, and the fallback variables from `main`.

### Start with fallback values (line 58)

```go
temp, hum, pres := fallbackTemp, fallbackHum, fallbackPres
```

Creates three `float64` variables and sets them to the fallback values. They
will be overwritten with real readings if the sensor works.

### Read the BME280 (lines 59–69)

```go
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
```

- `if useBME` — only try if the sensor is enabled.
- Each read returns two values: the measurement and an `error`.
- `t, err := ...` inside an `if` declares `t` and `err` only for that `if`.
- `err == nil` means the read succeeded.
- Conversions:
  - Temperature: milli-degrees → °C, divide by 1000.
  - Humidity: hundredths of a percent → %, divide by 100.
  - Pressure: milli-pascal → hPa, divide by 100000.
- If a read fails, that variable keeps its fallback value.

The driver returns whole numbers because microcontrollers often avoid floating
point. The divisions turn those numbers into normal units.

### Read the soil sensor (lines 71–74)

```go
raw := fallbackRaw
if useSoil {
    raw = int(soil.Get())
}
```

- Start with the fallback raw value.
- If the soil sensor is enabled, `soil.Get()` reads the real ADC value
  (0–65535) and `int(...)` converts it to a normal integer.

If `useSoil` is `false`, the API always reports `fallbackRaw` (35000).

### Convert raw soil value to percent (lines 76–77)

```go
pct := (soilDry - raw) * 100 / (soilDry - soilWet)
```

This maps the raw value onto a 0–100 % scale:

- `raw = soilDry` → 0 %
- `raw = soilWet` → 100 %

Example with `soilDry = 50000`, `soilWet = 20000`, `raw = 35000`:

```text
(50000 - 35000) * 100 / (50000 - 20000)
= 15000 * 100 / 30000
= 50
```

All values are integers, so the result is rounded down. Also, `soilDry` must
not equal `soilWet`, otherwise this divides by zero and the program panics.

### Clamp the percentage (lines 79–85)

```go
if pct < 0 {
    pct = 0
}
if pct > 100 {
    pct = 100
}
```

Readings can be slightly outside the calibrated range, so the result is forced
to stay between 0 and 100. Go's `if` does not need parentheses, but the `{`
must be on the same line as the condition.

### Number-to-text helper (line 88)

```go
f := func(x float64) string { return strconv.FormatFloat(x, 'f', 1, 64) }
```

Creates a small function and stores it in `f`. It turns a decimal number into
text with one digit after the point:

- `'f'` — normal decimal notation (not exponent notation).
- `1` — one digit after the decimal point.
- `64` — treat the value as a 64-bit float.

Example: `23.456` becomes `"23.5"`.

### Set the response type (line 92)

```go
w.Header().Set("Content-Type", "application/json")
```

Tells the client that the response body is JSON. Headers are metadata sent
before the body.

### Build and send the JSON (lines 93–98)

```go
io.WriteString(w, `{"temperature_c":`+f(temp)+
    `,"humidity_pct":`+f(hum)+
    `,"pressure_hpa":`+f(pres)+
    `,"soil_moisture_pct":`+strconv.Itoa(pct)+
    `,"soil_raw":`+strconv.Itoa(raw)+
    `,"needs_water":`+strconv.FormatBool(pct < 30)+`}`)
```

- Backtick strings are raw strings; `"` inside them does not need escaping.
- `+` joins text pieces.
- `f(temp)` converts a float to text.
- `strconv.Itoa(pct)` converts an integer to text.
- `pct < 30` produces `true` or `false`.
- `strconv.FormatBool(...)` turns that into `"true"` or `"false"`.
- So `needs_water` is `true` when soil moisture is below 30 %.
- `io.WriteString` writes the finished text to the HTTP response.

Change the `30` in `pct < 30` if you want a different watering threshold.

### Close the handler (line 99)

```go
})
```

Closes the anonymous function and the `HandleFunc` call.

### WiFi setup (lines 102–103)

```go
radio := link.Esplink{ArenaPoolSize: 48 * 1024}
netdev.UseNetdev(&radio)
```

- `link.Esplink` is the ESP WiFi driver.
- `ArenaPoolSize` is the amount of RAM reserved for the WiFi/network stack.
  - `48 * 1024` = 49,152 bytes = 48 KiB.
  - This memory is used for packets, buffers, and connection state.
  - Increase it if network operations fail with memory errors, or if you add
    TLS/HTTPS. Decrease it if you are low on RAM and networking still works.
- `&radio` passes the address (pointer) of the driver.
- `netdev.UseNetdev` tells Go's networking code to use this WiFi device.

### Connect to WiFi (lines 106–109)

```go
for radio.NetConnect(&nl.ConnectParams{Ssid: ssid, Passphrase: password}) != nil {
    println("WiFi failed, retrying in 5s...")
    time.Sleep(5 * time.Second)
}
```

- `NetConnect` tries to join the WiFi network.
- It returns an `error`: `nil` means success, non-`nil` means failure.
- `&nl.ConnectParams{...}` creates a settings struct and passes its address.
- `for condition { }` is Go's version of a `while` loop.
- The loop repeats while the result is **not** `nil`, i.e. while it fails.
- On failure it prints a message and waits 5 seconds before retrying.
- On success the condition becomes false and the loop ends.

### Wait for an IP address (lines 112–116)

```go
ip, _ := radio.Addr()
for !ip.IsValid() || ip.IsUnspecified() {
    time.Sleep(1 * time.Second)
    ip, _ = radio.Addr()
}
```

- `radio.Addr()` returns the current IP and an error.
- `_` throws the error away.
- `!` means NOT; `||` means OR.
- `IsValid()` is false when there is no address yet.
- `IsUnspecified()` is true for `0.0.0.0`.
- So the loop keeps waiting while the IP is invalid **or** still `0.0.0.0`.
- `=` is used on line 115 (not `:=`) because `ip` already exists.
- DHCP needs a moment after connecting, hence the retry.

This is the IP that is printed to the serial monitor.

### Start the web server (line 119)

```go
go http.ListenAndServe(ip.String()+":8080", nil)
```

- `go` starts the server in the background.
- `ListenAndServe` listens for HTTP requests.
- `ip.String()` turns the IP into text like `"192.168.1.123"`.
- `+ ":8080"` makes the address `"192.168.1.123:8080"`.
- `nil` means "use the default router", which knows `/api/readings` from
  line 54.
- Without `go`, this call would block forever and the loop below would never
  run.

If port 8080 is already in use, the server fails; the error is ignored here.

### Print the URL forever (lines 122–125)

```go
for {
    println("Open: http://" + ip.String() + ":8080/api/readings")
    time.Sleep(5 * time.Second)
}
```

- `for { }` with no condition is an infinite loop.
- It prints the full URL to the serial monitor every 5 seconds.
- It also keeps `main` alive. If `main` returned, the program would exit and
  the background web server would stop.

### End of `main` (line 126)

```go
}
```

Closes the `main` function.

---

## 5. Current configuration

| Setting | Current value | Meaning |
|---|---|---|
| `useBME` | `true` | Try the real BME280; fall back on read errors |
| `useSoil` | `false` | Skip the soil sensor and use `fallbackRaw` |
| `fallbackTemp` | `23.4` | Used if BME temperature read fails |
| `fallbackHum` | `51.2` | Used if BME humidity read fails |
| `fallbackPres` | `1013.0` | Used if BME pressure read fails |
| `fallbackRaw` | `35000` | Used while the soil sensor is disabled |
| `soilDry` | `50000` | Raw value in air = 0 % |
| `soilWet` | `20000` | Raw value in water = 100 % |
| `ArenaPoolSize` | `48 * 1024` | 48 KiB reserved for WiFi |
| Water threshold | `30` | `needs_water` is true below 30 % |

With the current settings, the soil reading is always based on `35000`, which
gives 50 % moisture. Set `useSoil := true` when the soil sensor is connected.

---

## 6. Example response

```json
{
  "temperature_c": 23.4,
  "humidity_pct": 51.2,
  "pressure_hpa": 1013.0,
  "soil_moisture_pct": 50,
  "soil_raw": 35000,
  "needs_water": false
}
```

The actual response is one line, for example:

```json
{"temperature_c":23.4,"humidity_pct":51.2,"pressure_hpa":1013.0,"soil_moisture_pct":50,"soil_raw":35000,"needs_water":false}
```

If the BME280 is connected and working, the first three values are real
measurements instead of the fallbacks.

---

## 7. Troubleshooting

**No IP appears in the monitor**
- Make sure the SSID and password are correct.
- The code retries every 5 seconds if WiFi fails.
- Try `tinygo monitor` or check your router's DHCP list.
- The IP can change after a reboot.

**Temperature/humidity/pressure never change**
- The BME280 may not be detected, so the fallback values are used.
- Check `useBME` and the I2C wiring.
- The code currently configures SDA on `D1` and SCL on `D0`.

**`soil_raw` is always 35000**
- `useSoil` is `false`. Set it to `true` when the sensor is connected.
- The code currently reads the soil sensor from `D3`.

**The pin comments look wrong**
- The comments above the I2C and soil setup mention older pins. The actual code
  uses `D1`/`D0` for I2C and `D3` for the soil sensor. Trust the code and update
  the comments when the wiring is final.

**The percentage looks wrong**
- Re-measure `soilDry` and `soilWet` with the real sensor.
- Make sure `soilDry` and `soilWet` are not equal.
- Capacitive sensors usually give high numbers when dry and low numbers when
  wet.
