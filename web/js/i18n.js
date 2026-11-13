/* DigitDNS panel chrome — theme.
 *
 * This file used to carry the interface language dictionary and switcher. The
 * panel is English-only now, so what is left is the theme toggle: the inline
 * script in index.html sets data-theme before first paint (so there is no
 * flash), and this module keeps the toggle buttons' labels and icons in step
 * with the stored choice.
 *
 * There are three themes: the original blue-black dark, a neutral grey, and
 * light. The toggle cycles dark -> grey -> light -> dark; the icon shows the
 * theme in force and the label names what the next click will select.
 */
(function () {
  var THEME_KEY = 'digitdns_theme';
  var THEMES = ['dark', 'grey', 'light'];
  var NAMES = { dark: 'Dark', grey: 'Grey', light: 'Light' };
  var HALF_CIRCLE_SVG =
    '<svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" ' +
    'stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">' +
    '<circle cx="12" cy="12" r="9"></circle>' +
    '<path d="M12 3a9 9 0 0 0 0 18z" fill="currentColor" stroke="none"></path>' +
    '</svg>';
  var theme = 'dark';

  function normaliseTheme(value) {
    return THEMES.indexOf(value) >= 0 ? value : 'dark';
  }

  function nextTheme(current) {
    var i = THEMES.indexOf(current);
    return THEMES[(i + 1) % THEMES.length];
  }

  function applyTheme(next, persist) {
    theme = normaliseTheme(next);
    var html = document.documentElement;
    html.setAttribute('data-theme', theme);
    /* Tailwind's own `dark` class is still kept consistent even though no dark:
       variant is used in this markup — grey is a dark theme too, so only light
       clears it. */
    html.classList.toggle('dark', theme !== 'light');
    if (persist !== false) {
      try { localStorage.setItem(THEME_KEY, theme); } catch (e) { /* private mode */ }
    }
    /* Mirrored on every apply, including the no-persist call init() makes, because
       this is not a new choice being stored — it is the palette the page is
       actually painting right now. The API documentation pages are rendered by
       the Go server, in a package that cannot read this origin's localStorage, so
       the cookie is the only channel that reaches them. Without it an operator on
       the light theme opens API Reference and gets a dark page in the next tab.
       Same key, same three values; the docs pages fall back to dark when the
       cookie is absent, which is this panel's default anyway. */
    try {
      document.cookie = THEME_KEY + '=' + theme + '; path=/; max-age=31536000; SameSite=Lax';
    } catch (e) { /* cookies disabled */ }
    syncSwitchLabels();
    document.dispatchEvent(new CustomEvent('digitdns:theme', { detail: { theme: theme } }));
  }

  /* The cycle button shows the theme in force, not the next one: moon for
     dark, a half-filled circle for grey, sun for light. The wrapper stays in
     place so the button keeps its size and position across the swap. */
  function setThemeIcon(button, current) {
    var holder = button.querySelector('[data-theme-icon]');
    if (!holder) {
      holder = document.createElement('span');
      holder.setAttribute('data-theme-icon', '');
      holder.style.display = 'inline-flex';
      holder.style.alignItems = 'center';
      holder.style.justifyContent = 'center';
      holder.style.lineHeight = '0';
      var existing = button.querySelector('[data-feather], svg');
      if (existing) {
        button.insertBefore(holder, existing);
        holder.appendChild(existing);
      } else {
        button.appendChild(holder);
      }
    }
    if (current === 'grey') {
      holder.innerHTML = HALF_CIRCLE_SVG;
      return;
    }
    var icon = holder.querySelector('i');
    if (!icon) {
      holder.innerHTML = '<i class="w-4 h-4"></i>';
      icon = holder.querySelector('i');
    }
    if (current === 'dark') {
      icon.setAttribute('data-feather', 'moon');
    } else {
      icon.setAttribute('data-feather', 'sun');
    }
  }

  /* Both toggles are addressed by attribute rather than by id: there are two of
     them (the sidebar for a desktop viewport and the mobile header), and only
     one is ever visible. An id would need two names; the attribute needs none. */
  function syncSwitchLabels() {
    var toggles = document.querySelectorAll('[data-theme-toggle]');
    var next = nextTheme(theme);
    var label = 'Theme: ' + NAMES[theme] + ' \u2014 switch to ' + NAMES[next];
    for (var i = 0; i < toggles.length; i++) {
      var t = toggles[i];
      t.setAttribute('title', label);
      t.setAttribute('aria-label', label);
      /* A three-state cycle is not an on/off switch; aria-pressed would lie. */
      t.removeAttribute('aria-pressed');
      setThemeIcon(t, theme);
    }
  }

  function wireSwitches() {
    var toggles = document.querySelectorAll('[data-theme-toggle]');
    for (var i = 0; i < toggles.length; i++) {
      if (toggles[i].__themeWired) continue;
      toggles[i].__themeWired = true;
      toggles[i].addEventListener('click', function () {
        applyTheme(nextTheme(theme));
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
    theme = normaliseTheme(document.documentElement.getAttribute('data-theme') ||
      stored(THEME_KEY, 'dark'));
    applyTheme(theme, false);
    wireSwitches();
    if (typeof window.safeFeatherReplace === 'function') window.safeFeatherReplace();
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
