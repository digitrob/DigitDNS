/* DigitDNS panel chrome — theme.
 *
 * This file used to carry the interface language dictionary and switcher. The
 * panel is English-only now, so what is left is the
 * theme toggle: the inline script in index.html sets data-theme before first
 * paint (so there is no flash), and this module keeps the toggle buttons'
 * labels and icons in step with the stored choice and the OS default.
 */
(function () {
  var THEME_KEY = 'hyperdns_theme';

  var theme = 'dark';

  function applyTheme(next, persist) {
    theme = next === 'light' ? 'light' : 'dark';
    var html = document.documentElement;
    html.setAttribute('data-theme', theme);
    /* Tailwind's own `dark` class is still kept consistent even though no dark:
       variant is used in this markup — leaving it correct costs nothing and
       means a future dark:… utility behaves. */
    html.classList.toggle('dark', theme === 'dark');
    if (persist !== false) {
      try { localStorage.setItem(THEME_KEY, theme); } catch (e) { /* private mode */ }
    }
    syncSwitchLabels();
    document.dispatchEvent(new CustomEvent('hyperdns:theme', { detail: { theme: theme } }));
  }

  /* Both toggles are addressed by attribute rather than by id: there are two of
     them (the sidebar for a desktop viewport and the mobile header), and only
     one is ever visible. An id would need two names; the attribute needs none. */
  function syncSwitchLabels() {
    var toggles = document.querySelectorAll('[data-theme-toggle]');
    var toLight = theme === 'dark';
    var label = toLight ? 'Switch to light theme' : 'Switch to dark theme';
    for (var i = 0; i < toggles.length; i++) {
      var t = toggles[i];
      t.setAttribute('title', label);
      t.setAttribute('aria-label', label);
      t.setAttribute('aria-pressed', theme === 'light' ? 'true' : 'false');
      var icon = t.querySelector('[data-feather], svg');
      if (icon && icon.tagName.toLowerCase() === 'i') {
        icon.setAttribute('data-feather', toLight ? 'sun' : 'moon');
      }
    }
  }

  function wireSwitches() {
    var toggles = document.querySelectorAll('[data-theme-toggle]');
    for (var i = 0; i < toggles.length; i++) {
      if (toggles[i].__themeWired) continue;
      toggles[i].__themeWired = true;
      toggles[i].addEventListener('click', function () {
        applyTheme(theme === 'dark' ? 'light' : 'dark');
        /* The icon name was just swapped by syncSwitchLabels; nothing redraws
           it but this. */
        if (typeof window.safeFeatherReplace === 'function') window.safeFeatherReplace();
      });
    }
    syncSwitchLabels();
  }

  function stored(key, fallback) {
    try {
      var v = localStorage.getItem(key);
      return v || fallback;
    } catch (e) {
      return fallback;
    }
  }

  function init() {
    /* The inline script in index.html already set data-theme before first paint;
       this recovers it into module state and wires the toggles, which it cannot
       do because it runs before <body> exists. */
    theme = document.documentElement.getAttribute('data-theme') === 'light'
      ? 'light' : stored(THEME_KEY, 'dark') === 'light' ? 'light' : 'dark';
    applyTheme(theme, false);
    wireSwitches();
    if (typeof window.safeFeatherReplace === 'function') window.safeFeatherReplace();
  }

  /* The API app.js consumed while the panel was bilingual. The language half is
     inert now — the panel is English-only — but the theme half is live and
     app.js listens for the hyperdns:theme event. */
  window.HyperI18N = {
    t: function (s) { return s; },
    lang: function () { return 'en'; },
    theme: function () { return theme; },
    setTheme: applyTheme,
    refresh: function () {},
    wire: wireSwitches
  };

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
