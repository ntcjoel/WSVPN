// Popup script for WSVPN Chrome Extension

const DEFAULTS = {
  enabled: true, proxyHost: '10.9.1.1', proxyPort: 1744,
  bypassChina: true, bypassLAN: true, customCIDR: []
};

async function load() {
  let s = await chrome.storage.local.get(DEFAULTS);
  let settings = { ...DEFAULTS, ...s };

  document.getElementById('enabled').checked = settings.enabled;
  document.getElementById('proxyHost').value = settings.proxyHost;
  document.getElementById('proxyPort').value = settings.proxyPort;
  document.getElementById('bypassChina').checked = settings.bypassChina;
  document.getElementById('bypassLAN').checked = settings.bypassLAN;
  document.getElementById('customCIDR').value = (settings.customCIDR || []).join('\n');
  updateStatusUI(settings.online !== false);
}

function updateStatusUI(on) {
  let dot = document.getElementById('dot');
  let text = document.getElementById('statusText');
  dot.className = 'status-dot ' + (on ? 'online' : 'offline');
  text.textContent = on ? 'Online' : 'Offline';
}

async function save() {
  let settings = {
    enabled: document.getElementById('enabled').checked,
    proxyHost: document.getElementById('proxyHost').value || '10.9.1.1',
    proxyPort: parseInt(document.getElementById('proxyPort').value) || 1744,
    bypassChina: document.getElementById('bypassChina').checked,
    bypassLAN: document.getElementById('bypassLAN').checked,
    customCIDR: document.getElementById('customCIDR').value.split('\n').map(s => s.trim()).filter(s => s)
  };
  await chrome.storage.local.set(settings);
  updateStatusUI(true);

  let btn = document.getElementById('save');
  btn.textContent = 'Applied!';
  setTimeout(() => { btn.textContent = 'Apply Settings'; }, 1200);
}

document.getElementById('enabled').addEventListener('change', save);
document.getElementById('save').addEventListener('click', save);
document.addEventListener('DOMContentLoaded', async () => {
  await load();
  // Check real-time status from background
  chrome.runtime.sendMessage('status', (r) => {
    if (r) updateStatusUI(r.online);
  });
});
