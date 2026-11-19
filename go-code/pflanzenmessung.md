# Pflanzenmessung: the code explained for Go beginners

This document explains `pflanzenmessung.go` line by line. The program runs on a
XIAO ESP32-C3 with TinyGo, reads a BME280 temperature/humidity/pressure sensor
and a soil moisture sensor, and serves the values as JSON at `/api/readings`.

It uses `httphi` (a small HTTP server for embedded systems) instead of Go's
`net/http`. Sections 4 and 5 explain exactly how the server, the route lookup,
the handler, the fixed byte buffer, and the response work.

---

## 1. How to get the IP

Flash the board with the serial monitor enabled:

```bash
cd go-code
tinygo flash -target=xiao-esp32c3 -monitor .
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
4. Registers the `/api/readings` route with `httphi`.
5. Configures the `httphi` router and allocates its fixed buffers.
6. Connects to WiFi.
7. Waits until DHCP gives the board a real IP address.
8. Starts a background loop that prints the URL every 5 seconds.
9. Starts the `httphi` web server. This call blocks forever.

The route and router are configured before WiFi is connected. No client can
reach the board until step 9, so this order is safe.

---

## 3. Go basics used in this file

- `package main`: this is a runnable program, not a library.
- `func main()`: where the program starts.
- `x := 5`: create a variable and assign a value; Go guesses the type.
- `x = 6`: change an existing variable (no `:`).
- `_`: "I do not need this value, throw it away".
- `nil`: "nothing" (no error, no value).
- **struct**: a bundle of named values, for example
  `machine.I2CConfig{SDA: ..., SCL: ...}`.
- **pointer**: `&x` means "the address of x". Some functions need a pointer so
  they can use or modify the original object.
- **anonymous function / closure**: a function with no name. It can use
  variables from the surrounding function, like `bme` or `soilDry` inside the
  HTTP handler.
- **error**: many Go functions return an extra `error` value. `nil` means
  success; anything else means failure.
- **goroutine**: a function started with `go` runs in the background.
- **array**: `[192]byte` is a value that contains exactly 192 bytes.
- **slice**: a small descriptor with a pointer, a length, and a capacity. It
  points at an array. `buf[:0]` is a slice that points at `buf` with length 0
  and capacity 192.
- `append`: adds bytes to a slice and returns the updated slice. It can write
  into the existing array or allocate a new, larger array.
- `...` after a string in `append`: append each byte of the string.
- `strconv.AppendFloat`, `strconv.AppendInt`, `strconv.AppendBool`: append text
  to a byte slice instead of creating a new string.
- `for { }` with no condition: an infinite loop.
- Integer division: `7 / 2` gives `3`, not `3.5`, because both values are ints.

---

## 4. Line-by-line explanation

### Header and package (lines 1-4)

```go
// Plant monitor REST API for the XIAO ESP32-C3 (TinyGo).
// Flash: tinygo flash -target=xiao-esp32c3 -monitor .
// Then open http://<IP-from-monitor>:8080/api/readings
package main
```

The first three lines are comments. Line 4 says this is a runnable `main`
package, which is required for a program that produces an executable.

### Imports (lines 6-16)

```go
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
```

- `machine`: TinyGo hardware access: pins, I2C, ADC.
- `strconv`: converts numbers to text by appending to a byte slice.
- `time`: sleeps and durations.
- `httphi`: the small embedded HTTP server and router.
- `bme280`: driver for the BME280 sensor.
- `netdev`: connects the WiFi driver to TinyGo's networking code.
- `nl "..."`: imports a package and gives it the short name `nl`.
- `link "..."`: imports a package and gives it the short name `link`.

### Start of `main` (line 18)

```go
func main() {
```

The entry point. Execution starts here.

### Startup delay (line 20)

```go
time.Sleep(2 * time.Second)
```

Waits 2 seconds. The USB serial monitor often needs a moment to connect;
without this delay, the first printed messages can be lost.

### WiFi name and password (lines 23-24)

```go
ssid := "MikroTik-AF2E92"
password := "" // empty = open WiFi
```

- `ssid` is the WiFi network name.
- `password` is the WiFi password.
- An empty password means an open network.

Change these to match your WiFi.

### Soil calibration (lines 28-29)

```go
soilDry := 50000
soilWet := 20000
```

Calibration values for the soil sensor:

- `soilDry`: raw ADC value when the probe is in air.
- `soilWet`: raw ADC value when the probe is in water.

Measure both with the real sensor, read `soil_raw` from the API, and update
these numbers if the sensor or calibration changes.

### Sensor on/off flags (lines 32-33)

```go
useBME := true
useSoil := false
```

- `useBME = true`: try to use the real BME280. If a read fails, the fallback
  value is used.
- `useSoil = false`: do not read the soil sensor; use `fallbackRaw` instead.
  Set this to `true` when the soil sensor is connected.

### Fallback values (lines 36-39)

```go
fallbackTemp := -23.4   // °C
fallbackHum := -51.2    // %
fallbackPres := -1013.0 // hPa
fallbackRaw := -35000   // soil ADC value (0-65535)
```

These are values used when a sensor is missing or a read fails. The current
values are negative; that makes it obvious when the API is returning fallback
data instead of real measurements. Replace them with realistic values for
normal operation.

The comment says `0-65535`, but the current `fallbackRaw` is negative on
purpose. Because `useSoil` is `false`, the API reports `-35000` right now.

### I2C bus setup (lines 41-43)

```go
i2c := machine.I2C0
i2c.Configure(machine.I2CConfig{SDA: machine.D1, SCL: machine.D0})
```

- `I2C0` is the first I2C bus.
- `Configure` sets the two wires used by I2C:
  - SDA (data) on pin `D1`.
  - SCL (clock) on pin `D0`.
- `machine.I2CConfig{...}` is a struct with settings filled in.

### BME280 setup (lines 45-46)

```go
bme := bme280.New(i2c)
bme.Configure()
```

- `bme280.New(i2c)` creates the sensor object on that I2C bus.
- `Configure()` initializes the chip and reads its calibration data.

This runs even if the sensor is missing. Missing sensors are handled later by
the fallback logic.

### Soil sensor / ADC setup (lines 48-51)

```go
machine.InitADC()
soil := machine.ADC{Pin: machine.D3}
soil.Configure(machine.ADCConfig{})
```

- `InitADC()` turns on the analog-to-digital converter, which converts a
  voltage into a number.
- `ADC{Pin: machine.D3}` says the analog signal is on pin `D3`.
- `ADCConfig{}` means "use the default ADC settings".

### Register the HTTP route (lines 53-56)

```go
var routes httphi.MuxSlice
routes.Handle("/api/readings", func(exch *httphi.Exchange) {
```

- `httphi.MuxSlice` is a list of route entries. It is the router table.
- `Handle` adds one entry to that list.
- The path is `/api/readings`.
- The second argument is the handler function that will run for a matching
  request.
- The handler is a closure, so it can use `bme`, `soil`, `useBME`, `useSoil`,
  `soilDry`, `soilWet`, and the fallback variables from `main`.
- The function is not called now. It is stored and called later by the server.

`Handle` takes either `"/path"` or `"METHOD /path"`. This code uses
`"/api/readings"` with no method, so every HTTP method matches. If you want
only GET, use `"GET /api/readings"`.

### Start with fallback values (line 58)

```go
temp, hum, pres := fallbackTemp, fallbackHum, fallbackPres
```

Creates three `float64` variables and sets them to the fallback values. They
will be overwritten with real readings if the sensor works.

### Read the BME280 (lines 59-69)

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

- `if useBME`: only try if the sensor is enabled.
- Each read returns two values: the measurement and an `error`.
- `t, err := ...` inside an `if` declares `t` and `err` only for that `if`.
- `err == nil` means the read succeeded.
- Conversions:
  - Temperature: milli-degrees to °C, divide by 1000.
  - Humidity: hundredths of a percent to %, divide by 100.
  - Pressure: milli-pascal to hPa, divide by 100000.
- If a read fails, that variable keeps its fallback value.

The driver returns whole numbers because microcontrollers often avoid floating
point. The divisions turn those numbers into normal units.

### Read the soil sensor (lines 71-74)

```go
raw := fallbackRaw
if useSoil {
    raw = int(soil.Get())
}
```

- Start with the fallback raw value.
- If the soil sensor is enabled, `soil.Get()` reads the real ADC value
  (0-65535) and `int(...)` converts it to a normal integer.

If `useSoil` is `false`, the API always reports `fallbackRaw`, which is
currently `-35000`.

### Convert raw soil value to percent (lines 76-77)

```go
pct := (soilDry - raw) * 100 / (soilDry - soilWet)
```

This maps the raw value onto a 0-100 % scale:

- `raw = soilDry` gives 0 %.
- `raw = soilWet` gives 100 %.

With the current values `soilDry = 50000`, `soilWet = 20000`, and
`raw = -35000`:

```text
(50000 - (-35000)) * 100 / (50000 - 20000)
= 85000 * 100 / 30000
= 283
```

All values are integers, so the result is rounded down. Also, `soilDry` must
not equal `soilWet`, otherwise this divides by zero and the program panics.

### Clamp the percentage (lines 78-83)

```go
if pct < 0 {
    pct = 0
}
if pct > 100 {
    pct = 100
}
```

Readings can be outside the calibrated range, so the result is forced to stay
between 0 and 100. In the current fallback case, `283` becomes `100`. Go's
`if` does not need parentheses, but the `{` must be on the same line as the
condition.

### Build the JSON in a fixed buffer (lines 85-101)

```go
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
```

- `var buf [192]byte` creates a local array of 192 bytes.
- `b := buf[:0]` creates a slice that points at `buf`, with length 0 and
  capacity 192.
- `append` and `strconv.Append*` write the JSON text into that array.
- `exch.Respond` sends the bytes in `b` as the response body.
- The detailed mechanics of the array, the slice, and every append are in
  section 5.6.

Change the `30` in `pct < 30` if you want a different watering threshold.

### Close the handler (line 102)

```go
})
```

Closes the anonymous function and the `Handle` call.

### Configure the router (lines 104-109)

```go
var router httphi.Router
cfg := httphi.DefaultRouterConfig(1, 2048, routes.MaxPathValues())
if err := router.Configure(&routes, cfg); err != nil {
    println("router config failed:", err.Error())
    return
}
```

- `httphi.Router` receives connections and hands each one to the route table.
- `DefaultRouterConfig(1, 2048, ...)` creates a memory configuration:
  - `1`: one worker goroutine, so one request is handled at a time.
  - `2048`: about 2 KiB of memory budget per connection.
  - `routes.MaxPathValues()`: the number of path values the route table can
    produce. `/api/readings` has no `{...}` wildcards, so this is `0`.
- `router.Configure` stores the route table, allocates the fixed buffers, and
  starts the worker goroutine.
- If configuration fails, the error is printed and `main` returns.

The `2048` is a budget, not the size of a single array. `DefaultRouterConfig`
splits it between the request-header buffer, a response-header reserve, the
header field table, the `Exchange` struct, and one queue slot. The exact split
is in `cfg`. Section 5.2 explains what `Configure` allocates.

### WiFi setup (lines 111-113)

```go
radio := link.Esplink{ArenaPoolSize: 48 * 1024}
netdev.UseNetdev(&radio)
```

- `link.Esplink` is the ESP WiFi driver.
- `ArenaPoolSize` is the amount of RAM reserved for the WiFi/network stack.
  - `48 * 1024` = 49,152 bytes = 48 KiB.
  - This is the ESP32-C3 default.
  - Increase it if network operations fail with memory errors. Decrease it if
    you are low on RAM and networking still works.
- `&radio` passes the address (pointer) of the driver.
- `netdev.UseNetdev` tells TinyGo's networking code to use this WiFi device.

### Connect to WiFi (lines 115-119)

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
- The loop repeats while the result is not `nil`, that is, while it fails.
- On failure it prints a message and waits 5 seconds before retrying.
- On success the condition becomes false and the loop ends.

### Wait for an IP address (lines 121-126)

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
- So the loop keeps waiting while the IP is invalid or still `0.0.0.0`.
- `=` is used inside the loop (not `:=`) because `ip` already exists.
- DHCP needs a moment after connecting, hence the retry.

This is the IP that is printed to the serial monitor.

### Print the URL in the background (lines 128-134)

```go
go func() {
    for {
        println("Open: http://" + ip.String() + ":8080/api/readings")
        time.Sleep(5 * time.Second)
    }
}()
```

- `go func() { ... }()` starts the function in a goroutine, so it runs in the
  background.
- `for { }` with no condition is an infinite loop.
- It prints the full URL to the serial monitor every 5 seconds.
- The `ListenAndServe` call below is what keeps the program alive. This
  goroutine only prints the URL.

### Start the HTTP server (lines 136-139)

```go
err := radio.ListenAndServe(&router, 8080)
println("HTTP server stopped:", err.Error())
```

- `ListenAndServe` starts the `httphi` server on port 8080.
- It creates a TCP listener, accepts connections forever, and gives each one to
  the router.
- It blocks and keeps serving until the listener fails.
- `err.Error()` turns the error into text so you can read it on the serial
  monitor.
- If you see `HTTP server stopped: ...`, something went wrong and the message
  tells you why.

The server listens on all addresses, so a changing DHCP IP is not a problem.

---

## 5. How the API works from start to finish

This section follows one client request through the whole system. The short
version is:

```text
client
  -> TCP accept in radio.ListenAndServe
  -> router.Handle
  -> free httphi.Exchange
  -> worker goroutine
  -> httphi.Handle reads and parses the request
  -> routes.LookupHandler finds the handler
  -> handler reads sensors and builds JSON in a local [192]byte array
  -> exch.Respond stages headers and writes status, headers, and body
  -> exchange is released and the connection is closed
```

### 5.1 The pieces

- `link.Esplink` is the WiFi driver and TCP stack.
- `radio.ListenAndServe` creates the TCP listener and accepts connections.
- `httphi.Router` owns a fixed pool of `Exchange` objects and the worker
  goroutine.
- `httphi.Exchange` represents one HTTP request/response cycle. It contains the
  request/response buffers and the connection.
- `httphi.MuxSlice` is the route table: a slice of route entries.
- The handler function is the code that reads the sensors and builds JSON.
- The local `[192]byte` array is only the response body buffer for one handler
  call. It is separate from the router's own buffers.

### 5.2 Startup: route table and router memory

The route is registered first:

```go
var routes httphi.MuxSlice
routes.Handle("/api/readings", handler)
```

`MuxSlice.Handle` stores a small route entry with:

- the method (`MethUndefined` because no method was given),
- the path string (`/api/readings`),
- the handler function,
- the number of wildcard path values (0 here),
- a specificity score used when several routes match.

Registration does not start a server. It only fills the table. `Handle`
panics at startup if the path is invalid, if the method is lowercase, or if the
same route is registered twice.

Next, `routes.MaxPathValues()` returns `0` because `/api/readings` has no
`{name}` or `{name...}` segments. This number tells the router how much scratch
space it needs for path values. Zero means none.

Then the router is configured:

```go
cfg := httphi.DefaultRouterConfig(1, 2048, routes.MaxPathValues())
router.Configure(&routes, cfg)
```

`DefaultRouterConfig(1, 2048, 0)` returns a `RouterConfig` with:

- `FixedNumGoroutines = 1`: one worker goroutine.
- `RequestHeaderBufferSize`: most of the 2048-byte budget.
- `ResponseHeaderMinBufferSize`: a 128-byte reserve for response headers.
- `RequestNumHeaderKVCap`: how many request header fields can be parsed,
  roughly the request buffer size divided by 32.

`Router.Configure` then does the following:

1. Validates the configuration.
2. Stores a pointer to the route table.
3. Computes `rawBuflen = RequestHeaderBufferSize + ResponseHeaderMinBufferSize`.
4. Allocates one `globbuf` byte slice of length `1 * rawBuflen`.
5. Allocates one `globpath` slice of length `1 * 0`, which is empty.
6. Configures one `Exchange` with:
   - `RawBuf` = the whole `globbuf`,
   - `RequestBufferLim` = the request-header part,
   - `NumHeaderKVCap` = the header field limit,
   - `NoRequestBufferGrowth = true`,
   - `PathValuesBuf` = the empty path value slice.
7. Creates a job channel with capacity 1.
8. Starts one goroutine that waits for jobs.

Inside `Exchange.Configure`, the single raw buffer is split into two regions:

```text
rawbuf:
+---------------------------+---------------------------+
| request header region     | response header region    |
| RequestBufferLim bytes    | the rest of rawbuf        |
+---------------------------+---------------------------+
```

The request parser writes incoming request bytes into the first region. When the
handler sets response headers, they are written into the unused space after the
parsed request header. The request and response headers therefore share one
allocation, but they do not overwrite each other.

`NoRequestBufferGrowth = true` is important. If a client sends a request header
that is larger than the configured request region, the exchange does not grow
the buffer. It answers with status 431 instead. Memory use stays fixed.

### 5.3 Accepting a connection

`radio.ListenAndServe(&router, 8080)` does the socket work:

1. It asks the lneto TCP stack for a TCP4 stream socket bound to all addresses
   on port 8080.
2. It starts listening on that socket.
3. It loops:
   - `listener.Accept()` waits for a client and returns a `net.Conn`.
   - `router.Handle(conn)` takes ownership of the connection.
   - On error, `conn.Close()` is called.
4. The loop runs until the listener returns an error.

`router.Handle` is non-blocking from the listener's point of view. It does not
serve the request itself. It does this:

1. Locks the router.
2. Tries to get a free `Exchange` from the freelist or from the fixed pool.
3. If no exchange is free, it returns `lneto.ErrExhausted`.
4. If an exchange is free, it puts a job containing that exchange into the job
   channel.
5. If the channel is full, it releases the exchange and returns
   `lneto.ErrPacketDrop`.
6. Unlocks and returns immediately.

The listener's error path closes the connection on `ErrExhausted` or
`ErrPacketDrop`. This is deliberate backpressure. The server refuses work
instead of allocating more memory.

With one worker and one exchange, only one request is handled at a time. While
that request is running, the exchange is not free, so a second connection is
refused and closed. This is fine for a plant monitor, and it also prevents two
handlers from using the shared I2C bus at the same time.

### 5.4 Reading and routing the request

The worker goroutine receives the job and calls:

```go
httphi.Handle(exch, mux, nil)
```

`httphi.Handle` reads and parses the request in these steps:

1. It resets the request header parser so the exchange's request buffer can be
   reused.
2. It reads from the connection into the request region with
   `ReadFromLimited`.
3. It parses the request line and headers with `TryParse`.
4. If the HTTP request was split across several TCP packets, `TryParse` says
   "need more", and the loop reads more bytes until the header is complete.
5. It checks the HTTP version. HTTP/1.0 and HTTP/1.1 are supported. Missing or
   unsupported versions get a 400 or 505 response.
6. It asks the parsed header for the request path and method.
7. It calls `mux.LookupHandler(method, path, exch.pathValues)`.

`MuxSlice.LookupHandler` loops through the route entries:

- If the route entry names a method and it does not match the request method,
  the entry is skipped.
- `MethUndefined` matches every method. Our route has no method prefix, so GET,
  POST, and every other method match.
- `/api/readings` has no wildcards, so the path check is a direct string
  comparison.
- If several routes match, the most specific one wins. Equal specificity is
  resolved by registration order.
- If no route matches, the handler is `nil`.

When the handler is `nil`, `httphi.Handle` writes a 404 response. This means
`GET /` returns 404 on this board because only `/api/readings` is registered.
That is normal. A 404 is still a complete HTTP response; it is not a timeout.

When a route matches, `httphi.Handle` calls the handler with the `*Exchange`.
The exchange is only valid during that call. The handler must not save the
exchange pointer or any slice that points into the exchange's buffers for later
use.

### 5.5 Running the handler

The handler does three jobs:

1. Read the sensors or use fallbacks.
2. Convert the soil value to a clamped percentage.
3. Build the JSON body in a local byte array.

The sensor part is described in section 4. The JSON part is where the fixed
buffer matters, so it is explained separately below.

### 5.6 The fixed byte buffer in exact detail

This is the key code:

```go
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
b = strconv.AppendBool(b, pct < 30)
b = append(b, '}')
exch.Respond(httphi.StatusOK, "application/json", b)
```

#### `var buf [192]byte` is an array, not a slice

`[192]byte` is an array type. The variable `buf` contains all 192 bytes. It is
not a pointer and it is not a growable object. Go zeroes the 192 bytes when the
handler starts.

The array is a local value in the handler. The compiler decides whether it
lives on the goroutine stack or is moved to the heap by escape analysis. The
important part for this code is that its size is fixed at 192 bytes and it does
not grow with the number of requests.

#### `b := buf[:0]` creates a slice descriptor

A slice is not the array. It is a small descriptor with three fields:

```text
b:
  pointer  -> &buf[0]
  length   = 0
  capacity = 192
```

`buf[:0]` means "make a slice that starts at the beginning of `buf`, has length
0, and can use up to the end of `buf`". It does not copy any bytes. It does not
allocate a new array. It only creates the three-field descriptor.

The length is how many bytes are currently used. The capacity is how many bytes
the slice can use before it needs a new array.

#### `append` writes into the array and returns a new descriptor

`append(b, x...)` works like this:

1. It calculates the new length: `len(b) + number of bytes being added`.
2. If the new length is less than or equal to `cap(b)`, the bytes fit in the
   existing array. `append` copies the new bytes into `buf[len(b):newLength]`
   and returns a new slice descriptor with the same pointer and the new length.
3. If the new length is greater than `cap(b)`, the bytes do not fit.
   `append` allocates a new, larger array, copies the old bytes into it, copies
   the new bytes after them, and returns a slice pointing at the new array.

This is why the code always writes:

```go
b = append(b, ...)
```

The returned descriptor may have a different length, and in the overflow case
it may have a different pointer. If the return value were ignored, `b` would
still have the old length, and the next append would overwrite the same
position.

`append(b, "text"...)` uses the special Go form for appending a string to a
byte slice. The `...` means "treat the string as a sequence of bytes". The bytes
of the string are copied into the slice. No NUL terminator is added, and no
separate `[]byte` is created for the string.

#### `strconv.Append*` follows the same contract

- `strconv.AppendFloat(b, temp, 'f', 1, 64)` appends the text form of `temp` to
  `b` and returns the longer slice.
  - `'f'` means fixed-point notation, not exponent notation.
  - `1` means one digit after the decimal point.
  - `64` means the value is a `float64`.
  - It writes characters such as `-23.4` directly into the spare capacity. It
    does not create a separate string.
- `strconv.AppendInt(b, int64(pct), 10)` appends an integer in base 10.
- `strconv.AppendBool(b, pct < 30)` appends `true` or `false`.

All three functions have the same signature pattern: they take the current
slice, append text, and return the extended slice. They are designed for exactly
this kind of buffer building.

#### Exact length progression for the current fallback values

The current fallback values produce this JSON:

```json
{"temperature_c":-23.4,"humidity_pct":-51.2,"pressure_hpa":-1013.0,"soil_moisture_pct":100,"soil_raw":-35000,"needs_water":false}
```

Here is how the length grows. The number in the `total` column is
`len(b)` after that append, and the capacity stays 192 because everything fits:

| Appended text | Bytes added | Total length |
|---|---:|---:|
| `{"temperature_c":` | 17 | 17 |
| `-23.4` | 5 | 22 |
| `,"humidity_pct":` | 16 | 38 |
| `-51.2` | 5 | 43 |
| `,"pressure_hpa":` | 16 | 59 |
| `-1013.0` | 7 | 66 |
| `,"soil_moisture_pct":` | 21 | 87 |
| `100` | 3 | 90 |
| `,"soil_raw":` | 12 | 102 |
| `-35000` | 6 | 108 |
| `,"needs_water":` | 15 | 123 |
| `false` | 5 | 128 |
| `}` | 1 | 129 |

The final `b` has length 129 and capacity 192. The 63 unused bytes are never
sent. `Content-Length` will be 129, not 192, because `Respond` uses `len(b)`.

If the JSON ever grew past 192 bytes, `append` would allocate a new array and
copy the data. The code would still be correct, but that request would no
longer be allocation-free. With the current fields and values, 192 is large
enough.

#### What happens to the buffer after `Respond`

`exch.Respond` writes the response before returning. The handler does not return
until that call is done. The bytes in `b` are therefore still valid while they
are being written to the connection. After the handler returns, the local array
is no longer needed. The next request creates a fresh `buf`.

### 5.7 Writing the response

`exch.Respond(httphi.StatusOK, "application/json", b)` does more than copy the
body. It builds the complete HTTP response in several steps.

First, `stageResponse` stages these header fields into the exchange's response
header region:

```text
Content-Type: application/json
Content-Length: 129
Connection: close
```

The `Content-Length` is `len(body)`, so it is the exact JSON length. The
`Connection: close` field tells the client that this connection will serve only
this one response.

Then it stages the status line:

```text
HTTP/1.1 200 OK
```

`StageHeader` writes the header field names and values directly into the
response region of the exchange's raw buffer. `StageHeaderInt` uses
`strconv.AppendInt` to write the number. `StageStatus` writes the status line
into a small fixed array inside the `Exchange`. No header map is created.

After staging, `Respond` calls `WriteBody`:

1. `WriteBody` sees that the header has not been written yet.
2. It calls `FlushHeader`.
3. `FlushHeader` writes the status line first.
4. It writes the staged header block next.
5. `WriteBody` then writes the body bytes from `b`.

The TCP stack takes the bytes and sends them to the client. The response does
not copy the JSON into the exchange's raw buffer; the body is written directly
from the handler's `b` slice.

The response header region is 128 bytes in the current configuration. The three
header fields fit comfortably in that region. The status line has its own small
fixed buffer inside the `Exchange`. If a handler staged too many headers,
`Respond` would record an error in the exchange instead of silently sending a
broken response. This code does not check that error because the response is
small and known to fit.

### 5.8 Releasing the exchange

After the handler returns, `httphi.Handle` calls `FlushHeader` only if the
handler did not already write it. In this code `Respond` already wrote the
header, so that call does nothing.

Then the worker calls `freeExch`. It:

1. Puts the exchange back on the router's freelist, up to a small fixed depth.
2. Calls `Release` on the exchange.
3. `Release` closes the connection.
4. Marks the exchange as free so the next connection can use it.

The next request reuses the same `Exchange` and the same raw buffer. The
handler's local `[192]byte` array is not reused by the router; it is a fresh
local value for each handler call.

### 5.9 Why this design avoids the old timeouts

The first version of this program used Go's `net/http` server. The espradio
documentation warns that `net/http` allocates about 10 kB on the Go heap for
every connection. TinyGo's garbage collector does not handle heap fragmentation
well, so the program can slow down or crash after many requests. The old API
sometimes accepted a TCP connection and then timed out without a response.

The `httphi` version avoids that pattern:

- The router allocates one `Exchange` and one raw buffer at startup.
- Connections reuse that buffer instead of allocating per request.
- The request parser is bounded by the configured buffer size.
- The handler writes JSON into a local fixed-size array instead of concatenating
  many strings.
- `strconv.Append*` writes digits into the slice instead of creating temporary
  strings.
- Excess connections are refused instead of causing unbounded allocation.
- Each connection serves one response and is then closed, so there is no
  keep-alive state to manage.

If the WiFi link itself is weak, requests can still fail at the network level.
The server, however, no longer grows the heap for every request.

---

## 6. Current configuration

| Setting | Current value | Meaning |
|---|---|---|
| `useBME` | `true` | Try the real BME280; fall back on read errors |
| `useSoil` | `false` | Skip the soil sensor and use `fallbackRaw` |
| `fallbackTemp` | `-23.4` | Used if BME temperature read fails |
| `fallbackHum` | `-51.2` | Used if BME humidity read fails |
| `fallbackPres` | `-1013.0` | Used if BME pressure read fails |
| `fallbackRaw` | `-35000` | Used while the soil sensor is disabled |
| `soilDry` | `50000` | Raw value in air = 0 % |
| `soilWet` | `20000` | Raw value in water = 100 % |
| `ArenaPoolSize` | `48 * 1024` | 48 KiB reserved for WiFi |
| Router workers | `1` | One request at a time |
| Memory per connection | `2048` | About 2 KiB budget |
| Water threshold | `30` | `needs_water` is true below 30 % |

With the current negative `fallbackRaw`, the percentage formula gives 283,
which the clamp turns into 100 %. This is useful for seeing the fallback path
and the clamp, but it is not a realistic soil reading. Set a sensible
`fallbackRaw` or enable `useSoil` when the sensor is connected.

---

## 7. Example response

With the current fallback values, the response is:

```json
{"temperature_c":-23.4,"humidity_pct":-51.2,"pressure_hpa":-1013.0,"soil_moisture_pct":100,"soil_raw":-35000,"needs_water":false}
```

The response is one line of 129 bytes. If the BME280 is connected and working,
the first three values are real measurements instead of the negative fallbacks.
If you set `fallbackRaw` to `35000`, the soil percentage becomes 50 %.
