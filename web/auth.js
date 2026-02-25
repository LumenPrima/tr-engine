/**
 * auth.js — Shared authentication for tr-engine web pages.
 *
 * Include via <script src="auth.js"></script> before any page scripts.
 *
 * What it does:
 * 1. Fetches token from /api/v1/auth-init (server-provided, CDN-safe)
 * 2. Falls back to localStorage('tr-engine-token')
 * 3. Patches window.fetch to inject Authorization header on same-origin /api/ calls
 * 4. Patches EventSource to append ?token= on same-origin URLs
 * 5. On 401 response, shows a token prompt modal → saves → reloads
 *
 * Pages don't need to change their fetch() or EventSource calls at all.
 */
(function () {
  'use strict';

  const STORAGE_KEY = 'tr-engine-token';

  // Load token from server (synchronous XHR so it's ready before page scripts run).
  // The /api/v1/auth-init endpoint has no file extension, so CDNs won't cache it.
  if (!window.__TR_AUTH_TOKEN__) {
    try {
      var xhr = new XMLHttpRequest();
      xhr.open('GET', '/api/v1/auth-init', false);
      xhr.send();
      if (xhr.status === 200) {
        var data = JSON.parse(xhr.responseText);
        if (data.token) window.__TR_AUTH_TOKEN__ = data.token;
      }
    } catch (e) { /* server may not support auth-init — fall through */ }
  }

  let token = window.__TR_AUTH_TOKEN__ || localStorage.getItem(STORAGE_KEY) || '';
  let prompted = false; // prevent multiple prompts

  // ── Patch fetch ──────────────────────────────────────────────────
  const _fetch = window.fetch;
  window.fetch = function (input, init) {
    const url = typeof input === 'string' ? input : input instanceof Request ? input.url : '';
    if (token && isLocalAPI(url)) {
      init = init || {};
      const headers = new Headers(init.headers || {});
      if (!headers.has('Authorization')) {
        headers.set('Authorization', 'Bearer ' + token);
      }
      init.headers = headers;
    }
    return _fetch.call(this, input, init).then(function (resp) {
      if (resp.status === 401 && isLocalAPI(url)) showPrompt();
      return resp;
    });
  };

  // ── Patch EventSource ────────────────────────────────────────────
  const _EventSource = window.EventSource;
  window.EventSource = function (url, opts) {
    if (token && isLocalAPI(url)) {
      const sep = url.includes('?') ? '&' : '?';
      url = url + sep + 'token=' + encodeURIComponent(token);
    }
    const es = new _EventSource(url, opts);
    // If EventSource gets an error immediately (401), the browser fires onerror
    // but we can't read the status. We rely on fetch interception for the prompt.
    return es;
  };
  // Preserve prototype so instanceof checks still work
  window.EventSource.prototype = _EventSource.prototype;
  window.EventSource.CONNECTING = _EventSource.CONNECTING;
  window.EventSource.OPEN = _EventSource.OPEN;
  window.EventSource.CLOSED = _EventSource.CLOSED;

  // ── Helpers ──────────────────────────────────────────────────────
  function isLocalAPI(url) {
    if (url.startsWith('/api/')) return true;
    try {
      const u = new URL(url, location.origin);
      return u.origin === location.origin && u.pathname.startsWith('/api/');
    } catch (e) {
      return false;
    }
  }

  function showPrompt() {
    if (prompted) return;
    prompted = true;

    // Check if a page-level auth prompt already exists (e.g. events.html)
    if (document.getElementById('auth-prompt')) return;

    const overlay = document.createElement('div');
    overlay.id = 'tr-auth-overlay';
    overlay.style.cssText = 'position:fixed;inset:0;background:rgba(0,0,0,.7);z-index:99999;display:flex;align-items:center;justify-content:center;font-family:system-ui,sans-serif';

    const box = document.createElement('div');
    box.style.cssText = 'background:#1a1a2e;border:1px solid #333;border-radius:8px;padding:24px;max-width:360px;width:90%;color:#e0e0e0';

    const title = document.createElement('h2');
    title.textContent = 'Authentication Required';
    title.style.cssText = 'margin:0 0 8px;font-size:16px;color:#fff';

    const desc = document.createElement('p');
    desc.textContent = 'This instance requires an API token. Enter it below — it will be saved in your browser.';
    desc.style.cssText = 'margin:0 0 16px;font-size:13px;color:#999;line-height:1.4';

    const input = document.createElement('input');
    input.type = 'password';
    input.placeholder = 'Bearer token';
    input.value = token;
    input.style.cssText = 'width:100%;box-sizing:border-box;padding:8px 12px;background:#0d0d1a;border:1px solid #444;border-radius:4px;color:#fff;font-size:14px;margin-bottom:12px';

    const btnRow = document.createElement('div');
    btnRow.style.cssText = 'display:flex;gap:8px';

    const submit = document.createElement('button');
    submit.textContent = 'Connect';
    submit.style.cssText = 'flex:1;padding:8px;background:#4a6cf7;color:#fff;border:none;border-radius:4px;cursor:pointer;font-size:14px';

    const clear = document.createElement('button');
    clear.textContent = 'Clear Token';
    clear.style.cssText = 'padding:8px 12px;background:transparent;color:#999;border:1px solid #444;border-radius:4px;cursor:pointer;font-size:13px';

    function doSubmit() {
      const val = input.value.trim();
      if (val) {
        localStorage.setItem(STORAGE_KEY, val);
      } else {
        localStorage.removeItem(STORAGE_KEY);
      }
      location.reload();
    }

    submit.onclick = doSubmit;
    input.onkeydown = function (e) { if (e.key === 'Enter') doSubmit(); };
    clear.onclick = function () {
      localStorage.removeItem(STORAGE_KEY);
      input.value = '';
      input.focus();
    };

    btnRow.appendChild(submit);
    btnRow.appendChild(clear);
    box.appendChild(title);
    box.appendChild(desc);
    box.appendChild(input);
    box.appendChild(btnRow);
    overlay.appendChild(box);
    document.body.appendChild(overlay);
    input.focus();
  }

  // ── Expose for pages that need programmatic access ───────────────
  window.trAuth = {
    getToken: function () { return token; },
    setToken: function (t) {
      token = t || '';
      if (token) localStorage.setItem(STORAGE_KEY, token);
      else localStorage.removeItem(STORAGE_KEY);
    },
    showPrompt: showPrompt,
  };
})();
