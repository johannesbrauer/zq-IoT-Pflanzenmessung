package main

import "github.com/soypat/lneto/http/httphi"

const webUIHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="light dark">
<title>Plant Monitor</title>
<link rel="icon" href="data:,">
<style>
:root {
  --bg-a: #eff8f1;
  --bg-b: #d9efe1;
  --glow: rgba(84, 201, 138, .35);
  --text: #13291f;
  --muted: #61776b;
  --card: rgba(255, 255, 255, .78);
  --card-border: rgba(255, 255, 255, .85);
  --line: rgba(19, 41, 31, .08);
  --accent: #2f9e63;
  --accent-2: #62d394;
  --temp: #dd6037;
  --hum: #3982c5;
  --pres: #8067c7;
  --soil: #2f9e63;
  --warn: #c2410c;
  --warn-bg: rgba(194, 65, 12, .1);
  --shadow: 0 22px 45px -24px rgba(19, 41, 31, .45);
}
@media (prefers-color-scheme: dark) {
  :root {
    --bg-a: #0b1712;
    --bg-b: #10241a;
    --glow: rgba(84, 201, 138, .18);
    --text: #e8f3ec;
    --muted: #93a99d;
    --card: rgba(19, 38, 29, .74);
    --card-border: rgba(255, 255, 255, .08);
    --line: rgba(232, 243, 236, .08);
    --accent: #5ecb8b;
    --accent-2: #8ee6b0;
    --temp: #f08a63;
    --hum: #6fb6e8;
    --pres: #b29bea;
    --soil: #5ecb8b;
    --warn: #fb923c;
    --warn-bg: rgba(251, 146, 60, .12);
    --shadow: 0 22px 45px -24px rgba(0, 0, 0, .75);
  }
}
* { box-sizing: border-box; }
html, body { min-height: 100%; }
body {
  margin: 0;
  color: var(--text);
  font-family: ui-sans-serif, system-ui, -apple-system, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
  background:
    radial-gradient(900px 500px at 15% -10%, var(--glow), transparent 60%),
    radial-gradient(700px 420px at 100% 0%, rgba(47, 158, 99, .18), transparent 55%),
    linear-gradient(145deg, var(--bg-a), var(--bg-b));
  background-attachment: fixed;
}
.wrap { max-width: 1000px; margin: 0 auto; padding: clamp(20px, 4vw, 48px); }
.top { display: flex; align-items: center; justify-content: space-between; gap: 16px; flex-wrap: wrap; margin-bottom: 22px; }
.brand { display: flex; align-items: center; gap: 14px; }
.logo { width: 50px; height: 50px; border-radius: 15px; display: grid; place-items: center; color: #fff; background: linear-gradient(135deg, var(--accent), var(--accent-2)); box-shadow: 0 14px 28px -12px rgba(47, 158, 99, .8); }
.logo svg { width: 27px; height: 27px; }
h1 { margin: 0; font-size: clamp(1.55rem, 3vw, 2.05rem); letter-spacing: -.025em; }
.subtitle { margin: 3px 0 0; color: var(--muted); font-size: .92rem; }
.status { display: inline-flex; align-items: center; gap: 9px; padding: 9px 15px; border-radius: 999px; background: var(--card); border: 1px solid var(--card-border); box-shadow: var(--shadow); font-size: .85rem; font-weight: 650; }
.dot { width: 9px; height: 9px; border-radius: 50%; background: var(--accent); box-shadow: 0 0 0 4px rgba(47, 158, 99, .16); transition: background .3s, box-shadow .3s; }
.status.offline .dot { background: var(--warn); box-shadow: 0 0 0 4px rgba(194, 65, 12, .16); }
.alert { display: none; align-items: center; gap: 13px; margin-bottom: 18px; padding: 15px 18px; border-radius: 16px; border: 1px solid rgba(194, 65, 12, .22); background: var(--warn-bg); color: var(--warn); }
.alert.visible { display: flex; }
.alert strong { display: block; font-size: .95rem; }
.alert span { display: block; margin-top: 2px; font-size: .85rem; color: var(--muted); }
.alert-icon { flex: 0 0 auto; width: 30px; height: 30px; border-radius: 50%; display: grid; place-items: center; background: var(--warn); color: #fff; font-weight: 800; }
.grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(215px, 1fr)); gap: 16px; }
.card { position: relative; overflow: hidden; padding: 20px; border-radius: 20px; background: var(--card); border: 1px solid var(--card-border); box-shadow: var(--shadow); backdrop-filter: blur(12px); -webkit-backdrop-filter: blur(12px); }
.card::before { content: ""; position: absolute; inset: 0 0 auto; height: 3px; background: var(--accent); opacity: .9; }
.temp::before { background: var(--temp); }
.hum::before { background: var(--hum); }
.pres::before { background: var(--pres); }
.soil::before { background: var(--soil); }
.card-top { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 16px; }
.label { font-size: .74rem; font-weight: 750; letter-spacing: .1em; text-transform: uppercase; color: var(--muted); }
.icon { width: 36px; height: 36px; border-radius: 11px; display: grid; place-items: center; background: var(--line); color: var(--accent); }
.icon svg { width: 21px; height: 21px; }
.temp .icon { color: var(--temp); }
.hum .icon { color: var(--hum); }
.pres .icon { color: var(--pres); }
.soil .icon { color: var(--soil); }
.value { display: flex; align-items: baseline; gap: 6px; font-variant-numeric: tabular-nums; }
.value .number { font-size: clamp(2.15rem, 5vw, 2.8rem); font-weight: 750; line-height: 1; letter-spacing: -.035em; }
.unit { font-size: 1rem; font-weight: 650; color: var(--muted); }
.hint { margin: 13px 0 0; color: var(--muted); font-size: .82rem; }
.soil { grid-column: 1 / -1; }
.bar { height: 10px; margin-top: 17px; border-radius: 999px; background: var(--line); overflow: hidden; }
.bar span { display: block; width: 0; height: 100%; border-radius: inherit; background: linear-gradient(90deg, var(--accent), var(--accent-2)); transition: width .55s ease; }
.foot { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; margin-top: 22px; color: var(--muted); font-size: .8rem; }
@media (max-width: 560px) {
  .top { align-items: flex-start; }
  .status { order: 2; }
  .soil { grid-column: auto; }
}
@media (prefers-reduced-motion: reduce) {
  *, *::before, *::after { animation: none !important; transition: none !important; }
}
</style>
</head>
<body>
<main class="wrap">
  <header class="top">
    <div class="brand">
      <div class="logo" aria-hidden="true">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M5 19c0-7 5-12 14-13 0 9-5 14-12 14H5z"/><path d="M5 19c3-5 6-7 10-9"/></svg>
      </div>
      <div>
        <h1>Plant Monitor</h1>
        <p class="subtitle">Live sensor readings from your plant</p>
      </div>
    </div>
    <div id="status" class="status" role="status" aria-live="polite">
      <span class="dot" aria-hidden="true"></span>
      <span id="statusText">Connecting</span>
    </div>
  </header>

  <div id="waterAlert" class="alert" role="alert">
    <span class="alert-icon" aria-hidden="true">!</span>
    <div>
      <strong>Water me!</strong>
      <span>Soil moisture is below 30%.</span>
    </div>
  </div>

  <section class="grid" aria-label="Sensor readings">
    <article class="card temp">
      <div class="card-top">
        <span class="label">Temperature</span>
        <span class="icon" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M10 13.5V5a2 2 0 1 1 4 0v8.5a4 4 0 1 1-4 0z"/></svg></span>
      </div>
      <div class="value"><span id="tempValue" class="number">--</span><span class="unit">°C</span></div>
      <p class="hint">Air temperature</p>
    </article>

    <article class="card hum">
      <div class="card-top">
        <span class="label">Humidity</span>
        <span class="icon" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3s6 6.5 6 11a6 6 0 0 1-12 0c0-4.5 6-11 6-11z"/></svg></span>
      </div>
      <div class="value"><span id="humValue" class="number">--</span><span class="unit">%</span></div>
      <p class="hint">Relative humidity</p>
    </article>

    <article class="card pres">
      <div class="card-top">
        <span class="label">Pressure</span>
        <span class="icon" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M4 15a8 8 0 1 1 16 0"/><path d="M12 15l4-4"/><path d="M12 19a4 4 0 1 0 0-8"/></svg></span>
      </div>
      <div class="value"><span id="presValue" class="number">--</span><span class="unit">hPa</span></div>
      <p class="hint">Atmospheric pressure</p>
    </article>

    <article class="card soil">
      <div class="card-top">
        <span class="label">Soil moisture</span>
        <span class="icon" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M12 21v-8"/><path d="M12 13c0-4 3-7 7-7 0 4-3 7-7 7z"/><path d="M12 16c0-3-2-5-5-5 0 3 2 5 5 5z"/></svg></span>
      </div>
      <div class="value"><span id="soilValue" class="number">--</span><span class="unit">%</span></div>
      <div class="bar" role="progressbar" aria-label="Soil moisture" aria-valuemin="0" aria-valuemax="100" aria-valuenow="0"><span id="soilBar"></span></div>
      <p class="hint">Water when below 30%</p>
    </article>
  </section>

  <footer class="foot">
    <span id="updated">Waiting for first reading...</span>
    <span>Refreshes every 3 seconds</span>
  </footer>
</main>
<script>
(function () {
  "use strict";
  var REFRESH_MS = 3000;
  var statusEl = document.getElementById("status");
  var statusText = document.getElementById("statusText");
  var updatedEl = document.getElementById("updated");
  var waterAlert = document.getElementById("waterAlert");
  var soilBar = document.getElementById("soilBar");
  var tempEl = document.getElementById("tempValue");
  var humEl = document.getElementById("humValue");
  var presEl = document.getElementById("presValue");
  var soilEl = document.getElementById("soilValue");
  var timer = null;
  var inFlight = false;

  function setStatus(live) {
    statusEl.classList.toggle("offline", !live);
    statusText.textContent = live ? "Live" : "Offline";
  }

  function format(value, digits) {
    var n = Number(value);
    if (!isFinite(n)) {
      return "--";
    }
    return digits === 0 ? String(Math.round(n)) : n.toFixed(digits);
  }

  function refresh() {
    if (inFlight) {
      return;
    }
    inFlight = true;
    fetch("/api/readings", { cache: "no-store" })
      .then(function (res) {
        if (!res.ok) {
          throw new Error("HTTP " + res.status);
        }
        return res.json();
      })
      .then(function (data) {
        tempEl.textContent = format(data.temperature_c, 1);
        humEl.textContent = format(data.humidity_pct, 1);
        presEl.textContent = format(data.pressure_hpa, 1);
        soilEl.textContent = format(data.soil_moisture_pct, 0);

        var pct = Number(data.soil_moisture_pct);
        pct = isFinite(pct) ? Math.max(0, Math.min(100, pct)) : 0;
        soilBar.style.width = pct + "%";
        soilBar.parentNode.setAttribute("aria-valuenow", String(Math.round(pct)));

        waterAlert.classList.toggle("visible", Boolean(data.needs_water));
        setStatus(true);
        updatedEl.textContent = "Updated " + new Date().toLocaleTimeString();
      })
      .catch(function () {
        setStatus(false);
        updatedEl.textContent = "Update failed, retrying...";
      })
      .then(function () {
        inFlight = false;
      });
  }

  function start() {
    if (timer !== null) {
      return;
    }
    refresh();
    timer = setInterval(refresh, REFRESH_MS);
  }

  function stop() {
    if (timer !== null) {
      clearInterval(timer);
      timer = null;
    }
  }

  document.addEventListener("visibilitychange", function () {
    if (document.hidden) {
      stop();
    } else {
      start();
    }
  });

  start();
})();
</script>
</body>
</html>
`

func serveWebUI(ex *httphi.Exchange) {
	if err := ex.RespondString(httphi.StatusOK, "text/html; charset=utf-8", webUIHTML); err != nil {
		println("serveWebUI:", err.Error())
	}
}
