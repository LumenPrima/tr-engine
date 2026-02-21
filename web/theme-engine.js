/**
 * THEME ENGINE — Event Horizon Design System
 * ═══════════════════════════════════════════════════════════════
 * Shared library that applies themes from THEME_CONFIG.
 * Expects theme-config.js to be loaded first.
 *
 * USAGE:
 *   <script src="/theme-config.js"></script>
 *   <script src="/theme-engine.js"></script>
 *
 * That's it. The engine will:
 *   1. Apply CSS variables from the saved theme (or default)
 *   2. Set data-* attributes for feature flags
 *   3. Build the floating theme switcher (if #themeSwitcher exists)
 *   4. Show the theme label toast (if #themeLabel exists)
 *   5. Persist the choice to localStorage
 *
 * Pages that want the switcher UI need these two elements:
 *   <div class="theme-switcher" id="themeSwitcher"></div>
 *   <div class="theme-label" id="themeLabel"></div>
 *
 * Pages that DON'T want the switcher can omit them entirely.
 * The theme still applies from localStorage — it just won't
 * show the picker or the toast.
 *
 * PUBLIC API (available as window.ThemeEngine):
 *   ThemeEngine.apply('obsidian')    — switch theme
 *   ThemeEngine.current()            — get current theme key
 *   ThemeEngine.list()               — get array of theme keys
 *   ThemeEngine.config               — raw THEME_CONFIG reference
 *   ThemeEngine.default              — the fallback theme key
 *
 * CONFIGURATION:
 *   Set window.THEME_ENGINE_OPTIONS before loading this script:
 *   window.THEME_ENGINE_OPTIONS = {
 *     default: 'appleGlass',    // fallback theme if nothing saved
 *     storageKey: 'eh-theme',   // localStorage key
 *     switcher: true,           // build theme picker (default: true)
 *     toast: true,              // show theme label on switch (default: true)
 *     toastDuration: 2000,      // ms to show toast (default: 2000)
 *     header: true,             // inject sticky site header (default: true)
 *     pageTitle: '',            // override card-title meta tag
 *     pageSubtitle: '',         // override card-description meta tag
 *     homeHref: 'index.html',   // home link target
 *   };
 * ═══════════════════════════════════════════════════════════════
 */

(function() {
  'use strict';

  // ── Options ──
  const opts = Object.assign({
    default: 'appleGlass',
    storageKey: 'eh-theme',
    switcher: true,
    toast: true,
    toastDuration: 2000,
    header: true,
    pageTitle: '',
    pageSubtitle: '',
    homeHref: 'index.html',
  }, window.THEME_ENGINE_OPTIONS || {});

  // ── Guards ──
  if (typeof THEME_CONFIG === 'undefined') {
    console.error('[ThemeEngine] THEME_CONFIG not found. Load theme-config.js before theme-engine.js');
    return;
  }

  const root = document.documentElement;
  let currentTheme = null;
  let labelTimeout = null;

  // ── Feature flag → data attribute mapping ──
  const FEATURE_MAP = {
    scanlines:      'scanlines',
    glowText:       'glow-text',
    squareElements: 'square-elements',
    gradientLogo:   'gradient-logo',
    invertedLabels: 'inverted-labels',
  };

  /**
   * Apply a theme by key.
   * This is the core function — everything flows from here.
   */
  function applyTheme(key) {
    const theme = THEME_CONFIG[key];
    if (!theme) {
      console.warn(`[ThemeEngine] Theme "${key}" not found`);
      return false;
    }

    currentTheme = key;

    // 1. Set all CSS custom properties
    for (const [prop, val] of Object.entries(theme.vars)) {
      root.style.setProperty(`--${prop}`, val);
    }

    // 2. Set feature flag data attributes on <body>
    for (const [flag, attr] of Object.entries(FEATURE_MAP)) {
      document.body.setAttribute(`data-${attr}`, String(theme.features[flag] || false));
    }

    // 3. Update switcher active state (header picker + any legacy floating switcher)
    document.querySelectorAll('.theme-btn').forEach(btn =>
      btn.classList.toggle('active', btn.dataset.theme === key)
    );

    // 4. Persist to localStorage
    try { localStorage.setItem(opts.storageKey, key); } catch (e) {}

    // 5. Show toast (if element exists)
    if (opts.toast) showLabel(theme.name);

    // 6. Dispatch event for other scripts to react
    window.dispatchEvent(new CustomEvent('themechange', {
      detail: { key, name: theme.name, theme }
    }));

    return true;
  }

  /**
   * Build the theme switcher buttons inside a given container element.
   */
  function buildSwitcher(container) {
    if (!container || !opts.switcher) return;
    container.innerHTML = '';
    for (const [key, theme] of Object.entries(THEME_CONFIG)) {
      const btn = document.createElement('button');
      btn.className = 'theme-btn';
      btn.dataset.theme = key;
      btn.dataset.name = theme.name;
      btn.style.background = theme.switcher.bg;
      btn.style.boxShadow = `inset 0 -8px 0 ${theme.switcher.accent}`;
      btn.addEventListener('click', () => applyTheme(key));
      container.appendChild(btn);
    }
  }

  /**
   * Inject the sticky site header with nav dropdown and theme picker.
   * Reads page title/subtitle from meta tags or THEME_ENGINE_OPTIONS.
   */
  function injectHeader() {
    // Read title/subtitle — options win, fall back to meta tags
    const metaTitle = (document.querySelector('meta[name="card-title"]') || {}).content || '';
    const metaDesc  = (document.querySelector('meta[name="card-description"]') || {}).content || '';
    const pageTitle    = opts.pageTitle    || metaTitle    || 'Event Horizon';
    const pageSubtitle = opts.pageSubtitle || metaDesc     || 'tr-engine';
    const homeHref     = opts.homeHref     || 'index.html';

    // Inject CSS
    const style = document.createElement('style');
    style.id = 'eh-header-styles';
    style.textContent = `
/* ── Event Horizon injected header ── */
.eh-header {
  position: sticky; top: 0; z-index: 200;
  background: var(--glass-bg);
  backdrop-filter: blur(var(--glass-blur, 20px));
  -webkit-backdrop-filter: blur(var(--glass-blur, 20px));
  border-bottom: 1px solid var(--glass-border);
  padding: 0 20px;
  display: flex; align-items: center; gap: 16px;
  height: 60px;
  transition: background 0.5s, border-color 0.4s;
}

/* Mark */
.eh-header-mark {
  width: 42px; height: 42px; flex-shrink: 0;
  background: var(--mark-bg, var(--accent));
  box-shadow: var(--mark-shadow, 0 0 10px var(--accent-glow));
  border-radius: var(--radius-sm);
  display: flex; align-items: center; justify-content: center;
  position: relative; overflow: hidden;
  transition: background 0.5s, border-radius 0.4s, box-shadow 0.5s;
  text-decoration: none;
}
.eh-header-mark::before {
  content: ''; position: absolute; top: 0; left: -5%; right: -5%; height: 52%;
  background: linear-gradient(180deg, rgba(255,255,255,0.3) 0%, transparent 100%);
  border-radius: 0 0 50% 50%;
}
.eh-header-mark svg { width: 20px; height: 20px; position: relative; z-index: 1; }
[data-square-elements="true"] .eh-header-mark { border-radius: 0; }

/* Nav dropdown */
.eh-nav-wrap { position: relative; }
.eh-nav-btn {
  display: flex; align-items: center; gap: 5px;
  background: none; border: none; cursor: pointer; padding: 4px 0;
  color: var(--text); transition: opacity 0.15s;
}
.eh-nav-btn:hover { opacity: 0.7; }
.eh-nav-title {
  font-family: var(--font-display);
  font-weight: var(--font-weight-display);
  font-size: 20px; letter-spacing: -0.02em; line-height: 1;
  color: var(--accent);
  transition: color 0.4s, font-family 0.3s, text-shadow 0.4s;
}
[data-glow-text="true"] .eh-nav-title {
  text-shadow: 0 0 16px var(--accent-glow);
}
[data-gradient-logo="true"] .eh-nav-title {
  background: linear-gradient(135deg, #3d7aed, #8b5cf6, #ec4899, #f97316);
  -webkit-background-clip: text; -webkit-text-fill-color: transparent; background-clip: text;
}
.eh-nav-chevron {
  color: var(--text-muted);
  transition: transform 0.25s, color 0.15s;
  flex-shrink: 0;
}
.eh-nav-wrap.open .eh-nav-chevron { transform: rotate(180deg); color: var(--accent); }

.eh-nav-dropdown {
  position: absolute; top: calc(100% + 10px); left: 0;
  min-width: 280px; max-width: 360px;
  /* background: var(--glass-bg); border: 1px solid var(--glass-border); */
  background: color-mix(in srgb, var(--bg) 90%, transparent);
  border-radius: var(--radius-sm);
  backdrop-filter: blur(72px); -webkit-backdrop-filter: blur(72px); 
  box-shadow: 0 4px 24px rgba(0,0,0,0.18);
  padding: 6px; z-index: 1000;
  opacity: 0; transform: translateY(-6px) scale(0.97);
  pointer-events: none; transition: opacity 0.2s, transform 0.2s;
}
.eh-nav-wrap.open .eh-nav-dropdown {
  opacity: 1; transform: translateY(0) scale(1); pointer-events: auto;
}
[data-square-elements="true"] .eh-nav-dropdown { border-radius: 0; }

.eh-nav-link {
  display: flex; align-items: center; gap: 10px; padding: 9px 10px;
  border-radius: var(--radius-xs);
  background: transparent; border: 1px solid transparent;
  text-decoration: none; color: var(--text-mid);
  transition: background 0.15s, border-color 0.15s, color 0.15s;
}
.eh-nav-link:hover {
  background: var(--tile-bg); border-color: var(--tile-border); color: var(--text);
}
[data-square-elements="true"] .eh-nav-link { border-radius: 0; }
.eh-nav-link-icon {
  width: 26px; height: 26px; border-radius: 7px; flex-shrink: 0;
  background: var(--tile-bg); border: 1px solid var(--tile-border);
  display: flex; align-items: center; justify-content: center;
  transition: all 0.15s;
}
[data-square-elements="true"] .eh-nav-link-icon { border-radius: 0; }
.eh-nav-link-icon svg { width: 12px; height: 12px; stroke: var(--text-muted); transition: stroke 0.15s; }
.eh-nav-link:hover .eh-nav-link-icon svg { stroke: var(--accent); }
.eh-nav-link-text { flex: 1; min-width: 0; }
.eh-nav-link-title { font-family: var(--font-body); font-size: 12.5px; font-weight: 600; color: inherit; }
.eh-nav-link-desc { font-family: var(--font-mono); font-size: 9px; color: var(--text-muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; margin-top: 2px; }
.eh-nav-link-arrow { font-size: 15px; font-weight: 300; color: var(--text-faint); transition: transform 0.15s, color 0.15s; }
.eh-nav-link:hover .eh-nav-link-arrow { transform: translateX(3px); color: var(--accent); }
.eh-nav-placeholder { font-family: var(--font-mono); font-size: 11px; color: var(--text-muted); padding: 12px 10px; text-align: center; }

/* Subtitle */
.eh-header-sub {
  font-family: var(--font-mono); font-size: 11px; letter-spacing: 0.08em;
  color: var(--text-muted); margin-left: 2px;
  transition: color 0.4s;
}

/* Right side */
.eh-header-right {
  margin-left: auto;
  display: flex; align-items: center; gap: 8px;
}

/* Picker (inside header) */
.eh-picker {
  display: flex; gap: 4px; padding: 4px;
  background: var(--tile-bg);
  border: 1px solid var(--border);
  border-radius: 10px;
  transition: all 0.5s;
}
[data-square-elements="true"] .eh-picker { border-radius: 0; }
.eh-picker .theme-btn {
  width: 22px; height: 22px; border-radius: 5px;
  border: 2px solid transparent;
  cursor: pointer; transition: all 0.25s;
  position: relative; overflow: visible; font-size: 0; flex-shrink: 0;
}
.eh-picker .theme-btn:hover { transform: scale(1.15); }
.eh-picker .theme-btn.active {
  border-color: var(--accent); box-shadow: 0 0 8px var(--accent-glow); transform: scale(1.1);
}
[data-square-elements="true"] .eh-picker .theme-btn { border-radius: 0; }
.eh-picker .theme-btn::after {
  content: attr(data-name);
  position: absolute; bottom: -24px; left: 50%; transform: translateX(-50%);
  font-size: 8px; font-family: var(--font-mono);
  letter-spacing: 1px; text-transform: uppercase;
  white-space: nowrap; color: var(--text-muted);
  background: var(--glass-bg); border: 1px solid var(--border);
  padding: 2px 6px; border-radius: 4px;
  opacity: 0; transition: opacity 0.2s; pointer-events: none;
  z-index: 10001;
}
.eh-picker .theme-btn:hover::after { opacity: 1; }

/* ── Mobile ── */
@media (max-width: 600px) {
  .eh-header { height: 50px; padding: 0 12px; gap: 10px; }
  .eh-header-mark { width: 32px; height: 32px; }
  .eh-header-mark svg { width: 16px; height: 16px; }
  .eh-nav-title { font-size: 16px; }
  .eh-header-sub { display: none; }
  .eh-picker { overflow-x: auto; -webkit-overflow-scrolling: touch; scrollbar-width: none; }
  .eh-picker::-webkit-scrollbar { display: none; }
  .eh-picker .theme-btn { width: 18px; height: 18px; }
  .eh-nav-dropdown { max-width: calc(100vw - 24px); min-width: unset; }
}
    `;
    document.head.appendChild(style);

    // Build header HTML
    const NAV_ICON = `<svg viewBox="0 0 24 24" fill="none" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 3a3 3 0 0 0-3 3v12a3 3 0 0 0 3 3 3 3 0 0 0 3-3 3 3 0 0 0-3-3H6a3 3 0 0 0-3 3 3 3 0 0 0 3 3 3 3 0 0 0 3-3V6a3 3 0 0 0-3-3 3 3 0 0 0-3 3 3 3 0 0 0 3 3h12a3 3 0 0 0 3-3 3 3 0 0 0-3-3z"/></svg>`;
    const MARK_ICON = `<svg viewBox="0 0 24 24" fill="none" stroke="white" stroke-width="2.2" stroke-linecap="round"><circle cx="12" cy="12" r="3"/><path d="M12 1v4M12 19v4M4.22 4.22l2.83 2.83M16.95 16.95l2.83 2.83M1 12h4M19 12h4M4.22 19.78l2.83-2.83M16.95 7.05l2.83-2.83"/></svg>`;

    const header = document.createElement('header');
    header.className = 'eh-header';
    header.id = 'eh-header';
    header.innerHTML = `
      <a class="eh-header-mark" href="${homeHref}" title="Event Horizon">${MARK_ICON}</a>
      <div class="eh-nav-wrap" id="eh-nav-wrap">
        <button class="eh-nav-btn" id="eh-nav-btn" aria-haspopup="true" aria-expanded="false">
          <span class="eh-nav-title">${pageTitle}</span>
          <svg class="eh-nav-chevron" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"/></svg>
        </button>
        <div class="eh-nav-dropdown" id="eh-nav-dropdown" role="menu">
          <div class="eh-nav-placeholder">loading…</div>
        </div>
      </div>
      <span class="eh-header-sub">${pageSubtitle}</span>
      <div class="eh-header-right">
        <div class="eh-picker" id="eh-picker"></div>
      </div>
    `;

    document.body.insertBefore(header, document.body.firstChild);

    // Wire up nav toggle
    const wrap = document.getElementById('eh-nav-wrap');
    const btn  = document.getElementById('eh-nav-btn');
    btn.addEventListener('click', () => {
      const isOpen = wrap.classList.toggle('open');
      btn.setAttribute('aria-expanded', isOpen);
      if (isOpen) loadNavPages();
    });
    document.addEventListener('click', e => {
      if (!wrap.contains(e.target)) wrap.classList.remove('open');
    });
    document.addEventListener('keydown', e => {
      if (e.key === 'Escape') wrap.classList.remove('open');
    });
  }

  /**
   * Fetch pages from the API and populate the nav dropdown.
   * Only fetches once; subsequent opens reuse the rendered links.
   */
  let _navLoaded = false;
  function loadNavPages() {
    if (_navLoaded) return;
    _navLoaded = true;

    const dropdown = document.getElementById('eh-nav-dropdown');
    const currentFile = window.location.pathname.split('/').pop() || 'index.html';

    fetch('/api/v1/pages')
      .then(r => r.json())
      .then(pages => {
        const others = pages.filter(p => {
          const file = (p.path || '').split('/').pop();
          return file !== currentFile;
        });
        if (!others.length) {
          dropdown.innerHTML = '<div class="eh-nav-placeholder">no other pages</div>';
          return;
        }
        const NAV_ICON = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/></svg>`;
        dropdown.innerHTML = others.map(p =>
          `<a href="${p.path}" class="eh-nav-link" role="menuitem">
            <div class="eh-nav-link-icon">${NAV_ICON}</div>
            <div class="eh-nav-link-text">
              <div class="eh-nav-link-title">${p.title}</div>
              ${p.description ? `<div class="eh-nav-link-desc">${p.description}</div>` : ''}
            </div>
            <span class="eh-nav-link-arrow">›</span>
          </a>`
        ).join('');
      })
      .catch(() => {
        dropdown.innerHTML = '<div class="eh-nav-placeholder">could not load pages</div>';
      });
  }

  /**
   * Show the floating theme name toast.
   */
  function showLabel(name) {
    const el = document.getElementById('themeLabel');
    if (!el) return;

    el.textContent = name;
    el.classList.remove('show');
    void el.offsetWidth; // trigger reflow for re-animation
    el.classList.add('show');

    clearTimeout(labelTimeout);
    labelTimeout = setTimeout(() => el.classList.remove('show'), opts.toastDuration);
  }

  /**
   * Read saved theme from localStorage, with fallback.
   */
  function getSavedTheme() {
    try {
      const saved = localStorage.getItem(opts.storageKey);
      if (saved && THEME_CONFIG[saved]) return saved;
    } catch (e) {}
    return opts.default;
  }

  // ── Keyboard shortcut: Ctrl/Cmd + Shift + T cycles themes ──
  document.addEventListener('keydown', function(e) {
    if ((e.ctrlKey || e.metaKey) && e.shiftKey && e.key === 'T') {
      e.preventDefault();
      const keys = Object.keys(THEME_CONFIG);
      const idx = keys.indexOf(currentTheme);
      const next = keys[(idx + 1) % keys.length];
      applyTheme(next);
    }
  });

  // ── Initialize ──
  // Wait for DOM if needed, but apply theme vars immediately
  // to prevent flash of unstyled content
  const initialTheme = getSavedTheme();

  // Apply vars to :root immediately (before DOM ready)
  const theme = THEME_CONFIG[initialTheme];
  if (theme) {
    for (const [prop, val] of Object.entries(theme.vars)) {
      root.style.setProperty(`--${prop}`, val);
    }
  }

  // Build UI and fully apply once DOM is ready
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', function() {
      if (opts.header !== false) injectHeader();
      buildSwitcher(document.getElementById('eh-picker'));
      applyTheme(initialTheme);
    });
  } else {
    if (opts.header !== false) injectHeader();
    buildSwitcher(document.getElementById('eh-picker'));
    applyTheme(initialTheme);
  }

  // ── Public API ──
  window.ThemeEngine = {
    apply: applyTheme,
    current: function() { return currentTheme; },
    list: function() { return Object.keys(THEME_CONFIG); },
    config: THEME_CONFIG,
    default: opts.default,
  };

})();

// bfcache fix — re-apply theme when browser restores page from back/forward cache
window.addEventListener('pageshow', function(event) {
  if (event.persisted) {
    try {
      const saved = localStorage.getItem('eh-theme');
      if (saved && window.ThemeEngine) ThemeEngine.apply(saved);
    } catch (e) {}
  }
});
