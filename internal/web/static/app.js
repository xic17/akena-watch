/* Akena Watch — Siempre en Guardia. Frontend sin dependencias: vanilla JS. */
"use strict";

// --- utilidades ---
const $ = (sel, el = document) => el.querySelector(sel);
const $$ = (sel, el = document) => [...el.querySelectorAll(sel)];

function esc(s) {
  return String(s ?? "").replace(/[&<>"']/g, (c) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
  }[c]));
}

async function api(path, opts = {}) {
  const res = await fetch(path, {
    headers: { "Content-Type": "application/json", ...(opts.headers || {}) },
    ...opts,
  });
  let body = null;
  try { body = await res.json(); } catch { /* sin cuerpo JSON */ }
  if (!res.ok) {
    throw new Error((body && body.error) ? body.error : "Error " + res.status);
  }
  return body;
}

function fmtTime(iso) {
  if (!iso) return "—";
  const d = new Date(iso);
  if (isNaN(d)) return "—";
  const diff = (Date.now() - d.getTime()) / 1000;
  if (diff < 60) return "hace " + Math.max(0, Math.round(diff)) + "s";
  if (diff < 3600) return "hace " + Math.round(diff / 60) + " min";
  if (diff < 86400) return "hace " + Math.round(diff / 3600) + " h";
  return d.toLocaleString();
}

function fmtLat(ms) {
  if (ms === null || ms === undefined || ms <= 0) return "—";
  return ms + " ms";
}

function toast(msg, kind = "ok") {
  let root = $("#toast-root");
  if (!root) {
    root = document.createElement("div");
    root.id = "toast-root";
    document.body.appendChild(root);
  }
  const el = document.createElement("div");
  el.className = "toast " + kind;
  el.textContent = msg;
  root.appendChild(el);
  setTimeout(() => { el.style.opacity = "0"; el.style.transition = "opacity .3s"; }, 2600);
  setTimeout(() => el.remove(), 3000);
}

// --- modal ---
function openModal(html, wide) {
  // Si la página no define #modal-root (p. ej. Herramientas o Acerca de),
  // lo creamos sobre la marcha, igual que toast(): así el botón Perfil
  // funciona en cualquier página sin depender de la plantilla.
  let root = $("#modal-root");
  if (!root) {
    root = document.createElement("div");
    root.id = "modal-root";
    document.body.appendChild(root);
  }
  root.innerHTML =
    '<div class="modal-backdrop" onclick="if(event.target===this)closeModal()">' +
    '<div class="modal card' + (wide ? " wide" : "") + '">' + html + "</div></div>";
  const first = $("#modal-root input, #modal-root select, #modal-root button");
  if (first) first.focus();
}
function closeModal() {
  const root = $("#modal-root");
  if (root) root.innerHTML = "";
}

// --- autenticación (setup / login) ---
function bindAuthForm(formId, endpoint, redirect) {
  const f = document.getElementById(formId);
  if (!f) return;
  f.addEventListener("submit", async (e) => {
    e.preventDefault();
    const errEl = $("#form-error");
    errEl.classList.add("hidden");
    const fd = new FormData(f);
    const username = fd.get("username").trim();
    const password = fd.get("password");
    const confirm = fd.get("confirm");
    if (confirm !== null && password !== confirm) {
      errEl.textContent = "Las contraseñas no coinciden";
      errEl.classList.remove("hidden");
      return;
    }
    try {
      // solo se envían los campos presentes en el formulario: el login no
      // acepta email/telegram_id y el servidor rechaza campos desconocidos.
      const payload = { username, password };
      const email = fd.get("email");
      const tg = fd.get("telegram_id");
      if (email !== null) payload.email = email.trim();
      if (tg !== null) payload.telegram_id = tg.trim();
      const res = await api(endpoint, { method: "POST", body: JSON.stringify(payload) });
      location.href = res.redirect || redirect;
    } catch (err) {
      errEl.textContent = err.message;
      errEl.classList.remove("hidden");
    }
  });
}
bindAuthForm("setup-form", "/api/setup", "/dashboard");
bindAuthForm("login-form", "/api/login", "/dashboard");

const logoutBtn = $("#logout-btn");
if (logoutBtn) {
  logoutBtn.addEventListener("click", async () => {
    try { await api("/api/logout", { method: "POST" }); } catch { /* ignorar */ }
    location.href = "/login";
  });
}

// --- mi perfil (correo + ID de Telegram) ---
const profileBtn = $("#profile-btn");
if (profileBtn) {
  profileBtn.addEventListener("click", async () => {
    try {
      const me = await api("/api/me");
      openModal(`
        <h2>Mi perfil</h2>
        <p class="modal-sub muted">${esc(me.username)} · ${esc(me.role === "admin" ? "Administrador" : "Colaborador")}</p>
        <form id="profile-form">
          <label>Correo (opcional)
            <input name="email" type="email" maxlength="254" autocomplete="off" value="${esc(me.email || "")}" placeholder="usuario@dominio.com">
          </label>
          <label>ID de Telegram (opcional)
            <input name="telegram_id" maxlength="32" autocomplete="off" spellcheck="false" value="${esc(me.telegram_id || "")}" placeholder="123456789 o -1001234567890">
          </label>
          <p class="field-note">Con tu ID en el perfil, los monitores con "Avisarme por Telegram" te notifican directamente.</p>
          <div class="modal-actions">
            <button class="btn ghost" type="button" onclick="closeModal()">Cerrar</button>
            <button class="btn primary" type="submit">Guardar</button>
          </div>
        </form>`);
      $("#profile-form").addEventListener("submit", async (e) => {
        e.preventDefault();
        const fd = new FormData(e.target);
        try {
          await api("/api/me", {
            method: "PUT",
            body: JSON.stringify({ email: (fd.get("email") || "").trim(), telegram_id: (fd.get("telegram_id") || "").trim() }),
          });
          toast("Perfil actualizado");
          closeModal();
        } catch (err) {
          toast(err.message, "bad");
        }
      });
    } catch (err) {
      toast(err.message, "bad");
    }
  });
}

// --- página de estado pública ---
// El slug viaja en un atributo data- para mantener la plantilla válida.
const STATUS_SLUG = (() => {
  const list = document.getElementById("sp-list");
  return list ? list.dataset.slug : "";
})();
if (STATUS_SLUG) {
  async function loadStatus() {
    try {
      const data = await api("/status/" + encodeURIComponent(STATUS_SLUG) + "?json=1");
      $("#sp-title").textContent = data.title;
      $("#sp-desc").textContent = data.desc;
      const list = $("#sp-list");
      if (!data.monitors.length) {
        list.innerHTML = '<p class="muted">No hay servicios publicados todavía.</p>';
      } else {
        list.innerHTML = data.monitors.map((m) => `
          <div class="card monitor-row${m.active === false ? " paused" : ""}">
            <span class="dot ${m.active === false ? "" : m.slow ? "slow" : m.status === "up" ? "up" : m.status === "down" ? "down" : ""}"></span>
            <div class="monitor-main">
              <div class="monitor-name">${esc(m.name)} <span class="badge">${esc(m.type)}</span>${m.active === false ? '<span class="badge paused">pausado</span>' : ""}${m.slow ? '<span class="badge slow">lento</span>' : ""}${m.maint ? '<span class="badge maint">mantenimiento</span>' : ""}</div>
              ${m.status === "down" && m.error && m.active !== false ? `<div class="monitor-url error-text small">${esc(m.error)}</div>` : ""}
              ${m.history ? `<div class="history-strip" title="Últimas 24 horas">${m.history.map((st) => `<span class="h-cell ${st}"></span>`).join("")}</div>` : ""}
            </div>
            <div class="monitor-stat">uptime 30 días<br><b>${m.uptime_30d !== undefined ? m.uptime_30d + "%" : "—"}</b></div>
            <div class="monitor-stat">${m.active === false ? '<span class="muted">pausado</span>' : m.slow ? '<span class="slow-text">' + fmtLat(m.latency_ms) + "</span>" : m.status === "up" ? fmtLat(m.latency_ms) : m.status === "down" ? '<span class="error-text">caído</span>' : '<span class="muted">—</span>'}</div>
          </div>`).join("");
      }
      $("#sp-updated").textContent = new Date().toLocaleTimeString();
    } catch (err) {
      $("#sp-list").innerHTML = `<p class="error">${esc(err.message)}</p>`;
    }
  }
  loadStatus();
  setInterval(loadStatus, 30000);
}

// --- dashboard ---
if (document.getElementById("monitor-list")) {
  const TYPE_LABEL = { http: "HTTP", tcp: "TCP", dns: "DNS" };
  // iconos en línea para pausar/reanudar (sin dependencias externas)
  const PAUSE_ICON = '<svg viewBox="0 0 12 12" width="11" height="11" fill="currentColor" aria-hidden="true"><rect x="2" y="1.5" width="3" height="9" rx="1"/><rect x="7" y="1.5" width="3" height="9" rx="1"/></svg>';
  const PLAY_ICON = '<svg viewBox="0 0 12 12" width="11" height="11" fill="currentColor" aria-hidden="true"><path d="M3 1.6 L10.4 6 L3 10.4 Z"/></svg>';
  let MONITORS = [];
  let NOTIFS = [];
  let USERS = [];
  let HB = {}; // heartbeats por monitor, para las gráficas

  // buildHB agrupa los heartbeats por monitor y los deja en orden cronológico.
  function buildHB(heartbeats) {
    const out = {};
    for (const h of heartbeats) {
      (out[h.monitor_id] = out[h.monitor_id] || []).push({
        lat: h.latency_ms || 0,
        status: h.status,
        checked_at: h.checked_at,
      });
    }
    for (const id in out) out[id].reverse();
    return out;
  }

  async function loadDashboard() {
    try {
      const [m, n, u, sp, hb] = await Promise.all([
        api("/api/monitors"),
        api("/api/notifications"),
        api("/api/users"),
        api("/api/statuspage"),
        api("/api/heartbeats?hours=24"),
      ]);
      MONITORS = m.monitors;
      NOTIFS = n.notifications;
      USERS = u.users;
      HB = buildHB(hb.heartbeats);
      renderSummary();
      renderMonitors();
      renderStatusSettingsBtn(sp);
      connectWS();
    } catch (err) {
      $("#monitor-list").innerHTML = `<p class="error">${esc(err.message)}</p>`;
    }
  }

  // --- resumen estadístico ---
  function renderSummary() {
    const el = $("#summary-cards");
    if (!el) return;
    const active = MONITORS.filter((m) => m.active);
    const up = active.filter((m) => m.last_heartbeat && m.last_heartbeat.status === "up").length;
    const down = active.filter((m) => m.last_heartbeat && m.last_heartbeat.status === "down").length;
    const pending = active.filter((m) => !m.last_heartbeat).length;
    const paused = MONITORS.length - active.length;
    const withUptime = active.filter((m) => typeof m.uptime_24h === "number");
    const avg = withUptime.length
      ? withUptime.reduce((a, m) => a + m.uptime_24h, 0) / withUptime.length
      : null;
    el.innerHTML = `
      <div class="stat-card"><span class="stat-value up">${up}</span><span class="stat-label">En línea</span></div>
      <div class="stat-card"><span class="stat-value down">${down}</span><span class="stat-label">Caídos</span></div>
      <div class="stat-card"><span class="stat-value amber">${pending}</span><span class="stat-label">Sin datos</span></div>
      <div class="stat-card"><span class="stat-value">${paused}</span><span class="stat-label">Pausados</span></div>
      <div class="stat-card"><span class="stat-value">${avg !== null ? avg.toFixed(1) + "%" : "—"}</span><span class="stat-label">Uptime medio 24 h</span></div>`;
  }

  // --- gráfica de latencia (SVG, sin librerías) ---
  function pointsFor(m) {
    return (HB[m.id] || []).map((h) => ({ lat: h.lat, status: h.status }));
  }

  function sparklineSVG(points) {
    const w = 140, h = 30;
    if (!points.length) return '<span class="muted small">sin datos</span>';
    const maxLat = Math.max(200, ...points.map((p) => p.lat || 0));
    const n = points.length;
    const x = (i) => (n === 1 ? w / 2 : (i / (n - 1)) * w);
    const y = (p) => h - 3 - Math.min(1, (p.lat || 0) / maxLat) * (h - 8);
    let d = "";
    let downs = "";
    points.forEach((p, i) => {
      d += (i ? "L" : "M") + x(i).toFixed(1) + " " + y(p).toFixed(1);
      if (p.status === "down") {
        downs += `<circle cx="${x(i).toFixed(1)}" cy="${y(p).toFixed(1)}" r="2.4" fill="var(--red)"/>`;
      }
    });
    const last = points[points.length - 1];
    const stroke = last.status === "down" ? "var(--red)" : "var(--green)";
    return `<svg class="spark" viewBox="0 0 ${w} ${h}" width="${w}" height="${h}" preserveAspectRatio="none" aria-hidden="true">
      <path d="${d}" fill="none" stroke="${stroke}" stroke-width="1.5" vector-effect="non-scaling-stroke"/>${downs}</svg>`;
  }

  function renderStatusSettingsBtn(sp) {
    $("#status-settings-btn").onclick = () => openStatusModal(sp);
  }

  // ¿puede el usuario actual editar este monitor? (propietario, admin o
  // compartición con edición; los accesos por grupo/manual son solo vista)
  function canEditClient(m) {
    if (ME_IS_ADMIN || m.owner_id === ME_ID) return true;
    return (m.shares || []).some((s) => s.user_id === ME_ID && s.can_edit);
  }

  function renderMonitors() {
    const list = $("#monitor-list");
    if (!MONITORS.length) {
      list.innerHTML = '<p class="muted">Aún no hay monitores. Crea el primero con "+ Nuevo monitor".</p>';
      return;
    }
    list.innerHTML = MONITORS.map((m) => {
      const lh = m.last_heartbeat;
      const slow = !!m.slow || (m.latency_threshold_ms > 0 && lh && lh.status === "up" && lh.latency_ms >= m.latency_threshold_ms);
      const dot = !m.active ? "" : !lh ? "" : slow ? "slow" : lh.status === "up" ? "up" : "down";
      return `
      <div class="card monitor-row${m.active ? "" : " paused"}" data-id="${m.id}" data-status="${lh ? lh.status : ""}">
        <span class="dot ${dot}"></span>
        <div class="monitor-main">
          <div class="monitor-name">
            ${esc(m.name)}
            <span class="badge">${TYPE_LABEL[m.type] || m.type}</span>
            ${m.group ? `<span class="badge group">📁 ${esc(m.group)}</span>` : ""}
            ${m.public ? '<span class="badge amber">público</span>' : ""}
            ${!m.active ? '<span class="badge paused">pausado</span>' : ""}
            ${slow ? '<span class="badge slow">lento</span>' : ""}
            ${m.maint ? '<span class="badge maint">mantenimiento</span>' : ""}
            ${m.owner !== undefined && m.owner_id !== ME_ID ? '<span class="badge">de ' + esc(m.owner) + "</span>" : ""}
          </div>
          <div class="monitor-url muted small">${esc(m.url)}</div>
          ${lh && lh.status === "down" && lh.error && m.active ? `<div class="monitor-url error-text small">${esc(lh.error)}</div>` : ""}
        </div>
        <div class="spark-wrap" data-cell="spark" title="Latencia · últimas 24 h">${sparklineSVG(pointsFor(m))}</div>
        <div class="monitor-stat" data-cell="latency">
          ${!m.active ? '<span class="muted">pausado</span>' : !lh ? '<span class="muted">—</span>' : slow ? `<span class="slow-text">${fmtLat(lh.latency_ms)}</span>` : lh.status === "up" ? fmtLat(lh.latency_ms) : '<span class="error-text">caído</span>'}
        </div>
        <div class="monitor-stat">uptime 24 h<br><b>${m.uptime_24h !== undefined ? m.uptime_24h + "%" : "—"}</b></div>
        <div class="monitor-stat" data-cell="last">último check<br><span class="muted small">${fmtTime(lh && lh.checked_at)}</span></div>
        <div class="monitor-actions">
          ${canEditClient(m) ? `<button class="btn tiny ghost icon-btn" type="button" title="${m.active ? "Pausar" : "Reanudar"}" onclick="togglePause(${m.id})">${m.active ? PAUSE_ICON : PLAY_ICON}</button>` : ""}
          <button class="btn tiny ghost" type="button" onclick="openMonitorDetails(${m.id})">Detalles</button>
          <button class="btn tiny ghost" type="button" onclick="testMonitor(${m.id}, this)">Probar</button>
          ${canEditClient(m) ? `<button class="btn tiny ghost" type="button" onclick="openMonitorModal(${m.id})">Editar</button>` : ""}
          ${canEditClient(m) ? `<button class="btn tiny ghost" type="button" title="Duplicar monitor" onclick="duplicateMonitor(${m.id})">Duplicar</button>` : ""}
          ${m.owner_id === ME_ID || ME_IS_ADMIN ? `<button class="btn tiny danger" type="button" onclick="deleteMonitor(${m.id})">Borrar</button>` : ""}
        </div>
      </div>`;
    }).join("");
  }

  function updateRow(mon) {
    const row = document.querySelector(`.monitor-row[data-id="${mon.id}"]`);
    if (!row) return;
    const lh = { status: mon.status, latency_ms: mon.latency_ms, error: mon.error, checked_at: mon.checked_at };
    const m = MONITORS.find((x) => x.id === mon.id);
    if (m) m.last_heartbeat = lh;
    const slow = !!mon.slow || (m && m.latency_threshold_ms > 0 && mon.status === "up" && mon.latency_ms >= m.latency_threshold_ms);
    row.dataset.status = mon.status;
    const dotEl = $(".dot", row);
    dotEl.className = "dot " + (slow ? "slow" : mon.status === "up" ? "up" : "down");
    const lat = $('[data-cell="latency"]', row);
    lat.innerHTML = slow ? `<span class="slow-text">${fmtLat(mon.latency_ms)}</span>`
      : mon.status === "up" ? fmtLat(mon.latency_ms) : '<span class="error-text">caído</span>';
    const nameEl = $(".monitor-name", row);
    let slowBadge = nameEl.querySelector(".badge.slow");
    if (slow && !slowBadge) {
      const b = document.createElement("span");
      b.className = "badge slow";
      b.textContent = "lento";
      nameEl.insertBefore(b, nameEl.querySelector(".badge").nextSibling);
    } else if (!slow && slowBadge) {
      slowBadge.remove();
    }
    const last = $('[data-cell="last"]', row);
    last.innerHTML = 'último check<br><span class="muted small">' + fmtTime(mon.checked_at) + "</span>";
    let errCell = $(".monitor-url.error-text", row);
    if (mon.status === "down" && mon.error) {
      if (!errCell) {
        errCell = document.createElement("div");
        errCell.className = "monitor-url error-text small";
        $(".monitor-main", row).appendChild(errCell);
      }
      errCell.textContent = mon.error;
    } else if (errCell) {
      errCell.remove();
    }
    // gráfica y resumen en tiempo real
    const arr = HB[mon.id] || (HB[mon.id] = []);
    arr.push({ lat: mon.latency_ms || 0, status: mon.status, checked_at: mon.checked_at });
    if (arr.length > 500) arr.shift();
    const spark = $('[data-cell="spark"]', row);
    if (spark) spark.innerHTML = sparklineSVG(pointsFor({ id: mon.id }));
    renderSummary();
  }

  // --- WebSocket: tiempo real ---
  function connectWS() {
    const proto = location.protocol === "https:" ? "wss" : "ws";
    const ws = new WebSocket(`${proto}://${location.host}/ws`);
    ws.onmessage = (ev) => {
      let msg;
      try { msg = JSON.parse(ev.data); } catch { return; }
      if (msg.type === "heartbeat") updateRow(msg.monitor);
    };
    ws.onclose = () => setTimeout(connectWS, 3000);
  }

  // --- acciones de monitores ---
  // --- vista de detalle de un monitor ---
  window.openMonitorDetails = async (id) => {
    const m = MONITORS.find((x) => x.id === id);
    if (!m) return;
    const notifNames = (m.notifier_ids || [])
      .map((nid) => (NOTIFS.find((n) => n.id === nid) || {}).name)
      .filter(Boolean);
    const shareNames = (m.shares || []).map((s) => s.username);

    openModal(`
      <h2>${esc(m.name)} <span class="badge">${TYPE_LABEL[m.type] || m.type}</span></h2>
      <p class="modal-sub muted">${esc(m.url)}</p>
      <div id="md-stats" class="tile-grid"><p class="muted small">Calculando…</p></div>
      <div class="chart-wrap"><div id="md-chart"><p class="muted small">Cargando gráfica…</p></div></div>
      <div class="chart-range">
        <button class="btn tiny ghost active" type="button" data-h="24">24 h</button>
        <button class="btn tiny ghost" type="button" data-h="168">7 días</button>
      </div>
      <p class="field-note">${m.group ? "Grupo " + esc(m.group) + " · " : ""}Intervalo ${m.interval_s} s · Timeout ${m.timeout_s} s · Reintentos ${m.max_retries}${notifNames.length ? " · Canales: " + esc(notifNames.join(", ")) : ""}${shareNames.length ? " · Compartido con: " + esc(shareNames.join(", ")) : ""}</p>
      <div id="md-events"><p class="muted small">Cargando eventos…</p></div>
      <div class="modal-actions">
        <button class="btn ghost" type="button" onclick="closeModal()">Cerrar</button>
      </div>`, true);

    const loadChart = async (hours, btn) => {
      $$("#md-range button").forEach((b) => b.classList.toggle("active", b === btn));
      try {
        const res = await api(`/api/monitors/${id}/heartbeats?hours=${hours}`);
        renderBigChart(res.heartbeats, hours);
      } catch (err) {
        $("#md-chart").innerHTML = `<p class="error">${esc(err.message)}</p>`;
      }
    };
    $$("#md-range button").forEach((b) => {
      b.onclick = () => loadChart(parseInt(b.dataset.h, 10), b);
    });

    try {
      const stats = await api(`/api/monitors/${id}/stats`);
      renderStatTiles(stats);
      renderEvents(stats.events);
    } catch (err) {
      $("#md-stats").innerHTML = `<p class="error">${esc(err.message)}</p>`;
    }
    loadChart(24, $("#md-range button.active"));
  };

  function renderStatTiles(stats) {
    const u = stats.uptime;
    const l = stats.latency;
    const tile = (label, value, cls = "") =>
      `<div class="tile"><span class="tile-value ${cls}">${value}</span><span class="tile-label">${label}</span></div>`;
    $("#md-stats").innerHTML =
      tile("Uptime 24 h", u["24h"] !== undefined ? u["24h"] + "%" : "—") +
      tile("Uptime 7 d", u["7d"] !== undefined ? u["7d"] + "%" : "—") +
      tile("Uptime 30 d", u["30d"] !== undefined ? u["30d"] + "%" : "—") +
      tile("Checks (7 d)", stats.checks.total + " · " + stats.checks.up + "↑ " + stats.checks.down + "↓") +
      tile("Latencia mín.", fmtLat(l.min), "up") +
      tile("Latencia media", fmtLat(Math.round(l.avg)), "") +
      tile("p95", fmtLat(l.p95), "amber") +
      tile("Latencia máx.", fmtLat(l.max), "down");
  }

  function renderEvents(events) {
    if (!events.length) {
      $("#md-events").innerHTML = '<p class="muted small">Sin eventos en los últimos 7 días.</p>';
      return;
    }
    $("#md-events").innerHTML = `
      <p class="field-note" style="margin-top:12px">Últimos eventos (7 días):</p>
      <div class="events-scroll"><table class="events-table">
        <thead><tr><th>Hora</th><th>Estado</th><th>Código</th><th>Latencia</th><th>Detalle</th></tr></thead>
        <tbody>${events.map((e) => `
          <tr>
            <td class="muted">${new Date(e.checked_at).toLocaleString()}</td>
            <td>${e.status === "up" ? '<span class="up-text">En línea</span>' : '<span class="error-text">Caído</span>'}</td>
            <td>${e.code || "—"}</td>
            <td>${fmtLat(e.latency_ms)}</td>
            <td class="muted small">${esc(e.error || "")}</td>
          </tr>`).join("")}
        </tbody>
      </table></div>`;
  }

  // gráfica grande de latencia (SVG, sin librerías)
  function renderBigChart(heartbeats, hours) {
    const points = heartbeats.slice().reverse(); // cronológico
    if (!points.length) {
      $("#md-chart").innerHTML = '<p class="muted small">Sin datos en el período.</p>';
      return;
    }
    const w = 520, h = 130;
    const maxLat = Math.max(200, ...points.map((p) => p.latency_ms || 0));
    const n = points.length;
    const x = (i) => (n === 1 ? w / 2 : (i / (n - 1)) * w);
    const y = (p) => h - 20 - Math.min(1, (p.latency_ms || 0) / maxLat) * (h - 48);
    let d = "";
    let downs = "";
    points.forEach((p, i) => {
      d += (i ? "L" : "M") + x(i).toFixed(1) + " " + y(p).toFixed(1);
      if (p.status === "down") {
        downs += `<circle cx="${x(i).toFixed(1)}" cy="${y(p).toFixed(1)}" r="3.2" fill="var(--red)"/>`;
      }
    });
    const last = points[n - 1];
    const stroke = last.status === "down" ? "var(--red)" : "var(--green)";
    const tick = (i) => {
      const t = new Date(points[i].checked_at);
      const label =
        (hours === 24
          ? t.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
          : t.toLocaleDateString([], { day: "2-digit", month: "2-digit" })) +
        (i === n - 1 ? " · ahora" : "");
      return `<text x="${x(i).toFixed(1)}" y="${h - 5}" font-size="10" fill="var(--muted)" text-anchor="${i === 0 ? "start" : i === n - 1 ? "end" : "middle"}">${esc(label)}</text>`;
    };
    $("#md-chart").innerHTML = `
      <svg class="big-chart" viewBox="0 0 ${w} ${h}" preserveAspectRatio="none" aria-hidden="true">
        <line x1="0" y1="${h - 16}" x2="${w}" y2="${h - 16}" stroke="var(--border)" stroke-width="1"/>
        <path d="${d}" fill="none" stroke="${stroke}" stroke-width="2" vector-effect="non-scaling-stroke"/>${downs}
        ${tick(0)}${tick(Math.floor((n - 1) / 2))}${tick(n - 1)}
      </svg>`;
  }

  window.testMonitor = async (id, btn) => {
    btn.disabled = true;
    btn.textContent = "…";
    try {
      const res = await api(`/api/monitors/${id}/test`, { method: "POST" });
      toast(res.status === "up" ? "OK · " + fmtLat(res.latency_ms) : "Falló · " + res.error,
        res.status === "up" ? "ok" : "bad");
    } catch (err) {
      toast(err.message, "bad");
    } finally {
      btn.disabled = false;
      btn.textContent = "Probar";
    }
  };

  window.deleteMonitor = async (id) => {
    const m = MONITORS.find((x) => x.id === id);
    if (!confirm(`¿Eliminar el monitor "${m ? m.name : id}" y todo su historial?`)) return;
    try {
      await api(`/api/monitors/${id}`, { method: "DELETE" });
      toast("Monitor eliminado");
      MONITORS = MONITORS.filter((x) => x.id !== id);
      renderSummary();
      renderMonitors();
    } catch (err) {
      toast(err.message, "bad");
    }
  };

  // pausar/reanudar un monitor desde el icono del listado
  window.togglePause = async (id) => {
    const m = MONITORS.find((x) => x.id === id);
    if (!m) return;
    const target = !m.active;
    try {
      const res = await api(`/api/monitors/${id}/active`, { method: "PUT", body: JSON.stringify({ active: target }) });
      const idx = MONITORS.findIndex((x) => x.id === id);
      if (idx >= 0) MONITORS[idx] = res.monitor;
      renderSummary();
      renderMonitors();
      toast(target ? "Monitor pausado" : "Monitor reanudado");
    } catch (err) {
      toast(err.message, "bad");
    }
  };

  // duplicar un monitor (configuración + canales, nombre con "(copia)")
  window.duplicateMonitor = async (id) => {
    try {
      const res = await api(`/api/monitors/${id}/duplicate`, { method: "POST" });
      MONITORS.push(res.monitor);
      renderSummary();
      renderMonitors();
      toast("Monitor duplicado");
    } catch (err) {
      toast(err.message, "bad");
    }
  };

  // --- modal de monitor (crear / editar) ---
  window.openMonitorModal = (id) => {
    const m = id ? MONITORS.find((x) => x.id === id) : null;
    const isEdit = !!m;
    const f = m || { type: "http", method: "GET", expected_status: 200, timeout_s: 10, interval_s: 60, max_retries: 1, active: true, notify: true, notify_owner: false, public: false, invert_keyword: false, body: "", group: "", notifier_ids: [], latency_threshold_ms: 0, slow_retries: 3, cert_alert_days: 0, maint_enabled: false, maint_weekday: 0, maint_start: "02:00", maint_end: "04:00" };
    const shares = (m && m.shares) || [];
    const activeNotifs = NOTIFS.filter((n) => n.active);
    const inactiveNotifs = NOTIFS.filter((n) => !n.active);
    const notifBoxes = activeNotifs.map((n) =>
      `<label class="check-row"><input type="checkbox" name="notif" value="${n.id}" ${(f.notifier_ids || []).includes(n.id) ? "checked" : ""}> ${esc(n.name)} <span class="badge">${esc(n.type)}</span></label>`).join("") +
      inactiveNotifs.map((n) =>
      `<label class="check-row" style="opacity:.55"><input type="checkbox" name="notif" value="${n.id}" disabled ${(f.notifier_ids || []).includes(n.id) ? "checked" : ""}> ${esc(n.name)} <span class="badge">${esc(n.type)}</span> <span class="badge">inactivo</span></label>`).join("") ||
      '<p class="field-note">No hay canales. Créalos en "Canales de alerta".</p>';

    const shareList = shares.map((sh) =>
      `<div class="share-row" style="display:flex;justify-content:space-between;gap:8px;align-items:center;padding:4px 0">
        <span>${esc(sh.username)} <span class="badge">${sh.can_edit ? "edición" : "lectura"}</span></span>
        <button class="btn tiny danger" type="button" onclick="removeShare(${f.id}, ${sh.user_id})">Quitar</button>
      </div>`).join("");

    const userOpts = USERS.filter((u) => u.id !== ME_ID).map((u) =>
      `<option value="${u.id}">${esc(u.username)}</option>`).join("");

    const html = `
      <h2>${isEdit ? "Editar monitor" : "Nuevo monitor"}</h2>
      <p class="modal-sub muted">Cada monitor se comprueba según su intervalo.</p>
      <form id="monitor-form">
        <div class="form-grid">
          <div class="full">
            <label>Nombre
              <input name="name" required maxlength="64" value="${esc(f.name || "")}" placeholder="p. ej. Web principal">
            </label>
          </div>
          <div>
            <label>Tipo
              <select name="type" id="m-type">
                <option value="http" ${f.type === "http" ? "selected" : ""}>HTTP / HTTPS</option>
                <option value="tcp" ${f.type === "tcp" ? "selected" : ""}>TCP</option>
                <option value="dns" ${f.type === "dns" ? "selected" : ""}>DNS</option>
              </select>
            </label>
          </div>
          <div>
            <label>Intervalo (segundos)
              <input name="interval_s" type="number" min="10" max="86400" required value="${f.interval_s}">
            </label>
          </div>
          <div class="full">
            <label>Destino (URL o host)
              <input name="url" required value="${esc(f.url || "")}" placeholder="${f.type === "http" ? "https://ejemplo.com" : "ejemplo.com:443 / ejemplo.com"}">
            </label>
          </div>
          <div class="full">
            <label>Grupo (opcional)
              <input name="group" maxlength="64" value="${esc(f.group || "")}" placeholder="web, api, base de datos…">
            </label>
            <p class="field-note">Los colaboradores pueden recibir acceso a monitores por grupo.</p>
          </div>
          <div id="m-http-fields" class="${f.type === "http" ? "" : "hidden"}">
            <label>Método
              <select name="method">
                ${["GET", "POST", "HEAD", "PUT", "PATCH", "DELETE", "OPTIONS"].map((mth) => `<option ${f.method === mth ? "selected" : ""}>${mth}</option>`).join("")}
              </select>
            </label>
            <label>Estado HTTP esperado
              <input name="expected_status" type="number" min="100" max="599" value="${f.expected_status}">
            </label>
            <label>Palabra clave (opcional)
              <input name="keyword" value="${esc(f.keyword || "")}" placeholder="buscar en la respuesta">
            </label>
            <label class="check-row"><input type="checkbox" name="invert_keyword" ${f.invert_keyword ? "checked" : ""}> Alertar si la palabra clave SÍ aparece</label>
            <label>Avisar si el certificado expira en ≤ (días)
              <input name="cert_alert_days" type="number" min="0" max="365" value="${f.cert_alert_days || 0}" title="0 = desactivado">
            </label>
            <p class="field-note">Comprueba el certificado TLS una vez al día y avisa cuando queden menos días que el umbral.</p>
          </div>
          <div>
            <label>Timeout (segundos)
              <input name="timeout_s" type="number" min="1" max="120" required value="${f.timeout_s}">
            </label>
            <label>Reintentos antes de alertar
              <input name="max_retries" type="number" min="1" max="10" value="${f.max_retries}">
            </label>
            <div id="m-body-field" class="${f.type === "http" ? "" : "hidden"}">
              <label>Cuerpo JSON (opcional)
                <textarea name="body" rows="4" spellcheck="false" placeholder='{"query": "estado"}'>${esc(f.body || "")}</textarea>
              </label>
              <p class="field-note">Se envía con Content-Type: application/json (útil con POST/PUT/PATCH).</p>
            </div>
          </div>
        </div>

        <div class="form-grid">
          <div>
            <label>Umbral de lentitud (ms)
              <input name="latency_threshold_ms" type="number" min="0" max="60000" value="${f.latency_threshold_ms || 0}" title="0 = desactivado">
            </label>
          </div>
          <div>
            <label>Checks seguidos para avisar
              <input name="slow_retries" type="number" min="1" max="10" value="${f.slow_retries || 3}">
            </label>
          </div>
          <div class="full">
            <p class="field-note">Si la latencia supera el umbral durante N checks seguidos (con estado "arriba"), se alerta como lento. 0 desactiva la función.</p>
          </div>
        </div>

        <label class="check-row"><input type="checkbox" name="active" ${f.active ? "checked" : ""}> Monitor activo</label>
        <label class="check-row"><input type="checkbox" name="notify" ${f.notify ? "checked" : ""}> Enviar alertas por los canales marcados</label>
        <label class="check-row"><input type="checkbox" name="notify_owner" ${f.notify_owner ? "checked" : ""}> Avisarme por Telegram (a mi ID del perfil)</label>
        <label class="check-row"><input type="checkbox" name="public" ${f.public ? "checked" : ""}> Mostrar en la página de estado pública</label>

        <div class="form-grid">
          <div class="full">
            <label class="check-row"><input type="checkbox" name="maint_enabled" ${f.maint_enabled ? "checked" : ""}> Ventana de mantenimiento (sin alertas)</label>
          </div>
          <div id="maint-fields" class="full ${f.maint_enabled ? "" : "hidden"}">
            <div class="form-grid">
              <label>Día
                <select name="maint_weekday">
                  ${["Domingo", "Lunes", "Martes", "Miércoles", "Jueves", "Viernes", "Sábado"].map((d, i) => `<option value="${i}" ${f.maint_weekday === i ? "selected" : ""}>${d}</option>`).join("")}
                </select>
              </label>
              <label>Desde
                <input name="maint_start" type="time" value="${esc(f.maint_start || "02:00")}">
              </label>
              <label>Hasta
                <input name="maint_end" type="time" value="${esc(f.maint_end || "04:00")}">
              </label>
              <div class="full">
                <p class="field-note">Durante la ventana no se envían alertas (caída, lentitud, certificado), pero los checks siguen registrando historial. Hora local del servidor.</p>
              </div>
            </div>
          </div>
        </div>

        <p class="field-note" style="margin-top:14px">Canales de alerta:</p>
        ${notifBoxes}

        ${isEdit ? `
          <p class="field-note" style="margin-top:14px">Compartir con:</p>
          ${shareList}
          <div style="display:flex;gap:8px;margin-top:6px">
            <select id="share-user" style="flex:1">${userOpts}</select>
            <select id="share-mode" style="width:auto">
              <option value="false">lectura</option>
              <option value="true">edición</option>
            </select>
            <button class="btn tiny" type="button" onclick="addShare(${f.id})">Compartir</button>
          </div>` : ""}

        <div class="modal-actions">
          <button class="btn ghost" type="button" onclick="closeModal()">Cancelar</button>
          <button class="btn primary" type="submit">${isEdit ? "Guardar cambios" : "Crear monitor"}</button>
        </div>
      </form>`;

    openModal(html);

    const typeSel = $("#m-type");
    const toggleHttp = () => {
      const isHttp = typeSel.value === "http";
      $("#m-http-fields").classList.toggle("hidden", !isHttp);
      const bodyField = $("#m-body-field");
      if (bodyField) bodyField.classList.toggle("hidden", !isHttp);
    };
    typeSel.addEventListener("change", toggleHttp);

    // mostrar/ocultar los campos de la ventana de mantenimiento
    const maintCheck = $('input[name="maint_enabled"]', $("#monitor-form"));
    const maintFields = $("#maint-fields");
    if (maintCheck && maintFields) {
      maintCheck.addEventListener("change", () => maintFields.classList.toggle("hidden", !maintCheck.checked));
    }

    $("#monitor-form").addEventListener("submit", async (e) => {
      e.preventDefault();
      const fd = new FormData(e.target);
      const payload = {
        name: fd.get("name").trim(),
        group: fd.get("group").trim(),
        type: fd.get("type"),
        url: fd.get("url").trim(),
        method: fd.get("method") || "GET",
        expected_status: parseInt(fd.get("expected_status") || "200", 10),
        keyword: fd.get("keyword") || "",
        body: fd.get("body") || "",
        invert_keyword: fd.get("invert_keyword") === "on",
        timeout_s: parseInt(fd.get("timeout_s"), 10),
        interval_s: parseInt(fd.get("interval_s"), 10),
        max_retries: parseInt(fd.get("max_retries") || "1", 10),
        latency_threshold_ms: parseInt(fd.get("latency_threshold_ms") || "0", 10),
        slow_retries: parseInt(fd.get("slow_retries") || "3", 10),
        cert_alert_days: parseInt(fd.get("cert_alert_days") || "0", 10),
        maint_enabled: fd.get("maint_enabled") === "on",
        maint_weekday: parseInt(fd.get("maint_weekday") || "0", 10),
        maint_start: fd.get("maint_start") || "",
        maint_end: fd.get("maint_end") || "",
        active: fd.get("active") === "on",
        notify: fd.get("notify") === "on",
        notify_owner: fd.get("notify_owner") === "on",
        public: fd.get("public") === "on",
        notifier_ids: $$('input[name="notif"]:checked', e.target).map((c) => parseInt(c.value, 10)),
      };
      try {
        if (isEdit) {
          const res = await api(`/api/monitors/${f.id}`, { method: "PUT", body: JSON.stringify(payload) });
          const idx = MONITORS.findIndex((x) => x.id === f.id);
          MONITORS[idx] = res.monitor;
          toast("Monitor actualizado");
        } else {
          const res = await api("/api/monitors", { method: "POST", body: JSON.stringify(payload) });
          MONITORS.push(res.monitor);
          toast("Monitor creado");
        }
        closeModal();
        renderSummary();
        renderMonitors();
      } catch (err) {
        toast(err.message, "bad");
      }
    });
  };

  window.addShare = async (monitorId) => {
    const uid = parseInt($("#share-user").value, 10);
    const canEdit = $("#share-mode").value === "true";
    try {
      await api(`/api/monitors/${monitorId}/share/${uid}`, { method: "PUT", body: JSON.stringify({ can_edit: canEdit }) });
      toast("Compartido");
      openMonitorModal(monitorId);
    } catch (err) {
      toast(err.message, "bad");
    }
  };

  window.removeShare = async (monitorId, userId) => {
    try {
      await api(`/api/monitors/${monitorId}/share/${userId}`, { method: "DELETE" });
      toast("Se quitó la compartición");
      openMonitorModal(monitorId);
    } catch (err) {
      toast(err.message, "bad");
    }
  };

  // --- modal de canales de alerta ---
  $("#notifs-btn").addEventListener("click", () => openNotifsModal());

  function notifConfigFields(type, cfg = {}) {
    if (type === "webhook") {
      return `<label>URL del webhook
        <input name="cfg_url" type="url" required value="${esc(cfg.url || "")}" placeholder="https://hook.example.com/...">
      </label>
      <label>Cuerpo JSON personalizado (opcional)
        <textarea name="cfg_body" rows="5" spellcheck="false" placeholder='{"text": "{{monitorName}} está {{status}}", "url": "{{monitorUrl}}"}'>${esc(cfg.body || "")}</textarea>
      </label>
      <p class="field-note">Variables: {{monitorName}} {{monitorUrl}} {{monitorType}} {{status}} {{msg}} {{latency}} {{time}} {{localtime}}</p>
      <p class="field-note">Si se deja vacío, se envía el mensaje por defecto: {"text": "..."}</p>`;
    }
    if (type === "telegram") {
      return `<label>Bot token
        <input name="cfg_bot_token" value="${esc(cfg.bot_token || "")}" placeholder="123456:ABC-DEF...">
      </label>
      <label>Chat ID
        <input name="cfg_chat_id" value="${esc(cfg.chat_id || "")}" placeholder="-1001234567890">
      </label>`;
    }
    return `<div class="form-grid">
      <div><label>Host SMTP <input name="cfg_host" required value="${esc(cfg.host || "")}" placeholder="smtp.ejemplo.com"></label></div>
      <div><label>Puerto <input name="cfg_port" type="number" min="1" max="65535" required value="${cfg.port || 587}"></label></div>
      <div><label>Usuario <input name="cfg_user" autocomplete="off" value="${esc(cfg.user || "")}"></label></div>
      <div><label>Contraseña <input name="cfg_pass" type="password" autocomplete="new-password" value="${esc(cfg.pass || "")}"></label></div>
      <div><label>Desde <input name="cfg_from" required value="${esc(cfg.from || "")}" placeholder="akena@dominio.com"></label></div>
      <div><label>Para <input name="cfg_to" required value="${esc(cfg.to || "")}" placeholder="quien@dominio.com"></label></div>
    </div>`;
  }

  function openNotifsModal() {
    const rows = NOTIFS.map((n) => `
      <div style="display:flex;justify-content:space-between;align-items:center;gap:10px;padding:8px 0;border-bottom:1px solid var(--border)">
        <div>
          <strong>${esc(n.name)}</strong>
          <span class="badge">${esc(n.type)}</span>
          ${n.active ? "" : '<span class="badge">inactivo</span>'}
        </div>
        <div style="display:flex;gap:6px">
          <button class="btn tiny ghost" type="button" onclick="testNotif(${n.id})">Probar</button>
          <button class="btn tiny ghost" type="button" onclick="openNotifForm(${n.id})">Editar</button>
          <button class="btn tiny danger" type="button" onclick="deleteNotif(${n.id})">Borrar</button>
        </div>
      </div>`).join("") || '<p class="muted">Sin canales de alerta todavía.</p>';

    openModal(`
      <h2>Canales de alerta</h2>
      <p class="modal-sub muted">Webhook, Telegram y email SMTP. Luego asócialos a cada monitor.</p>
      <div style="max-height:260px;overflow-y:auto">${rows}</div>
      <div class="modal-actions">
        <button class="btn ghost" type="button" onclick="closeModal()">Cerrar</button>
        <button class="btn primary" type="button" onclick="openNotifForm()">+ Nuevo canal</button>
      </div>`);
  }

  window.openNotifForm = (id) => {
    const n = id ? NOTIFS.find((x) => x.id === id) : null;
    const isEdit = !!n;
    const cfg = (n && n.config) || {};
    openModal(`
      <h2>${isEdit ? "Editar canal" : "Nuevo canal"}</h2>
      <form id="notif-form">
        <label>Nombre
          <input name="name" required maxlength="64" value="${esc(n ? n.name : "")}" placeholder="p. ej. Telegram del equipo">
        </label>
        <label>Tipo
          <select name="type" id="n-type" ${isEdit ? "disabled" : ""}>
            <option value="webhook" ${(n && n.type) === "webhook" || !n ? "selected" : ""}>Webhook</option>
            <option value="telegram" ${(n && n.type) === "telegram" ? "selected" : ""}>Telegram</option>
            <option value="smtp" ${(n && n.type) === "smtp" ? "selected" : ""}>Email SMTP</option>
          </select>
        </label>
        <div id="n-fields">${notifConfigFields(n ? n.type : "webhook", cfg)}</div>
        <label class="check-row"><input type="checkbox" name="active" ${!n || n.active ? "checked" : ""}> Canal activo</label>
        <div class="modal-actions">
          <button class="btn ghost" type="button" onclick="closeModal()">Cancelar</button>
          <button class="btn primary" type="submit">${isEdit ? "Guardar" : "Crear canal"}</button>
        </div>
      </form>`);

    const typeSel = $("#n-type");
    const syncFields = () => {
      $("#n-fields").innerHTML = notifConfigFields(typeSel.value, {});
    };
    typeSel.addEventListener("change", syncFields);

    $("#notif-form").addEventListener("submit", async (e) => {
      e.preventDefault();
      const fd = new FormData(e.target);
      const type = isEdit ? n.type : fd.get("type");
      const cfg = {};
      if (type === "webhook") { cfg.url = fd.get("cfg_url"); cfg.body = fd.get("cfg_body") || ""; }
      if (type === "telegram") { cfg.bot_token = fd.get("cfg_bot_token"); cfg.chat_id = fd.get("cfg_chat_id"); }
      if (type === "smtp") {
        cfg.host = fd.get("cfg_host"); cfg.port = parseInt(fd.get("cfg_port") || "587", 10);
        cfg.user = fd.get("cfg_user"); cfg.pass = fd.get("cfg_pass");
        cfg.from = fd.get("cfg_from"); cfg.to = fd.get("cfg_to");
      }
      const payload = { name: fd.get("name").trim(), type, config: cfg, active: fd.get("active") === "on" };
      try {
        if (isEdit) {
          const res = await api(`/api/notifications/${n.id}`, { method: "PUT", body: JSON.stringify(payload) });
          const idx = NOTIFS.findIndex((x) => x.id === n.id);
          NOTIFS[idx] = res.notification;
          toast("Canal actualizado");
        } else {
          const res = await api("/api/notifications", { method: "POST", body: JSON.stringify(payload) });
          NOTIFS.push(res.notification);
          toast("Canal creado");
        }
        closeModal();
        openNotifsModal();
      } catch (err) {
        toast(err.message, "bad");
      }
    });
  };

  window.testNotif = async (id) => {
    try {
      await api(`/api/notifications/${id}/test`, { method: "POST" });
      toast("Prueba enviada — revisa el canal");
    } catch (err) {
      toast(err.message, "bad");
    }
  };

  window.deleteNotif = async (id) => {
    const n = NOTIFS.find((x) => x.id === id);
    if (!confirm(`¿Eliminar el canal "${n ? n.name : id}"?`)) return;
    try {
      await api(`/api/notifications/${id}`, { method: "DELETE" });
      NOTIFS = NOTIFS.filter((x) => x.id !== id);
      toast("Canal eliminado");
      openNotifsModal();
    } catch (err) {
      toast(err.message, "bad");
    }
  };

  // --- modal de página de estado ---
  function openStatusModal(sp) {
    openModal(`
      <h2>Página de estado pública</h2>
      <p class="modal-sub muted">Los monitores marcados como "públicos" aparecerán aquí, sin necesidad de iniciar sesión.</p>
      <form id="status-form">
        <label>Título
          <input name="title" maxlength="100" value="${esc(sp.title || "")}">
        </label>
        <label>Descripción
          <textarea name="desc" maxlength="500">${esc(sp.desc || "")}</textarea>
        </label>
        <p class="field-note">Dirección pública: <a href="${esc(sp.url || "")}" target="_blank">${esc(sp.url || "")}</a> (${sp.public_count} monitor(es) publicado(s))</p>
        <div class="modal-actions">
          <button class="btn ghost" type="button" onclick="closeModal()">Cerrar</button>
          <button class="btn primary" type="submit">Guardar</button>
        </div>
      </form>`);
    $("#status-form").addEventListener("submit", async (e) => {
      e.preventDefault();
      const fd = new FormData(e.target);
      try {
        await api("/api/statuspage", { method: "PUT", body: JSON.stringify({ title: fd.get("title").trim(), desc: fd.get("desc").trim() }) });
        toast("Página de estado actualizada");
        closeModal();
      } catch (err) {
        toast(err.message, "bad");
      }
    });
  }

  $("#new-monitor-btn").addEventListener("click", () => openMonitorModal());

  // ME_ID se resuelve antes de cargar el dashboard para etiquetar
  // correctamente los monitores ajenos en los listados.
  let ME_ID = 0;
  let ME_IS_ADMIN = false;
  api("/api/me").then((me) => { ME_ID = me.id; ME_IS_ADMIN = me.role === "admin"; }).catch(() => {}).finally(() => loadDashboard());
}

// --- gestión de usuarios (solo admin) ---
if (document.getElementById("user-list")) {
  let USERS = [];
  let ALL_MONITORS = [];
  let GROUPS = [];

  async function loadUsers() {
    try {
      const [res, mon, grp] = await Promise.all([
        api("/api/users"),
        api("/api/monitors"), // el admin ve todos: para asignar acceso manual
        api("/api/groups"),
      ]);
      USERS = res.users;
      ALL_MONITORS = mon.monitors;
      GROUPS = grp.groups;
      renderUsers();
    } catch (err) {
      $("#user-list").innerHTML = `<p class="error">${esc(err.message)}</p>`;
    }
  }

  function renderUsers() {
    $("#user-list").innerHTML = `
      <table class="user-table">
        <thead><tr><th>Usuario</th><th>Rol</th><th>Monitores</th><th>Creado</th><th></th></tr></thead>
        <tbody>
          ${USERS.map((u) => `
            <tr>
              <td>
                <strong>${esc(u.username)}</strong>
                ${u.email ? `<div class="muted small">${esc(u.email)}</div>` : ""}
                ${u.telegram_id ? `<div class="muted small">✈️ ${esc(u.telegram_id)}</div>` : ""}
                ${u.groups && u.groups.length ? `<div class="muted small">Grupos: ${esc(u.groups.join(", "))}</div>` : ""}
                ${u.access && u.access.length ? `<div class="muted small">${u.access.length} monitor(es) asignados</div>` : ""}
              </td>
              <td><span class="role-${esc(u.role)}">${u.role === "admin" ? "Administrador" : "Colaborador"}</span></td>
              <td>${u.monitors}</td>
              <td class="muted">${esc(u.created_at)}</td>
              <td style="text-align:right">
                <button class="btn tiny ghost" type="button" onclick="editUser(${u.id})">Editar</button>
                <button class="btn tiny danger" type="button" onclick="deleteUser(${u.id})">Borrar</button>
              </td>
            </tr>`).join("")}
        </tbody>
      </table>`;
  }

  // sección compartida "Monitores que puede ver" (crear y editar usuario)
  function accessSectionHTML(u) {
    const groups = u.groups || [];
    const access = u.access || [];
    return `
      <div id="user-access" class="${u.role === "admin" ? "hidden" : ""}">
        <p class="field-note" style="margin-top:14px">Monitores que puede ver:</p>
        <p class="field-note">Por grupos</p>
        <div class="access-box">
          ${GROUPS.length ? GROUPS.map((g) =>
            `<label class="check-row"><input type="checkbox" name="group" value="${esc(g.name)}" ${groups.includes(g.name) ? "checked" : ""}> ${esc(g.name)} <span class="badge">${g.count}</span></label>`).join("") :
            '<p class="muted small">Aún no hay grupos. Asigna grupos a los monitores.</p>'}
        </div>
        <p class="field-note">Monitores específicos</p>
        <div class="access-box">
          ${ALL_MONITORS.length ? ALL_MONITORS.map((m) =>
            `<label class="check-row"><input type="checkbox" name="monitor" value="${m.id}" ${access.includes(m.id) ? "checked" : ""}> ${esc(m.name)} <span class="badge">${esc(m.group || m.type)}</span></label>`).join("") :
            '<p class="muted small">No hay monitores en el sistema.</p>'}
        </div>
        <p class="field-note">Los administradores ven todos los monitores.</p>
      </div>`;
  }

  function bindAccessToggle(formEl) {
    const accessSection = $("#user-access", formEl);
    const roleSel = $("select[name='role']", formEl);
    roleSel.addEventListener("change", () => accessSection.classList.toggle("hidden", roleSel.value === "admin"));
  }

  function collectAccess(formEl) {
    const groups = $$('input[name="group"]:checked', formEl).map((c) => c.value);
    const monitors = $$('input[name="monitor"]:checked', formEl).map((c) => parseInt(c.value, 10));
    return { groups, monitors };
  }

  window.editUser = (id) => {
    const u = USERS.find((x) => x.id === id);
    if (!u) return;
    openModal(`
      <h2>Editar usuario</h2>
      <p class="modal-sub muted">${esc(u.username)}</p>
      <form id="edit-user-form">
        <label>Correo (opcional)
          <input name="email" type="email" maxlength="254" autocomplete="off" value="${esc(u.email || "")}" placeholder="usuario@dominio.com">
        </label>
        <label>ID de Telegram (opcional)
          <input name="telegram_id" maxlength="32" autocomplete="off" spellcheck="false" value="${esc(u.telegram_id || "")}" placeholder="123456789 o -1001234567890">
        </label>
        <label>Rol
          <select name="role">
            <option value="collaborator" ${u.role === "collaborator" ? "selected" : ""}>Colaborador</option>
            <option value="admin" ${u.role === "admin" ? "selected" : ""}>Administrador</option>
          </select>
        </label>
        ${accessSectionHTML(u)}
        <div class="modal-actions">
          <button class="btn ghost" type="button" onclick="closeModal()">Cancelar</button>
          <button class="btn primary" type="submit">Guardar</button>
        </div>
      </form>`);
    bindAccessToggle($("#edit-user-form"));
    $("#edit-user-form").addEventListener("submit", async (e) => {
      e.preventDefault();
      const fd = new FormData(e.target);
      const isAdmin = fd.get("role") === "admin";
      try {
        const jobs = [api(`/api/users/${id}`, {
          method: "PUT",
          body: JSON.stringify({ role: fd.get("role"), email: (fd.get("email") || "").trim(), telegram_id: (fd.get("telegram_id") || "").trim() }),
        })];
        if (!isAdmin) {
          const { groups, monitors } = collectAccess(e.target);
          jobs.push(api(`/api/users/${id}/groups`, { method: "PUT", body: JSON.stringify({ groups }) }));
          jobs.push(api(`/api/users/${id}/access`, { method: "PUT", body: JSON.stringify({ monitor_ids: monitors }) }));
        }
        await Promise.all(jobs);
        toast("Usuario actualizado");
        closeModal();
        loadUsers();
      } catch (err) {
        toast(err.message, "bad");
      }
    });
  };

  window.deleteUser = async (id) => {
    const u = USERS.find((x) => x.id === id);
    if (!confirm(`¿Eliminar al usuario ${u ? u.username : id}? Se borrarán sus monitores, historial y canales.`)) return;
    try {
      await api(`/api/users/${id}`, { method: "DELETE" });
      toast("Usuario eliminado");
      loadUsers();
    } catch (err) {
      toast(err.message, "bad");
    }
  };

  $("#new-user-btn").addEventListener("click", () => {
    openModal(`
      <h2>Nuevo usuario</h2>
      <p class="modal-sub muted">Los colaboradores gestionan sus propios monitores.</p>
      <form id="user-form">
        <label>Usuario
          <input name="username" required minlength="3" maxlength="32" autocomplete="off">
        </label>
        <label>Correo (opcional)
          <input name="email" type="email" maxlength="254" autocomplete="off" placeholder="usuario@dominio.com">
        </label>
        <label>ID de Telegram (opcional)
          <input name="telegram_id" maxlength="32" autocomplete="off" spellcheck="false" placeholder="123456789 o -1001234567890">
        </label>
        <label>Contraseña
          <input name="password" type="password" required minlength="8" autocomplete="new-password">
        </label>
        <label>Rol
          <select name="role">
            <option value="collaborator">Colaborador</option>
            <option value="admin">Administrador</option>
          </select>
        </label>
        ${accessSectionHTML({ role: "collaborator", groups: [], access: [] })}
        <div class="modal-actions">
          <button class="btn ghost" type="button" onclick="closeModal()">Cancelar</button>
          <button class="btn primary" type="submit">Crear usuario</button>
        </div>
      </form>`);
    bindAccessToggle($("#user-form"));
    $("#user-form").addEventListener("submit", async (e) => {
      e.preventDefault();
      const fd = new FormData(e.target);
      const isAdmin = fd.get("role") === "admin";
      try {
        const res = await api("/api/users", {
          method: "POST",
          body: JSON.stringify({ username: fd.get("username").trim(), email: (fd.get("email") || "").trim(), telegram_id: (fd.get("telegram_id") || "").trim(), password: fd.get("password"), role: fd.get("role") }),
        });
        if (!isAdmin) {
          const { groups, monitors } = collectAccess(e.target);
          await Promise.all([
            api(`/api/users/${res.user.id}/groups`, { method: "PUT", body: JSON.stringify({ groups }) }),
            api(`/api/users/${res.user.id}/access`, { method: "PUT", body: JSON.stringify({ monitor_ids: monitors }) }),
          ]);
        }
        toast("Usuario creado");
        closeModal();
        loadUsers();
      } catch (err) {
        toast(err.message, "bad");
      }
    });
  });

  loadUsers();
}

// --- herramientas: pestañas ---
const toolsTabs = document.getElementById("tools-tabs");
if (toolsTabs) {
  const panels = { ping: "tab-ping", whois: "tab-whois", dns: "tab-dns", http: "tab-http", tls: "tab-tls", ports: "tab-ports" };
  const saved = localStorage.getItem("akena_tool_tab");
  const switchTab = (name) => {
    $$(".tab", toolsTabs).forEach((b) => {
      const active = b.dataset.tab === name;
      b.classList.toggle("active", active);
      b.setAttribute("aria-selected", active ? "true" : "false");
    });
    for (const key in panels) {
      document.getElementById(panels[key]).classList.toggle("hidden", key !== name);
    }
    localStorage.setItem("akena_tool_tab", name);
  };
  $$(".tab", toolsTabs).forEach((b) => b.addEventListener("click", () => switchTab(b.dataset.tab)));
  switchTab(panels[saved] ? saved : "ping");
}

// --- herramientas: ping en tiempo real ---
const pingTool = document.getElementById("ping-tool");
if (pingTool) {
  const $f = (id) => document.getElementById(id);
  let pingWS = null;
  let pingPending = null;
  let pingPoints = []; // latencias para la gráfica (null = pérdida)
  let pingStats = { sent: 0, received: 0, lost: 0, min: 0, max: 0, sum: 0 };
  let pingErrorShown = false;

  function connectPingWS() {
    const proto = location.protocol === "https:" ? "wss" : "ws";
    pingWS = new WebSocket(`${proto}://${location.host}/ws/ping`);
    pingWS.onopen = () => {
      if (pingPending) {
        pingWS.send(JSON.stringify(pingPending));
        pingPending = null;
      }
    };
    pingWS.onmessage = (ev) => {
      let msg;
      try { msg = JSON.parse(ev.data); } catch { return; }
      if (msg.type === "result") onPingResult(msg);
      else if (msg.type === "done") stopPingUI(true);
      else if (msg.type === "error") { toast(msg.error, "bad"); stopPingUI(false); }
    };
    pingWS.onclose = () => setTimeout(connectPingWS, 2000);
  }

  function onPingResult(r) {
    pingStats.sent++;
    if (r.ok) {
      pingStats.received++;
      pingStats.sum += r.latency_ms;
      if (!pingStats.min || r.latency_ms < pingStats.min) pingStats.min = r.latency_ms;
      if (r.latency_ms > pingStats.max) pingStats.max = r.latency_ms;
      pingPoints.push(r.latency_ms);
    } else {
      pingStats.lost++;
      pingPoints.push(null);
    }
    if (pingPoints.length > 120) pingPoints.shift();
    renderPingStats();
    renderPingChart();
    prependPingLog(r);
    // error permanente (p. ej. ICMP sin permisos): detener con un solo aviso
    if (r.fatal) {
      showPingError(r.error);
      stopPingUI(false);
    }
  }

  function showPingError(text) {
    const box = $f("ping-error");
    box.textContent = "⚠️ " + text;
    box.classList.remove("hidden");
    pingErrorShown = true;
  }

  function renderPingStats() {
    const loss = pingStats.sent ? Math.round((pingStats.lost / pingStats.sent) * 100) : 0;
    $f("ps-sent").textContent = pingStats.sent;
    $f("ps-recv").textContent = pingStats.received;
    $f("ps-lost").textContent = pingStats.lost;
    $f("ps-loss").textContent = loss + "%";
    $f("ps-min").textContent = pingStats.min ? pingStats.min + " ms" : "—";
    $f("ps-avg").textContent = pingStats.received ? Math.round(pingStats.sum / pingStats.received) + " ms" : "—";
    $f("ps-max").textContent = pingStats.max ? pingStats.max + " ms" : "—";
  }

  function renderPingChart() {
    const el = $f("ping-chart");
    if (!el) return;
    $f("ping-chart-wrap").classList.remove("hidden");
    const w = 480, h = 80;
    const vals = pingPoints.filter((p) => p !== null);
    if (!vals.length) { el.innerHTML = ""; return; }
    const maxLat = Math.max(200, ...vals);
    const n = pingPoints.length;
    const x = (i) => (n === 1 ? w / 2 : (i / (n - 1)) * w);
    const y = (v) => h - 6 - Math.min(1, v / maxLat) * (h - 16);
    let d = "";
    let startNew = true;
    pingPoints.forEach((v, i) => {
      if (v === null) { startNew = true; return; }
      d += (startNew ? "M" : "L") + x(i).toFixed(1) + " " + y(v).toFixed(1);
      startNew = false;
    });
    const lost = pingPoints.map((v, i) =>
      v === null ? `<circle cx="${x(i).toFixed(1)}" cy="${h - 6}" r="2.2" fill="var(--red)"/>` : "").join("");
    el.innerHTML = `<svg viewBox="0 0 ${w} ${h}" preserveAspectRatio="none" class="spark" aria-hidden="true">
      ${d ? `<path d="${d}" fill="none" stroke="var(--amber-strong)" stroke-width="1.5" vector-effect="non-scaling-stroke"/>` : ""}${lost}</svg>`;
  }

  function prependPingLog(r) {
    const log = $f("ping-log");
    const div = document.createElement("div");
    div.className = "ping-line " + (r.ok ? "ok" : "bad");
    div.innerHTML = `<span class="muted">#${r.seq}</span>` +
      (r.ok ? `<span class="up-text">${fmtLat(r.latency_ms)}</span>` : `<span class="error-text">fallo</span>`) +
      `<span class="muted small">${esc(r.error || "")}</span>`;
    log.prepend(div);
    while (log.children.length > 100) log.lastChild.remove();
  }

  function stopPingUI(final) {
    $f("ping-start").disabled = false;
    $f("ping-stop").disabled = true;
    if (final && !pingErrorShown) toast("Ping finalizado");
  }

  const pingForm = $f("ping-form");
  const portLabel = $f("ping-port").closest("label");
  const protoSel = $("select[name='proto']", pingForm);
  const togglePort = () => portLabel.classList.toggle("hidden", protoSel.value !== "tcp");
  protoSel.addEventListener("change", togglePort);

  pingForm.addEventListener("submit", (e) => {
    e.preventDefault();
    const fd = new FormData(pingForm);
    pingPending = {
      type: "start",
      host: fd.get("host").trim(),
      proto: fd.get("proto"),
      port: parseInt(fd.get("port") || "443", 10),
      interval_ms: parseInt(fd.get("interval"), 10),
      count: parseInt(fd.get("count") || "0", 10),
    };
    pingStats = { sent: 0, received: 0, lost: 0, min: 0, max: 0, sum: 0 };
    pingPoints = [];
    pingErrorShown = false;
    $f("ping-error").classList.add("hidden");
    $f("ping-log").innerHTML = "";
    $f("ping-stats").classList.remove("hidden");
    $f("ping-chart-wrap").classList.add("hidden");
    renderPingStats();
    $f("ping-start").disabled = true;
    $f("ping-stop").disabled = false;
    if (!pingWS || pingWS.readyState !== WebSocket.OPEN) {
      connectPingWS(); // enviará pingPending cuando el socket abra
    } else {
      pingWS.send(JSON.stringify(pingPending));
      pingPending = null;
    }
  });

  $f("ping-stop").addEventListener("click", () => {
    if (pingWS && pingWS.readyState === WebSocket.OPEN) pingWS.send(JSON.stringify({ type: "stop" }));
    stopPingUI(false);
  });

  connectPingWS();
}

// --- herramientas: whois ---
const whoisTool = document.getElementById("whois-tool");
if (whoisTool) {
  const $wf = (id) => document.getElementById(id);
  let whoisText = "";

  $wf("whois-form").addEventListener("submit", async (e) => {
    e.preventDefault();
    const fd = new FormData(e.target);
    const domain = fd.get("domain").trim();
    const btn = $wf("whois-btn");
    btn.disabled = true;
    btn.textContent = "Consultando…";
    $wf("whois-error").classList.add("hidden");
    $wf("whois-output").classList.add("hidden");
    $wf("whois-copy").disabled = true;
    try {
      const res = await api("/api/whois?domain=" + encodeURIComponent(domain));
      whoisText = res.text;
      $wf("whois-output").textContent = whoisText;
      $wf("whois-output").classList.remove("hidden");
      $wf("whois-copy").disabled = false;
    } catch (err) {
      const box = $wf("whois-error");
      box.textContent = "⚠️ " + err.message;
      box.classList.remove("hidden");
    } finally {
      btn.disabled = false;
      btn.textContent = "Consultar";
    }
  });

  $wf("whois-copy").addEventListener("click", async () => {
    try {
      await navigator.clipboard.writeText(whoisText);
      toast("Texto WHOIS copiado");
    } catch {
      toast("No se pudo copiar", "bad");
    }
  });
}

// --- herramientas: dns lookup ---
const dnsTool = document.getElementById("dns-tool");
if (dnsTool) {
  const $df = (id) => document.getElementById(id);
  $df("dns-form").addEventListener("submit", async (e) => {
    e.preventDefault();
    const fd = new FormData(e.target);
    const host = fd.get("host").trim();
    const type = fd.get("type");
    const btn = $df("dns-btn");
    btn.disabled = true;
    btn.textContent = "Consultando…";
    $df("dns-error").classList.add("hidden");
    $df("dns-result").classList.add("hidden");
    try {
      const res = await api("/api/dns?host=" + encodeURIComponent(host) + "&type=" + encodeURIComponent(type));
      $df("dns-meta").textContent = res.records.length
        ? `${res.records.length} registro(s) ${res.type} de ${res.host} · ${res.elapsed_ms} ms`
        : `Sin registros ${res.type} para ${res.host} (el host existe) · ${res.elapsed_ms} ms`;
      $df("dns-rows").innerHTML = res.records.map((r) =>
        `<tr><td><span class="badge">${esc(r.type)}</span></td><td>${esc(r.value)}</td></tr>`).join("");
      $df("dns-result").classList.remove("hidden");
    } catch (err) {
      const box = $df("dns-error");
      box.textContent = "⚠️ " + err.message;
      box.classList.remove("hidden");
    } finally {
      btn.disabled = false;
      btn.textContent = "Consultar";
    }
  });
}

// --- herramientas: inspección HTTP ---
const httpTool = document.getElementById("http-tool");
if (httpTool) {
  const $hf = (id) => document.getElementById(id);

  $hf("http-form").addEventListener("submit", async (e) => {
    e.preventDefault();
    const fd = new FormData(e.target);
    const btn = $hf("http-btn");
    btn.disabled = true;
    btn.textContent = "Inspeccionando…";
    $hf("http-error").classList.add("hidden");
    $hf("http-result").classList.add("hidden");

    // headers extra: una por línea, formato "Nombre: valor"
    const headers = {};
    (fd.get("headers") || "").split("\n").forEach((line) => {
      const i = line.indexOf(":");
      if (i > 0) headers[line.slice(0, i).trim()] = line.slice(i + 1).trim();
    });

    try {
      const res = await api("/api/httpcheck", {
        method: "POST",
        body: JSON.stringify({
          url: fd.get("url").trim(),
          method: fd.get("method"),
          headers,
          body: fd.get("body") || "",
        }),
      });
      renderHTTPResult(res);
    } catch (err) {
      const box = $hf("http-error");
      box.textContent = "⚠️ " + err.message;
      box.classList.remove("hidden");
    } finally {
      btn.disabled = false;
      btn.textContent = "Inspeccionar";
    }
  });

  function renderHTTPResult(res) {
    const ok = res.status >= 200 && res.status < 400;
    $hf("http-summary").innerHTML =
      `${esc(res.method)} ${esc(res.final_url || res.url)} ` +
      `<span class="badge${ok ? "" : " amber"}">${esc(res.status_text || res.status || "")}</span>` +
      (res.total_ms !== undefined ? ` · ${res.total_ms.toFixed(1)} ms total` : "") +
      (res.error ? `<div class="error-text">⚠️ ${esc(res.error)}</div>` : "");

    const t = (label, ms) =>
      `<div class="stat-card"><span class="stat-value">${ms && ms > 0 ? ms.toFixed(1) + " ms" : "—"}</span><span class="stat-label">${label}</span></div>`;
    $hf("http-timings").innerHTML =
      t("DNS", res.dns_ms) + t("Conexión", res.connect_ms) + t("TLS", res.tls_ms) +
      t("TTFB", res.ttfb_ms) + t("Total", res.total_ms);

    $hf("http-redirects").innerHTML = res.redirects && res.redirects.length
      ? "<p class=\"field-note\">Redirecciones</p>" +
        res.redirects.map((rd, i) =>
          `<div class="muted small">${i + 1}. ${esc(rd.status)} → ${esc(rd.url)}</div>`).join("")
      : "";

    $hf("http-cert").innerHTML = res.cert && res.cert.subject
      ? `🔒 TLS ${esc(res.tls_proto || "")} · certificado: ${esc(res.cert.subject)} — expira ${esc(res.cert.expires)} ` +
        `<span class="badge${res.cert.days_left <= 14 ? " amber" : ""}">${res.cert.days_left} días</span>`
      : "";

    $hf("http-headers").innerHTML = Object.entries(res.headers || {}).map(([k, v]) =>
      `<tr><td>${esc(k)}</td><td>${esc(v)}</td></tr>`).join("");

    $hf("http-body-meta").textContent = res.body_len !== undefined ? `(${res.body_len} bytes leídos)` : "";
    $hf("http-body").textContent = res.body_preview || "(cuerpo vacío)";
    $hf("http-result").classList.remove("hidden");
  }
}

// --- herramientas: certificado TLS ---
const tlsTool = document.getElementById("tls-tool");
if (tlsTool) {
  const $tf = (id) => document.getElementById(id);

  $tf("tls-form").addEventListener("submit", async (e) => {
    e.preventDefault();
    const fd = new FormData(e.target);
    const btn = $tf("tls-btn");
    btn.disabled = true;
    btn.textContent = "Comprobando…";
    $tf("tls-error").classList.add("hidden");
    $tf("tls-result").classList.add("hidden");
    try {
      const res = await api("/api/tlscheck", {
        method: "POST",
        body: JSON.stringify({ host: fd.get("host").trim(), port: parseInt(fd.get("port") || "443", 10) }),
      });
      renderTLSResult(res);
    } catch (err) {
      const box = $tf("tls-error");
      box.textContent = "⚠️ " + err.message;
      box.classList.remove("hidden");
    } finally {
      btn.disabled = false;
      btn.textContent = "Comprobar";
    }
  });

  function renderTLSResult(res) {
    if (!res.success) {
      const box = $tf("tls-error");
      box.textContent = "⚠️ " + (res.error || "no se pudo conectar");
      box.classList.remove("hidden");
      return;
    }
    const c = res.cert || {};
    const status = c.expired ? '<span class="badge amber">expirado</span>'
      : c.not_yet_valid ? '<span class="badge amber">aún no válido</span>'
      : c.days_left <= 14 ? '<span class="badge amber">expira pronto</span>'
      : '<span class="badge">válido</span>';
    $tf("tls-summary").innerHTML =
      `${esc(res.host)}:${res.port} → ${status} · ${esc(res.tls_proto || "")} · ${res.handshake_ms} ms handshake`;

    const daysCls = c.days_left <= 14 ? (c.expired ? "down" : "amber") : "up";
    const t = (label, val, cls) =>
      `<div class="stat-card"><span class="stat-value${cls ? " " + cls : ""}">${val}</span><span class="stat-label">${label}</span></div>`;
    // las tarjetas grandes son para valores numéricos; los textos van a la tabla
    $tf("tls-stats").innerHTML =
      t("Días restantes", c.days_left !== undefined ? c.days_left : "—", daysCls) +
      t("Handshake", res.handshake_ms !== undefined ? res.handshake_ms + " ms" : "—");

    const row = (k, v) => `<tr><td style="width:30%;opacity:.8">${k}</td><td>${v}</td></tr>`;
    $tf("tls-rows").innerHTML = [
      row("Emisor", esc(c.issuer || "—")),
      row("Protocolo", esc(res.tls_proto || "—")),
      row("Cipher", esc(res.cipher || "—")),
      row("Sujeto", esc(c.subject || "—")),
      row("SANs", esc((c.sans || []).join(", ") || "—")),
      row("Válido desde", esc(c.not_before || "—")),
      row("Válido hasta", esc(c.not_after || "—")),
      row("Nº de serie", esc(c.serial || "—")),
      row("Firma", esc(c.sig_alg || "—")),
      row("Clave", esc(c.key || "—")),
      row("Cadena", res.chain_len + " certificado(s)"),
    ].join("");
    $tf("tls-result").classList.remove("hidden");
  }
}

// --- herramientas: escaneo de puertos ---
const portsTool = document.getElementById("ports-tool");
if (portsTool) {
  const $pf = (id) => document.getElementById(id);
  const presets = {
    web: [80, 443, 8080, 8443, 3000],
    db: [3306, 5432, 6379, 27017, 11211],
  };

  $pf("ports-preset").addEventListener("change", (e) => {
    $pf("ports-range").classList.toggle("hidden", e.target.value !== "range");
  });

  $pf("ports-form").addEventListener("submit", async (e) => {
    e.preventDefault();
    const fd = new FormData(e.target);
    const btn = $pf("ports-btn");
    btn.disabled = true;
    btn.textContent = "Escaneando…";
    $pf("ports-error").classList.add("hidden");
    $pf("ports-result").classList.add("hidden");

    const payload = { host: fd.get("host").trim() };
    const preset = fd.get("preset");
    if (preset === "range") {
      payload.range_start = parseInt(fd.get("range_start") || "1", 10);
      payload.range_end = parseInt(fd.get("range_end") || "1000", 10);
    } else if (presets[preset]) {
      payload.ports = presets[preset];
    }
    try {
      const res = await api("/api/portscan", { method: "POST", body: JSON.stringify(payload) });
      renderPortsResult(res);
    } catch (err) {
      const box = $pf("ports-error");
      box.textContent = "⚠️ " + err.message;
      box.classList.remove("hidden");
    } finally {
      btn.disabled = false;
      btn.textContent = "Escanear";
    }
  });

  function renderPortsResult(res) {
    $pf("ports-summary").innerHTML =
      `${res.open.length} de ${res.total} puertos abiertos en ${esc(res.host)} · ${res.elapsed_ms} ms`;
    $pf("ports-rows").innerHTML = res.open.length
      ? res.open.map((p) =>
          `<tr><td><span class="badge">${p.port}</span></td><td>${esc(p.service || "—")}</td><td>${p.latency_ms} ms</td></tr>`).join("")
      : '<tr><td colspan="3" class="muted">Ninguno abierto (el host puede filtrar puertos o estar caído)</td></tr>';
    $pf("ports-result").classList.remove("hidden");
  }
}
