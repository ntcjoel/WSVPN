package main

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
)

// adminPage is the embedded management UI.
const adminPage = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>WSVPN Admin</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font:13px -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;background:#f5f5f5;color:#333;padding:20px}
h1{font-size:18px;margin-bottom:16px}
h2{font-size:14px;margin:16px 0 8px}
.card{background:#fff;border-radius:6px;padding:16px;margin-bottom:12px;box-shadow:0 1px 3px rgba(0,0,0,.1)}
table{width:100%;border-collapse:collapse}
th,td{text-align:left;padding:8px 6px;border-bottom:1px solid #eee}
th{font-weight:600;color:#666;font-size:11px;text-transform:uppercase}
tr:hover{background:#fafafa}
.status{display:inline-block;width:8px;height:8px;border-radius:50%;margin-right:4px}
.online{background:#0a0}
.offline{background:#ccc}
.btn{padding:4px 12px;border:1px solid #ddd;border-radius:3px;cursor:pointer;font-size:12px;background:#fff}
.btn:hover{background:#f0f0f0}
.btn-primary{background:#1a73e8;color:#fff;border-color:#1a73e8}
.btn-primary:hover{background:#1557b0}
.btn-danger{color:#c5221f;border-color:#c5221f}
.btn-danger:hover{background:#fce8e6}
.row{display:flex;gap:8px;margin:8px 0}
.row input,.row select{flex:1;padding:6px 8px;border:1px solid #ccc;border-radius:3px;font-size:12px}
.tag{display:inline-block;padding:1px 6px;border-radius:3px;font-size:11px;background:#e8f0fe;color:#1967d2;margin:0 2px}
.mono{font-family:monospace;font-size:11px}
#token-area{margin-bottom:12px}
#token-area input{padding:6px 8px;border:1px solid #ccc;border-radius:3px;width:280px}
</style>
</head>
<body>
<h1>WSVPN Admin</h1>

<div class="card" id="token-area">
  <input type="password" id="token" placeholder="Admin token" onchange="loadAll()">
</div>

<div class="card">
  <h2>Server Status</h2>
  <div id="status">Loading...</div>
</div>

<div class="card">
  <h2>Connected Clients</h2>
  <table><thead><tr><th>Status</th><th>Name</th><th>UUID</th><th>IP</th><th>Traffic In</th><th>Traffic Out</th></tr></thead>
  <tbody id="connected"></tbody></table>
</div>

<div class="card">
  <h2>Configured Clients</h2>
  <div class="row">
    <input type="text" id="new-name" placeholder="Name">
    <input type="text" id="new-uuid" placeholder="UUID">
    <input type="text" id="new-ip" placeholder="IP (e.g. 10.9.1.5)">
    <button class="btn btn-primary" onclick="addClient()">Add Client</button>
  </div>
  <table><thead><tr><th>Name</th><th>UUID</th><th>IP</th><th>Enabled</th><th>Actions</th></tr></thead>
  <tbody id="configured"></tbody></table>
</div>

<div class="card">
  <h2>System</h2>
  <div id="system">Loading...</div>
</div>

<script>
let clients = [];
let health = {};

async function api(path, method, body) {
  let token = document.getElementById('token').value;
  if (!token) { alert('Enter admin token first'); return null; }
  let sep = path.includes('?') ? '&' : '?';
  let opts = {method: method || 'GET', headers:{}};
  if (body) { opts.headers['Content-Type']='application/json'; opts.body=JSON.stringify(body); }
  let r = await fetch('/ws/admin/api'+path+sep+'token='+encodeURIComponent(token), opts);
  if (!r.ok) { let t = await r.text(); throw new Error(t); }
  let ct = r.headers.get('content-type')||'';
  return ct.includes('json') ? r.json() : r.text();
}

async function loadAll() {
  try {
    health = await api('/health') || {};
    clients = await api('/clients') || {clients:[],network:''};
    render();
  } catch(e) { console.error(e); }
}

function render() {
  // Server status
  let s = health;
  document.getElementById('status').innerHTML =
    'Status: <b>'+ (s.status||'?') +'</b> | '+
    'Uptime: <b>'+ (s.uptime||'?') +'</b> | '+
    'Clients: <b>'+ ((s.clients||{}).connected||0) +'/'+ ((s.clients||{}).configured||0) +'</b> | '+
    'Traffic: in <b>'+ fmtBytes(s.traffic?.bytes_in||0) +'</b> out <b>'+ fmtBytes(s.traffic?.bytes_out||0) +'</b>';

  // Connected clients
  let connected = (s.clients||{}).client_details||[];
  let rows = connected.map(c =>
    '<tr><td><span class="status online"></span>Online</td>'+
    '<td>'+h(c.name||c.id)+'</td><td class="mono">'+h(c.id)+'</td><td class="mono">'+h(c.ip)+'</td>'+
    '<td>—</td><td>—</td></tr>'
  ).join('');
  document.getElementById('connected').innerHTML = rows || '<tr><td colspan=6>No clients connected</td></tr>';

  // Configured clients
  let cl = clients.clients || [];
  let connIds = connected.map(c => c.id);
  rows = cl.map(c =>
    '<tr>'+
    '<td>'+h(c.name)+'</td><td class="mono">'+h(c.uuid)+'</td><td class="mono">'+h(c.ip)+'</td>'+
    '<td>'+(c.enabled!==false?'<span class="tag">Yes</span>':'<span class="tag" style="background:#fce8e6;color:#c5221f">No</span>')+'</td>'+
    '<td>'+
    '<button class="btn btn-sm" onclick="editClient(\''+h(c.uuid)+'\')">Edit</button> '+
    '<button class="btn btn-sm btn-danger" onclick="deleteClient(\''+h(c.uuid)+'\')">Delete</button>'+
    '</td></tr>'
  ).join('');
  document.getElementById('configured').innerHTML = rows || '<tr><td colspan=5>No clients configured</td></tr>';

  // System
  let sys = s.system||{};
  document.getElementById('system').innerHTML =
    'Go: <b>'+h(sys.go_version)+'</b> | '+
    'Goroutines: <b>'+sys.goroutines+'</b> | '+
    'Memory: <b>'+fmtBytes(sys.memory_alloc_bytes)+'</b> | '+
    'CPUs: <b>'+sys.cpus+'</b>';
}

function addClient() {
  let name = document.getElementById('new-name').value.trim();
  let uuid = document.getElementById('new-uuid').value.trim();
  let ip = document.getElementById('new-ip').value.trim();
  if (!uuid || !ip) { alert('UUID and IP required'); return; }
  let c = {uuid:uuid, ip:ip, name:name||uuid, enabled:true};
  clients.clients.push(c);
  api('/clients','POST',{clients:clients.clients}).then(()=>loadAll());
  document.getElementById('new-name').value='';
  document.getElementById('new-uuid').value='';
  document.getElementById('new-ip').value='';
}

function editClient(uuid) {
  let c = clients.clients.find(x=>x.uuid===uuid);
  if (!c) return;
  let name = prompt('Name:',c.name);
  let ip = prompt('IP:',c.ip);
  let enabled = confirm('Enabled? Click OK for Yes, Cancel for No');
  if (name!==null) c.name = name;
  if (ip!==null) c.ip = ip;
  c.enabled = enabled;
  api('/clients','POST',{clients:clients.clients}).then(()=>loadAll());
}

function deleteClient(uuid) {
  if (!confirm('Delete client '+uuid+'?')) return;
  clients.clients = clients.clients.filter(x=>x.uuid!==uuid);
  api('/clients','POST',{clients:clients.clients}).then(()=>loadAll());
}

function fmtBytes(b) {
  if (b<1024) return b+' B';
  if (b<1048576) return (b/1024).toFixed(1)+' KB';
  if (b<1073741824) return (b/1048576).toFixed(1)+' MB';
  return (b/1073741824).toFixed(1)+' GB';
}
function h(s) { return (s||'').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;'); }
</script>
</body>
</html>`

// handleAdmin serves the admin page (auth via token query param).
func (s *Server) HandleAdmin(w http.ResponseWriter, r *http.Request) {
	config := s.getConfig()
	token := r.URL.Query().Get("token")
	if token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(config.AdminToken)) != 1 {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`<html><body><h2>Unauthorized</h2><p>Enter admin token to continue.</p>
<form><input name=token type=password placeholder="Admin token"><button>Go</button></form>
</body></html>`))
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(adminPage))
}

// HandleAdminAPI handles /ws/admin/api/* endpoints.
func (s *Server) HandleAdminAPI(w http.ResponseWriter, r *http.Request) {
	config := s.getConfig()
	token := r.URL.Query().Get("token")
	if token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(config.AdminToken)) != 1 {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/ws/admin/api")
	path = strings.TrimPrefix(path, "/")

	switch {
	case path == "health":
		s.HandleHealth(w, r)

	case path == "clients" || path == "clients/":
		switch r.Method {
		case "GET":
			s.handleClientsGet(w, r)
		case "POST":
			s.handleClientsPost(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}

	case path == "reload":
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.handleConfigReloadAPI(w, r)

	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleClientsGet(w http.ResponseWriter, r *http.Request) {
	clients := s.clientManager.GetClients()
	network := s.clientManager.GetNetwork()

	resp := map[string]interface{}{
		"clients": clients,
		"network": network,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleClientsPost(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Clients []ClientConfig `json:"clients"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	config := s.getConfig()
	if err := s.clientManager.SaveClients(config.ClientsFile, req.Clients, s.clientManager.GetNetwork()); err != nil {
		http.Error(w, "Failed to save: "+err.Error(), http.StatusInternalServerError)
		return
	}

	structuredLog.Info("admin_api", "Clients updated via admin API", map[string]interface{}{
		"count": len(req.Clients),
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleConfigReloadAPI(w http.ResponseWriter, r *http.Request) {
	config := s.getConfig()
	if err := s.clientManager.Reload(config.ClientsFile); err != nil {
		http.Error(w, "Reload failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "reloaded"})
}
