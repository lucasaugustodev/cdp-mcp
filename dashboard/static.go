package dashboard

const dashboardHTML = `<!DOCTYPE html>
<html>
<head>
<title>CDP MCP Dashboard</title>
<meta charset="utf-8">
<style>
* { box-sizing: border-box; }
body { background: #0a0a0a; color: #e0e0e0; font-family: system-ui, -apple-system, sans-serif; margin: 0; padding: 20px; }
h1 { font-size: 20px; margin-bottom: 4px; }
.status { color: #666; font-size: 12px; margin-bottom: 16px; }
.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(400px, 1fr)); gap: 16px; }
.card { background: #1a1a1a; border-radius: 12px; border: 1px solid #2a2a2a; overflow: hidden; }
.card-header { padding: 12px 16px; border-bottom: 1px solid #2a2a2a; display: flex; justify-content: space-between; align-items: center; }
.card-header h3 { margin: 0; font-size: 14px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 280px; }
.badge { font-size: 10px; padding: 2px 8px; border-radius: 10px; background: rgba(34,197,94,0.2); color: #4ade80; }
.badge-off { background: rgba(239,68,68,0.2); color: #f87171; }
.screenshot { width: 100%; cursor: crosshair; display: block; background: #111; min-height: 200px; }
.card-footer { padding: 8px 16px; font-size: 11px; color: #666; border-top: 1px solid #2a2a2a; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.activity { background: #111; border-radius: 12px; border: 1px solid #2a2a2a; margin-top: 16px; padding: 16px; max-height: 300px; overflow-y: auto; }
.activity h3 { margin: 0 0 12px; font-size: 14px; }
.log-entry { font-size: 12px; padding: 4px 0; border-bottom: 1px solid #1a1a1a; font-family: 'Cascadia Code', 'Fira Code', monospace; }
.log-tool { color: #60a5fa; }
.log-args { color: #a78bfa; }
.log-result { color: #4ade80; }
.log-time { color: #666; }
.empty { color: #444; text-align: center; padding: 40px; font-size: 14px; }
.type-bar { padding: 8px 16px; border-top: 1px solid #2a2a2a; display: flex; gap: 8px; }
.type-bar input { flex: 1; background: #0a0a0a; border: 1px solid #333; color: #e0e0e0; padding: 6px 10px; border-radius: 6px; font-size: 12px; outline: none; }
.type-bar input:focus { border-color: #60a5fa; }
.type-bar button { background: #2a2a2a; color: #e0e0e0; border: 1px solid #333; padding: 6px 12px; border-radius: 6px; font-size: 12px; cursor: pointer; }
.type-bar button:hover { background: #333; }
</style>
</head>
<body>
<h1>CDP MCP Dashboard</h1>
<div class="status" id="status">Connecting...</div>
<div class="grid" id="sessions"></div>
<div class="activity">
  <h3>Activity Log</h3>
  <div id="log"></div>
</div>
<script>
const sessionsEl = document.getElementById('sessions');
const logEl = document.getElementById('log');
const statusEl = document.getElementById('status');
let sessions = {};
let ws;

function connectWS() {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  ws = new WebSocket(proto + '//' + location.host + '/ws');
  ws.onopen = () => { statusEl.textContent = 'Connected'; };
  ws.onclose = () => {
    statusEl.textContent = 'Disconnected. Reconnecting...';
    setTimeout(connectWS, 2000);
  };
  ws.onmessage = (evt) => {
    try {
      const msg = JSON.parse(evt.data);
      if (msg.type === 'activity') {
        addLogEntry(msg.data);
      }
    } catch(e) {}
  };
}
connectWS();

function addLogEntry(entry) {
  const div = document.createElement('div');
  div.className = 'log-entry';
  const t = new Date(entry.timestamp).toLocaleTimeString();
  div.innerHTML = '<span class="log-time">' + t + '</span> ' +
    '<span class="log-tool">' + escHtml(entry.tool) + '</span> ' +
    '<span class="log-args">' + escHtml(entry.args || '') + '</span> ' +
    '<span class="log-result">' + escHtml(entry.result || '') + '</span>';
  logEl.insertBefore(div, logEl.firstChild);
  // Keep max 100 entries
  while (logEl.children.length > 100) logEl.removeChild(logEl.lastChild);
}

function escHtml(s) {
  const d = document.createElement('div');
  d.textContent = s;
  return d.innerHTML;
}

function fetchSessions() {
  fetch('/api/sessions').then(r => r.json()).then(data => {
    sessions = {};
    if (!data || data.length === 0) {
      sessionsEl.innerHTML = '<div class="empty">No active CDP sessions. Connect to an app via the MCP tools.</div>';
      return;
    }
    data.forEach(s => { sessions[s.id] = s; });
    renderSessions(data);
  }).catch(() => {});
}

function renderSessions(list) {
  // Only update DOM if session list changed
  const ids = list.map(s => s.id).sort().join(',');
  if (sessionsEl.dataset.ids === ids) return;
  sessionsEl.dataset.ids = ids;
  sessionsEl.innerHTML = '';
  list.forEach(s => {
    const card = document.createElement('div');
    card.className = 'card';
    card.id = 'card-' + s.id;
    card.innerHTML =
      '<div class="card-header"><h3>' + escHtml(s.title || 'Untitled') + '</h3><span class="badge">Connected</span></div>' +
      '<img class="screenshot" id="ss-' + s.id + '" alt="screenshot" />' +
      '<div class="card-footer">' + escHtml(s.url || '') + ' (port ' + s.port + ')</div>' +
      '<div class="type-bar"><input type="text" placeholder="Type text and press Enter..." id="input-' + s.id + '" /><button onclick="sendType(\'' + s.id + '\')">Send</button></div>';
    sessionsEl.appendChild(card);

    // Click handler
    const img = document.getElementById('ss-' + s.id);
    img.addEventListener('click', (e) => {
      const rect = img.getBoundingClientRect();
      const scaleX = (img.naturalWidth || 1440) / rect.width;
      const scaleY = (img.naturalHeight || 900) / rect.height;
      const x = Math.round((e.clientX - rect.left) * scaleX);
      const y = Math.round((e.clientY - rect.top) * scaleY);
      fetch('/api/sessions/' + s.id + '/click', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({x: x, y: y})
      });
    });

    // Type handler
    const input = document.getElementById('input-' + s.id);
    input.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') { sendType(s.id); }
    });
  });
}

window.sendType = function(id) {
  const input = document.getElementById('input-' + id);
  if (!input || !input.value) return;
  fetch('/api/sessions/' + id + '/type', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({text: input.value})
  });
  input.value = '';
};

function refreshScreenshots() {
  Object.keys(sessions).forEach(id => {
    const img = document.getElementById('ss-' + id);
    if (img) {
      img.src = '/api/sessions/' + id + '/screenshot?' + Date.now();
    }
  });
}

// Poll sessions every 2s
setInterval(fetchSessions, 2000);
fetchSessions();

// Refresh screenshots every 500ms
setInterval(refreshScreenshots, 500);

// Load recent activity
fetch('/api/activity').then(r => r.json()).then(data => {
  if (data) data.reverse().forEach(addLogEntry);
}).catch(() => {});
</script>
</body>
</html>`
