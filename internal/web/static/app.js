// geokeep 前端：单文件 vanilla JS。
// 所有 URL 必须经 base() 拼接，保证子路径反代场景下能正确寻址。
(() => {
  const BASE = window.__GEOKEEP_BASE__ || "";
  const base = (p) => BASE + p;
  const $ = (id) => document.getElementById(id);
  const show = (id, on = true) => $(id).classList.toggle("hidden", !on);

  let me = null;
  let map = null;
  let layer = null;
  let devices = [];

  async function api(path, opts = {}) {
    const headers = Object.assign({ "Content-Type": "application/json" }, opts.headers || {});
    const r = await fetch(base(path), { credentials: "same-origin", ...opts, headers });
    if (!r.ok) {
      const text = await r.text().catch(() => "");
      throw new Error(text || `HTTP ${r.status}`);
    }
    const ct = r.headers.get("content-type") || "";
    return ct.includes("application/json") ? r.json() : r.text();
  }

  function fmtLocal(epoch) {
    const d = new Date(epoch * 1000);
    return d.toLocaleString();
  }

  function epochOfLocal(value) {
    if (!value) return 0;
    return Math.floor(new Date(value).getTime() / 1000);
  }

  function setRange(kind) {
    const now = new Date();
    let start;
    if (kind === "today") start = new Date(now.getFullYear(), now.getMonth(), now.getDate());
    else if (kind === "7d") start = new Date(now.getTime() - 7 * 24 * 3600 * 1000);
    else if (kind === "30d") start = new Date(now.getTime() - 30 * 24 * 3600 * 1000);
    $("from").value = toInputLocal(start);
    $("to").value = toInputLocal(now);
  }

  function toInputLocal(d) {
    const pad = (n) => String(n).padStart(2, "0");
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
  }

  async function loadDevices() {
    const data = await api("/api/v1/devices");
    devices = data.devices || [];
    const box = $("devices");
    box.innerHTML = "";
    devices.forEach((d) => {
      const id = `dev-${d.ID || d.id}`;
      const did = d.ID || d.id;
      const name = d.Name || d.name;
      box.insertAdjacentHTML(
        "beforeend",
        `<label><input type="checkbox" data-device="${did}" checked> ${name}</label>`
      );
      void id;
    });
  }

  async function applyQuery() {
    const from = epochOfLocal($("from").value);
    const to = epochOfLocal($("to").value);
    const sample = $("sample").value;
    const params = new URLSearchParams({ from, to, sample });
    const selected = Array.from(document.querySelectorAll("[data-device]"))
      .filter((el) => el.checked)
      .map((el) => el.dataset.device);
    selected.forEach((id) => params.append("device_id", id));
    const data = await api("/api/v1/points?" + params);
    drawPoints(data.points);
    const sum = await api("/api/v1/stats/summary?" + new URLSearchParams({ from, to }));
    $("stats").textContent = `点数 ${sum.count}  距离 ${(sum.distance_m / 1000).toFixed(2)} km`;
  }

  function drawPoints(points) {
    if (layer) layer.remove();
    if (!points || points.length === 0) {
      layer = null;
      return;
    }
    const latlngs = points.map((p) => [p.lat, p.lon]);
    layer = L.layerGroup([
      L.polyline(latlngs, { color: "#3a7afe", weight: 3, opacity: 0.85 }),
      ...latlngs.map((ll, i) =>
        L.circleMarker(ll, { radius: 3, color: "#1e4ec0", weight: 1 }).bindTooltip(
          fmtLocal(points[i].ts)
        )
      ),
    ]).addTo(map);
    map.fitBounds(L.latLngBounds(latlngs).pad(0.1));
  }

  async function initBootstrap() {
    const cfg = await api("/api/v1/bootstrap");
    if (!cfg.initialized) {
      show("setup", true);
      return;
    }
    try {
      me = await api("/api/v1/me");
      enterMain(cfg);
    } catch {
      show("login", true);
    }
  }

  async function enterMain(cfg) {
    show("setup", false);
    show("login", false);
    show("main", true);
    $("me").textContent = me ? me.email : "";
    map = L.map("map").setView([0, 0], 2);
    L.tileLayer(cfg.osm_tile || "https://tile.openstreetmap.org/{z}/{x}/{y}.png", {
      attribution: "© OpenStreetMap contributors",
      maxZoom: 19,
    }).addTo(map);
    setRange("today");
    await loadDevices();
    await applyQuery();
    await refreshImports();
    await refreshExports();
  }

  async function refreshImports() {
    const data = await api("/api/v1/imports");
    $("import-list").innerHTML = (data.imports || [])
      .slice(0, 10)
      .map(
        (im) =>
          `<div>#${im.ID} ${im.Source} ${im.Status} 写入${im.Processed} dup${im.Doubles}</div>`
      )
      .join("");
  }

  async function refreshExports() {
    const data = await api("/api/v1/exports");
    $("export-list").innerHTML = (data.exports || [])
      .slice(0, 10)
      .map((ex) => {
        const dl =
          ex.Status === "completed"
            ? `<a href="${base("/api/v1/exports/" + ex.ID + "/download")}">下载</a>`
            : ex.Status;
        return `<div>#${ex.ID} ${ex.FileFormat} ${dl}</div>`;
      })
      .join("");
  }

  function bind() {
    $("setup-btn").onclick = async () => {
      try {
        const r = await api("/api/v1/setup", {
          method: "POST",
          body: JSON.stringify({ email: $("setup-email").value, password: $("setup-pw").value }),
        });
        const el = $("setup-result");
        el.classList.remove("hidden");
        el.textContent = `初始化成功！API Key（请妥善保存，仅显示一次）：\n${r.api_key}\n\n请登录继续。`;
        setTimeout(() => {
          show("setup", false);
          show("login", true);
        }, 200);
      } catch (e) {
        alert("失败: " + e.message);
      }
    };

    $("login-btn").onclick = async () => {
      try {
        await api("/api/v1/auth/login", {
          method: "POST",
          body: JSON.stringify({ email: $("login-email").value, password: $("login-pw").value }),
        });
        location.reload();
      } catch (e) {
        $("login-err").textContent = "登录失败: " + e.message;
      }
    };

    $("logout-btn").onclick = async () => {
      await api("/api/v1/auth/logout", { method: "POST" });
      location.reload();
    };

    document.querySelectorAll("[data-range]").forEach((b) => {
      b.onclick = () => setRange(b.dataset.range);
    });
    $("apply").onclick = applyQuery;
    $("import-btn").onclick = async () => {
      const f = $("import-file").files[0];
      if (!f) {
        alert("请选择文件");
        return;
      }
      const fd = new FormData();
      fd.append("file", f);
      const fmt = $("import-format").value;
      if (fmt) fd.append("format", fmt);
      const r = await fetch(base("/api/v1/imports"), {
        method: "POST",
        body: fd,
        credentials: "same-origin",
      });
      if (!r.ok) {
        alert("失败: " + (await r.text()));
        return;
      }
      setTimeout(refreshImports, 500);
      setTimeout(refreshImports, 2500);
    };
    $("export-btn").onclick = async () => {
      const from = epochOfLocal($("from").value);
      const to = epochOfLocal($("to").value);
      const format = $("export-format").value;
      await api("/api/v1/exports", {
        method: "POST",
        body: JSON.stringify({ format, start_at: from, end_at: to, name: "geokeep-" + format }),
      });
      setTimeout(refreshExports, 500);
      setTimeout(refreshExports, 2500);
    };

    $("settings-btn").onclick = async () => {
      const m = await api("/api/v1/me");
      $("apikey-tail").textContent = m.api_key_suffix;
      show("settings", true);
    };
    $("settings-close").onclick = () => show("settings", false);
    $("rotate-btn").onclick = async () => {
      if (!confirm("旧 API Key 会立即失效，确定继续？")) return;
      const r = await api("/api/v1/api-key/rotate", { method: "POST" });
      const el = $("rotate-result");
      el.classList.remove("hidden");
      el.textContent = "新 API Key（请妥善保存，仅显示一次）：\n" + r.api_key;
    };
    $("backup-btn").onclick = () => {
      window.location.href = base("/api/v1/backup");
    };
    $("restore-btn").onclick = async () => {
      const f = $("restore-file").files[0];
      if (!f) {
        alert("请选择 .db 文件");
        return;
      }
      const fd = new FormData();
      fd.append("file", f);
      const r = await fetch(base("/api/v1/restore"), {
        method: "POST",
        body: fd,
        credentials: "same-origin",
      });
      if (!r.ok) {
        alert("失败: " + (await r.text()));
        return;
      }
      alert("已上传，请重启服务以完成恢复。");
    };
  }

  bind();
  initBootstrap().catch((e) => {
    document.body.insertAdjacentHTML("afterbegin", `<pre class="err">${e.message}</pre>`);
  });
})();
