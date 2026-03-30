package dashboard

const dashboardHTML = `<!DOCTYPE html>
<html>
<head>
<title>CDP MCP Dashboard</title>
<meta charset="utf-8">
<style>
* { box-sizing: border-box; margin: 0; padding: 0; }
body { background: #0a0a0a; color: #e0e0e0; font-family: system-ui, -apple-system, sans-serif; height: 100vh; overflow: hidden; display: flex; flex-direction: column; }

/* Tab Bar */
.tab-bar { display: flex; align-items: center; background: #111; border-bottom: 1px solid #2a2a2a; padding: 0 12px; height: 44px; flex-shrink: 0; }
.tab { display: flex; align-items: center; gap: 6px; padding: 8px 16px; font-size: 13px; color: #ccc; cursor: pointer; border-bottom: 2px solid transparent; transition: all 0.15s; user-select: none; white-space: nowrap; }
.tab:hover { color: #fff; background: rgba(255,255,255,0.04); }
.tab.active { color: #fff; border-bottom-color: #60a5fa; }
.tab .dot { width: 7px; height: 7px; border-radius: 50%; flex-shrink: 0; }
.dot-connected { background: #4ade80; box-shadow: 0 0 4px #4ade80; }
.dot-disconnected { background: #555; }
.tab-add { padding: 6px 12px; font-size: 16px; color: #888; cursor: pointer; border: 1px solid #333; border-radius: 6px; background: transparent; margin-left: 4px; line-height: 1; }
.tab-add:hover { color: #e0e0e0; border-color: #555; }
.tab-spacer { flex: 1; }
.tab-right { display: flex; align-items: center; gap: 12px; }
.tab-btn { padding: 6px 14px; font-size: 12px; color: #ccc; cursor: pointer; border: 1px solid #333; border-radius: 6px; background: transparent; display: flex; align-items: center; gap: 6px; }
.tab-btn:hover { color: #e0e0e0; border-color: #555; background: rgba(255,255,255,0.04); }
.tab-btn.teach-active { border-color: #ef4444; color: #f87171; background: rgba(239,68,68,0.1); }
.recipe-badge { background: rgba(168,85,247,0.2); color: #c084fc; padding: 2px 8px; border-radius: 10px; font-size: 11px; }

/* Main Layout */
.main { display: flex; flex: 1; overflow: hidden; }

/* Screenshot Area */
.screen-area { flex: 1; display: flex; flex-direction: column; overflow: hidden; }
.screenshot-wrap { flex: 1; display: flex; align-items: center; justify-content: center; background: #0a0a0a; padding: 8px; position: relative; overflow: hidden; min-height: 0; }
.screenshot { max-width: 100%; max-height: 100%; cursor: crosshair; display: block; border-radius: 6px; object-fit: contain; }
.screenshot.teach-border { border: 3px solid #ef4444; box-shadow: 0 0 20px rgba(239,68,68,0.3); }
.no-conn { display: flex; align-items: center; justify-content: center; flex: 1; color: #888; font-size: 14px; }

/* Activity Log */
.activity-log { height: 100px; flex-shrink: 0; background: #0f0f0f; border-top: 1px solid #2a2a2a; overflow-y: auto; padding: 8px 12px; }
.activity-log h4 { font-size: 11px; color: #888; text-transform: uppercase; letter-spacing: 0.5px; margin-bottom: 6px; }
.log-entry { font-size: 11px; padding: 2px 0; font-family: 'Cascadia Code', 'Fira Code', monospace; color: #ccc; }
.log-time { color: #888; }
.log-tool { color: #60a5fa; }
.log-args { color: #c084fc; }
.log-result { color: #4ade80; }

/* Tasks Panel */
.tasks-panel { width: 260px; flex-shrink: 0; background: #111; border-left: 1px solid #2a2a2a; display: flex; flex-direction: column; overflow: hidden; }
.tasks-header { display: flex; justify-content: space-between; align-items: center; padding: 12px 14px; border-bottom: 1px solid #2a2a2a; }
.tasks-header h3 { font-size: 14px; color: #e0e0e0; }
.tasks-add-btn { font-size: 11px; color: #60a5fa; cursor: pointer; background: none; border: 1px solid rgba(96,165,250,0.3); padding: 3px 10px; border-radius: 4px; }
.tasks-add-btn:hover { background: rgba(96,165,250,0.1); }
.tasks-list { flex: 1; overflow-y: auto; padding: 8px; }
.task-item { background: #1a1a1a; border: 1px solid #2a2a2a; border-radius: 8px; padding: 10px 12px; margin-bottom: 6px; }
.task-name { font-size: 13px; color: #e0e0e0; display: flex; align-items: center; gap: 6px; }
.task-icon { font-size: 14px; }
.task-meta { font-size: 11px; color: #888; margin-top: 4px; display: flex; justify-content: space-between; align-items: center; }
.task-badge { font-size: 10px; padding: 1px 6px; border-radius: 8px; }
.badge-enabled { background: rgba(74,222,128,0.15); color: #4ade80; }
.badge-disabled { background: rgba(255,255,255,0.06); color: #888; }
.tasks-empty { color: #888; font-size: 12px; text-align: center; padding: 24px 12px; }
.recipes-section { border-top: 1px solid #2a2a2a; padding: 10px 14px; }
.recipes-section h4 { font-size: 12px; color: #888; margin-bottom: 8px; }
.recipe-chips { display: flex; flex-wrap: wrap; gap: 6px; }
.recipe-chip { font-size: 11px; padding: 4px 10px; border-radius: 12px; background: rgba(168,85,247,0.12); color: #c084fc; border: 1px solid rgba(168,85,247,0.2); cursor: pointer; }
.recipe-chip:hover { background: rgba(168,85,247,0.2); }
.recipes-empty { color: #888; font-size: 11px; }

/* Add App Modal */
.modal-overlay { display: none; position: fixed; inset: 0; background: rgba(0,0,0,0.7); z-index: 100; align-items: center; justify-content: center; }
.modal-overlay.open { display: flex; }
.modal { background: #1a1a1a; border: 1px solid #2a2a2a; border-radius: 12px; width: 680px; max-height: 80vh; overflow-y: auto; }
.modal-header { display: flex; justify-content: space-between; align-items: center; padding: 16px 20px; border-bottom: 1px solid #2a2a2a; }
.modal-header h2 { font-size: 16px; color: #e0e0e0; }
.modal-close { background: none; border: none; color: #888; font-size: 20px; cursor: pointer; padding: 4px 8px; }
.modal-close:hover { color: #e0e0e0; }
.modal-body { display: grid; grid-template-columns: 1fr 1fr; gap: 0; }
.modal-col { padding: 16px 20px; }
.modal-col:first-child { border-right: 1px solid #2a2a2a; }
.modal-col h3 { font-size: 13px; color: #ccc; margin-bottom: 12px; }

.inv-item { display: flex; justify-content: space-between; align-items: center; padding: 8px 10px; background: #111; border: 1px solid #2a2a2a; border-radius: 6px; margin-bottom: 6px; }
.inv-name { font-size: 12px; color: #e0e0e0; flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.inv-badge { font-size: 9px; color: #4ade80; background: rgba(74,222,128,0.12); padding: 1px 6px; border-radius: 8px; margin: 0 8px; flex-shrink: 0; }
.inv-btn { font-size: 11px; padding: 3px 10px; border-radius: 4px; cursor: pointer; border: 1px solid #333; background: #222; color: #ccc; flex-shrink: 0; }
.inv-btn:hover { background: #333; }
.inv-btn.added { color: #4ade80; border-color: rgba(74,222,128,0.3); cursor: default; }
.inv-loading { color: #888; font-size: 12px; padding: 12px 0; }

.form-group { margin-bottom: 10px; }
.form-group label { display: block; font-size: 11px; color: #888; margin-bottom: 4px; }
.form-group input[type="text"], .form-group input[type="url"], .form-group input[type="email"], .form-group input[type="password"] {
  width: 100%; background: #111; border: 1px solid #333; color: #e0e0e0; padding: 7px 10px; border-radius: 6px; font-size: 12px; outline: none;
}
.form-group input:focus { border-color: #60a5fa; }
.form-row { display: flex; gap: 12px; }
.form-check { display: flex; align-items: center; gap: 6px; font-size: 12px; color: #ccc; }
.form-check input { accent-color: #60a5fa; }
.form-submit { width: 100%; padding: 8px; background: #60a5fa; color: #0a0a0a; border: none; border-radius: 6px; font-size: 13px; font-weight: 600; cursor: pointer; margin-top: 8px; }
.form-submit:hover { background: #3b82f6; }

/* Scrollbar */
::-webkit-scrollbar { width: 6px; }
::-webkit-scrollbar-track { background: transparent; }
::-webkit-scrollbar-thumb { background: #333; border-radius: 3px; }
::-webkit-scrollbar-thumb:hover { background: #555; }
</style>
</head>
<body>

<!-- Tab Bar -->
<div class="tab-bar">
  <div id="tabs"></div>
  <button class="tab-add" onclick="openAddModal()" title="Add App">+</button>
  <div class="tab-spacer"></div>
  <div class="tab-right">
    <button class="tab-btn" id="teachBtn" onclick="toggleTeach()">
      <span id="teachIcon">&#9673;</span> <span id="teachLabel">Teach</span>
    </button>
    <span class="recipe-badge" id="recipeCount">0 recipes</span>
  </div>
</div>

<!-- Main -->
<div class="main">
  <!-- Screenshot + Activity -->
  <div class="screen-area">
    <div class="screenshot-wrap" id="screenWrap">
      <div class="no-conn" id="noConn">No app selected. Add or select an app above.</div>
      <img class="screenshot" id="screenshot" alt="screenshot" style="display:none" />
    </div>
    <div class="activity-log">
      <h4>Activity</h4>
      <div id="log"></div>
    </div>
  </div>

  <!-- Tasks Panel -->
  <div class="tasks-panel">
    <div class="tasks-header">
      <h3>Tasks</h3>
      <button class="tasks-add-btn">+ Add</button>
    </div>
    <div class="tasks-list" id="tasksList">
      <div class="tasks-empty">No tasks yet</div>
    </div>
    <div class="recipes-section">
      <h4>Recipes</h4>
      <div class="recipe-chips" id="recipeChips">
        <span class="recipes-empty">No recipes</span>
      </div>
    </div>
  </div>
</div>

<!-- Add App Modal -->
<div class="modal-overlay" id="addModal">
  <div class="modal">
    <div class="modal-header">
      <h2>Add App</h2>
      <button class="modal-close" onclick="closeAddModal()">&times;</button>
    </div>
    <div class="modal-body">
      <div class="modal-col">
        <h3>Installed Apps</h3>
        <div id="inventoryList"><div class="inv-loading">Scanning...</div></div>
      </div>
      <div class="modal-col">
        <h3>Web App</h3>
        <div class="form-group"><label>URL</label><input type="url" id="waUrl" placeholder="https://example.com" /></div>
        <div class="form-group"><label>Name</label><input type="text" id="waName" placeholder="My App" /></div>
        <div class="form-group"><label>Email</label><input type="email" id="waEmail" placeholder="user@example.com" /></div>
        <div class="form-group"><label>Password</label><input type="password" id="waPass" placeholder="password" /></div>
        <div class="form-row" style="margin-top:8px">
          <label class="form-check"><input type="checkbox" id="waHeadless" /> Headless</label>
          <label class="form-check"><input type="checkbox" id="waAutostart" /> Auto-start</label>
        </div>
        <button class="form-submit" onclick="addWebApp()">Create</button>
      </div>
    </div>
  </div>
</div>

<script>
const logEl = document.getElementById('log');
const tabsEl = document.getElementById('tabs');
const ssImg = document.getElementById('screenshot');
const noConn = document.getElementById('noConn');
const teachBtn = document.getElementById('teachBtn');
const teachLabel = document.getElementById('teachLabel');
const tasksList = document.getElementById('tasksList');
const recipeChips = document.getElementById('recipeChips');
const recipeCount = document.getElementById('recipeCount');

let apps = [];
let selectedAppId = null;
let teaching = false;
let ws;

// ---- WebSocket ----
function connectWS() {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  ws = new WebSocket(proto + '//' + location.host + '/ws');
  ws.onclose = () => setTimeout(connectWS, 2000);
  ws.onmessage = (evt) => {
    try {
      const msg = JSON.parse(evt.data);
      if (msg.type === 'activity') addLogEntry(msg.data);
    } catch(e) {}
  };
}
connectWS();

// ---- Activity Log ----
function addLogEntry(entry) {
  const div = document.createElement('div');
  div.className = 'log-entry';
  const t = new Date(entry.timestamp).toLocaleTimeString([], {hour:'2-digit',minute:'2-digit'});
  div.innerHTML = '<span class="log-time">' + t + '</span> <span class="log-tool">' + esc(entry.tool) + '</span> <span class="log-args">' + esc(entry.args||'') + '</span> <span class="log-result">' + esc(entry.result||'') + '</span>';
  logEl.insertBefore(div, logEl.firstChild);
  while (logEl.children.length > 100) logEl.removeChild(logEl.lastChild);
}

function esc(s) { const d = document.createElement('div'); d.textContent = s; return d.innerHTML; }

// ---- Apps / Tabs ----
function fetchApps() {
  fetch('/api/apps').then(r => r.json()).then(data => {
    apps = data || [];
    renderTabs();
    if (apps.length > 0 && !selectedAppId) {
      selectApp(apps[0].id);
    } else if (selectedAppId) {
      // Refresh tasks/recipes for selected
      fetchTasks();
      fetchRecipes();
    }
  }).catch(() => {});
}

function renderTabs() {
  tabsEl.innerHTML = '';
  apps.forEach(app => {
    const tab = document.createElement('div');
    tab.className = 'tab' + (app.id === selectedAppId ? ' active' : '');
    const dotClass = app.connected ? 'dot-connected' : 'dot-disconnected';
    tab.innerHTML = '<span class="dot ' + dotClass + '"></span>' + esc(app.name || app.id);
    tab.onclick = () => selectApp(app.id);
    tabsEl.appendChild(tab);
  });
}

function selectApp(id) {
  selectedAppId = id;
  renderTabs();
  fetch('/api/apps/' + id + '/select', { method: 'POST' });
  ssImg.style.display = 'block';
  noConn.style.display = 'none';
  fetchTasks();
  fetchRecipes();
}

// ---- Screenshot ----
function refreshScreenshot() {
  if (!selectedAppId) return;
  const app = apps.find(a => a.id === selectedAppId);
  if (!app || !app.connected) {
    ssImg.style.display = 'none';
    noConn.style.display = 'flex';
    noConn.textContent = app ? 'App "' + app.name + '" is disconnected.' : 'No app selected.';
    return;
  }
  ssImg.style.display = 'block';
  noConn.style.display = 'none';
  ssImg.src = '/api/apps/' + selectedAppId + '/screenshot?' + Date.now();
}

// ---- Click on screenshot ----
ssImg.addEventListener('click', (e) => {
  if (!selectedAppId) return;
  const rect = ssImg.getBoundingClientRect();
  const scaleX = (ssImg.naturalWidth || 1440) / rect.width;
  const scaleY = (ssImg.naturalHeight || 900) / rect.height;
  const x = Math.round((e.clientX - rect.left) * scaleX);
  const y = Math.round((e.clientY - rect.top) * scaleY);
  fetch('/api/apps/' + selectedAppId + '/click', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({x, y})
  });
});

// ---- Keyboard on screenshot ----
ssImg.tabIndex = 0;
ssImg.addEventListener('keypress', (e) => {
  if (!selectedAppId) return;
  e.preventDefault();
  fetch('/api/apps/' + selectedAppId + '/type', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({text: e.key})
  });
});

// ---- Tasks ----
const taskIcons = { polling: '\u23F0', monitor: '\uD83D\uDC41', workflow: '\u26A1', schedule: '\uD83D\uDCC5' };

function fetchTasks() {
  if (!selectedAppId) return;
  fetch('/api/apps/' + selectedAppId + '/tasks').then(r => r.json()).then(data => {
    if (!data || data.length === 0) {
      tasksList.innerHTML = '<div class="tasks-empty">No tasks for this app</div>';
      return;
    }
    tasksList.innerHTML = '';
    data.forEach(t => {
      const div = document.createElement('div');
      div.className = 'task-item';
      const icon = taskIcons[t.type] || '\u26A1';
      const badgeCls = t.enabled ? 'badge-enabled' : 'badge-disabled';
      const badgeTxt = t.enabled ? 'Active' : 'Off';
      let metaStr = '';
      if (t.lastRun) metaStr += 'Last: ' + t.lastRun;
      if (t.nextRun) metaStr += (metaStr ? ' | ' : '') + 'Next: ' + t.nextRun;
      div.innerHTML = '<div class="task-name"><span class="task-icon">' + icon + '</span>' + esc(t.name) + '</div>' +
        '<div class="task-meta"><span>' + esc(metaStr || 'Never run') + '</span><span class="task-badge ' + badgeCls + '">' + badgeTxt + '</span></div>';
      tasksList.appendChild(div);
    });
  }).catch(() => {});
}

// ---- Recipes ----
function fetchRecipes() {
  if (!selectedAppId) return;
  fetch('/api/apps/' + selectedAppId + '/recipes').then(r => r.json()).then(data => {
    const names = data || [];
    recipeCount.textContent = names.length + ' recipe' + (names.length !== 1 ? 's' : '');
    if (names.length === 0) {
      recipeChips.innerHTML = '<span class="recipes-empty">No recipes</span>';
      return;
    }
    recipeChips.innerHTML = '';
    names.forEach(n => {
      const chip = document.createElement('span');
      chip.className = 'recipe-chip';
      chip.textContent = n;
      recipeChips.appendChild(chip);
    });
  }).catch(() => {});
}

// ---- Teach ----
function toggleTeach() {
  if (!selectedAppId) return;
  if (!teaching) {
    fetch('/api/apps/' + selectedAppId + '/teach/start', { method: 'POST' }).then(r => r.json()).then(() => {
      teaching = true;
      teachBtn.classList.add('teach-active');
      teachLabel.textContent = 'Stop';
      ssImg.classList.add('teach-border');
    }).catch(() => {});
  } else {
    fetch('/api/apps/' + selectedAppId + '/teach/stop', { method: 'POST' }).then(r => r.json()).then(data => {
      teaching = false;
      teachBtn.classList.remove('teach-active');
      teachLabel.textContent = 'Teach';
      ssImg.classList.remove('teach-border');
      if (data && data.steps > 0) {
        const name = prompt('Save recipe as:', 'recipe-' + Date.now());
        if (name) {
          // Steps already saved by stop endpoint if name provided via query
          fetchRecipes();
        }
      }
    }).catch(() => {});
  }
}

// ---- Add App Modal ----
function openAddModal() {
  document.getElementById('addModal').classList.add('open');
  loadInventory();
}

function closeAddModal() {
  document.getElementById('addModal').classList.remove('open');
}

function loadInventory() {
  const el = document.getElementById('inventoryList');
  el.innerHTML = '<div class="inv-loading">Scanning for apps...</div>';
  fetch('/api/inventory').then(r => r.json()).then(data => {
    if (!data || data.length === 0) {
      el.innerHTML = '<div class="inv-loading">No apps found. Try launching an app with CDP enabled.</div>';
      return;
    }
    el.innerHTML = '';
    const existingIds = new Set(apps.map(a => a.id));
    data.forEach(item => {
      const div = document.createElement('div');
      div.className = 'inv-item';
      const isAdded = existingIds.has(item.id);
      div.innerHTML = '<span class="inv-name" title="' + esc(item.url||'') + '">' + esc(item.title||item.id) + '</span>' +
        (item.cdp ? '<span class="inv-badge">CDP</span>' : '') +
        '<button class="inv-btn' + (isAdded ? ' added' : '') + '" ' + (isAdded ? 'disabled' : '') + ' onclick="addInventoryApp(this,\'' + esc(item.id) + '\',\'' + esc(item.title||'') + '\',\'' + esc(item.url||'') + '\',' + (item.port||0) + ')">' + (isAdded ? 'Added' : 'Add') + '</button>';
      el.appendChild(div);
    });
  }).catch(() => { el.innerHTML = '<div class="inv-loading">Scan failed.</div>'; });
}

window.addInventoryApp = function(btn, id, title, url, port) {
  btn.disabled = true;
  btn.textContent = '...';
  fetch('/api/apps/add', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({ id: id, name: title, url: url, cdp_port: port, type: 'detected' })
  }).then(r => {
    if (r.ok) {
      btn.textContent = 'Added';
      btn.classList.add('added');
      fetchApps();
    } else { btn.textContent = 'Error'; btn.disabled = false; }
  }).catch(() => { btn.textContent = 'Error'; btn.disabled = false; });
};

function addWebApp() {
  const url = document.getElementById('waUrl').value.trim();
  const name = document.getElementById('waName').value.trim() || url;
  const email = document.getElementById('waEmail').value.trim();
  const pass = document.getElementById('waPass').value;
  const headless = document.getElementById('waHeadless').checked;
  const autostart = document.getElementById('waAutostart').checked;
  if (!url) return;
  const id = name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
  const creds = {};
  if (email) creds.email = email;
  if (pass) creds.password = pass;
  fetch('/api/apps/add', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({ id: id, name: name, url: url, type: 'webapp', headless: headless, auto_start: autostart, credentials: creds })
  }).then(r => {
    if (r.ok) {
      closeAddModal();
      fetchApps();
    }
  }).catch(() => {});
}

// ---- Polling ----
setInterval(fetchApps, 2000);
fetchApps();
setInterval(refreshScreenshot, 500);

// Load recent activity
fetch('/api/activity').then(r => r.json()).then(data => {
  if (data) data.reverse().forEach(addLogEntry);
}).catch(() => {});
</script>
</body>
</html>`
