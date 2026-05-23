// Popup script for WSVPN Chrome Extension

const DEFAULTS = {
  enabled: true,
  proxyHost: '10.9.1.1',
  proxyPort: 1744,
  mode: 'bypass',
  bypassChina: true,
  bypassLAN: true,
  customCIDR: []
};

// Load current settings into form
async function load() {
  const s = await chrome.storage.local.get(DEFAULTS);
  const settings = { ...DEFAULTS, ...s };

  document.getElementById('enabled').checked = settings.enabled;
  document.getElementById('proxyHost').value = settings.proxyHost;
  document.getElementById('proxyPort').value = settings.proxyPort;
  document.getElementById('bypassChina').checked = settings.bypassChina;
  document.getElementById('bypassLAN').checked = settings.bypassLAN;
  document.getElementById('customCIDR').value = (settings.customCIDR || []).join('\n');

  updateStatus(settings.enabled);
}

function updateStatus(enabled) {
  const el = document.getElementById('status');
  if (enabled) {
    el.textContent = 'ON';
    el.className = 'on';
  } else {
    el.textContent = 'OFF';
    el.className = 'off';
  }
}

// Save settings and apply
async function save() {
  const customCIDR = document.getElementById('customCIDR').value
    .split('\n')
    .map(l => l.trim())
    .filter(l => l && l.includes('/'));

  const settings = {
    enabled: document.getElementById('enabled').checked,
    proxyHost: document.getElementById('proxyHost').value || '10.9.1.1',
    proxyPort: parseInt(document.getElementById('proxyPort').value) || 1744,
    mode: 'bypass',
    bypassChina: document.getElementById('bypassChina').checked,
    bypassLAN: document.getElementById('bypassLAN').checked,
    customCIDR: customCIDR
  };

  await chrome.storage.local.set(settings);
  updateStatus(settings.enabled);

  // Visual feedback
  const btn = document.getElementById('save');
  btn.textContent = 'Applied!';
  setTimeout(() => { btn.textContent = 'Apply'; }, 1000);
}

// Real-time toggle
document.getElementById('enabled').addEventListener('change', async () => {
  await save();
});

document.getElementById('save').addEventListener('click', save);
document.addEventListener('DOMContentLoaded', load);
