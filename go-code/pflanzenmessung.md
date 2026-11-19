# Pflanzenmessung: code documentation

This program runs on a XIAO ESP32-C3 with TinyGo. It reads a BME280 sensor and
a soil moisture sensor and serves the values as JSON at `/api/readings` plus a
live dashboard at `/`.

It uses `httphi`, a small HTTP server for microcontrollers, instead of Go's
`net/http`. The API and HTTP parts are explained in detail in section 5.

---

## 1. Quick start

Flash the board and open the serial monitor:

```bash
cd go-code
tinygo flash -target=xiao-esp32c3 -monitor .
```

After the WiFi connection, the monitor prints the URL every 5 seconds:

```text
Open: http://192.168.1.123:8080/  (API: /api/readings)
```

Open `http://192.168.1.123:8080/` in a browser to see the dashboard. It updates
automatically every 3 seconds. The raw JSON is still available at:

```bash
curl http://192.168.1.123:8080/api/readings
```

The IP comes from DHCP and can change after a reboot. You can also find the
board in your router's client list. If the monitor is closed, reopen it with
`tinygo monitor`.

---

## 2. What the program does

1. Waits 2 seconds so the serial monitor can connect.
2. Stores WiFi settings, calibration values, sensor flags, and fallbacks.
3. Sets up I2C for the BME280 and the ADC for the soil sensor.
4. Registers the `/api/readings` route and the `/` dashboard route.
5. Configures the `httphi` router and allocates its fixed buffers.
6. Connects to WiFi and waits for a real IP address.
7. Starts a background loop that prints the URL.
8. Starts the web server. This last call blocks forever.

The route and router are prepared before WiFi is connected. No client can reach
the board until the server starts, so this order is safe.

---

## 3. Settings and hardware

| Setting | Current value | Meaning |
|---|---|---|
| `useBME` | `true` | Use the BME280; fall back on read errors |
| `useSoil` | `false` | Skip the soil sensor and use `fallbackRaw` |
| `fallbackTemp` | `-23.4` | Used if the temperature read fails |
| `fallbackHum` | `-51.2` | Used if the humidity read fails |
| `fallbackPres` | `-1013.0` | Used if the pressure read fails |
| `fallbackRaw` | `-35000` | Used while the soil sensor is disabled |
| `soilDry` | `50000` | Raw value in air, 0 % |
| `soilWet` | `20000` | Raw value in water, 100 % |
| `ArenaPoolSize` | `48 * 1024` | 48 KiB for the WiFi stack |
| Router workers | `1` | One request at a time |
| Router memory | `2048` | About 2 KiB budget per connection |
| Water threshold | `30` | `needs_water` is true below 30 % |

The current fallback values are negative on purpose. That makes it easy to see
when no real sensor data is available. For normal operation you should use
realistic values.

Hardware pins:

- BME280: SDA on `D1`, SCL on `D0`.
- Soil sensor: analog input on `D3`.

Sensor units and conversions:

- Temperature is returned in milli-degrees, so divide by 1000.
- Humidity is returned in hundredths of a percent, so divide by 100.
- Pressure is returned in milli-pascal, so divide by 100000 to get hPa.

If a BME280 read fails, the old fallback value stays in the variable. If
`useSoil` is `false`, the soil value is always `fallbackRaw`.

The soil percentage is calculated as:

```text
pct = (soilDry - raw) * 100 / (soilDry - soilWet)
```

The result is an integer and is clamped to 0-100. With the current
`fallbackRaw = -35000` the formula gives 283, which the clamp turns into 100.
`soilDry` and `soilWet` must not be equal, otherwise the program divides by zero.

---

## 4. Go basics used here

- `package main` and `func main()`: this is a runnable program.
- `x := 5` creates a variable; `x = 6` changes an existing one.
- `nil` means "nothing". For an error, `nil` means success.
- A `struct` groups named values, for example `machine.I2CConfig{...}`.
- `&x` is a pointer to `x`. Some functions need a pointer so they can use the
  original object.
- A closure is a function that can use variables from outside, such as `bme`
  inside the HTTP handler.
- `go f()` starts a goroutine, which runs in the background.
- An array like `[192]byte` contains the bytes directly.
- A slice like `buf[:0]` describes a part of an array using a pointer, a length,
  and a capacity.
- `append` adds bytes and returns the updated slice.
- `strconv.AppendFloat`, `AppendInt`, and `AppendBool` add text to a byte slice
  without creating a separate string.
- `for { }` is an infinite loop.
- Integer division rounds down: `7 / 2` is `3`.

---

## 5. Code walkthrough

| Lines | Part |
|---|---|
| 20-51 | Settings, hardware setup, and sensor objects |
| 53-102 | Route registration and the request handler |
| 104-109 | Router configuration |
| 111-126 | WiFi connection and IP wait |
| 128-139 | URL printing and web server |

The setup part is straightforward. The BME280 and ADC are configured even if a
sensor is missing; the handler handles missing sensors later.

The route is registered with:

```go
var routes httphi.MuxSlice
routes.Handle("/api/readings", handler)
routes.Handle("/{$}", serveWebUI)
```

This only fills the route table. It does not start a server. The first handler
is a closure, so it can use `bme`, `soil`, the flags, the calibration values,
and the fallbacks from `main`. The second route matches only the exact root
path; the dashboard itself lives in `webui.go`.

The router is configured with:

```go
cfg := httphi.DefaultRouterConfig(1, 2048, routes.MaxPathValues())
router.Configure(&routes, cfg)
```

This allocates the fixed HTTP buffers and starts one worker goroutine. The
numbers and memory layout are explained in section 6.

The WiFi loop retries every 5 seconds until the connection succeeds. After that
the program waits for DHCP to provide an IP that is valid and not `0.0.0.0`.

The URL loop is a goroutine that prints the address every 5 seconds. The
`ListenAndServe` call below it blocks, and that is what keeps the program alive.

---

## 6. How the API and HTTP server work

This section follows one request from the network connection to the JSON
response.

```text
client
  -> TCP accept
  -> router.Handle
  -> free Exchange
  -> worker goroutine
  -> read and parse request
  -> route lookup
  -> handler reads sensors and builds JSON
  -> stage headers, write status, headers, and body
  -> release exchange and close connection
```

### 6.1 Startup: route table and fixed memory

`MuxSlice.Handle` stores a route entry with:

- the HTTP method (`MethUndefined` because no method was given),
- the path `/api/readings`,
- the handler function,
- the number of path values (0 here),
- a specificity score used when several routes match.

A route without a method matches every method. If you want only GET, register
`"GET /api/readings"` instead.

`routes.MaxPathValues()` returns 0 because `/api/readings` has no `{...}`
wildcards. This tells the router that it needs no path value scratch space.

`DefaultRouterConfig(1, 2048, 0)` creates this configuration:

- `FixedNumGoroutines = 1`: one worker, so one request at a time.
- `RequestHeaderBufferSize`: most of the 2048-byte budget.
- `ResponseHeaderMinBufferSize`: a 128-byte reserve.
- `RequestNumHeaderKVCap`: how many request header fields can be parsed,
  roughly the request buffer size divided by 32.

The 2048 is a budget, not one array. `Router.Configure` uses it to allocate:

- one raw buffer with a request-header region and a response-header region,
- one `Exchange`,
- one path value slice of length 0,
- one job channel with capacity 1,
- one worker goroutine.

Inside the exchange, the raw buffer looks like this:

```text
+---------------------------+---------------------------+
| request header region     | response header region    |
| RequestBufferLim bytes    | the rest of the buffer    |
+---------------------------+---------------------------+
```

The request parser writes into the first region. Response headers are written
into the free space after the parsed request header. Both use one allocation
but do not overwrite each other.

`NoRequestBufferGrowth = true` keeps memory fixed. A request header that is too
large gets status 431 instead of making the buffer grow.

### 6.2 From TCP connection to worker

`radio.ListenAndServe(&router, 8080)` creates a TCP4 stream listener on all
addresses at port 8080. It then loops:

1. `listener.Accept()` waits for a client.
2. `router.Handle(conn)` takes ownership of the connection.
3. On error, the connection is closed.

`router.Handle` does not serve the request itself. It:

1. Locks the router.
2. Tries to get a free `Exchange` from the pool or freelist.
3. If no exchange is free, returns `ErrExhausted`.
4. If an exchange is free, puts a job in the channel.
5. If the channel is full, releases the exchange and returns `ErrPacketDrop`.
6. Returns immediately.

The caller closes the connection when `Handle` returns an error. This is
deliberate backpressure: the board refuses work instead of allocating more
memory.

With one worker and one exchange, only one request runs at a time. While it
runs, the exchange is not free, so another connection is refused and closed.
This is fine for a plant monitor, and it also prevents two handlers from using
the shared I2C bus at the same time.

### 6.3 Reading and routing the request

The worker calls `httphi.Handle(exch, mux, nil)`. That function:

1. Resets the request parser so the request buffer can be reused.
2. Reads request bytes into the request region.
3. Parses the request line and headers.
4. If the request arrived in several TCP packets, reads more until the header
   is complete.
5. Accepts HTTP/1.0 and HTTP/1.1. Other versions get a 400 or 505 response.
6. Gets the path and method and calls `mux.LookupHandler`.
7. Calls the matching handler, or writes 404 if no route matches.

`MuxSlice.LookupHandler` checks each route entry:

- A route with a method only matches that method.
- `MethUndefined` matches every method. Our route has no method prefix.
- `/api/readings` has no wildcards, so the path check is a direct comparison.
- `/{$}` matches the exact root path `/` and nothing else.
- If several routes match, the most specific one wins.
- If no route matches, the handler is `nil`.

`GET /` serves the dashboard. Unknown paths still return 404, which is a
complete response, not a timeout.

The handler receives the `*Exchange`. The exchange is only valid during the
handler call. Do not save the pointer or any slice that points into its buffers
for later use.

### 6.4 The handler and the fixed JSON buffer

The handler does three things:

1. Reads the sensors or uses fallbacks.
2. Converts the soil value to a clamped percentage.
3. Builds the JSON in a local byte array.

The JSON code is the important part:

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

#### Array and slice

`var buf [192]byte` creates an array. The variable contains the 192 bytes
directly. Go zeroes them when the handler starts. The array is local to the
handler. The compiler may keep it on the stack or move it, but its size is
fixed at 192 bytes.

`b := buf[:0]` creates a slice. A slice is a small descriptor:

```text
b:
  pointer  -> &buf[0]
  length   = 0
  capacity = 192
```

It does not copy bytes and does not allocate a new array. It only describes a
view into `buf`.

#### What append does

`append(b, x...)` calculates the new length. If it fits in the capacity, it
copies the new bytes into `buf[len(b):newLength]` and returns a new slice
descriptor with the same pointer and a larger length. If it does not fit, it
allocates a new, larger array, copies the old bytes, and returns a slice that
points at the new array.

This is why the code always writes `b = append(b, ...)`. The returned slice can
have a different length, and in the overflow case it can have a different
pointer. Ignoring the return value would keep the old length and overwrite the
same position on the next append.

`append(b, "text"...)` appends each byte of the string. The `...` means "use the
string as a sequence of bytes". No NUL terminator is added.

`strconv.AppendFloat`, `AppendInt`, and `AppendBool` work the same way: they
append text to the slice and return the extended slice. They do not create a
separate string. `'f'` means fixed-point notation, `1` means one decimal place,
and `64` means the value is a `float64`.

#### Exact length progression

The current fallback values produce this JSON:

```json
{"temperature_c":-23.4,"humidity_pct":-51.2,"pressure_hpa":-1013.0,"soil_moisture_pct":100,"soil_raw":-35000,"needs_water":false}
```

The length grows like this. The capacity stays 192 because everything fits.

| Appended text | Added | Total |
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

The final slice has length 129 and capacity 192. The 63 unused bytes are never
sent. `Content-Length` is 129, not 192, because `Respond` uses `len(b)`.

If the JSON ever grew past 192 bytes, `append` would allocate a new array and
copy the data. The code would still work, but that request would allocate. With
the current fields, 192 is large enough.

### 6.5 Writing the response

`exch.Respond(httphi.StatusOK, "application/json", b)` builds the response:

1. It stages `Content-Type: application/json`.
2. It stages `Content-Length: len(b)`, which is 129 for the current fallback
   values.
3. It stages `Connection: close`.
4. It stages `HTTP/1.1 200 OK`.

The headers are written directly into the response region of the exchange's raw
buffer. The status line is written into a small fixed buffer inside the
exchange. No header map is created.

Then `WriteBody`:

1. Flushes the status line and headers.
2. Writes the body bytes from `b` directly to the connection.

The JSON is not copied into the exchange's buffer. It is sent from the
handler's local `b` slice. `Respond` finishes before the handler returns, so the
bytes are still valid while they are written.

The three header fields fit in the 128-byte response reserve. The code ignores
the error returned by `Respond` because this response is small and known to fit.

### 6.6 Cleanup and why there are no keep-alive connections

After the handler returns, `httphi.Handle` calls `FlushHeader` only if the
handler did not already write it. Here `Respond` already wrote it, so that call
does nothing.

The worker then returns the exchange to the router's freelist and calls
`Release`. `Release` closes the connection and marks the exchange as free. The
next request can reuse the same exchange and raw buffer. The handler's local
`[192]byte` array is new for each call.

Each connection serves exactly one response. `Connection: close` tells the
client not to wait for another response on that connection. This removes
per-connection state and keeps memory use simple.

### 6.7 Why this avoids the old timeouts

The old version used Go's `net/http`. The espradio documentation warns that
`net/http` allocates about 10 kB on the Go heap per connection. TinyGo's garbage
collector does not handle heap fragmentation well, so the program can slow down
or crash after many requests. That caused the API timeouts.

The `httphi` version:

- allocates its exchange and buffers once at startup,
- reuses them for every connection,
- refuses excess connections instead of allocating more memory,
- builds JSON in a fixed local array without string concatenation,
- writes numbers with `strconv.Append*` instead of creating temporary strings.

If the WiFi signal is weak, requests can still fail at the network level. The
server itself no longer grows the heap for every request.

### 6.8 The web dashboard

`webui.go` holds the dashboard as one `const webUIHTML` string: HTML, CSS, and
JavaScript are inline, so the page needs no internet access or external files.
`serveWebUI` sends it with:

```go
ex.RespondString(httphi.StatusOK, "text/html; charset=utf-8", webUIHTML)
```

`RespondString` writes the constant directly and does not allocate a copy. In
the browser, a small script fetches `/api/readings` every 3 seconds and updates
the four cards. It stops polling while the tab is hidden and refreshes again
when the tab becomes visible. No extra server route, worker, or buffer is
needed for the updates.

---

## 7. Example response

With the current fallback values, the response is one line of 129 bytes:

```json
{"temperature_c":-23.4,"humidity_pct":-51.2,"pressure_hpa":-1013.0,"soil_moisture_pct":100,"soil_raw":-35000,"needs_water":false}
```

If the BME280 is connected and working, the first three values are real
measurements. If you set `fallbackRaw` to `35000`, the soil percentage becomes
50 %.
