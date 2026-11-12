// =======================================================
// DigitDNS Front-End Application Logic (ES6)
// Bulletproof, Offline-Safe, Self-Healing
// =======================================================


// authToken is empty until a real login. Earlier builds seeded the literal
// 'hdns_session_admin' here and wrote it to localStorage, which the server has
// never accepted — it only produced a guaranteed 401 on first load and made a
// logged-out browser look logged in.
let authToken = localStorage.getItem('digitdns_token') || '';

// The admin namespace. The panel is not at the root of the host: the
// SPA lives at /<admin-path>/dash/... and every dashboard API at
// /<admin-path>/api/..., where <admin-path> is a 16-hex-character value drawn
// once per install. The bundle is built without knowing it — the same bytes ship
// to every install, and baking one in would publish the hidden path — so it is
// read from the address bar instead. The shape is exactly what the server
// generates (16 lowercase hex characters, security.go's IsValidAdminPath), so a
// match is reliable, and anything else means the page was reached some other way,
// in which case the empty prefix degrades to the root paths.
const ADMIN_BASE = (function () {
  const m = window.location.pathname.match(/^\/([0-9a-f]{16})(?=\/|$)/);
  return m ? '/' + m[1] : '';
})();
const DASH_BASE = ADMIN_BASE + '/dash';

// api() prefixes a server path with the admin namespace. Every dashboard call —
// fetches, the SSE stream, the docs link — goes through this, because the day
// one call site forgets the prefix is the day that call starts answering 404
// while every other tab keeps working.
const api = (path) => ADMIN_BASE + path;
// The bridge the ES modules read shared state through. Modules cannot import
// from this classic script, and a captured copy of any of these would go stale
// across a re-login or a config refresh — so each accessor resolves the live
// value at call time.
window.__hdns = {
  getToken: () => authToken,
  setToken: (v) => { authToken = v; },
  getConfig: () => currentConfig,
  api: (p) => api(p),
  DASH_BASE: DASH_BASE,
  showToast: (m, t) => showToast(m, t),
  errorMessage: (r, f) => errorMessage(r, f),
};

let sseSource = null;
let sseConnecting = false;
let sseGeneration = 0;
let streamFallbackTimer = null;
let streamFallbackCheck = null;
let streamFallbackInFlight = false;
let streamLastEventAt = 0;
let streamFallbackSig = '';
let qpsChart = null;
let historyChart = null;
let historyRange = '24h';
let historyTimer = null;
let historyInFlight = false;
let historySeq = 0;
let historyInitialized = false;
// Top upstreams can be ranked per server (default) or aggregated per provider.
let historyTopUpstreamByProvider = false;
let historyTopData = {};
let currentConfig = null;
let isStreamPaused = false;

// SSE reconnect controller.
//
// The live-log stream authenticates with a one-time ticket in its URL (?ticket=),
// which the server burns on first connect (internal/web/sse_ticket.go). EventSource's
// native reconnect replays the exact same URL, so every automatic retry presents an
// already-spent ticket, falls through to the (absent) Authorization header, and is
// answered 401 — after which the browser gives up for good and the log freezes. We
// therefore drive reconnects ourselves: close the dead source, mint a fresh ticket,
// and re-open on a silent exponential backoff.
let sseBackoffTimer = null;
let sseBackoffMs = 0;
const SSE_BACKOFF_START_MS = 1000;
const SSE_BACKOFF_MAX_MS = 30000;

// The live query table is a rendering of this buffer rather than the record itself.
//
// The filter dropdown and the search box used to be read inside the append path — at the
// moment a query arrived and nowhere else — so they only ever applied to arriving rows.
// Selecting BLOCK left every DIRECT row sitting on screen; typing a domain into the search
// box did nothing at all until the next query happened to show up, which on a resolver
// serving one household can be minutes. Neither control has an event listener in this file,
// so there was nothing to re-render with: the operator's only evidence that the filter works
// is that later rows obey it. That reads as a broken control, not a quiet network.
//
// Holding the queries as data makes the filter, the search box, and Clear all cheap and
// exact, and it makes "the last STREAM_BUFFER_MAX queries received" true regardless of what
// the view is currently showing — the old 80-row cap applied to whatever survived the
// filter, so a narrow filter silently discarded history it had already been handed.
//
// 100 matches the daemon's own ring (recentLogsCap): it is the most the server will ever
// replay on connect, so a larger client buffer could only hold rows no refresh would show.
const STREAM_BUFFER_MAX = 100;
let streamBuffer = [];

// streamEpoch invalidates work queued before a Clear. A frame coalesced from arrivals
// that have since been cleared must not repaint them, and bumping this counter is how
// the click handler tells any in-flight rAF callback that its batch is stale.
let streamEpoch = 0;

// streamRendered mirrors the rows actually in the table, in DOM order, holding the same
// query objects as streamBuffer. The two differ while a batch is being inserted: the
// buffer has every arrival the moment it lands, but the rows are added one at a time on
// the next frame. Keeping the rendered order as data is what lets insertStreamQuery find
// a row's slot without walking (or rebuilding) the DOM, and it is the array a dropped
// arrival is removed from when the buffer is full.
let streamRendered = [];
// streamPendingDropped holds the queries the capped buffer evicted in this frame, so the
// matching rows leave the DOM too instead of lingering past STREAM_BUFFER_MAX.
let streamPendingDropped = [];

// streamSort is the live log's order. It lives with the buffer rather than in the
// render call because pushStreamQuery has to consult it to decide where an arriving
// row belongs, and the sort control only has to assign it and redraw.
let streamSort = 'time-desc';
// streamRefreshTimer bounds a manual refresh: the reconnect itself is async and a
// lost ticket must not leave the button spinning forever.
let streamRefreshTimer = null;

// streamSortField returns the value one query sorts on, as a string, for the given
// order. Time is the epoch in milliseconds and latency is milliseconds, both
// zero-padded so that a plain string compare keeps them in numeric order — the
// comparator below then needs no per-field branch at insertion time.
function streamSortField(q, mode) {
  switch (mode) {
    case 'time-asc':
    case 'time-desc': {
      const t = q.timestamp ? new Date(q.timestamp).getTime() : Date.now();
      return String(t).padStart(15, '0');
    }
    case 'client':
      return ((q.account_name || 'Public') + ' ' + (q.client_ip || '')).toLowerCase();
    case 'domain':
      return (q.domain || '').toLowerCase();
    case 'action':
      return (q.action || 'DIRECT').toLowerCase();
    case 'latency-desc':
    case 'latency-asc': {
      const lat = (typeof q.latency_ms === 'number')
        ? q.latency_ms
        : ((typeof q.latency === 'number') ? q.latency / 1000000 : 0);
      return String(Math.round(lat * 10)).padStart(12, '0');
    }
    default:
      return '';
  }
}

// compareStreamQueries orders two queries under the current streamSort. The secondary
// key keeps rows with equal values in a stable order, so a burst of queries to the same
// domain does not shuffle among themselves on every redraw.
function compareStreamQueries(a, b) {
  const ka = streamSortField(a, streamSort);
  const kb = streamSortField(b, streamSort);
  let cmp = ka < kb ? -1 : ka > kb ? 1 : 0;
  if (cmp === 0) {
    const ta = a.timestamp ? new Date(a.timestamp).getTime() : 0;
    const tb = b.timestamp ? new Date(b.timestamp).getTime() : 0;
    cmp = ta - tb;
  }
  // time-desc / latency-desc are the two orders that want the larger key first.
  return (streamSort === 'time-desc' || streamSort === 'latency-desc') ? -cmp : cmp;
}

function safeFeatherReplace() {
  try {
    if (typeof feather !== 'undefined' && feather.replace) {
      feather.replace();
    }
  } catch (e) {
    console.warn('Feather icons render notice:', e);
  }
}

// errorMessage pulls the server's explanation out of a failed response. Handlers
// answer with {"error":"..."} and the text is the actionable part — which policy
// rule a password failed, or that the IP is locked out rather than mistyped.
async function errorMessage(res, fallback) {
  try {
    const text = await res.text();
    if (text) {
      try {
        const parsed = JSON.parse(text);
        if (parsed && parsed.error) return String(parsed.error);
      } catch (e) {
        const trimmed = text.trim();
        if (trimmed && trimmed.length < 200) return trimmed;
      }
    }
  } catch (e) { /* fall through to the generic message */ }
  return res.status === 429 ? 'Too many attempts. Try again later.' : fallback;
}

// ---- Server location (GeoIP) -------------------------------------------------
//
// The country arrives with every config render (`server.country`), so the flag
// beside the address is free. The database's date and the manual refresh come
// from /api/settings/geoip, fetched once per page load and after an update rather
// than on every stats tick.
//
// Flags are the bundled 4:3 SVGs under the panel's own /flags/ directory. A
// missing file hides the picture and leaves the address alone; nothing external
// is loaded, and the CSP's img-src 'self' is enough.

const regionNames = (() => {
  // English on purpose: the panel ships one language, and a browser-localised
  // country name next to English labels reads as a bug.
  try { return new Intl.DisplayNames(['en'], { type: 'region' }); } catch (e) { return null; }
})();

function countryName(code) {
  const upper = String(code || '').toUpperCase();
  if (!/^[A-Z]{2}$/.test(upper)) return '';
  try {
    const name = regionNames && regionNames.of(upper);
    return name && name !== upper ? name : upper;
  } catch (e) { return upper; }
}

function flagURL(code) {
  const lower = String(code || '').toLowerCase();
  return /^[a-z]{2}$/.test(lower) ? ADMIN_BASE + '/flags/' + lower + '.svg' : '';
}

function paintCountryFlag(img, code) {
  if (!img) return;
  const url = flagURL(code);
  if (!url) {
    img.classList.add('hidden');
    img.removeAttribute('src');
    return;
  }
  img.onerror = () => { img.classList.add('hidden'); };
  img.src = url;
  img.alt = countryName(code);
  img.title = countryName(code);
  img.classList.remove('hidden');
}

function renderServerLocation(code) {
  paintCountryFlag(document.getElementById('header-country-flag'), code);
}

let geoipStatusLoaded = false;
let geoipUpdating = false;

function renderGeoipCard(status) {
  const country = status && status.country ? status.country : '';
  paintCountryFlag(document.getElementById('geoip-flag'), country);
  const nameEl = document.getElementById('geoip-country');
  if (nameEl) nameEl.textContent = country ? countryName(country) : 'Unknown';
  const dbEl = document.getElementById('geoip-db');
  if (dbEl) {
    if (!status || !status.available) {
      dbEl.textContent = 'DB: not downloaded';
      dbEl.title = 'GeoIP database: not downloaded yet — press Update database';
    } else {
      const date = status.database_date || 'date unknown';
      dbEl.textContent = `DB ${date}`;
      dbEl.title = `GeoIP database updated ${date} · updates daily`;
    }
  }
  const errEl = document.getElementById('geoip-error');
  if (errEl) {
    const message = status && status.error ? status.error : '';
    errEl.textContent = message;
    errEl.classList.toggle('hidden', !message);
  }
  // A manual update can be the first thing that ever downloads the file, so the
  // sidebar may still be showing no flag even though the card now knows one.
  renderServerLocation(country);
}

async function loadGeoipStatus() {
  try {
    const res = await fetch(api('/api/settings/geoip'), {
      headers: { 'Authorization': `Bearer ${authToken}` },
    });
    if (!res.ok) throw new Error(await errorMessage(res, 'Could not read the GeoIP status'));
    renderGeoipCard(await res.json());
  } catch (e) {
    const errEl = document.getElementById('geoip-error');
    if (errEl) {
      errEl.textContent = e.message || 'Could not read the GeoIP status';
      errEl.classList.remove('hidden');
    }
  }
}

async function updateGeoipDatabase() {
  if (geoipUpdating) return;
  const btn = document.getElementById('geoip-update-btn');
  const label = btn ? btn.querySelector('span') : null;
  const original = label ? label.textContent : '';
  geoipUpdating = true;
  if (btn) btn.disabled = true;
  if (label) label.textContent = 'Updating…';
  try {
    const res = await fetch(api('/api/settings/geoip/update'), {
      method: 'POST',
      headers: { 'Authorization': `Bearer ${authToken}` },
    });
    if (!res.ok) throw new Error(await errorMessage(res, 'The GeoIP database could not be updated'));
    renderGeoipCard(await res.json());
    showToast('GeoIP database updated', 'success');
  } catch (e) {
    const message = e.message || 'The GeoIP database could not be updated';
    showToast(message, 'error');
    const errEl = document.getElementById('geoip-error');
    if (errEl) { errEl.textContent = message; errEl.classList.remove('hidden'); }
  } finally {
    geoipUpdating = false;
    if (btn) btn.disabled = false;
    if (label) label.textContent = original;
  }
}

// clientAction posts one subscriber mutation and reports what actually happened.
//
// Each of the five callers used to be its own try/catch that either ignored res.ok
// or swallowed the throw in `catch (err) {}`. So a refused delete showed "Client
// deleted" and the follow-up loadClients() quietly put the row back — which reads
// as the dashboard being out of sync rather than as the server having said no. A
// dropped connection showed nothing at all, and the operator was left looking at a
// row they believed they had just changed.
//
// It returns true only when the server confirmed the change, so a caller can chain
// on the result rather than assume.
async function clientAction(path, payload, successMsg, tone) {
  try {
    const res = await fetch(api(path), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${authToken}` },
      body: JSON.stringify(payload)
    });
    if (!res.ok) {
      showToast(await errorMessage(res, 'The server refused the request'), 'error');
      return false;
    }
    showToast(successMsg, tone || 'info');
    // Only on success: nothing changed on a refusal, so the rows on screen are
    // already correct and a reload would just hide the error toast behind a redraw.
    loadClients();
    return true;
  } catch (err) {
    // fetch rejects only on a transport failure, so this is the daemon being gone or
    // the tab being offline. Worth saying: the row on screen is now unverified.
    showToast('Could not reach the server — the list may be out of date.', 'error');
    return false;
  }
}

// isProbablyIP is a typo catcher, not the authority. The server re-parses with
// net.ParseIP and stores the canonical form, because whitelisting is an exact string
// compare against the address the listener reports — this check only exists so an
// obvious slip is caught while the operator can still see and fix what they typed.
//
// Leading zeros are rejected on purpose: Go's parser refuses them too, since
// "010.1.1.1" is octal to some tools and decimal to others.
function isProbablyIP(value) {
  if (/^\d{1,3}(\.\d{1,3}){3}$/.test(value)) {
    return value.split('.').every((part) => Number(part) <= 255 && (part === '0' || !part.startsWith('0')));
  }
  // A colon is what separates a v6 attempt from a mistyped v4. Hand-rolling the full
  // v6 grammar here would only find new ways to disagree with Go's parser, so the
  // shape is checked and the verdict is left to the server.
  return value.includes(':') && /^[0-9a-fA-F:.]+$/.test(value) && !value.includes(':::');
}

// Init when DOM is loaded
document.addEventListener('submit', (e) => {
  const id = e && e.target && e.target.id;
  if (id === 'created-secret-form' || id === 'twofa-enable-form' ||
      id === 'twofa-confirm-form' || id === 'twofa-disable-form') {
    e.preventDefault();
  }
}, true);

// "Remember me" on the login overlay. The token in localStorage is not enough on
// its own: dashboard sessions live in the daemon's memory and are revoked when it
// restarts or after two idle hours, so the operator lands back on this form with
// nothing filled in. Ticking the box stores the submitted pair under its own key
// and fills both fields on the next load. That pair is plain JSON in localStorage
// — anything running on this origin can read it — which is the trade the tick
// asks the operator to accept; the box is off the moment it is unticked and the
// stored copy is deleted with it. Nothing here is sent anywhere.
const REMEMBER_KEY = 'digitdns_remember';

function readRememberedLogin() {
  try {
    const raw = localStorage.getItem(REMEMBER_KEY);
    if (!raw) return null;
    const saved = JSON.parse(raw);
    if (!saved || typeof saved.u !== 'string' || typeof saved.p !== 'string') return null;
    return saved;
  } catch (e) {
    // A corrupt or unreadable entry is treated as "nothing remembered".
    return null;
  }
}

function saveRememberedLogin(username, password) {
  try {
    localStorage.setItem(REMEMBER_KEY, JSON.stringify({ u: username, p: password }));
  } catch (e) {
    // Blocked or full storage: the login still worked, only the convenience is lost.
  }
}

function forgetRememberedLogin() {
  try { localStorage.removeItem(REMEMBER_KEY); } catch (e) {}
}

// Runs before checkAuthAndBoot so the fields are filled by the time the overlay
// is unhidden. The password field only takes focus when there is no live session
// to boot into — otherwise it would pull focus off the dashboard underneath.
function applyRememberedLogin() {
  const userInput = document.getElementById('login-username');
  const passInput = document.getElementById('login-password');
  const box = document.getElementById('login-remember');
  if (!userInput || !passInput || !box) return;
  const saved = readRememberedLogin();
  box.checked = !!saved;
  if (!saved) return;
  userInput.value = saved.u;
  passInput.value = saved.p;
  if (!authToken && !document.getElementById('login-modal')?.classList.contains('hidden')) {
    passInput.focus();
  }
}

document.addEventListener('DOMContentLoaded', () => {
  safeFeatherReplace();
  // Chart.js is not loaded here. The first successful stats poll reaches
  // pushChartData, which calls initChart after the dashboard has painted. Loading
  // its 201 KB on DOMContentLoaded also did so on the login page, where no chart
  // exists. The tiles and the rest of the shell are useful without it.
  // Before checkAuthAndBoot, which unhides the login overlay synchronously: the observers
  // inside have to be watching already or they miss the modal that is up first.
  initModalA11y();
  // Select the tab from the URL before the first API call. The pre-paint script already
  // stopped the wrong tab from painting; this applies the real classes and nav state so
  // bootDashboard can load only the tab that is actually on screen.
  handleRouteFromURL(false);
  checkAuthAndBoot();
  // Filled after checkAuthAndBoot: opening the overlay runs the focus trap, which
  // would otherwise land the cursor on the theme toggle and override the password
  // focus. Everything here is synchronous, so no empty fields are painted.
  applyRememberedLogin();

  // The API docs live under the admin namespace too, and the anchors' hrefs in
  // index.html are static ("/api/v2/docs", "/api/v3/docs") that
  // cannot know the install's path. Rewritten here from the same value every
  // fetch uses, so the page each button opens is the one the server actually
  // serves. All three versions need this: only v1 used to be rewritten, so the
  // new v2 and v3 links pointed at the host root and 404'd.
  // Selected by attribute presence and matched in JS rather than built into the
  // selector: a querySelector string of the form a[href="/api/…"] looks exactly
  // like an HTML attribute interpolation to the static escaping sweep in
  // internal/web/dashboard_escaping_test.go, which would flag a value that never
  // becomes markup. Comparing getAttribute keeps that sweep honest.
  const docsPaths = ['/api/v2/docs', '/api/v3/docs'];
  document.querySelectorAll('a[href]').forEach((a) => {
    if (docsPaths.includes(a.getAttribute('href'))) {
      a.href = api(a.getAttribute('href'));
    }
  });

  // Login form submit
  const loginForm = document.getElementById('login-form');
  if (loginForm) {
    loginForm.addEventListener('submit', async (e) => {
      e.preventDefault();
      const u = document.getElementById('login-username').value.trim();
      // Not trimmed: the password is compared byte for byte, and an older
      // record can hold any plaintext the operator originally chose.
      const p = document.getElementById('login-password').value;
      // The 2FA row stays hidden until the server says a code is needed; once
      // it has said so once in this tab, the row stays visible so a mistyped
      // code can be corrected without the field vanishing.
      const codeInput = document.getElementById('login-code');
      const code = codeInput ? (codeInput.value || '').trim() : '';
      const errDiv = document.getElementById('login-error');
      if (errDiv) errDiv.classList.add('hidden');

      try {
        const res = await fetch(api('/api/auth/login'), {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ username: u, password: p, code })
        });

        if (!res.ok) {
          // The body is parsed exactly once, here, and both the message and the
          // second-factor flag below read that one parse. errorMessage() calls
          // res.text() — it consumes the stream — so any later res.clone() on
          // this response throws "body already used" and the flag check would
          // silently never fire. That ordering bug is why a correct password
          // with 2FA on showed "Invalid credentials" and no code field.
          let errJSON = null;
          try { errJSON = await res.clone().json(); } catch (e) { /* body was not JSON */ }
          if (errDiv) {
            // The server distinguishes bad credentials from a lockout; showing
            // "Invalid credentials" for a 429 sends the operator hunting for a
            // typo when the real answer is "wait fifteen minutes".
            errDiv.innerText = await errorMessage(res, 'Invalid credentials');
            errDiv.classList.remove('hidden');
          }
          // The server flags the one case that needs the code field: the
          // password verified but the second factor did not (twofactor_required
          // in the 401 body). Revealing the row on every bad password — the old
          // behaviour, keyed on the status alone — is how a first-time operator
          // on an install with no 2FA enrolled was greeted by a TWO-FACTOR CODE
          // box. Once genuinely shown the row stays up for this tab, so a
          // mistyped code can be corrected without the field vanishing.
          if (res.status === 401 && p && codeInput) {
            const needs2fa = !!(errJSON && errJSON.twofactor_required);
            if (needs2fa) codeInput.closest('div').classList.remove('hidden');
          }
          return;
        }

        const data = await res.json();
        authToken = data.token;
        localStorage.setItem('digitdns_token', authToken);
        // Only a verified login is worth remembering: saving the pair before the
        // server answered would keep a typo, and a 401 would leave the next visit
        // prefilled with a password known to be wrong.
        const rememberBox = document.getElementById('login-remember');
        if (rememberBox && rememberBox.checked) saveRememberedLogin(u, p);
        else forgetRememberedLogin();
        // Deliberately not hidden before the reload. The dashboard underneath may
        // still hold the previous session's state (an expired-session line in a
        // panel, a stale "STALE — NO TELEMETRY" badge). Hiding the overlay first
        // revealed that stale page for a frame before the reload replaced it, which
        // is the "session expired" flash operators reported. The reload removes
        // both the overlay and the stale content at the same time.
        // Reload the document instead of booting the dashboard in place. If this
        // tab was opened before a server update, its markup is the old build and
        // bootDashboard() would only refresh the data, leaving the stale UI on
        // screen until a manual refresh. A reload revalidates index.html and the
        // asset bundle, so the operator lands on the UI that matches the server.
        // The weak-password nag is re-evaluated by loadConfig() after the reload.
        window.location.reload();
      } catch (err) {
        if (errDiv) {
          errDiv.innerText = 'Server connection failed';
          errDiv.classList.remove('hidden');
        }
      }
    });
  }

  // Unticking clears the stored pair at once rather than waiting for the next
  // successful login: "remember me" off should not leave a password behind.
  const rememberToggle = document.getElementById('login-remember');
  if (rememberToggle) {
    rememberToggle.addEventListener('change', () => {
      if (!rememberToggle.checked) forgetRememberedLogin();
    });
  }

  // Password change form submit
  const pwdForm = document.getElementById('change-pwd-form');
  if (pwdForm) {
    pwdForm.addEventListener('submit', async (e) => {
      e.preventDefault();
      const userField = document.getElementById('new-admin-user');
      const currentField = document.getElementById('current-admin-pass');
      const submitted = userField ? userField.value.trim() : '';
      const newPassword = document.getElementById('new-admin-pass').value;
      const currentPassword = currentField ? currentField.value : '';

      // The username field is prefilled with the live value, so "unchanged" has to
      // be measured against it — otherwise reopening the modal and saving would
      // read as a rename and demand a re-authentication for nothing.
      const liveUser = currentConfig?.server?.admin_username || 'admin';
      const newUsername = submitted && submitted !== liveUser ? submitted : '';
      if (!newPassword && !newUsername) {
        hideChangePwdModal();
        return;
      }
      // The server re-authenticates before touching either credential, so a
      // change without this field can only ever come back 403.
      if (!currentPassword) {
        showToast('Enter your current password to confirm the change', 'error');
        if (currentField) currentField.focus();
        return;
      }

      const payload = { current_password: currentPassword };
      if (newUsername) payload.admin_username = newUsername;
      if (newPassword) payload.admin_password = newPassword;
      // The second factor rides the same body. The
      // row is revealed when the config shows 2FA enabled; sending it while 2FA
      // is off is harmless (the server ignores it).
      const codeRow = document.getElementById('change-pwd-2fa-row');
      const codeInput = document.getElementById('change-pwd-code');
      if (codeRow) codeRow.classList.toggle('hidden', !currentConfig?.auth?.totp_enabled);
      if (codeInput && codeInput.value.trim()) payload.code = codeInput.value.trim();

      try {
        const res = await fetch(api('/api/config/server'), {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${authToken}`
          },
          body: JSON.stringify(payload)
        });

        if (!res.ok) {
          // The server explains exactly which policy rule the password failed;
          // a generic "Failed" left the operator guessing.
          showToast(await errorMessage(res, 'Failed to update credentials'), 'error');
          return;
        }

        const data = await res.json().catch(() => ({}));
        // A password change revokes every session, including this one, and hands
        // back a replacement. A username-only change does not, and sends no
        // token — overwriting the stored one with undefined would log us out.
        if (data.token) {
          authToken = data.token;
          localStorage.setItem('digitdns_token', authToken);
        }
        // A remembered login is only useful while it matches the live credentials;
        // a rotated password or renamed account would otherwise leave the next
        // visit prefilled with values that cannot work.
        if ((newPassword || newUsername) && readRememberedLogin()) {
          saveRememberedLogin(data.username || newUsername || liveUser, newPassword || currentPassword);
        }
        // Keep the local copy in step so the Settings card and the next open of
        // this modal show the name that is now in force.
        if (currentConfig?.server && data.username) {
          currentConfig.server.admin_username = data.username;
          const adminNameEl = document.getElementById('current-admin-name');
          if (adminNameEl) adminNameEl.innerText = data.username;
        }
        // The nag is satisfied, so a later voluntary open must not be forced.
        if (currentConfig?.server && newPassword) {
          currentConfig.server.admin_password_weak = false;
        }
        hideChangePwdModal();
        showToast(newPassword
          ? 'Credentials updated — other sessions have been signed out'
          : 'Username updated', 'success');
      } catch (err) {
        showToast('Error updating credentials', 'error');
      }
    });
  }

  // Voluntary credential rotation from the Settings tab.
  document.getElementById('open-change-pwd-btn')?.addEventListener('click', () => {
    showChangePwdModal(false);
  });
  document.getElementById('change-pwd-cancel')?.addEventListener('click', () => {
    hideChangePwdModal();
  });

  document.getElementById('logout-btn')?.addEventListener('click', () => {
    signOut();
  });
});

// signOut revokes the session on the daemon, then clears local state.
//
// Removing the token from localStorage is not a logout: the session stayed live
// server-side for the rest of its lifetime, so any copy of it — a shared browser,
// a proxy log, a captured Authorization header — kept working after the operator
// believed they had signed out. The network call revokes exactly this session;
// sibling sessions on other devices are deliberately left alone.
//
// The local state is cleared in `finally` so a daemon that is unreachable, or a
// token already expired, still returns the operator to the login screen instead
// of trapping them in a dashboard they can no longer use.
async function signOut() {
  try {
    await fetch(api('/api/auth/logout'), {
      method: 'POST',
      headers: { 'Authorization': 'Bearer ' + authToken, 'Content-Type': 'application/json' },
      body: '{}',
    });
  } catch (e) {
    // Nothing to surface: the operator asked to leave, and the only thing left
    // to do either way is drop the local session.
  } finally {
    stopLiveStream();
    localStorage.removeItem('digitdns_token');
    authToken = '';
    showLoginModal();
  }
}

// =======================================================
// AUTH & BOOTSTRAP
// =======================================================
function checkAuthAndBoot() {
  if (!authToken) {
    showLoginModal();
  } else {
    bootDashboard();
  }
}

function showLoginModal() {
  document.getElementById('login-modal')?.classList.remove('hidden');
}

// The credential modal has two modes. Forced is the security nag: the stored
// password fails the policy, so a new one is mandatory and there is nothing safe
// to cancel back to. Voluntary is the operator rotating a compliant password from
// Settings — the escape hatch has to be there, and a username-only change is
// legitimate, so the password field stops being mandatory.
function showChangePwdModal(forced = true) {
  const modal = document.getElementById('change-pwd-modal');
  if (!modal) return;

  const title = document.getElementById('change-pwd-title');
  const badge = document.getElementById('change-pwd-badge');
  const intro = document.getElementById('change-pwd-intro');
  const cancel = document.getElementById('change-pwd-cancel');
  const icon = document.getElementById('change-pwd-icon');
  const newPass = document.getElementById('new-admin-pass');
  const userField = document.getElementById('new-admin-user');

  if (forced) {
    if (title) title.innerText = 'Security Setup';
    if (badge) badge.innerText = 'WEAK ADMIN PASSWORD DETECTED';
    if (intro) {
      intro.innerText = 'This account is still on a password that fails the current policy. '
        + 'Anyone who can reach this dashboard may be able to log in with it. '
        + 'Set a new administrator password now:';
    }
    cancel?.classList.add('hidden');
    newPass?.setAttribute('required', 'required');
    if (newPass) newPass.placeholder = 'At least 10 characters';
  } else {
    if (title) title.innerText = 'Administrator Credentials';
    if (badge) badge.innerText = 'ROTATE YOUR DASHBOARD LOGIN';
    if (intro) {
      intro.innerText = 'Confirm with your current password. Saving a new password signs out '
        + 'every session, including this browser, and issues you a fresh one. '
        + 'Leave the password blank to change only the username.';
    }
    cancel?.classList.remove('hidden');
    // Not required here: the server accepts a username-only change.
    newPass?.removeAttribute('required');
    if (newPass) newPass.placeholder = 'Leave blank to keep the current password';
  }
  if (icon) {
    icon.setAttribute('data-feather', forced ? 'alert-triangle' : 'user-check');
  }
  // The field is prefilled with the live username so that submitting it unchanged
  // is a no-op rather than an accidental rename.
  if (userField) {
    userField.value = currentConfig?.server?.admin_username || userField.value || 'admin';
  }

  modal.classList.remove('hidden');
  // Through the helper rather than a bare window.feather test: the icon above was just swapped
  // by setAttribute, so this call is what draws it, and the helper is the version that also
  // checks the replace function exists and swallows a failure. An exception thrown here would
  // abort showChangePwdModal after the modal is already visible — losing the focus() below, on
  // the one modal that cannot be dismissed when it is forced.
  safeFeatherReplace();
  document.getElementById('current-admin-pass')?.focus();
}

function hideChangePwdModal() {
  document.getElementById('change-pwd-modal')?.classList.add('hidden');
  const cur = document.getElementById('current-admin-pass');
  const np = document.getElementById('new-admin-pass');
  if (cur) cur.value = '';
  if (np) np.value = '';
}

// ── Static modal behaviour ───────────────────────────────────────────────────────────────
//
// index.html carries five modals as static markup and showDialog builds a sixth kind at
// runtime. The runtime one has always been correct — role="dialog", aria-modal, a labelled
// heading, a focus trap, Escape, focus return — and the five static ones had none of it.
// A screen reader announced the login overlay as an ordinary div; Tab walked straight out of
// it into the dashboard's own controls behind the backdrop, before anyone had authenticated;
// and the only way out of the diagnostics panel was to find the small × in its corner.
//
// The dismissible ones declare their own close control with data-modal-close and the handlers
// below click it rather than hiding the modal themselves. Some of those buttons do more than
// remove a class — change-pwd-cancel runs hideChangePwdModal, which also clears both password
// fields so a typed-then-abandoned password is not left sitting in the DOM — and a second,
// parallel way to close would skip that half. The ones that only hide today cost nothing by
// going through the same path, and stop costing nothing the moment one of them grows a reset.
//
// Two modals are not dismissible, on purpose. login-modal is the authentication gate, and
// change-pwd-modal in forced mode is the weak-password gate: there is nothing safe behind
// either. change-pwd-modal does name change-pwd-cancel, but showChangePwdModal hides that
// button whenever the modal is forced, and modalCloseControl below returns null for a hidden
// control — so Escape is a no-op in exactly the mode where cancelling is not offered, and the
// two can never drift apart.
const MODAL_IDS = ['login-modal', 'change-pwd-modal', 'diagnostics-modal', 'edit-client-modal', 'add-client-modal', 'client-created-modal', 'bulk-import-modal', 'diff-modal'];

// Every one of these modals is shown and hidden by toggling .hidden, so reading the class is
// both the truth and cheap enough for a keydown handler.
function isShown(el) {
  return !!el && !el.classList.contains('hidden');
}

// The topmost open modal, in document order. Nothing stacks two static modals today, but
// Escape has to pick one, and document order is the right tiebreak: they all carry z-50, and
// with equal z-index the later element paints on top.
function topmostOpenModal() {
  let found = null;
  for (const id of MODAL_IDS) {
    const el = document.getElementById(id);
    if (isShown(el)) found = el;
  }
  return found;
}

// The panel is the single child of the backdrop container in all five.
function modalPanel(modal) {
  return modal.firstElementChild || modal;
}

// The close control the modal declares, or null when it declares none or the one it declares
// is currently hidden.
function modalCloseControl(modal) {
  const id = modal.getAttribute('data-modal-close');
  if (!id) return null;
  const btn = document.getElementById(id);
  return isShown(btn) ? btn : null;
}

// MODAL_FOCUSABLE is showDialog's trap selector with :not([disabled]) extended to select and
// textarea and hidden inputs excluded. The disabled part is not hypothetical: runFullDiagnostics
// disables #rerun-diagnostics-btn for the length of a run, and that button lives inside
// diagnostics-modal — a trap that stopped on it would park the keyboard on a dead control.
const MODAL_FOCUSABLE = 'button:not([disabled]), input:not([disabled]):not([type="hidden"]), select:not([disabled]), textarea:not([disabled]), [href]';

function modalFocusables(panel) {
  // offsetParent is null for anything display:none — a collapsed section, a .hidden button —
  // and none of these panels is itself position:fixed, so the check is safe here.
  return [...panel.querySelectorAll(MODAL_FOCUSABLE)].filter((el) => el.offsetParent !== null);
}

// A modal can hold its own popover — the expiry datepicker inside edit-client-modal is the only
// one today — and Escape there means "close the popover", not "throw away the form behind it".
// The popovers are marked in the markup rather than listed here, and each is closed the way its
// own handlers already close it: by putting .hidden back. Before this existed Escape did nothing
// at all in the datepicker, so nothing is being taken away.
function closeNestedPopover(modal) {
  for (const pop of modal.querySelectorAll('[data-modal-popover]')) {
    if (isShown(pop)) {
      pop.classList.add('hidden');
      return true;
    }
  }
  return false;
}

// Escape closes, Tab stays inside. Both are delegated from document once, at parse time, so
// they are live before the login overlay is unhidden — bootDashboard is where the rest of the
// listeners are attached and it only runs once authentication has succeeded.
document.addEventListener('keydown', (e) => {
  // showDialog's own trap runs on the capture phase and calls preventDefault, so a press it
  // has already dealt with arrives here marked. Without this, Escape on a confirm dialog
  // opened over the edit-client modal would close both — the dialog and the modal that asked
  // the question.
  if (e.defaultPrevented) return;
  if (e.key !== 'Escape' && e.key !== 'Tab') return;
  // A showDialog overlay on top owns the keyboard outright. Its own capture-phase trap handles
  // Tab, but it only calls preventDefault on the two wrap cases — so in the middle of its tab
  // order the press arrives here unmarked, and the static modal underneath would pull focus back
  // out of the dialog that is actually in front. defaultPrevented alone is not enough for that.
  if (activeDialog) return;

  const modal = topmostOpenModal();
  if (!modal) return;

  if (e.key === 'Escape') {
    // A popover inside the modal takes the press first, so Escape in the datepicker closes the
    // datepicker rather than discarding the client edit behind it.
    if (closeNestedPopover(modal)) {
      e.preventDefault();
      return;
    }
    const closeBtn = modalCloseControl(modal);
    if (!closeBtn) return;
    e.preventDefault();
    closeBtn.click();
    return;
  }

  // aria-modal="true" tells assistive technology the rest of the page is inert; the trap is
  // what makes that true for the keyboard as well. A real trap, not a wrap: focus that has
  // already escaped — a click on the backdrop, a focus the browser restored across a reload —
  // is pulled back in rather than left outside.
  const panel = modalPanel(modal);
  const focusables = modalFocusables(panel);
  if (!focusables.length) return;
  const first = focusables[0];
  const last = focusables[focusables.length - 1];
  const inside = panel.contains(document.activeElement);
  if (e.shiftKey && (!inside || document.activeElement === first)) {
    e.preventDefault();
    last.focus();
  } else if (!e.shiftKey && (!inside || document.activeElement === last)) {
    e.preventDefault();
    first.focus();
  }
});

// A press that lands on the backdrop itself, never one that bubbled up from inside the panel.
// mousedown rather than click, matching showDialog: a drag that starts inside a text field and
// releases over the backdrop is a selection, not a dismissal.
document.addEventListener('mousedown', (e) => {
  const el = e.target;
  if (!(el instanceof Element) || !el.id || !MODAL_IDS.includes(el.id) || !isShown(el)) return;
  const closeBtn = modalCloseControl(el);
  if (closeBtn) closeBtn.click();
});

// modalReturnFocus holds, per modal, whatever had focus when it opened.
const modalReturnFocus = new WeakMap();

// Focus in on open, focus back on close, observed rather than wired into each of the thirteen
// places that add or remove .hidden — one behaviour in one place, and no risk of the next
// modal being added without it.
//
// The "only if focus is not already inside" guard is what preserves the deliberate choices the
// show functions make: showChangePwdModal focuses the current-password field and runs
// synchronously, while this callback is a microtask, so it sees that focus and leaves it alone.
function initModalA11y() {
  for (const id of MODAL_IDS) {
    const modal = document.getElementById(id);
    if (!modal) continue;

    let wasOpen = isShown(modal);
    const observer = new MutationObserver(() => {
      const open = isShown(modal);
      if (open === wasOpen) return;
      wasOpen = open;

      if (open) {
        modalReturnFocus.set(modal, document.activeElement);
        const panel = modalPanel(modal);
        if (!panel.contains(document.activeElement)) {
          modalFocusables(panel)[0]?.focus();
        }
        return;
      }

      // Closing drops focus to <body>, which puts the next Tab back at the top of the
      // document — a long walk back to the button the operator had just pressed.
      const back = modalReturnFocus.get(modal);
      modalReturnFocus.delete(modal);
      if (back instanceof HTMLElement && back.isConnected && back.offsetParent !== null) {
        try { back.focus(); } catch (err) { /* went away mid-close */ }
      }
    });
    observer.observe(modal, { attributes: true, attributeFilter: ['class'] });
  }
}

let areEventListenersAttached = false;

async function bootDashboard() {
  if (!areEventListenersAttached) {
    initEventListeners();
    initClientEventListeners();
    initAPIEvents();
    areEventListenersAttached = true;
  }
  // Apply the last known arrangement before the config request lands. The
  // authoritative layout still comes from the server below, but a reload no
  // longer paints the default grid first and then jumps every card into place.
  try {
    const cached = localStorage.getItem(DASH_LAYOUT_CACHE_KEY);
    if (cached) applyDashboardLayout(JSON.parse(cached));
  } catch (e) { /* corrupt cache is ignored */ }
  const authed = await loadConfig();
  if (!authed) return; // 401 -> login modal is already shown
  loadOnboarding();
  // Load only the tab the URL asked for. Clients, agents, filtering and history
  // have their own loaders and are reached from switchTab; fetching the whole
  // client list here used to delay the first usable screen on every reload.
  handleRouteFromURL(true);
  startStatsPolling();
  // The live log must keep collecting while the operator browses in another
  // tab or another dashboard screen. The stream is started once here and lives
  // until sign-out; pushStreamQuery already skips DOM work when the Log tab is
  // not visible, so the connection costs a buffer update rather than a repaint.
  startLiveStream();
  initHistory();
  initStatsSettings();
  initFiltering();
}

// =======================================================
// CONFIG & POLICIES SYNC
// =======================================================
// loadConfig fetches the account probe and the configuration, and renders the Settings and
// Policies panels from them.
//
// Both fetches used to be checked for 401 and nothing else, and both bodies were parsed with
// res.json() regardless of the status. That failed in two directions on any other error status.
//
// The /api/config half was the damaging one. A 500 returns a JSON error body, so `currentConfig`
// became {error: "…"} — truthy, which is all `saveRules` guards on — and renderConfig then read
// cfg.rules?.custom_proxied off it and got an empty list, painting the proxy list as empty. Press
// Save on that screen and saveRules writes those empty arrays back: every custom domain erased on
// the server because one fetch returned 500. The operator sees a plausible screen and one click
// destroys the configuration.
//
// The /api/auth/me half failed quietly instead. meData.password_weak is undefined in an error
// body, so `weak` came out false and the forced credential-change modal was skipped — the weak
// password gate opening because its probe broke, which is the wrong direction for a gate.
//
// So: neither body is parsed unless the response is ok, and nothing is rendered from a config
// that did not arrive. Failing with the old screen still on display and a toast that says why is
// strictly better than replacing it with a confident, wrong one.
async function loadConfig() {
  if (!authToken) {
    showLoginModal();
    return false;
  }
  try {
    // Two independent reads: the account probe supplies the weak-password flag, the
    // config supplies everything the shell renders. They used to run one after the
    // other, adding a full round trip to every boot; now they share one.
    const [meRes, cfgRes] = await Promise.all([
      fetch(api('/api/auth/me'), {
        headers: { 'Authorization': `Bearer ${authToken}` }
      }),
      fetch(api('/api/config'), {
        headers: { 'Authorization': `Bearer ${authToken}` }
      }),
    ]);

    if (meRes.status === 401 || cfgRes.status === 401) {
      showLoginModal();
      return false;
    }
    if (!meRes.ok) {
      showToast(await errorMessage(meRes, 'Could not read the account state.'), 'error');
      return false;
    }
    if (!cfgRes.ok) {
      showToast(await errorMessage(cfgRes, 'Could not load the configuration.'), 'error');
      return false;
    }

    const [meData, cfgData] = await Promise.all([meRes.json(), cfgRes.json()]);
    const weak = !!(meData.password_weak || meData.is_default_password);
    currentConfig = cfgData;
    renderConfig(currentConfig);
    // The transport listener state is not in /api/config; fetch it once so the
    // card and the global listener notice reflect what is actually listening.
    refreshTransportView();
    // Settings and agent data are loaded by the tab that needs them, not on
    // every dashboard boot. The four calls that used to be here put four extra
    // round trips in front of the panel for screens the operator may never open.
    // Raised after the config lands so the modal can prefill the real username
    // rather than the markup's placeholder.
    if (weak) showChangePwdModal();
    return true;
  } catch (e) {
    console.error('Failed to load config:', e);
    showToast('Could not reach the server to load the configuration.', 'error');
    return false;
  }
}

let isAPIKeyMasked = true;

// renderTLSCertDetails fills the SSL/TLS Manager card's two validity lines
// (DoH/DoT and ADMIN PANEL) from config.dot_cert / config.panel_cert and
// flips each Let's Encrypt button to its Renew label when that row's
// certificate has expired. Same contract as renderSubCertDetails in
// twofa.js: nil means "no details", which renders as a slate "none" line
// and leaves the button's issue label alone — the button that CREATES the
// first certificate must never announce a renewal that cannot exist yet.
function renderTLSCertDetails(cfg) {
  const fmt = ts => new Date(ts).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });

  function render(detailsId, cert, issueLabel, renewLabel, btnId) {
    const details = document.getElementById(detailsId);
    const btn = document.getElementById(btnId);
    if (!details) return;
    if (!cert || !cert.not_after) {
      details.textContent = 'No certificate yet — this listener is not serving TLS, or its pair could not be read.';
      details.className = 'text-[11px] font-mono text-slate-500 mt-1';
      if (btn) btn.textContent = issueLabel;
      return;
    }
    if (cert.expired) {
      details.textContent = `Certificate expired ${fmt(cert.not_after)} — renew below.`;
      details.className = 'text-[11px] font-mono text-red-400 mt-1';
      if (btn) btn.textContent = renewLabel;
    } else {
      const daysLeft = Math.ceil((new Date(cert.not_after) - Date.now()) / 86400000);
      details.textContent = `Certificate for ${cert.domain || 'this listener'}: valid ${fmt(cert.not_before)} → ${fmt(cert.not_after)} · ${daysLeft} day${daysLeft === 1 ? '' : 's'} left`;
      details.className = 'text-[11px] font-mono text-emerald-400/80 mt-1';
      if (btn) btn.textContent = issueLabel;
    }
  }

  render('dot-cert-details', cfg.dot_cert, 'Issue certificate', 'Renew certificate', 'issue-dot-ssl-btn');
  render('panel-cert-details', cfg.panel_cert, 'Issue certificate', 'Renew certificate', 'issue-ssl-btn');

  // The DoH/DoT auto-renew toggle's live state. The snapshot carries the
  // positive form; a config predating the field reads as undefined, and
  // defaulting that to checked matches the daemon's "renewal keeps running"
  // reading of the same absence.
  const dotAutoRenew = document.getElementById('dot-auto-renew-input');
  if (dotAutoRenew) {
    const dotCfg = cfg.certificates && cfg.certificates.dot ? cfg.certificates.dot : null;
    const byo = !!(dotCfg && dotCfg.source === 'files');
    dotAutoRenew.checked = byo
      ? !!dotCfg.auto_renew
      : (!cfg.tls || cfg.tls.dot_auto_renew_acme !== false);
    // A BYO pair is renewed by whatever issued it (certbot, the CA, the
    // operator). The daemon's daily loop deliberately does not touch it.
    dotAutoRenew.disabled = byo;
    dotAutoRenew.title = byo
      ? 'This certificate was supplied by the operator; renew it outside DigitDNS.'
      : '';
  }
  const panelAutoRenew = document.getElementById('panel-auto-renew-input');
  if (panelAutoRenew) {
    const panelCfg = cfg.certificates && cfg.certificates.panel ? cfg.certificates.panel : null;
    const byo = !!(panelCfg && panelCfg.source === 'files');
    panelAutoRenew.checked = byo
      ? !!panelCfg.auto_renew
      : (!cfg.tls || cfg.tls.panel_auto_renew_acme !== false);
    panelAutoRenew.disabled = byo;
    panelAutoRenew.title = byo
      ? 'This certificate was supplied by the operator; renew it outside DigitDNS.'
      : '';
  }
  if (typeof window.syncCertManagerFields === 'function') {
    window.syncCertManagerFields(true);
  }
}


// ── Dashboard layout engine ───────────────────────────────────────────────────
// The dashboard is an explicit 12-column grid. A card owns x/y/w/h in that grid,
// not just DOM order: that is what lets a card move to any column and row, keeps
// a tall card from stretching its neighbours, and makes the layout survive a
// reload. Positions are stored under the existing dashboard_layout policy.
//
// Pure helpers (DASH_LAYOUT_PURE_START .. DASH_LAYOUT_PURE_END) are extracted by
// tests/web/dashboard-layout.test.cjs and must not touch the DOM or window.
// DASH_LAYOUT_PURE_START
const DASH_COLS = 12;
const DASH_ROW_PX = 12;
const DASH_COL_GAP = 12;
const DASH_ROW_GAP = 12;
const DASH_MAX_ROWS = 240;
const DASH_LAYOUT_VERSION = 3;
const DASH_LAYOUT_CACHE_KEY = 'digitdns_dashboard_layout_v3';
const DASH_CARD_MIN = {
  qps: { w: 2, h: 5 },
  cache: { w: 2, h: 5 },
  sni: { w: 2, h: 5 },
  cpu: { w: 2, h: 5 },
  ram: { w: 2, h: 5 },
  traffic: { w: 2, h: 5 },
  chart: { w: 4, h: 10 },
  upstream: { w: 3, h: 10 },
  latency: { w: 4, h: 8 },
  'history-chart': { w: 4, h: 10 },
  'history-top-domains': { w: 2, h: 8 },
  'history-top-blocked': { w: 2, h: 8 },
  'history-top-clients': { w: 2, h: 8 },
  'history-top-upstreams': { w: 2, h: 8 },
  'history-top-protocols': { w: 2, h: 8 },
  'history-top-relay': { w: 2, h: 8 },
};

function dashInt(value, fallback) {
  const n = Number(value);
  return Number.isFinite(n) ? Math.round(n) : fallback;
}

function dashMin(id) {
  return DASH_CARD_MIN[id] || { w: 2, h: 5 };
}

function dashClampCard(card, id) {
  const min = dashMin(id);
  const w = Math.max(min.w, Math.min(DASH_COLS, dashInt(card && card.w, min.w)));
  const x = Math.max(0, Math.min(DASH_COLS - w, dashInt(card && card.x, 0)));
  const h = Math.max(min.h, Math.min(DASH_MAX_ROWS, dashInt(card && card.h, min.h)));
  const y = Math.max(0, Math.min(DASH_MAX_ROWS - h, dashInt(card && card.y, 0)));
  return { x, y, w, h };
}

function dashOverlaps(a, b) {
  return a.x < b.x + b.w && a.x + a.w > b.x && a.y < b.y + b.h && a.y + a.h > b.y;
}

function dashSortOrder(cards, ids) {
  return [...ids].sort((a, b) => {
    const A = cards[a] || { x: 0, y: 0 };
    const B = cards[b] || { x: 0, y: 0 };
    return A.y - B.y || A.x - B.x || ids.indexOf(a) - ids.indexOf(b);
  });
}

// Vertical gravity. The anchor (the card the operator just dropped or resized)
// keeps its chosen y so a deliberate placement is not undone; every other card
// falls to the nearest free row in its own columns.
function dashCompact(cards, order, anchorId) {
  const ids = Array.from(new Set([...order, ...Object.keys(cards)]));
  const rank = new Map(order.map((id, i) => [id, i]));
  const anchor = anchorId && cards[anchorId] ? anchorId : '';
  const byPosition = (a, b) => {
    const A = cards[a] || { x: 0, y: 0 };
    const B = cards[b] || { x: 0, y: 0 };
    return A.y - B.y || A.x - B.x || (rank.get(a) ?? 0) - (rank.get(b) ?? 0);
  };
  const sequence = anchor
    ? [anchor, ...ids.filter((id) => id !== anchor).sort(byPosition)]
    : ids.sort(byPosition);
  const placed = [];
  sequence.forEach((id) => {
    if (!cards[id]) return;
    const card = { ...cards[id] };
    if (id !== anchor) card.y = 0;
    let guard = 0;
    while (placed.some((other) => dashOverlaps(card, other)) && guard++ < DASH_MAX_ROWS * 2) {
      card.y += 1;
    }
    cards[id] = card;
    placed.push(card);
  });
  return cards;
}

function dashDefaultCards(name, ids) {
  const cards = {};
  if (name === 'metrics') {
    ids.forEach((id, i) => {
      cards[id] = { x: (i * 2) % DASH_COLS, y: Math.floor((i * 2) / DASH_COLS) * 7, w: 2, h: 7 };
    });
    return cards;
  }
  if (name === 'main') {
    const defaults = {
      chart: { x: 0, y: 0, w: 8, h: 19 },
      upstream: { x: 8, y: 0, w: 4, h: 33 },
      latency: { x: 0, y: 19, w: 8, h: 13 },
    };
    ids.forEach((id, i) => {
      cards[id] = defaults[id] || { x: (i * 4) % DASH_COLS, y: 0, w: 4, h: 11 };
    });
    return cards;
  }
  if (name === 'history') {
    // KPI strip, a full-width chart, then the six ranked tables in two rows of
    // three. These are the defaults; every card can still be moved or resized.
    const kpis = [
      'history-total', 'history-blocked', 'history-proxied',
      'history-cache', 'history-clients', 'history-latency',
    ];
    const tables = [
      'history-top-domains', 'history-top-blocked', 'history-top-clients',
      'history-top-upstreams', 'history-top-protocols', 'history-top-relay',
    ];
    kpis.forEach((id, i) => { cards[id] = { x: i * 2, y: 0, w: 2, h: 7 }; });
    cards['history-chart'] = { x: 0, y: 7, w: 12, h: 19 };
    tables.forEach((id, i) => {
      cards[id] = { x: (i % 3) * 4, y: 26 + Math.floor(i / 3) * 11, w: 4, h: 11 };
    });
    ids.forEach((id, i) => {
      if (!cards[id]) cards[id] = { x: (i * 2) % DASH_COLS, y: 48, w: 2, h: 5 };
    });
    return cards;
  }
  ids.forEach((id, i) => {
    cards[id] = { x: (i * 2) % DASH_COLS, y: 0, w: 2, h: 5 };
  });
  return cards;
}

// Accepts the v3 {version, grids} document, the v2 document whose y/h are in
// 24px rows, and the old {order, cards:{w,h}} document. v2 is migrated by
// doubling y/h; the old format is scaled to the 12-column model and then to the
// 12px row unit.
function dashNormalizeLayout(raw, gridIds) {
  const source = raw && typeof raw === 'object' ? raw : {};
  const version = dashInt(source.version, 0);
  const v3 = version >= DASH_LAYOUT_VERSION && source.grids && typeof source.grids === 'object';
  const v2 = version === 2 && source.grids && typeof source.grids === 'object';
  const out = { version: DASH_LAYOUT_VERSION, grids: {} };
  // A card can move between dashboard grids. Treat every card named by a saved
  // group as claimed by that grid; only cards with no saved home fall back to
  // their original markup grid. Without this, a history card dropped into the
  // main grid would be re-added to history on the next load.
  const claimed = new Set();
  if (v3 || v2) {
    Object.values(source.grids).forEach((group) => {
      const saved = group && group.cards && typeof group.cards === 'object' ? group.cards : {};
      Object.keys(saved).forEach((id) => claimed.add(id));
    });
  }
  Object.entries(gridIds).forEach(([name, markupIds]) => {
    const group = (v3 || v2) ? (source.grids[name] || {}) : {};
    const supplied = group.cards && typeof group.cards === 'object' ? group.cards : {};
    const ids = (v3 || v2)
      ? Array.from(new Set([...Object.keys(supplied), ...markupIds.filter((id) => !claimed.has(id))]))
      : markupIds;
    const defaults = dashDefaultCards(name, ids);
    const cards = {};
    if (v3) {
      ids.forEach((id) => { cards[id] = dashClampCard(supplied[id] || defaults[id], id); });
    } else if (v2) {
      ids.forEach((id) => {
        const saved = supplied[id];
        if (!saved || typeof saved !== 'object') {
          cards[id] = dashClampCard(defaults[id], id);
          return;
        }
        cards[id] = dashClampCard({
          ...saved,
          y: Math.round((40 * dashInt(saved.y, 0)) / 20),
          h: Math.round((40 * dashInt(saved.h, dashMin(id).h) - 12) / 20),
        }, id);
      });
    } else {
      const submitted = Array.isArray(source.order && source.order[name]) ? source.order[name] : [];
      const order = submitted.filter((id, i) => ids.includes(id) && submitted.indexOf(id) === i);
      ids.forEach((id) => { if (!order.includes(id)) order.push(id); });
      const scale = name === 'metrics' ? 2 : 4;
      let x = 0;
      let y = 0;
      let bottom = 0;
      order.forEach((id) => {
        const legacy = source.cards && source.cards[id];
        const base = defaults[id];
        let w = legacy && Number.isFinite(Number(legacy.w)) ? dashInt(Number(legacy.w) * scale, base.w) : base.w;
        let h = legacy && Number.isFinite(Number(legacy.h)) ? dashInt(Number(legacy.h) * 4, base.h) : base.h;
        w = Math.max(dashMin(id).w, Math.min(DASH_COLS, w));
        h = Math.max(dashMin(id).h, Math.min(DASH_MAX_ROWS, h));
        if (x + w > DASH_COLS) {
          x = 0;
          y = bottom;
        }
        cards[id] = { x, y, w, h };
        x += w;
        bottom = Math.max(bottom, y + h);
        if (x >= DASH_COLS) {
          x = 0;
          y = bottom;
        }
      });
      ids.forEach((id) => {
        if (cards[id]) return;
        const base = defaults[id];
        if (x + base.w > DASH_COLS) {
          x = 0;
          y = bottom;
        }
        cards[id] = { ...base, x, y };
        x += base.w;
        bottom = Math.max(bottom, y + base.h);
      });
    }
    let order = dashSortOrder(cards, ids);
    if (!v3) {
      dashCompact(cards, order, '');
      order = dashSortOrder(cards, ids);
    }
    out.grids[name] = { cards, order };
  });
  return out;
}

/** True when a candidate rectangle is inside the grid and does not overlap. */
function dashCardFits(card, cards, ignoreId) {
  if (!card) return false;
  if (card.x < 0 || card.y < 0 || card.w < 1 || card.h < 1) return false;
  if (card.x + card.w > DASH_COLS || card.y + card.h > DASH_MAX_ROWS) return false;
  return !Object.entries(cards).some(([id, other]) => id !== ignoreId && dashOverlaps(card, other));
}

/** Resize a card from any edge or corner. dx/dy are cell deltas from the start. */
function dashResizeRect(cfg, edge, dx, dy) {
  const min = dashMin(cfg.id);
  let x = cfg.x;
  let y = cfg.y;
  let w = cfg.w;
  let h = cfg.h;
  if (edge.includes('e')) w = cfg.w + dx;
  if (edge.includes('s')) h = cfg.h + dy;
  if (edge.includes('w')) {
    x = Math.max(0, Math.min(cfg.x + dx, cfg.x + cfg.w - min.w));
    w = cfg.x + cfg.w - x;
  }
  if (edge.includes('n')) {
    y = Math.max(0, Math.min(cfg.y + dy, cfg.y + cfg.h - min.h));
    h = cfg.y + cfg.h - y;
  }
  w = Math.max(min.w, Math.min(DASH_COLS - x, w));
  h = Math.max(min.h, Math.min(DASH_MAX_ROWS - y, h));
  return { x, y, w, h };
}

function dashCloneLayout(layout) {
  return JSON.parse(JSON.stringify(layout));
}
// DASH_LAYOUT_PURE_END

let dashLayout = { version: DASH_LAYOUT_VERSION, grids: {} };
let dashOriginalIds = null;
let dashEditing = false;
let dashLayoutDirty = false;

function dashGridEls() {
  return Array.from(document.querySelectorAll('#tab-dashboard [data-dash-grid]'));
}

function dashCardsOf(grid) {
  return Array.from(grid.children).filter((el) => el.matches('[data-dash-card]'));
}

function dashIsDesktop() {
  // Free placement fixes every grid at twelve columns, and the six KPI cards are
  // two columns each. Below ~1280px the content column leaves those cards too
  // narrow for their value/meta rows, so a small laptop keeps the responsive
  // stacked grids instead of the absolute layout.
  return window.matchMedia('(min-width: 1280px)').matches;
}

function dashGridIds() {
  const ids = {};
  dashGridEls().forEach((grid) => {
    ids[grid.dataset.dashGrid] = dashCardsOf(grid).map((el) => el.dataset.dashCard);
  });
  return ids;
}

// The markup order, captured before any layout reorders the DOM. Reset must
// restore this, not whichever order the last drag left behind.
function dashOriginalGridIds() {
  if (!dashOriginalIds) dashOriginalIds = dashGridIds();
  return dashOriginalIds;
}

function dashGridMetrics(grid) {
  const rect = grid.getBoundingClientRect();
  const style = getComputedStyle(grid);
  const columnGap = parseFloat(style.columnGap) || DASH_COL_GAP;
  const rowGap = parseFloat(style.rowGap) || DASH_ROW_GAP;
  const colWidth = Math.max(1, (grid.clientWidth - columnGap * (DASH_COLS - 1)) / DASH_COLS);
  return { rect, columnGap, rowGap, colWidth };
}

function dashApplyCardStyle(card, cfg) {
  card.style.gridColumn = `${cfg.x + 1} / span ${cfg.w}`;
  card.style.gridRow = `${cfg.y + 1} / span ${cfg.h}`;
  card.style.height = '';
  card.style.overflow = 'auto';
}

function resizeDashCharts() {
  if (qpsChart) qpsChart.resize();
  if (historyChart) historyChart.resize();
}

function dashFlip(before) {
  if (!before || !before.size) return;
  requestAnimationFrame(() => {
    document.querySelectorAll('#tab-dashboard [data-dash-card]').forEach((card) => {
      const old = before.get(card);
      if (!old) return;
      const now = card.getBoundingClientRect();
      const dx = old.left - now.left;
      const dy = old.top - now.top;
      if (Math.abs(dx) < 1 && Math.abs(dy) < 1) return;
      card.style.transition = 'none';
      card.style.transform = `translate(${dx}px, ${dy}px)`;
      requestAnimationFrame(() => {
        card.style.transition = 'transform 180ms ease';
        card.style.transform = '';
        setTimeout(() => { card.style.transition = ''; }, 220);
      });
    });
    resizeDashCharts();
  });
}

function applyDashboardLayout(raw, animate) {
  if (!document.getElementById('tab-dashboard')) return;
  if (raw !== undefined) dashLayout = dashNormalizeLayout(raw, dashGridIds());
  const section = document.getElementById('tab-dashboard');
  const desktop = dashIsDesktop();
  const before = animate ? new Map(
    Array.from(document.querySelectorAll('#tab-dashboard [data-dash-card]')).map((card) => [card, card.getBoundingClientRect()]),
  ) : null;
  section.classList.toggle('dash-custom-layout', desktop);
  dashGridEls().forEach((grid) => {
    const name = grid.dataset.dashGrid;
    const group = dashLayout.grids[name];
    if (!group) return;
    // Pull every card named by the group into this grid. This is what makes a
    // cross-grid drop persist: the card's DOM parent follows its layout group.
    const allCards = Array.from(document.querySelectorAll('#tab-dashboard [data-dash-card]'));
    const cards = group.order
      .map((id) => allCards.find((card) => card.dataset.dashCard === id))
      .filter(Boolean);
    cards.forEach((card) => grid.appendChild(card));
    if (desktop) {
      grid.style.gridTemplateColumns = `repeat(${DASH_COLS}, minmax(0, 1fr))`;
      grid.style.gridAutoRows = `${DASH_ROW_PX}px`;
      grid.style.columnGap = `${DASH_COL_GAP}px`;
      grid.style.rowGap = `${DASH_ROW_GAP}px`;
      cards.forEach((card) => {
        const cfg = group.cards[card.dataset.dashCard];
        if (cfg) dashApplyCardStyle(card, cfg);
      });
    } else {
      ['gridTemplateColumns', 'gridAutoRows', 'columnGap', 'rowGap'].forEach((key) => {
        grid.style[key] = '';
      });
      cards.forEach((card) => {
        card.style.gridColumn = '';
        card.style.gridRow = '';
        card.style.height = '';
        card.style.overflow = '';
      });
    }
  });
  if (animate) dashFlip(before);
  else requestAnimationFrame(resizeDashCharts);
  // Cache the arrangement so the next reload can apply it before the config
  // request lands instead of painting the default grid and jumping.
  try { localStorage.setItem(DASH_LAYOUT_CACHE_KEY, JSON.stringify(dashLayout)); } catch (e) {}
  document.documentElement.classList.remove('dash-layout-pending');
}

// Edits stay in memory while the editor is open. The only write happens when
// the pencil is clicked to leave edit mode; there is deliberately no debounce,
// so an experiment that is not confirmed never reaches the database.
function markDashLayoutDirty() {
  dashLayoutDirty = true;
}

async function saveDashboardLayout(announce) {
  if (!dashLayoutDirty) return true;
  dashLayoutDirty = false;
  if (currentConfig) currentConfig.dashboard_layout = dashLayout;
  try {
    const res = await fetch(api('/api/dashboard/layout'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${authToken}` },
      body: JSON.stringify({ layout: dashLayout }),
    });
    if (!res.ok) throw new Error(await errorMessage(res, 'Could not save the dashboard layout'));
    if (announce) showToast('Dashboard layout saved', 'success');
    return true;
  } catch (e) {
    dashLayoutDirty = true;
    showToast(e.message || 'Could not save the dashboard layout', 'error');
    return false;
  }
}

function syncDashEditUi() {
  const section = document.getElementById('tab-dashboard');
  const bar = document.getElementById('dash-edit-bar');
  const hint = document.getElementById('dash-edit-hint');
  if (!section) return;
  section.classList.toggle('is-editing', dashEditing);
  document.querySelectorAll('.dash-edit-toggle').forEach((btn) => {
    btn.classList.toggle('is-editing', dashEditing);
    btn.setAttribute('aria-pressed', dashEditing ? 'true' : 'false');
    const label = dashEditing ? 'Exit dashboard layout editing' : 'Edit dashboard layout';
    btn.title = label;
    btn.setAttribute('aria-label', label);
  });
  if (bar) bar.classList.toggle('hidden', !dashEditing);
  document.querySelectorAll('.dash-reset-toggle').forEach((btn) => {
    btn.classList.toggle('hidden', !dashEditing);
  });
  document.querySelectorAll('.sidebar-nav-row, .mobile-nav-slot').forEach((row) => {
    row.classList.toggle('has-dash-reset', dashEditing);
  });
  if (hint) {
    hint.textContent = dashIsDesktop()
      ? 'Drag a card anywhere · resize from any edge or corner · saved when you exit'
      : 'Long-press a card to reorder it · saved when you exit';
  }
}

function clearDashHandles() {
  document.querySelectorAll('.dash-edit-cover, .dash-resize, .dash-placeholder, .dash-drag-ghost').forEach((el) => el.remove());
  document.querySelectorAll('.dash-drag-origin').forEach((el) => el.classList.remove('dash-drag-origin'));
}

// Pick the dashboard grid whose top is nearest above the pointer. Using the
// grid's current bounding box would reject the blank area left below a card
// that was just made smaller, because that area sits outside the grid until a
// card occupies it. Nearest-top keeps that free space addressable.
function dashGridAtPoint(x, y) {
  let best = null;
  dashGridEls().forEach((grid) => {
    const rect = grid.getBoundingClientRect();
    if (rect.width <= 0 || rect.height <= 0) return;
    if (x < rect.left || x > rect.right) return;
    if (rect.top > y) return;
    if (!best || rect.top > best.rect.top) best = { grid, rect };
  });
  return best ? best.grid : null;
}

function dashScrollParent(el) {
  let node = el && el.parentElement;
  while (node && node !== document.body && node !== document.documentElement) {
    const style = getComputedStyle(node);
    if (/(auto|scroll)/.test(style.overflowY) && node.scrollHeight > node.clientHeight) return node;
    node = node.parentElement;
  }
  return null;
}

function beginDashDragMobile(e, card, grid, overlay) {
  if (e.button !== undefined && e.button !== 0) return;
  e.preventDefault();
  const startX = e.clientX;
  const startY = e.clientY;
  let active = false;
  let holdTimer = null;
  const activate = () => {
    active = true;
    card.classList.add('dash-dragging');
    document.body.style.userSelect = 'none';
  };
  holdTimer = setTimeout(activate, 450);
  const move = (ev) => {
    if (!active) {
      if (Math.hypot(ev.clientX - startX, ev.clientY - startY) > 8) cleanup();
      return;
    }
    ev.preventDefault();
    const under = document.elementFromPoint(ev.clientX, ev.clientY);
    const target = under && under.closest('[data-dash-card]');
    if (target && target !== card && target.parentElement === grid) {
      const r = target.getBoundingClientRect();
      const before = ev.clientY < r.top + r.height / 2;
      grid.insertBefore(card, before ? target : target.nextSibling);
    }
  };
  const finish = () => {
    const wasActive = active;
    cleanup();
    if (!wasActive) return;
    const name = grid.dataset.dashGrid;
    const group = dashLayout.grids[name];
    if (group) {
      group.order = dashCardsOf(grid).map((el) => el.dataset.dashCard);
      markDashLayoutDirty();
    }
  };
  const cleanup = () => {
    clearTimeout(holdTimer);
    card.classList.remove('dash-dragging');
    document.body.style.userSelect = '';
    document.removeEventListener('pointermove', move);
    document.removeEventListener('pointerup', finish);
    document.removeEventListener('pointercancel', finish);
  };
  document.addEventListener('pointermove', move, { passive: false });
  document.addEventListener('pointerup', finish, { once: true });
  document.addEventListener('pointercancel', finish, { once: true });
}

function beginDashDrag(e, card, grid) {
  if (!dashIsDesktop()) {
    beginDashDragMobile(e, card, grid);
    return;
  }
  if (e.button !== undefined && e.button !== 0) return;
  e.preventDefault();
  const name = grid.dataset.dashGrid;
  const sourceName = name;
  const id = card.dataset.dashCard;
  const group = dashLayout.grids[name];
  const cfg = group && group.cards[id];
  if (!cfg) return;
  let activeName = name;
  let activeGrid = grid;
  let activeGroup = group;
  const startRect = card.getBoundingClientRect();
  const offsetX = e.clientX - startRect.left;
  const offsetY = e.clientY - startRect.top;
  const ghost = card.cloneNode(true);
  ghost.querySelectorAll('.dash-edit-cover, .dash-resize').forEach((el) => el.remove());
  ghost.classList.add('dash-drag-ghost');
  ghost.style.width = `${startRect.width}px`;
  ghost.style.height = `${startRect.height}px`;
  ghost.style.left = `${startRect.left}px`;
  ghost.style.top = `${startRect.top}px`;
  const placeholder = document.createElement('div');
  placeholder.className = 'dash-placeholder';
  document.body.append(ghost, placeholder);
  card.classList.add('dash-drag-origin');
  document.body.style.userSelect = 'none';
  let target = { x: cfg.x, y: cfg.y, w: cfg.w, h: cfg.h };
  let moved = false;
  let lastPointer = { x: e.clientX, y: e.clientY };
  let dragActive = true;
  let autoScrollFrame = 0;
  const scroller = dashScrollParent(grid);
  // Recompute the drop cell from the current pointer and the grid's current
  // viewport position. This is called on pointermove *and* on scroll, so a
  // scrolled page keeps the placeholder under the card instead of freezing it
  // at the position that was valid before the scroll.
  const updateFromPointer = () => {
    const left = lastPointer.x - offsetX;
    const top = lastPointer.y - offsetY;
    ghost.style.left = `${left}px`;
    ghost.style.top = `${top}px`;
    // The dashboard is split into several grids (metrics, main, history). A
    // card may be dropped into any of them; probe the grid under the pointer,
    // but only commit to it once the candidate cell is actually free.
    let probeName = activeName;
    let probeGrid = activeGrid;
    let probeGroup = activeGroup;
    const hovered = dashGridAtPoint(lastPointer.x, lastPointer.y);
    if (hovered) {
      const hoveredName = hovered.dataset.dashGrid;
      const hoveredGroup = dashLayout.grids[hoveredName];
      if (hoveredGroup) {
        probeName = hoveredName;
        probeGrid = hovered;
        probeGroup = hoveredGroup;
      }
    }
    const m = dashGridMetrics(probeGrid);
    const x = Math.max(0, Math.min(DASH_COLS - cfg.w, Math.round((left - m.rect.left) / (m.colWidth + m.columnGap))));
    const y = Math.max(0, Math.min(DASH_MAX_ROWS - cfg.h, Math.round((top - m.rect.top) / (DASH_ROW_PX + m.rowGap))));
    const candidate = { x, y, w: cfg.w, h: cfg.h };
    // Free placement: the card keeps the exact cell it is dropped on. An
    // occupied cell is refused, so cards can never overlap and the last valid
    // position stays under the pointer.
    if (!dashCardFits(candidate, probeGroup.cards, id)) return;
    activeName = probeName;
    activeGrid = probeGrid;
    activeGroup = probeGroup;
    target = candidate;
    moved = moved || activeName !== sourceName || target.x !== cfg.x || target.y !== cfg.y;
    placeholder.style.left = `${m.rect.left + target.x * (m.colWidth + m.columnGap)}px`;
    placeholder.style.top = `${m.rect.top + target.y * (DASH_ROW_PX + m.rowGap)}px`;
    placeholder.style.width = `${cfg.w * m.colWidth + (cfg.w - 1) * m.columnGap}px`;
    placeholder.style.height = `${cfg.h * DASH_ROW_PX + (cfg.h - 1) * m.rowGap}px`;
  };
  const move = (ev) => {
    ev.preventDefault();
    lastPointer = { x: ev.clientX, y: ev.clientY };
    updateFromPointer();
  };
  // Dragging a tall card from the top of a scrolled dashboard to a free cell
  // near the bottom needs the page to scroll while the pointer sits at the
  // edge. Without this, the target is reachable only by letting go and losing
  // the card's drag origin.
  const onScroll = () => {
    if (dragActive) updateFromPointer();
  };
  if (scroller) scroller.addEventListener('scroll', onScroll, { passive: true });
  else window.addEventListener('scroll', onScroll, { passive: true });
  const autoScroll = () => {
    if (!dragActive) return;
    const viewTop = scroller ? scroller.getBoundingClientRect().top : 0;
    const viewBottom = scroller ? scroller.getBoundingClientRect().bottom : window.innerHeight;
    const margin = 70;
    let delta = 0;
    if (lastPointer.y < viewTop + margin) {
      delta = -Math.max(6, Math.round((viewTop + margin - lastPointer.y) / 6));
    } else if (lastPointer.y > viewBottom - margin) {
      delta = Math.max(6, Math.round((lastPointer.y - (viewBottom - margin)) / 6));
    }
    if (delta !== 0) {
      if (scroller) scroller.scrollTop += delta;
      else window.scrollBy(0, delta);
      updateFromPointer();
    }
    autoScrollFrame = requestAnimationFrame(autoScroll);
  };
  autoScrollFrame = requestAnimationFrame(autoScroll);
  const finish = (cancelled) => {
    cleanup();
    if (cancelled || !moved) return;
    const source = dashLayout.grids[sourceName];
    const dest = dashLayout.grids[activeName];
    if (!source || !dest) return;
    delete source.cards[id];
    dest.cards[id] = dashClampCard({ ...cfg, x: target.x, y: target.y }, id);
    source.order = dashSortOrder(source.cards, Object.keys(source.cards));
    dest.order = dashSortOrder(dest.cards, Object.keys(dest.cards));
    markDashLayoutDirty();
    applyDashboardLayout(undefined, true);
  };
  const cleanup = () => {
    dragActive = false;
    cancelAnimationFrame(autoScrollFrame);
    if (scroller) scroller.removeEventListener('scroll', onScroll);
    else window.removeEventListener('scroll', onScroll);
    ghost.remove();
    placeholder.remove();
    card.classList.remove('dash-drag-origin');
    document.body.style.userSelect = '';
    document.removeEventListener('pointermove', move);
    document.removeEventListener('pointerup', onUp);
    document.removeEventListener('pointercancel', onCancel);
  };
  const onUp = () => finish(false);
  const onCancel = () => finish(true);
  document.addEventListener('pointermove', move, { passive: false });
  document.addEventListener('pointerup', onUp, { once: true });
  document.addEventListener('pointercancel', onCancel, { once: true });
}

function beginDashResize(e, card, grid, edge) {
  e.preventDefault();
  e.stopPropagation();
  const name = grid.dataset.dashGrid;
  const id = card.dataset.dashCard;
  const group = dashLayout.grids[name];
  const cfg = group && group.cards[id];
  if (!cfg) return;
  const startX = e.clientX;
  const startY = e.clientY;
  const startCfg = { ...cfg, id };
  let lastRect = { x: cfg.x, y: cfg.y, w: cfg.w, h: cfg.h };
  let changed = false;
  const move = (ev) => {
    const m = dashGridMetrics(grid);
    const dx = Math.round((ev.clientX - startX) / (m.colWidth + m.columnGap));
    const dy = Math.round((ev.clientY - startY) / (DASH_ROW_PX + m.rowGap));
    const candidate = dashResizeRect(startCfg, edge, dx, dy);
    // Free placement: stop at the last valid size instead of pushing other
    // cards around. The chosen height is never auto-grown; content scrolls.
    if (!dashCardFits(candidate, group.cards, id)) return;
    lastRect = candidate;
    changed = changed || candidate.x !== cfg.x || candidate.y !== cfg.y ||
      candidate.w !== cfg.w || candidate.h !== cfg.h;
    card.style.gridColumn = `${candidate.x + 1} / span ${candidate.w}`;
    card.style.gridRow = `${candidate.y + 1} / span ${candidate.h}`;
    resizeDashCharts();
  };
  const finish = () => {
    document.removeEventListener('pointermove', move);
    document.removeEventListener('pointerup', finish);
    document.removeEventListener('pointercancel', finish);
    card.style.zIndex = '';
    if (!group || !changed) return;
    group.cards[id] = { ...group.cards[id], ...lastRect };
    group.order = dashSortOrder(group.cards, Object.keys(group.cards));
    markDashLayoutDirty();
    applyDashboardLayout(undefined, true);
  };
  card.style.zIndex = '20';
  document.addEventListener('pointermove', move, { passive: false });
  document.addEventListener('pointerup', finish, { once: true });
  document.addEventListener('pointercancel', finish, { once: true });
}

function setDashEditing(on) {
  const section = document.getElementById('tab-dashboard');
  if (!section) return;
  dashEditing = on;
  clearDashHandles();
  if (on) {
    dashGridEls().forEach((grid) => {
      dashCardsOf(grid).forEach((card) => {
        const overlay = document.createElement('div');
        overlay.className = 'dash-edit-cover';
        overlay.setAttribute('aria-hidden', 'true');
        overlay.addEventListener('pointerdown', (e) => beginDashDrag(e, card, grid));
        card.appendChild(overlay);
        if (dashIsDesktop()) {
          const titles = {
            n: 'Drag to resize top edge',
            e: 'Drag to resize right edge',
            s: 'Drag to resize bottom edge',
            w: 'Drag to resize left edge',
            ne: 'Drag to resize top-right corner',
            nw: 'Drag to resize top-left corner',
            se: 'Drag to resize bottom-right corner',
            sw: 'Drag to resize bottom-left corner',
          };
          Object.entries(titles).forEach(([edge, title]) => {
            const handle = document.createElement('button');
            handle.type = 'button';
            handle.className = `dash-resize dash-resize-${edge}`;
            handle.title = title;
            handle.setAttribute('aria-label', title);
            handle.addEventListener('pointerdown', (e) => beginDashResize(e, card, grid, edge));
            card.appendChild(handle);
          });
        }
      });
    });
  }
  syncDashEditUi();
  if (!on) resizeDashCharts();
}

function resetDashboardLayout() {
  dashLayout = dashNormalizeLayout({}, dashOriginalGridIds());
  markDashLayoutDirty();
  applyDashboardLayout(undefined, true);
  showToast('Dashboard layout reset', 'success');
}

async function toggleDashboardEditing() {
  if (dashEditing) {
    if (!(await saveDashboardLayout(true))) return;
    setDashEditing(false);
    return;
  }
  if (activeDashTab !== 'dashboard') switchTab('dashboard');
  setDashEditing(true);
}

function initDashboardEditor() {
  dashOriginalIds = dashGridIds();
  [
    document.getElementById('dash-edit-btn'),
    document.getElementById('dash-edit-btn-mobile'),
  ].filter(Boolean).forEach((btn) => {
    btn.addEventListener('click', (e) => {
      e.preventDefault();
      e.stopPropagation();
      toggleDashboardEditing();
    });
  });
  [
    document.getElementById('dash-reset-btn'),
    document.getElementById('dash-reset-btn-sidebar'),
    document.getElementById('dash-reset-btn-mobile'),
  ].filter(Boolean).forEach((btn) => {
    btn.addEventListener('click', (e) => {
      e.preventDefault();
      e.stopPropagation();
      resetDashboardLayout();
    });
  });
  window.addEventListener('resize', () => {
    clearTimeout(initDashboardEditor._resizeTimer);
    initDashboardEditor._resizeTimer = setTimeout(() => {
      applyDashboardLayout();
      if (dashEditing) syncDashEditUi();
    }, 150);
  });
}

function renderConfig(cfg) {
  if (!cfg) return;
  if (cfg !== currentConfig) {
    transportDraft = null;
    transportInlineError = '';
  }
  currentConfig = cfg;

  // Normalize config structures to prevent runtime crashes on missing fields
  if (!cfg.rules || typeof cfg.rules !== 'object') cfg.rules = {};
  ['custom_proxied', 'custom_proxied_disabled', 'custom_blocked', 'custom_direct'].forEach(k => {
    if (!Array.isArray(cfg.rules[k])) cfg.rules[k] = [];
  });
  if (!cfg.rules.custom_records || typeof cfg.rules.custom_records !== 'object' || Array.isArray(cfg.rules.custom_records)) {
    cfg.rules.custom_records = {};
  }
  if (!cfg.access || typeof cfg.access !== 'object') cfg.access = {};
  if (!Array.isArray(cfg.access.doh_tokens)) cfg.access.doh_tokens = [];
  normalizeTransportSettings(cfg);

  // The subscription record may be absent on an older config response; an
  // empty object keeps the renderers below total.
  if (!cfg.subscription || typeof cfg.subscription !== 'object') cfg.subscription = {};

  // The settings panels read straight from the config they were handed.
  if (typeof window.renderSubscriptionSettings === 'function') window.renderSubscriptionSettings();
  if (typeof window.renderAdminPath === 'function') window.renderAdminPath();
  if (typeof window.renderTwoFactor === 'function') window.renderTwoFactor();

  // Version badge (single source of truth: version.json embedded in the binary).
  //
  // The mobile header deliberately gets the version *without* the commit hash. At 9px mono in
  // a 56px sticky header the hash suffix is the difference between a badge sitting beside the
  // wordmark and one that wraps, taking the brand line out through the top border. The hash
  // still reaches both places it is actually used from: the desktop sidebar badge, which has
  // room, and the footer. The badge text is version-only; the footer shows version + hash.
  if (cfg.version && cfg.version.display) {
    const versionOnly = cfg.version.display;
    const versionWithHash = `${cfg.version.display} [${cfg.version.hash}]`;

    const footer = document.getElementById('app-version-footer');
    if (footer) footer.innerText = versionWithHash;

    const aboutVersion = document.getElementById('about-version');
    if (aboutVersion) aboutVersion.innerText = versionOnly;

    const mobileBadge = document.getElementById('app-version-badge-mobile');
    if (mobileBadge) {
      mobileBadge.innerText = versionOnly;
      mobileBadge.title = versionWithHash;
    }
  }

  // Once the running version is known, ask the server whether a newer one
  // is published on the project's main branch. Runs once per page load and is
  // non-fatal when the box is offline.
  if (!window.__hdnsUpdateChecked) {
    window.__hdnsUpdateChecked = true;
    checkForUpdate();
  }

  // Header & guide public IP
  const pubIP = cfg.server.public_ip || '127.0.0.1';
  const headerIPEl = document.getElementById('header-public-ip');
  if (headerIPEl) headerIPEl.innerText = pubIP;
  // The country rides the same response, so the sidebar flag costs no request.
  renderServerLocation(cfg.server.country);
  if (!geoipStatusLoaded) {
    geoipStatusLoaded = true;
    loadGeoipStatus();
  }

  // Administrator Credentials card. The password verifier is deliberately not in
  // this payload any more, so the username is all there is to show.
  const adminNameEl = document.getElementById('current-admin-name');
  if (adminNameEl) adminNameEl.innerText = cfg.server.admin_username || 'admin';
  const guideWinEl = document.getElementById('guide-win-ip');
  if (guideWinEl) guideWinEl.innerText = pubIP;
  const guideConsoleEl = document.getElementById('guide-console-ip');
  if (guideConsoleEl) guideConsoleEl.innerText = pubIP;
  const guideDohEl = document.getElementById('guide-doh-url');
  if (guideDohEl) {
    // The daemon's own URL builder: domain over IP, https scheme, the DoH
    // listener's own port. The old line guessed http://IP:web_port and got
    // the scheme, the port and the host wrong in one string.
    const doh = (typeof cfg.doh_url === 'string' && cfg.doh_url)
      ? cfg.doh_url
      : `http://${pubIP}:${(cfg.dns && cfg.dns.doh_port) || 8443}/dns-query`;
    guideDohEl.innerText = doh;
    guideDohEl.title = doh;
  }
  const guideDoH3Note = document.getElementById('guide-doh3-note');
  if (guideDoH3Note) {
    // DoH3 shares the DoH port and is always on.
    guideDoH3Note.classList.remove('hidden');
  }
  const guideDoQCard = document.getElementById('guide-doq-card');
  if (guideDoQCard) {
    guideDoQCard.classList.toggle('hidden', !(cfg.transports && cfg.transports.doq_enabled));
  }
  const guideDoQEl = document.getElementById('guide-doq-url');
  if (guideDoQEl) {
    const port = (cfg.transports && cfg.transports.doq_port) || 853;
    const host = (cfg.tls && (cfg.tls.dot_domain || cfg.tls.domain)) || pubIP || '';
    guideDoQEl.innerText = cfg.doq_url || (host ? `quic://${host}:${port}` : 'quic://…');
    guideDoQEl.title = guideDoQEl.innerText;
  }

  // The guide's DoT hostname prefers the dedicated DoH/DoT domain when one
  // is set — it is the only name the 853 certificate is guaranteed to cover.
  const dotHost = (cfg.tls && cfg.tls.dot_domain) || (cfg.tls && cfg.tls.domain) || '';
  if (dotHost) {
    const guideDotEl = document.getElementById('guide-dot-hostname');
    if (guideDotEl) guideDotEl.innerText = dotHost;
  }
  if (cfg.tls && cfg.tls.domain) {
    const sslDomInput = document.getElementById('ssl-domain-input');
    if (sslDomInput) sslDomInput.value = cfg.tls.domain;
  }
  if (cfg.tls) {
    const dotDomainInput = document.getElementById('dot-domain-input');
    if (dotDomainInput) dotDomainInput.value = cfg.tls.dot_domain || '';
  }

  // The registration contact is prefilled for the same reason the domain is, and not
  // only for convenience: /api/tls/issue stores both fields from one request, so an
  // operator who fixes a typo in the domain while this box sits empty sends an empty
  // email, and the stored contact — the address Let's Encrypt sends expiry warnings to
  // — is overwritten with nothing.
  if (cfg.tls && cfg.tls.email) {
    const sslEmailInput = document.getElementById('ssl-email-input');
    if (sslEmailInput) sslEmailInput.value = cfg.tls.email;
  }

  renderTLSCertDetails(cfg);
  renderTransportSettings();

  // Render API Key & Bind
  renderAPIKeyDisplay();

  const apiBind = (cfg.server && cfg.server.api_bind) ? cfg.server.api_bind : '127.0.0.1';
  // v3.1.0: the multi-CIDR list outranks the single api_bind. When it is
  // non-empty it is what the REST gate enforces, so the page must reflect it.
  const apiBindList = (cfg.server && Array.isArray(cfg.server.api_bind_list)) ? cfg.server.api_bind_list : [];
  const hasList = apiBindList.length > 0;
  // Phase D: reflect the live idle window (the accessor clamps; 0 here means
  // the default 15 the server applies).
  const idleEl = document.getElementById('session-idle-minutes');
  if (idleEl) {
    const live = Number(cfg.server?.session_idle_minutes);
    idleEl.value = Number.isFinite(live) && live > 0 ? live : 15;
  }
  renderProxyIPv6Controls();
  const apiBindBadge = document.getElementById('api-bind-badge');

  if (apiBindBadge) {
    if (hasList) {
      apiBindBadge.innerText = apiBindList.length + ' rule' + (apiBindList.length === 1 ? '' : 's') + ' (Restricted)';
      apiBindBadge.className = 'px-2.5 py-1 rounded-full text-[10px] font-mono font-bold bg-amber-500/20 text-amber-300 border border-amber-500/30';
    } else if (apiBind === '0.0.0.0') {
      apiBindBadge.innerText = '0.0.0.0 (Public HTTPS)';
      apiBindBadge.className = 'px-2.5 py-1 rounded-full text-[10px] font-mono font-bold bg-cyan-500/20 text-cyan-300 border border-cyan-500/30';
    } else if (apiBind === '127.0.0.1') {
      apiBindBadge.innerText = '127.0.0.1 (Localhost Only)';
      apiBindBadge.className = 'px-2.5 py-1 rounded-full text-[10px] font-mono font-bold bg-emerald-500/20 text-emerald-300 border border-emerald-500/30';
    } else {
      apiBindBadge.innerText = apiBind + ' (Restricted)';
      apiBindBadge.className = 'px-2.5 py-1 rounded-full text-[10px] font-mono font-bold bg-amber-500/20 text-amber-300 border border-amber-500/30';
    }
  }
  // Reflect the effective rule into the selector. A non-empty list selects the
  // "Custom" button and fills the textarea; otherwise the single value selects
  // one of the two sentinels, or — for a lone custom IP/CIDR — the Custom button
  // with that one value in the textarea.
  document.querySelectorAll('.api-bind-opt').forEach(btn => {
    const bind = btn.getAttribute('data-bind');
    const isActive = hasList
      ? bind === '__custom__'
      : (bind === apiBind) ||
        (bind === '__custom__' && apiBind !== '127.0.0.1' && apiBind !== '0.0.0.0');
    btn.classList.toggle('is-active', isActive);
  });
  const customWrap = document.getElementById('api-bind-custom-wrap');
  const customInput = document.getElementById('api-bind-custom-input');
  const customList = document.getElementById('api-bind-list-input');
  if (customWrap && customList) {
    const isCustom = hasList || (apiBind !== '127.0.0.1' && apiBind !== '0.0.0.0');
    customWrap.classList.toggle('hidden', !isCustom);
    // The textarea always shows the list in force. A lone custom single value is
    // seeded into it so "Convert this to a list" starts from what is there.
    customList.value = hasList ? apiBindList.join('\n') : (isCustom ? apiBind : '');
    if (customInput) customInput.value = '';
  }

  updateCodeSnippets(pubIP, cfg.server?.api_key || 'digitdns_live_your_key_here');

  // Render Custom Rules Lists (with safe optional chaining)
  renderProxiedSuggestions();
  refreshProxiedListIfVisible();
  proxiedFillViewport();
  renderList('custom-blocked-list', cfg.rules?.custom_blocked || [], 'remove-blocked');
  renderList('tokens-list', cfg.access?.doh_tokens || [], 'remove-token');
  renderCustomRecords(cfg.rules?.custom_records || {});
  loadCustomGroups();
  if (!dashEditing) applyDashboardLayout(cfg.dashboard_layout || {});
}

// A domain, optionally wildcarded, as the matcher accepts it: labels of [a-z0-9-]
// joined by dots, at least two labels, and an optional leading "*." to cover a whole
// subtree. Deliberately not anchored to a TLD list — the matcher does not consult one
// either, and a country or brand TLD would only be rejected here for looking unusual.
const DOMAIN_RE = /^(\*\.)?([a-z0-9](?:[a-z0-9-]*[a-z0-9])?)(?:\.([a-z0-9](?:[a-z0-9-]*[a-z0-9])?))+$/;

// parseDomainList turns a free-form blob into the domains worth keeping. Newlines,
// commas, semicolons and any run of spaces are separators; a "#" starts a comment that
// runs to the end of its line. Each token is lowercased, a trailing root dot is
// stripped, and the survivors are sorted into three buckets — ready to add, already
// present (in the list or earlier in the blob), or malformed. Nothing here touches the
// DOM; the caller merges `accepted` and reports the other two.
function parseDomainList(text, existing) {
  const have = new Set(existing || []);
  const seen = new Set();
  const accepted = [];
  const duplicates = [];
  const invalid = [];
  const tokens = String(text || '')
    .split(/\r?\n/)
    .map((line) => line.split('#')[0])
    .join('\n')
    .split(/[\s,;]+/);
  for (const raw of tokens) {
    let d = raw.trim().toLowerCase();
    if (!d) continue;
    if (d.endsWith('.')) d = d.slice(0, -1);
    if (!d) continue;
    if (!DOMAIN_RE.test(d)) { invalid.push(d); continue; }
    if (have.has(d) || seen.has(d)) { duplicates.push(d); continue; }
    seen.add(d);
    accepted.push(d);
  }
  return { accepted, duplicates, invalid };
}

// RECORD_DOMAIN_RE is DOMAIN_RE without the "*.", because a custom static record is
// an exact-hostname A/AAAA answer: the matcher resolves it by exact key lookup, so a
// wildcard record would be stored and then never answer anything.
const RECORD_DOMAIN_RE = /^([a-z0-9](?:[a-z0-9-]*[a-z0-9])?)(?:\.([a-z0-9](?:[a-z0-9-]*[a-z0-9])?))+$/;

// isIPAddress accepts the two literals a static record may point at: a dotted-quad
// IPv4 (each octet 0-255, no leading zeros) or an IPv6 address. It is deliberately
// strict on IPv4 and structural on IPv6 — the browser cannot parse an address for us,
// and the server stores the string verbatim, so a typo must be caught here.
function isIPAddress(value) {
  const s = String(value || '').trim();
  if (!s) return false;
  if (/^\d{1,3}(\.\d{1,3}){3}$/.test(s)) {
    return s.split('.').every((part) => {
      const n = Number(part);
      return String(n) === part && n <= 255;
    });
  }
  return /^(([0-9a-f]{1,4}:){7}[0-9a-f]{1,4}|([0-9a-f]{1,4}:){1,7}:|([0-9a-f]{1,4}:){1,6}:[0-9a-f]{1,4}|([0-9a-f]{1,4}:){1,5}(:[0-9a-f]{1,4}){1,2}|([0-9a-f]{1,4}:){1,4}(:[0-9a-f]{1,4}){1,3}|([0-9a-f]{1,4}:){1,3}(:[0-9a-f]{1,4}){1,4}|([0-9a-f]{1,4}:){1,2}(:[0-9a-f]{1,4}){1,5}|[0-9a-f]{1,4}:((:[0-9a-f]{1,4}){1,6})|:((:[0-9a-f]{1,4}){1,7}|:))$/i.test(s);
}

// parseRecordList turns a free-form blob into the domain -> IP mappings the Custom
// Static Records panel stores. Each line carries one domain and one address, split by
// whitespace, a comma, a semicolon or an "=". A "#" starts a comment. The domain is
// lowercased and de-dotted; the address must be an IPv4 or IPv6 literal. Records
// already present are collected as `updated` (the address changes) or `unchanged` (the
// same address), so re-uploading a corrected table replaces stale addresses instead of
// piling duplicates on top. Nothing here touches the DOM.
function parseRecordList(text, existing) {
  const have = existing || {};
  const seen = new Set();
  const accepted = [];
  const updated = [];
  const unchanged = [];
  const invalid = [];
  for (const raw of String(text || '').split(/\r?\n/)) {
    const line = raw.split('#')[0].trim();
    if (!line) continue;
    const parts = line.split(/[\s,;=]+/).filter(Boolean);
    if (parts.length !== 2) { invalid.push(line); continue; }
    let d = parts[0].trim().toLowerCase();
    if (d.endsWith('.')) d = d.slice(0, -1);
    const ip = parts[1].trim();
    if (!RECORD_DOMAIN_RE.test(d) || !isIPAddress(ip) || seen.has(d)) { invalid.push(line); continue; }
    seen.add(d);
    if (Object.prototype.hasOwnProperty.call(have, d)) {
      if (have[d] === ip) unchanged.push(d);
      else updated.push({ domain: d, ip });
    } else {
      accepted.push({ domain: d, ip });
    }
  }
  return { accepted, updated, unchanged, invalid };
}

function renderList(containerId, items, removeClass) {
  const container = document.getElementById(containerId);
  if (!container) return;
  container.innerHTML = '';
  if (!items || items.length === 0) {
    container.innerHTML = '<div class="text-slate-500 text-xs py-1">No custom entries yet</div>';
    return;
  }
  items.forEach(item => {
    const row = document.createElement('div');
    row.className = 'flex items-center justify-between py-1.5 px-3 rounded-lg bg-slate-950/60 border border-slate-800 text-xs';
    // The list entries are operator-supplied strings that arrive back from the
    // settings API, and the row is built with innerHTML — so they are escaped like
    // every other stored value the dashboard renders. data-val survives escaping
    // unchanged: the parser decodes the entities before dataset reads it.
    row.innerHTML = `
      <span class="font-mono text-cyan-300">${escapeHTML(item)}</span>
      <button class="${removeClass} inline-flex items-center justify-center w-6 h-6 shrink-0 rounded text-slate-500 hover:text-red-400 hover:bg-red-500/10 transition" data-val="${escapeHTML(item)}" aria-label="Remove ${escapeHTML(item)}">
        <i data-feather="x" class="w-3.5 h-3.5"></i>
      </button>
    `;
    container.appendChild(row);
  });
  safeFeatherReplace();
}

// ---- Client-facing QUIC transports -----------------------------------------
//
// Every transport in this card saves as it changes. Switches apply at once;
// text fields apply on blur; the SNI-token and mTLS modes stay staged until
// their required fields are present, then apply automatically. The applied
// state lives in currentConfig.transports; transportDraft holds the in-progress
// form so a DoH3 auto-save cannot commit it by accident.
let transportDraft = null;
let transportSaving = false;
let transportInlineError = '';
// Global notice state. The upstream health set is refreshed from /api/stats; the
// signature lets the banner repaint only when the set actually changes.
let lastGlobalNoticeSignature = '';

function normalizeTransportSettings(cfg) {
  if (!cfg) return { plain_dns_enabled: true, plain_dns_port: 53, plain_dns_listening: false, doh_enabled: true, doh_port: 8443, doh_listening: false, doq_enabled: false, doh3_enabled: true, doq_port: 853, doq_auth: 'ip', doq_token_domain: '', doq_tokens: [], doh3_listening: false, doq_listening: false, dot_enabled: true, dot_listening: false, dot_auth: 'ip', dot_port: 853, dot_token_domain: '', dot_tokens: [], dnscrypt_enabled: false, dnscrypt_port: 5443, dnscrypt_provider_name: '2.dnscrypt-cert.digitdns.local', dnscrypt_stamp: '', dnscrypt_cert_not_after: 0, dnscrypt_listening: false, mtls_enabled: false, mtls_ca_pem: '', ip_allowlist: { allow_all: false, client_accounts: 0, client_entries: 0, resolver_entries: 0 }, last_error: '' };
  if (!cfg.transports || typeof cfg.transports !== 'object') cfg.transports = {};
  const t = cfg.transports;
  t.doq_enabled = !!t.doq_enabled;
  // DoH3 shares the DoH port but has its own switch.
  t.doh3_enabled = !!t.doh3_enabled;
  t.doq_port = Number.isFinite(Number(t.doq_port)) && Number(t.doq_port) > 0 ? Number(t.doq_port) : 853;
  t.doq_auth = t.doq_auth === 'sni-token' ? 'sni-token' : 'ip';
  t.doq_token_domain = typeof t.doq_token_domain === 'string' ? t.doq_token_domain : '';
  if (!Array.isArray(t.doq_tokens)) t.doq_tokens = [];
  t.doh3_listening = !!t.doh3_listening;
  t.doq_listening = !!t.doq_listening;
  t.doh3_port = Number.isFinite(Number(t.doh3_port)) && Number(t.doh3_port) > 0 ? Number(t.doh3_port) : 8443;
  t.dot_auth = t.dot_auth === 'sni-token' ? 'sni-token' : 'ip';
  t.dot_port = Number.isFinite(Number(t.dot_port)) && Number(t.dot_port) > 0 ? Number(t.dot_port) : 853;
  t.dot_listening = !!t.dot_listening;
  t.dot_enabled = t.dot_enabled !== false;
  t.dot_port = Number.isFinite(Number(t.dot_port)) && Number(t.dot_port) > 0 ? Number(t.dot_port) : 853;
  t.dot_token_domain = typeof t.dot_token_domain === 'string' ? t.dot_token_domain : '';
  if (!Array.isArray(t.dot_tokens)) t.dot_tokens = [];
  t.dnscrypt_enabled = !!t.dnscrypt_enabled;
  t.dnscrypt_port = Number.isFinite(Number(t.dnscrypt_port)) && Number(t.dnscrypt_port) > 0 ? Number(t.dnscrypt_port) : 5443;
  t.dnscrypt_provider_name = typeof t.dnscrypt_provider_name === 'string' ? t.dnscrypt_provider_name : '';
  t.dnscrypt_stamp = typeof t.dnscrypt_stamp === 'string' ? t.dnscrypt_stamp : '';
  t.dnscrypt_cert_not_after = Number(t.dnscrypt_cert_not_after) || 0;
  t.dnscrypt_listening = !!t.dnscrypt_listening;
  t.plain_dns_enabled = t.plain_dns_enabled !== false;
  t.plain_dns_port = Number.isFinite(Number(t.plain_dns_port)) && Number(t.plain_dns_port) > 0 ? Number(t.plain_dns_port) : 53;
  t.plain_dns_listening = !!t.plain_dns_listening;
  t.doh_enabled = t.doh_enabled !== false;
  t.doh_port = Number.isFinite(Number(t.doh_port)) && Number(t.doh_port) > 0 ? Number(t.doh_port) : (Number(t.doh3_port) || 8443);
  t.doh_listening = !!t.doh_listening;
  t.mtls_enabled = !!t.mtls_enabled;
  t.mtls_ca_pem = typeof t.mtls_ca_pem === 'string' ? t.mtls_ca_pem : '';
  const al = (t.ip_allowlist && typeof t.ip_allowlist === 'object') ? t.ip_allowlist : {};
  t.ip_allowlist = {
    allow_all: !!al.allow_all,
    client_accounts: Number(al.client_accounts) || 0,
    client_entries: Number(al.client_entries) || 0,
    resolver_entries: Number(al.resolver_entries) || 0,
  };
  t.last_error = typeof t.last_error === 'string' ? t.last_error : '';
  return t;
}

function copyTransportDraft(t) {
  return {
    doh3_enabled: !!t.doh3_enabled,
    doq_enabled: !!t.doq_enabled,
    doq_port: t.doq_port || 853,
    doq_auth: t.doq_auth === 'sni-token' ? 'sni-token' : 'ip',
    doq_token_domain: t.doq_token_domain || '',
    doq_tokens: (t.doq_tokens || []).slice(),
    dot_enabled: t.dot_enabled !== false,
    dot_port: t.dot_port || 853,
    dot_auth: t.dot_auth === 'sni-token' ? 'sni-token' : 'ip',
    dot_token_domain: t.dot_token_domain || '',
    dot_tokens: (t.dot_tokens || []).slice(),
    dnscrypt_enabled: !!t.dnscrypt_enabled,
    dnscrypt_port: t.dnscrypt_port || 5443,
    dnscrypt_provider_name: t.dnscrypt_provider_name || '',
    plain_dns_enabled: t.plain_dns_enabled !== false,
    plain_dns_port: t.plain_dns_port || 53,
    doh_enabled: t.doh_enabled !== false,
    doh_port: t.doh_port || 8443,
    mtls_enabled: !!t.mtls_enabled,
    mtls_ca_pem: t.mtls_ca_pem || '',
  };
}

function ensureTransportDraft(applied) {
  if (!transportDraft) transportDraft = copyTransportDraft(applied);
  return transportDraft;
}

function withTransportDraft(mutate) {
  const draft = ensureTransportDraft(normalizeTransportSettings(currentConfig));
  mutate(draft);
  renderTransportSettings();
  void autoSaveTransportDraft();
}

// transportDraftProblem returns the reason the staged form cannot be applied
// yet; empty means it is safe to send. These are the same rules the server
// enforces, kept here so an incomplete SNI-token or mTLS edit stays local
// instead of bouncing off a 400.
function transportDraftProblem(draft) {
  const plainPort = Number(draft.plain_dns_port);
  if (!Number.isInteger(plainPort) || plainPort < 1 || plainPort > 65535) {
    return 'Plain DNS port must be between 1 and 65535.';
  }
  const dohPort = Number(draft.doh_port);
  if (!Number.isInteger(dohPort) || dohPort < 1 || dohPort > 65535) {
    return 'DoH port must be between 1 and 65535.';
  }
  const dotPort = Number(draft.dot_port);
  if (!Number.isInteger(dotPort) || dotPort < 1 || dotPort > 65535) {
    return 'DoT port must be between 1 and 65535.';
  }
  const doqPort = Number(draft.doq_port);
  if (!Number.isInteger(doqPort) || doqPort < 1 || doqPort > 65535) {
    return 'DoQ port must be between 1 and 65535.';
  }
  const dnscryptPort = Number(draft.dnscrypt_port);
  if (!Number.isInteger(dnscryptPort) || dnscryptPort < 1 || dnscryptPort > 65535) {
    return 'DNSCrypt port must be between 1 and 65535.';
  }
  if (draft.doq_enabled && draft.doq_auth === 'sni-token') {
    if (!draft.doq_token_domain) return 'DoQ SNI-token mode needs a token domain.';
    if (!draft.doq_tokens.length) return 'DoQ SNI-token mode needs at least one token.';
  }
  if (draft.dot_enabled && draft.dot_auth === 'sni-token') {
    if (!draft.dot_token_domain) return 'DoT SNI-token mode needs a token domain.';
    if (!draft.dot_tokens.length) return 'DoT SNI-token mode needs at least one token.';
  }
  if (draft.mtls_enabled && !draft.mtls_ca_pem.trim()) {
    return 'Paste the client CA bundle to enable mTLS.';
  }
  return '';
}

function transportSaveBody(draft) {
  return {
    plain_dns_enabled: !!draft.plain_dns_enabled,
    plain_dns_port: Number(draft.plain_dns_port),
    doh_enabled: !!draft.doh_enabled,
    doh_port: Number(draft.doh_port),
    dot_port: Number(draft.dot_port),
    doh3_enabled: !!draft.doh3_enabled,
    doq_enabled: !!draft.doq_enabled,
    doq_port: Number(draft.doq_port),
    doq_auth: draft.doq_auth,
    doq_token_domain: draft.doq_token_domain,
    doq_tokens: draft.doq_tokens,
    dnscrypt_enabled: !!draft.dnscrypt_enabled,
    dnscrypt_port: Number(draft.dnscrypt_port),
    dnscrypt_provider_name: draft.dnscrypt_provider_name,
    dot_enabled: !!draft.dot_enabled,
    dot_auth: draft.dot_auth,
    dot_token_domain: draft.dot_token_domain,
    dot_tokens: draft.dot_tokens,
    mtls_enabled: !!draft.mtls_enabled,
    mtls_ca_pem: draft.mtls_ca_pem,
  };
}

function transportDraftDirty(applied, draft) {
  return draft.doh3_enabled !== applied.doh3_enabled ||
    draft.doq_enabled !== applied.doq_enabled ||
    Number(draft.doq_port) !== Number(applied.doq_port) ||
    draft.doq_auth !== applied.doq_auth ||
    draft.doq_token_domain !== applied.doq_token_domain ||
    JSON.stringify(draft.doq_tokens) !== JSON.stringify(applied.doq_tokens) ||
    draft.dot_enabled !== applied.dot_enabled ||
    Number(draft.dot_port) !== Number(applied.dot_port) ||
    draft.dot_auth !== applied.dot_auth ||
    draft.dot_token_domain !== applied.dot_token_domain ||
    JSON.stringify(draft.dot_tokens) !== JSON.stringify(applied.dot_tokens) ||
    draft.dnscrypt_enabled !== applied.dnscrypt_enabled ||
    Number(draft.dnscrypt_port) !== Number(applied.dnscrypt_port) ||
    draft.dnscrypt_provider_name !== applied.dnscrypt_provider_name ||
    draft.plain_dns_enabled !== applied.plain_dns_enabled ||
    Number(draft.plain_dns_port) !== Number(applied.plain_dns_port) ||
    draft.doh_enabled !== applied.doh_enabled ||
    Number(draft.doh_port) !== Number(applied.doh_port) ||
    draft.mtls_enabled !== applied.mtls_enabled ||
    draft.mtls_ca_pem !== applied.mtls_ca_pem;
}

function transportGuideHost() {
  const cfg = currentConfig || {};
  // Plain DNS is addressed by IP: an operator can paste it into any client
  // without DNS. Domain names remain the fallback only when no address exists.
  return (cfg.server && cfg.server.public_ip) ||
    (cfg.tls && (cfg.tls.dot_domain || cfg.tls.domain)) || '';
}

function transportCertHost() {
  const cfg = currentConfig || {};
  // DoT/DoH/DoH3/DoQ validate the certificate against the SNI, so those cards
  // advertise the certificate's domain, not the public IP.
  return (cfg.tls && (cfg.tls.dot_domain || cfg.tls.domain)) || transportGuideHost();
}

function setTransportText(el, text) {
  if (!el) return;
  if (el.textContent !== text) el.textContent = text;
}

function renderTransportEndpoints(applied, draft) {
  const host = transportGuideHost();
  const cfg = currentConfig || {};
  const missingHost = 'Configure a domain and certificate first';

  const plainPort = draft.plain_dns_port || 53;
  const plainEl = document.getElementById('plain-endpoint');
  setTransportText(plainEl, host ? `${host}:${plainPort}` : missingHost);

  const dotPort = draft.dot_port || 853;
  const dotEl = document.getElementById('dot-endpoint');
  if (dotEl) {
    // The DoT certificate is issued for the DoH/DoT domain, and Android
    // Private DNS validates it against the SNI. Show that name, not the IP;
    // fall back to the address only when no domain is configured.
    const dotBase = (cfg.tls && (cfg.tls.dot_domain || cfg.tls.domain)) || host;
    const dotName = draft.dot_auth === 'sni-token' && draft.dot_token_domain
      ? `<token>.${draft.dot_token_domain}`
      : (dotBase || missingHost);
    setTransportText(dotEl, dotBase ? `tls://${dotName}:${dotPort}` : missingHost);
  }

  const dohPort = draft.doh_port || 8443;
  const certHost = transportCertHost();
  const dohEl = document.getElementById('doh-endpoint');
  if (!certHost) {
    setTransportText(dohEl, missingHost);
  } else {
    const authority = dohPort === 443 ? certHost : `${certHost}:${dohPort}`;
    setTransportText(dohEl, `https://${authority}/dns-query`);
  }

  const doqPort = draft.doq_port || 853;
  const doqEl = document.getElementById('doq-endpoint');
  setTransportText(doqEl, (draft.doq_auth === 'sni-token' && draft.doq_token_domain)
    ? `quic://<token>.${draft.doq_token_domain}:${doqPort}`
    : (certHost ? `quic://${certHost}:${doqPort}` : missingHost));

  const doh3PortEl = document.getElementById('doh3-port-input');
  if (doh3PortEl && doh3PortEl.value !== String(dohPort)) doh3PortEl.value = dohPort;
  const doh3El = document.getElementById('doh3-endpoint');
  if (!certHost) {
    setTransportText(doh3El, missingHost);
  } else {
    const authority = dohPort === 443 ? certHost : `${certHost}:${dohPort}`;
    setTransportText(doh3El, `h3://${authority}/dns-query`);
  }

  const statusClass = (tone) => `transport-state font-mono text-[10px] ${tone}`;
  // Only change a status label when its text/tone actually changes. A blanket
  // rewrite on every render makes all six cards flash while one switch saves.
  const writeStatus = (el, text, tone) => {
    if (!el) return;
    if (el.innerText !== text) el.innerText = text;
    const cls = statusClass(tone);
    if (el.className !== cls) el.className = cls;
  };
  const setSimpleStatus = (id, enabled, listening, portText) => {
    const el = document.getElementById(id);
    if (!el) return;
    if (!enabled) {
      writeStatus(el, 'Off', 'text-slate-500');
    } else if (listening) {
      writeStatus(el, `Listening · ${portText}`, 'text-emerald-300');
    } else {
      writeStatus(el, 'Enabled — not listening', 'text-amber-300');
    }
  };

  setSimpleStatus('plain-status', draft.plain_dns_enabled, applied.plain_dns_listening, `UDP+TCP ${applied.plain_dns_port}`);
  setSimpleStatus('doh-status', draft.doh_enabled, applied.doh_listening, `TCP ${applied.doh_port}`);
  setSimpleStatus('doh3-status', true, applied.doh3_listening, `UDP ${applied.doh_port}`);

  const dotStatus = document.getElementById('dot-status');
  if (dotStatus) {
    const dotNeedsInput = draft.dot_enabled && draft.dot_auth === 'sni-token' &&
      (!draft.dot_token_domain || !draft.dot_tokens.length);
    if (!draft.dot_enabled) {
      writeStatus(dotStatus, 'Off', 'text-slate-500');
    } else if (dotNeedsInput) {
      writeStatus(dotStatus, 'Needs input', 'text-amber-300');
    } else if (applied.dot_listening) {
      writeStatus(dotStatus, `Listening · TCP ${applied.dot_port}`, 'text-emerald-300');
    } else {
      writeStatus(dotStatus, 'Enabled — not listening', 'text-amber-300');
    }
  }

  const doqStatus = document.getElementById('doq-status');
  if (doqStatus) {
    const doqNeedsInput = draft.doq_enabled && draft.doq_auth === 'sni-token' &&
      (!draft.doq_token_domain || !draft.doq_tokens.length);
    if (!draft.doq_enabled) {
      writeStatus(doqStatus, 'Off', 'text-slate-500');
    } else if (doqNeedsInput) {
      writeStatus(doqStatus, 'Needs input', 'text-amber-300');
    } else if (applied.doq_listening) {
      writeStatus(doqStatus, `Listening · UDP ${applied.doq_port}`, 'text-emerald-300');
    } else {
      writeStatus(doqStatus, 'Enabled — not listening', 'text-amber-300');
    }
  }

  const dnscryptStatus = document.getElementById('dnscrypt-status');
  if (dnscryptStatus) {
    if (!draft.dnscrypt_enabled) {
      writeStatus(dnscryptStatus, 'Off', 'text-slate-500');
    } else if (applied.dnscrypt_listening) {
      writeStatus(dnscryptStatus, `Listening · UDP+TCP ${applied.dnscrypt_port}`, 'text-emerald-300');
    } else {
      writeStatus(dnscryptStatus, 'Enabled — not listening', 'text-amber-300');
    }
  }

  const mtlsStatus = document.getElementById('mtls-status');
  if (mtlsStatus) {
    if (draft.mtls_enabled) {
      writeStatus(mtlsStatus, 'Enforced', 'text-emerald-300');
    } else {
      writeStatus(mtlsStatus, 'Off', 'text-slate-500');
    }
  }

  const stampEl = document.getElementById('dnscrypt-stamp');
  if (stampEl) {
    const stamp = applied.dnscrypt_stamp || '';
    setTransportText(stampEl, stamp || 'Enable DNSCrypt to generate a stamp');
    // The full stamp is long; the box truncates it, so expose it on hover and
    // leave textContent intact for the copy button.
    if (stampEl.title !== (stamp || '')) stampEl.title = stamp || '';
  }
  const expiryEl = document.getElementById('dnscrypt-cert-expiry');
  if (expiryEl) {
    setTransportText(expiryEl, applied.dnscrypt_cert_not_after
      ? `expires ${new Date(applied.dnscrypt_cert_not_after * 1000).toLocaleDateString()}`
      : '');
  }
}

function setTransportBadge(el, text, className, title, visible) {
  if (!el) return;
  // Compare with textContent: innerText reflects CSS text-transform, so the
  // uppercase badge always looked "different" and was rewritten on every
  // render, which made the transport names flash.
  if (el.textContent !== text) el.textContent = text;
  if (el.className !== className) el.className = className;
  if (el.title !== title) el.title = title;
  el.classList.toggle('hidden', !visible);
}

function updateTransportBadge(applied) {
  const badge = document.getElementById('transports-state-badge');
  if (!badge) return;
  const states = [
    { on: applied.plain_dns_enabled && applied.plain_dns_listening, name: 'Plain DNS' },
    { on: applied.dot_enabled && applied.dot_listening, name: 'DoT' },
    { on: applied.doh_enabled && applied.doh_listening, name: 'DoH' },
    { on: applied.doq_enabled && applied.doq_listening, name: 'DoQ' },
    { on: applied.doh3_listening, name: 'DoH3' },
    { on: applied.dnscrypt_enabled && applied.dnscrypt_listening, name: 'DNSCrypt' },
  ];
  const running = states.filter(s => s.on).map(s => s.name);
  const missing = states.filter(s => !s.on).map(s => s.name);
  const offBadge = document.getElementById('transports-off-badge');
  const failed = !!(transportInlineError || applied.last_error);

  // Deliberately no "Saving…" replacement: the badges keep the last applied
  // names until the response lands. Otherwise every autosave erased the names
  // and repainted them, which read as the transport text flickering.
  if (running.length > 0) {
    const text = running.length === states.length ? 'All Transports On' : `On: ${running.join(', ')}`;
    setTransportBadge(
      badge,
      text,
      'badge badge-direct',
      failed ? 'A transport change failed; see the error below.' : `Transports on: ${running.join(', ')}`,
      true,
    );
  } else {
    badge.classList.add('hidden');
  }

  if (missing.length > 0) {
    setTransportBadge(
      offBadge,
      `Off: ${missing.join(', ')}`,
      'badge badge-cached',
      `Transports off: ${missing.join(', ')}`,
      true,
    );
  } else if (offBadge) {
    offBadge.classList.add('hidden');
  }
}

function renderDoQTokens(draft) {
  const list = document.getElementById('doq-tokens-list');
  if (!list) return;
  // Rebuilding an unchanged list makes its text flash on every autosave.
  const signature = draft.doq_tokens.join('|');
  if (list.dataset.signature === signature) return;
  list.dataset.signature = signature;
  list.innerHTML = '';
  if (!draft.doq_tokens.length) {
    list.innerHTML = '<div class="text-slate-500 text-xs py-1">No DoQ tokens yet</div>';
    return;
  }
  draft.doq_tokens.forEach(token => {
    const row = document.createElement('div');
    row.className = 'flex items-center justify-between py-1.5 px-3 rounded-lg bg-slate-950/60 border border-slate-800 text-xs';
    row.innerHTML = `
      <span class="font-mono text-emerald-300">${escapeHTML(token)}</span>
      <button class="remove-doq-token inline-flex items-center justify-center w-6 h-6 shrink-0 rounded text-slate-500 hover:text-red-400 hover:bg-red-500/10 transition" data-val="${escapeHTML(token)}" aria-label="Remove ${escapeHTML(token)}">
        <i data-feather="x" class="w-3.5 h-3.5"></i>
      </button>
    `;
    list.appendChild(row);
  });
  safeFeatherReplace();
}

function renderDoTTokens(draft) {
  const list = document.getElementById('dot-tokens-list');
  if (!list) return;
  // Rebuilding an unchanged list makes its text flash on every autosave.
  const signature = draft.dot_tokens.join('|');
  if (list.dataset.signature === signature) return;
  list.dataset.signature = signature;
  list.innerHTML = '';
  if (!draft.dot_tokens.length) {
    list.innerHTML = '<div class="text-slate-500 text-xs py-1">No DoT tokens yet</div>';
    return;
  }
  draft.dot_tokens.forEach(token => {
    const row = document.createElement('div');
    row.className = 'flex items-center justify-between py-1.5 px-3 rounded-lg bg-slate-950/60 border border-slate-800 text-xs';
    row.innerHTML = `
      <span class="font-mono text-sky-300">${escapeHTML(token)}</span>
      <button class="remove-dot-token inline-flex items-center justify-center w-6 h-6 shrink-0 rounded text-slate-500 hover:text-red-400 hover:bg-red-500/10 transition" data-val="${escapeHTML(token)}" aria-label="Remove ${escapeHTML(token)}">
        <i data-feather="x" class="w-3.5 h-3.5"></i>
      </button>
    `;
    list.appendChild(row);
  });
  safeFeatherReplace();
}

function renderDoqIpAllowlist(applied) {
  const counts = document.getElementById('doq-ip-counts');
  const warning = document.getElementById('doq-ip-warning');
  if (!counts || !warning) return;
  const al = applied.ip_allowlist;
  const plural = (n, one, many) => `${n} ${n === 1 ? one : many}`;
  setTransportText(counts, [
    plural(al.client_accounts, 'client', 'clients'),
    plural(al.client_entries, 'client IP/CIDR', 'client IPs/CIDRs'),
    plural(al.resolver_entries, 'resolver range', 'resolver ranges'),
  ].join(' · '));
  let message = '';
  if (al.allow_all) {
    message = 'Public Mode is on — any source can use the IP-authenticated transports (DoQ, DoT, DNSCrypt), so the allowlist is not restricting access. Switch Access Mode to whitelist to enforce it.';
  } else if (al.client_entries === 0 && al.resolver_entries === 0) {
    message = 'No IPs are allowlisted — IP-authenticated transports will refuse every source. Add addresses in Clients → Allowed IPs or the Server-wide Resolver Allowlist.';
  }
  setTransportText(warning, message);
  warning.classList.toggle('hidden', !message);
}

// refreshTransportView re-reads the listener state and allowlist counts without
// touching the staged transport draft. Client and access-mode saves change
// those counts, and the card is often visible in the same settings page.
async function refreshTransportView() {
  if (!currentConfig || !authToken || transportSaving) return;
  try {
    const res = await fetch(api('/api/settings/transports'), {
      headers: { 'Authorization': `Bearer ${authToken}` }
    });
    if (!res.ok) return;
    currentConfig.transports = normalizeTransportSettings({ transports: await res.json() });
    renderTransportSettings();
  } catch (ignore) { /* keep the last known card state */ }
}

// renderGlobalNotices is the one place the shell explains listener state that
// applies to every tab. Per-resolver health stays in the Upstream Resolvers card;
// a page-wide banner for a single probe failure broke the layout. Public access mode deliberately has no banner:
// the Clients page already owns that switch, and a persistent banner duplicated
// it across the whole panel.
function renderGlobalNotices() {
  const el = document.getElementById('global-notices');
  if (!el) return;
  const notices = [];

  const t = currentConfig?.transports;
  if (t) {
    const down = [];
    if (!t.doh3_listening) down.push('DoH3');
    if (t.doq_enabled && !t.doq_listening) down.push('DoQ');
    if (down.length) {
      const reason = String(t.last_error || '').trim();
      notices.push({
        tone: 'error',
        text: `${down.join(' and ')} ${down.length === 1 ? 'is' : 'are'} enabled but not listening.${reason ? ' ' + reason : ''}`,
        action: 'transports',
        label: 'Open transports',
      });
    }
  }

  const signature = JSON.stringify(notices);
  if (signature === lastGlobalNoticeSignature) return;
  lastGlobalNoticeSignature = signature;
  if (notices.length === 0) {
    el.innerHTML = '';
    el.classList.add('hidden');
    return;
  }
  el.classList.remove('hidden');
  el.innerHTML = notices.map((n) => {
    const tone = n.tone === 'error'
      ? 'border-red-500/30 bg-red-500/10 text-red-300'
      : 'border-amber-500/30 bg-amber-500/10 text-amber-300';
    return `<div class="rounded-lg border ${tone} px-3 py-2 text-[11px] flex items-start gap-2">
      <span class="flex-1">${escapeHTML(n.text)}</span>
      <button type="button" class="global-notice-btn shrink-0 font-bold" data-notice-action="${escapeHTML(n.action)}">${escapeHTML(n.label)}</button>
    </div>`;
  }).join('');
}

document.addEventListener('click', (e) => {
  const btn = e.target.closest('.global-notice-btn');
  if (!btn) return;
  const action = btn.dataset.noticeAction;
  if (action === 'transports') {
    switchTab('settings', false);
    requestAnimationFrame(() => document.getElementById('transports-card')?.scrollIntoView({ block: 'start', behavior: 'smooth' }));
  } else if (action === 'upstreams') {
    switchTab('dashboard', false);
    requestAnimationFrame(() => document.querySelector('[data-dash-card="upstream"]')?.scrollIntoView({ block: 'center', behavior: 'smooth' }));
  }
});

function renderTransportSettings() {
  const card = document.getElementById('transports-card');
  if (!card || !currentConfig) return;
  const applied = normalizeTransportSettings(currentConfig);
  const draft = ensureTransportDraft(applied);

  const plainEnabled = document.getElementById('plain-enabled-input');
  if (plainEnabled) {
    if (plainEnabled.checked !== !!draft.plain_dns_enabled) plainEnabled.checked = !!draft.plain_dns_enabled;
  }
  const plainPort = document.getElementById('plain-port-input');
  if (plainPort) plainPort.value = draft.plain_dns_port || 53;

  const dohEnabled = document.getElementById('doh-enabled-input');
  if (dohEnabled) {
    if (dohEnabled.checked !== !!draft.doh_enabled) dohEnabled.checked = !!draft.doh_enabled;
  }
  const dohPort = document.getElementById('doh-port-input');
  if (dohPort) dohPort.value = draft.doh_port || 8443;

  const doh3 = document.getElementById('doh3-enabled-input');
  if (doh3) {
    doh3.checked = !!draft.doh3_enabled;
    // DoH3 shares the DoH listener, so it cannot be on while DoH is off.
    doh3.disabled = !draft.doh_enabled;
  }
  const doq = document.getElementById('doq-enabled-input');
  if (doq) {
    if (doq.checked !== !!draft.doq_enabled) doq.checked = !!draft.doq_enabled;
  }
  const auth = document.getElementById('doq-auth-select');
  if (auth) auth.value = draft.doq_auth;
  const port = document.getElementById('doq-port-input');
  if (port) port.value = draft.doq_port || 853;
  const domain = document.getElementById('doq-token-domain-input');
  if (domain) domain.value = draft.doq_token_domain || '';

  const fields = document.getElementById('doq-token-fields');
  if (fields) fields.classList.toggle('hidden', draft.doq_auth !== 'sni-token');
  const warning = document.getElementById('doq-cert-warning');
  if (warning) warning.classList.toggle('hidden', draft.doq_auth !== 'sni-token');

  const dotEnabled = document.getElementById('dot-enabled-input');
  if (dotEnabled) {
    if (dotEnabled.checked !== !!draft.dot_enabled) dotEnabled.checked = !!draft.dot_enabled;
  }
  const dotPort = document.getElementById('dot-port-input');
  if (dotPort) dotPort.value = draft.dot_port || 853;
  const dotAuth = document.getElementById('dot-auth-select');
  if (dotAuth) dotAuth.value = draft.dot_auth;
  const dotDomain = document.getElementById('dot-token-domain-input');
  if (dotDomain) dotDomain.value = draft.dot_token_domain || '';
  const dotFields = document.getElementById('dot-token-fields');
  if (dotFields) dotFields.classList.toggle('hidden', draft.dot_auth !== 'sni-token');

  const dnscryptEnabled = document.getElementById('dnscrypt-enabled-input');
  if (dnscryptEnabled) {
    if (dnscryptEnabled.checked !== !!draft.dnscrypt_enabled) dnscryptEnabled.checked = !!draft.dnscrypt_enabled;
  }
  const dnscryptPort = document.getElementById('dnscrypt-port-input');
  if (dnscryptPort) dnscryptPort.value = draft.dnscrypt_port || 5443;
  const dnscryptProvider = document.getElementById('dnscrypt-provider-input');
  if (dnscryptProvider) {
    // The 2.dnscrypt-cert. prefix is mandatory and would push the meaningful
    // part out of the field; show the suffix and let the server add it back.
    dnscryptProvider.value = (draft.dnscrypt_provider_name || '').replace(/^2\.dnscrypt-cert\./i, '');
  }

  const mtlsEnabled = document.getElementById('mtls-enabled-input');
  if (mtlsEnabled) {
    if (mtlsEnabled.checked !== !!draft.mtls_enabled) mtlsEnabled.checked = !!draft.mtls_enabled;
  }
  const mtlsCA = document.getElementById('mtls-ca-input');
  if (mtlsCA) mtlsCA.value = draft.mtls_ca_pem || '';
  const mtlsWarning = document.getElementById('mtls-warning');
  if (mtlsWarning) mtlsWarning.classList.toggle('hidden', !(draft.mtls_enabled && !draft.mtls_ca_pem.trim()));

  const saveStatus = document.getElementById('transports-save-status');
  if (saveStatus) {
    const problem = transportDraftProblem(draft);
    const dirty = transportDraftDirty(applied, draft);
    if (transportSaving) {
      saveStatus.innerText = 'Saving…';
      saveStatus.className = 'text-[11px] text-slate-400 ms-auto';
    } else if (problem) {
      saveStatus.innerText = problem;
      saveStatus.className = 'text-[11px] text-amber-300 ms-auto';
    } else if (dirty) {
      saveStatus.innerText = 'Applying…';
      saveStatus.className = 'text-[11px] text-slate-400 ms-auto';
    } else {
      saveStatus.innerText = 'All changes saved';
      saveStatus.className = 'text-[11px] text-emerald-300 ms-auto';
    }
  }

  const errorEl = document.getElementById('transports-last-error');
  if (errorEl) {
    const message = transportInlineError || applied.last_error || '';
    errorEl.classList.toggle('hidden', !message);
    errorEl.innerText = message;
  }

  renderTransportEndpoints(applied, draft);
  renderDoQTokens(draft);
  renderDoTTokens(draft);
  renderDoqIpAllowlist(applied);
  updateTransportBadge(applied);
  renderGlobalNotices();
}

async function autoSaveTransportDraft() {
  if (transportSaving || !currentConfig || !authToken) return;
  const applied = normalizeTransportSettings(currentConfig);
  const draft = ensureTransportDraft(applied);
  if (!transportDraftDirty(applied, draft) && !transportInlineError && !applied.last_error) {
    renderTransportSettings();
    return;
  }
  if (transportDraftProblem(draft)) {
    // Incomplete SNI-token or mTLS state stays local; the footer says what is
    // missing and the next completed edit applies it.
    transportInlineError = '';
    renderTransportSettings();
    return;
  }

  transportSaving = true;
  transportInlineError = '';
  renderTransportSettings();
  try {
    const res = await fetch(api('/api/settings/transports'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${authToken}` },
      body: JSON.stringify(transportSaveBody(draft))
    });
    if (!res.ok) {
      transportInlineError = await errorMessage(res, 'Could not apply the transport settings.');
      transportSaving = false;
      // The server rolled back to the previous listeners. Keep the draft so the
      // operator can correct the field that failed, but re-read listener state.
      await refreshTransportView();
      renderTransportSettings();
      showToast(transportInlineError, 'error');
      return;
    }
    currentConfig.transports = normalizeTransportSettings({ transports: await res.json() });
    transportSaving = false;
    void loadResolutionSettings();
    // An edit made while the request was in flight is not in the response;
    // keep the draft and send it now that the first save is done.
    const stillDirty = transportDraft && transportDraftDirty(normalizeTransportSettings(currentConfig), transportDraft);
    if (!stillDirty) transportDraft = null;
    renderTransportSettings();
    if (stillDirty) void autoSaveTransportDraft();
  } catch (e) {
    transportInlineError = 'Could not apply the transport settings.';
    transportSaving = false;
    renderTransportSettings();
    showToast(transportInlineError, 'error');
  }
}

// ---- Proxied domains: sort and windowed rendering ---------------------------
//
// The routing list is the one list on the panel that can hold thousands of
// entries, so it is drawn a batch at a time rather than printed in full.
// PROXIED_BATCH is how many rows each slice shows; changing the sort resets
// proxiedVisible to it, and the sentinel at the bottom of the scroll region
// grows it as it comes into view. proxiedHasMore is the renderer's answer to
// "is there another batch", read by both the observer and the fallback button.
const PROXIED_BATCH = 50;
let proxiedSort = 'added-desc';
// null means all statuses; the chips toggle one of these on and off.
let proxiedStatusFilter = null;
let proxiedVisible = PROXIED_BATCH;
let proxiedHasMore = false;
// Created once and left watching the sentinel. It is not rebuilt on render,
// because the sentinel is a sibling of the rows container rather than a child of
// it: renderProxiedList rewrites the rows' innerHTML and never touches it.
let proxiedObserver = null;

// proxiedOrderedRows is the whole list in the operator's chosen order. Insertion
// order is array order: a new domain is pushed onto the end, so "Added (newest)"
// is the reverse. Disabled entries keep their own order after the enabled ones,
// which is the grouping the status sort exposes.
// One unified view of every managed list. A domain can appear in more than one
// stored list after a hand-edited save; the effective status follows the matcher
// precedence Direct > Blocked > Proxy > Paused, so the row shows what the engine
// will actually do.
const DOMAIN_STATUS_PRECEDENCE = { paused: 0, proxy: 1, blocked: 2, direct: 3 };
// Matches Android's DomainStatus enum order for the STATUS sort.
const DOMAIN_STATUS_RANK = { proxy: 0, blocked: 1, paused: 2, direct: 3 };
const DOMAIN_STATUS_META = {
  proxy: { label: 'PROXY', badge: 'badge-proxy', domainClass: 'proxy-status-text' },
  paused: { label: 'PAUSED', badge: 'badge-unknown', domainClass: 'text-slate-400' },
  blocked: { label: 'BLOCKED', badge: 'badge-block', domainClass: 'text-red-300' },
  direct: { label: 'DIRECT', badge: 'badge-direct', domainClass: 'text-emerald-300' },
};

// The four routing lists, in the order a status takes precedence.
const DOMAIN_RULE_LISTS = ['custom_proxied', 'custom_proxied_disabled', 'custom_blocked', 'custom_direct'];

function proxiedOrderedRows() {
  const rules = currentConfig?.rules || {};
  const byDomain = new Map();
  const add = (list, status) => {
    if (!Array.isArray(list)) return;
    list.forEach((domain) => {
      if (!domain) return;
      const prev = byDomain.get(domain);
      if (!prev || DOMAIN_STATUS_PRECEDENCE[status] > DOMAIN_STATUS_PRECEDENCE[prev]) {
        byDomain.set(domain, status);
      }
    });
  };
  // Low precedence first so a higher one can overwrite without losing the key's
  // insertion position for the common one-list case.
  add(rules.custom_proxied, 'proxy');
  add(rules.custom_proxied_disabled, 'paused');
  add(rules.custom_blocked, 'blocked');
  add(rules.custom_direct, 'direct');

  let rows = Array.from(byDomain, ([domain, status]) => ({ domain, status }));
  // *.d is redundant when d is in the same list: the plain rule already covers every
  // subdomain. Hide the duplicate row; a wildcard in a different list still matters.
  rows = rows.filter(({ domain, status }) => {
    if (!domain.startsWith('*.')) return true;
    return byDomain.get(domain.slice(2)) !== status;
  });
  const byName = (a, b) => a.domain.localeCompare(b.domain, undefined, { sensitivity: 'base' });
  // A wildcard row sits in the same alphabetical slot as its base, so a scope
  // change does not move it under A-Z.
  const byBase = (a, b) => {
    const ka = a.domain.replace(/^\*\./, '');
    const kb = b.domain.replace(/^\*\./, '');
    return ka.localeCompare(kb, undefined, { sensitivity: 'base' }) || byName(a, b);
  };
  const added = rules.custom_domain_added || {};
  const stamp = (domain) => {
    const base = domain.startsWith('*.') ? domain.slice(2) : domain;
    const t = Date.parse(added[domain] || added[base] || '');
    return Number.isFinite(t) ? t : 0;
  };
  const anyStamp = rows.some((r) => stamp(r.domain) > 0);

  switch (proxiedSort) {
    case 'added-asc':
      return anyStamp ? rows.slice().sort((a, b) => stamp(a.domain) - stamp(b.domain)) : rows;
    case 'az': return rows.slice().sort(byBase);
    case 'za': return rows.slice().sort((a, b) => byBase(b, a));
    case 'status':
      return rows.slice().sort((a, b) => DOMAIN_STATUS_RANK[a.status] - DOMAIN_STATUS_RANK[b.status]);
    case 'added-desc':
    default:
      return anyStamp ? rows.slice().sort((a, b) => stamp(b.domain) - stamp(a.domain)) : rows.slice().reverse();
  }
}

// domainRemovalPatterns is every stored pattern one visible row stands for.
//
// The table hides *.base when base is in the same list, because the plain rule
// already covers its subdomains. Deleting that row therefore has to delete the
// hidden twin too, or the next render brings a wildcard row back where the
// operator just removed one. A wildcard row is only itself.
function domainRemovalPatterns(domain) {
  const raw = String(domain || '').trim();
  if (!raw || !currentConfig || !currentConfig.rules) return [];
  const rules = currentConfig.rules;
  if (raw.startsWith('*.')) return [raw];
  const patterns = [raw];
  const twin = '*.' + raw;
  for (const key of DOMAIN_RULE_LISTS) {
    if (Array.isArray(rules[key]) && rules[key].includes(raw) && rules[key].includes(twin)) {
      patterns.push(twin);
    }
  }
  return patterns;
}

// removeDomainEverywhere deletes a row from all four routing lists, not just the
// proxied pair. The Domains table shows blocked and direct rows too, and those
// used to survive the row's Remove button untouched.
function removeDomainEverywhere(domain) {
  if (!currentConfig || !currentConfig.rules) return false;
  const patterns = domainRemovalPatterns(domain);
  if (!patterns.length) return false;
  const rules = currentConfig.rules;
  for (const key of DOMAIN_RULE_LISTS) {
    if (Array.isArray(rules[key])) rules[key] = rules[key].filter((d) => !patterns.includes(d));
  }
  // The "Added" stamp belongs to the rule: keeping it would make a re-added
  // domain look older than it is and sort it into the wrong place.
  const added = rules.custom_domain_added || {};
  for (const pattern of patterns) delete added[pattern];
  rules.custom_domain_added = added;
  return true;
}

// proxiedFilterNeedle is what the table is currently narrowed to, or '' for the
// whole list. Two inputs feed it: the search box in the control row, and the Add
// box above it — typing a name into Add is how an operator discovers that the
// domain already exists before creating a duplicate, so it narrows the same list.
// The search box wins when both carry text, because it is the control the operator
// reached for deliberately; Add is cleared by the add itself.
function proxiedFilterNeedle() {
  const search = (document.getElementById('proxied-search')?.value || '').trim().toLowerCase();
  if (search) return search;
  return (document.getElementById('new-proxied-input')?.value || '').trim().toLowerCase();
}

// proxiedFilteredRows is proxiedOrderedRows in the operator's order, narrowed to
// the rows matching proxiedFilterNeedle. Everything that draws or selects rows
// reads this, so the count, the empty state, the select-all box and the bulk
// selection all agree on what "the list" means while a filter is active.
function proxiedFilteredRows() {
  let ordered = proxiedOrderedRows();
  if (proxiedStatusFilter) {
    ordered = ordered.filter((r) => r.status === proxiedStatusFilter);
  }
  const needle = proxiedFilterNeedle();
  if (!needle) return ordered;
  return ordered.filter((r) => r.domain.toLowerCase().includes(needle));
}

function updateDomainFilterChips() {
  document.querySelectorAll('.domain-filter-chip').forEach((chip) => {
    const active = chip.dataset.status === proxiedStatusFilter;
    chip.classList.toggle('is-active', active);
    chip.setAttribute('aria-pressed', active ? 'true' : 'false');
  });
}

// proxiedSuggestions fills the datalist behind the Add box with the domains that are
// already configured. Called once per config render, never per keystroke: rewriting a
// datalist while its own suggestion popup is open dismisses the popup under the
// operator's finger. The cap keeps the list quick to open — the browser matches on
// prefix, so the useful tail of a very large list would never be read.
//
// The search box has no datalist at all: it narrows the table as it is typed, exactly
// like the live log's search, with no suggestion popup in front of the list.
const PROXIED_SUGGESTION_CAP = 500;

function renderProxiedSuggestions() {
  const domains = proxiedOrderedRows()
    .map((r) => r.domain)
    .sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' }))
    .slice(0, PROXIED_SUGGESTION_CAP);
  const options = domains.map((d) => `<option value="${escapeHTML(d)}"></option>`).join('');
  const list = document.getElementById('proxied-add-suggestions');
  if (list) list.innerHTML = options;
}

// proxiedSelected holds the domains ticked for a bulk action. It is a set of domain
// strings rather than row indexes: a re-render (a sort, a filter, the next batch, a
// save) would otherwise move the selection onto whichever row happened to land on the
// same index. A filter never clears it: a selection survives narrowing the list, which
// is what lets an operator search a batch of names and act on all of them at once.
let proxiedSelected = new Set();

// proxiedPruneSelection drops domains no longer in the list, so a bulk action can never
// operate on an entry a reload or another operator has since removed.
function proxiedPruneSelection() {
  const live = new Set(proxiedOrderedRows().map((r) => r.domain));
  for (const d of Array.from(proxiedSelected)) {
    if (!live.has(d)) proxiedSelected.delete(d);
  }
}

// updateProxiedBulkBar reflects the selection in the count line, the bulk buttons'
// disabled state, and the header checkbox — checked when every row currently shown is
// ticked, indeterminate when only some are. It counts the filtered rows, not the whole
// list: with a search active, "select all" means the matches the operator can see.
// The bulk assign menu: All services is a visual header (assigning "all" has no
// meaning), No service clears the label, and every service carries its favicon
// or the same letter mark the Services box uses.
const BULK_ALL_MARK = '<span class="service-mark" aria-hidden="true">' +
  '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round">' +
  '<circle cx="12" cy="12" r="9"/><path d="M3 12h18"/>' +
  '<path d="M12 3a15 15 0 0 1 0 18M12 3a15 15 0 0 0 0 18"/></svg></span>';
const BULK_NONE_MARK = '<span class="service-mark" aria-hidden="true">' +
  '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round">' +
  '<circle cx="12" cy="12" r="9"/><path d="M3 12h18"/>' +
  '<path d="M12 3a15 15 0 0 1 0 18M12 3a15 15 0 0 0 0 18"/>' +
  '<line x1="4" y1="4" x2="20" y2="20"/></svg></span>';

function serviceMarkHTML(id, name) {
  const icons = (currentConfig && currentConfig.rules && currentConfig.rules.service_icons) || {};
  if (icons[id]) return `<img class="service-mark" src="${escapeHTML(icons[id])}" alt="">`;
  const hue = serviceHue(name);
  const style = `background:hsl(${hue} 45% 22%);color:hsl(${hue} 70% 70%)`;
  return `<span class="service-mark" style="${escapeHTML(style)}">${escapeHTML((name || '?').trim().charAt(0).toUpperCase())}</span>`;
}

function renderBulkServiceOptions() {
  const menu = document.getElementById('proxied-bulk-service-menu');
  if (!menu) return;
  const services = (currentConfig && currentConfig.rules && currentConfig.rules.services) || {};
  const entries = Object.entries(services).sort((a, b) => a[1].localeCompare(b[1]));
  menu.innerHTML =
    `<div class="proxied-bulk-option is-disabled" role="option" aria-disabled="true">${BULK_ALL_MARK}<span>All services</span></div>` +
    `<button type="button" class="proxied-bulk-option" role="option" data-bulk-service="__none__">${BULK_NONE_MARK}<span>No service</span></button>` +
    entries.map(([id, name]) =>
      `<button type="button" class="proxied-bulk-option" role="option" data-bulk-service="${escapeHTML(id)}">` +
      `${serviceMarkHTML(id, name)}<span class="truncate">${escapeHTML(name)}</span></button>`).join('');
}

function renderBulkEgressOptions() {
  const menu = document.getElementById('proxied-bulk-egress-menu');
  if (!menu) return;
  const inherit = `<button type="button" class="proxied-bulk-option" role="option" data-bulk-egress="__inherit__">
    <span class="service-mark" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><path d="M4 12h12"/><path d="m12 6 6 6-6 6"/></svg></span>
    <span>Inherit service</span></button>`;
  const local = `<button type="button" class="proxied-bulk-option" role="option" data-bulk-egress="__local__">
    <span class="service-mark" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><rect x="3" y="4" width="18" height="16" rx="2"/><path d="M3 9h18"/></svg></span>
    <span>This Server</span></button>`;
  const group = (label) => `<div class="proxied-bulk-group">${escapeHTML(label)}</div>`;
  const agents = agentsCache.length
    ? group('Relay Agents') + agentsCache.map((a) => `<button type="button" class="proxied-bulk-option" role="option" data-bulk-egress="${escapeHTML(a.id)}">
        ${agentFlagHTML(a)}
        <span class="truncate">${escapeHTML(a.name || a.id)} · ${escapeHTML(a.country || '')}${!agentIsOnline(a) ? ' (offline)' : ''}</span>
      </button>`).join('')
    : '';
  const socks = egressesCache.filter((e) => e.kind === 'socks5');
  const socksHTML = socks.length
    ? group('SOCKS5') + socks.map((e) => `<button type="button" class="proxied-bulk-option" role="option" data-bulk-egress="${escapeHTML(egressValue(e))}">
        ${egressFlagHTML(e) || '<i data-feather="shuffle" class="w-3.5 h-3.5"></i>'}
        <span class="truncate">${escapeHTML(e.name || e.id)} · ${escapeHTML(e.country || '')}${!egressIsOnline(e) ? ' (offline)' : ''}</span>
      </button>`).join('')
    : '';
  const wireguard = egressesCache.filter((e) => e.kind === 'wireguard');
  const wgHTML = wireguard.length
    ? group('WireGuard') + wireguard.map((e) => `<button type="button" class="proxied-bulk-option" role="option" data-bulk-egress="${escapeHTML(egressValue(e))}">
        ${egressFlagHTML(e) || '<i data-feather="lock" class="w-3.5 h-3.5"></i>'}
        <span class="truncate">${escapeHTML(e.name || e.id)} · ${escapeHTML(e.country || '')}${!egressIsOnline(e) ? ' (offline)' : ''}</span>
      </button>`).join('')
    : '';
  const empty = !agents && !socksHTML && !wgHTML
    ? '<div class="proxied-bulk-option is-disabled" role="option" aria-disabled="true"><span>No egress options yet</span></div>'
    : '';
  menu.innerHTML = inherit + local + agents + socksHTML + wgHTML + empty;
}

function closeBulkEgressMenu() {
  document.getElementById('proxied-bulk-egress-menu')?.classList.add('hidden');
  const btn = document.getElementById('proxied-bulk-egress');
  if (btn) btn.setAttribute('aria-expanded', 'false');
}

async function assignEgressToSelected(agentId) {
  if (!agentId || proxiedSelected.size === 0) return;
  closeBulkEgressMenu();
  const domains = Array.from(proxiedSelected);
  const rules = currentConfig.rules;
  const links = Object.assign({}, rules.domain_agents || {});
  for (const domain of domains) {
    const key = String(domain || '').toLowerCase();
    if (agentId === '__inherit__') delete links[key]; else links[key] = agentId;
  }
  rules.domain_agents = links;
  const ok = await saveRules(true);
  if (!ok) return;
  renderProxiedList();
  const agent = agentForId(agentId);
  const egress = egressForId(agentId);
  showToast(
    agentId === '__inherit__'
      ? `Cleared ${domains.length} domain override(s); services apply`
      : agentId === '__local__'
        ? `Set ${domains.length} domain(s) to This Server`
        : `Routed ${domains.length} domain(s) via ${egress ? egress.name : (agent ? agent.name : agentId)}`,
    'success',
  );
}

function closeBulkServiceMenu() {
  document.getElementById('proxied-bulk-service-menu')?.classList.add('hidden');
  const btn = document.getElementById('proxied-bulk-service');
  if (btn) btn.setAttribute('aria-expanded', 'false');
}

async function assignServiceToSelected(serviceId) {
  if (!serviceId || proxiedSelected.size === 0) return;
  closeBulkServiceMenu();
  const domains = Array.from(proxiedSelected);
  const rules = currentConfig.rules;
  const links = Object.assign({}, rules.domain_services || {});
  const id = serviceId === '__none__' ? '' : serviceId;
  for (const domain of domains) {
    const key = String(domain || '').toLowerCase();
    if (id) links[key] = id; else delete links[key];
  }
  rules.domain_services = links;
  await saveRules();
  renderServices();
  renderProxiedList();
  showToast(
    id
      ? `Assigned ${domains.length} domain(s) to ${serviceName(id)}`
      : `Cleared the service label on ${domains.length} domain(s)`,
    'success',
  );
}

function updateProxiedBulkBar() {
  const n = proxiedSelected.size;
  const shown = proxiedFilteredRows();
  const selectedRows = proxiedOrderedRows().filter((r) => proxiedSelected.has(r.domain));
  const pausable = selectedRows.length > 0 &&
    selectedRows.every((r) => r.status === 'proxy' || r.status === 'paused');
  const countEl = document.getElementById('proxied-selected-count');
  if (countEl) countEl.textContent = n === 0 ? 'No domains selected' : `${n} selected`;
  const pause = document.getElementById('proxied-bulk-pause');
  const resume = document.getElementById('proxied-bulk-resume');
  const remove = document.getElementById('proxied-bulk-remove');
  if (pause) pause.disabled = !pausable;
  if (resume) resume.disabled = !pausable;
  if (remove) remove.disabled = n === 0;
  const bulkService = document.getElementById('proxied-bulk-service');
  if (bulkService) {
    bulkService.disabled = n === 0;
    if (n === 0) closeBulkServiceMenu();
  }
  const bulkEgress = document.getElementById('proxied-bulk-egress');
  if (bulkEgress) {
    bulkEgress.disabled = n === 0;
    if (n === 0) closeBulkEgressMenu();
  }
  const all = document.getElementById('proxied-select-all');
  if (all) {
    const selectedShown = shown.filter((r) => proxiedSelected.has(r.domain)).length;
    all.checked = shown.length > 0 && selectedShown === shown.length;
    all.indeterminate = selectedShown > 0 && selectedShown < shown.length;
  }
}

// The per-row service picker. A native select cannot show images in its
// options, so this is a body-level listbox positioned under the clicked trigger;
// the table's own scroll container would clip a menu placed inside the row.
let servicePickerDomain = null;
let servicePickerTrigger = null;

function closeServicePicker() {
  document.getElementById('service-picker-menu')?.classList.add('hidden');
  if (servicePickerTrigger) servicePickerTrigger.setAttribute('aria-expanded', 'false');
  servicePickerDomain = null;
  servicePickerTrigger = null;
}

function openServicePicker(domain, trigger) {
  const menu = document.getElementById('service-picker-menu');
  if (!menu) return;
  if (servicePickerDomain === domain && !menu.classList.contains('hidden')) {
    closeServicePicker();
    return;
  }
  const services = (currentConfig.rules && currentConfig.rules.services) || {};
  const entries = Object.entries(services).sort((a, b) => a[1].localeCompare(b[1]));
  menu.innerHTML =
    `<button type="button" class="proxied-bulk-option" role="option" data-service-id="__none__">${BULK_NONE_MARK}<span>No service</span></button>` +
    entries.map(([id, name]) =>
      `<button type="button" class="proxied-bulk-option" role="option" data-service-id="${escapeHTML(id)}">` +
      `${serviceMarkHTML(id, name)}<span class="truncate">${escapeHTML(name)}</span></button>`).join('');
  menu.classList.remove('hidden');
  servicePickerDomain = domain;
  servicePickerTrigger = trigger;
  trigger.setAttribute('aria-expanded', 'true');

  const r = trigger.getBoundingClientRect();
  const mw = menu.offsetWidth;
  const mh = menu.offsetHeight;
  menu.style.left = `${Math.max(8, Math.min(r.left, innerWidth - mw - 8))}px`;
  menu.style.top = `${r.bottom + 4 + mh > innerHeight ? Math.max(8, r.top - mh - 4) : r.bottom + 4}px`;
}

// The per-domain relay picker. This is the row-level control: the RELAY cell
// opens it, and it writes one domain_agents entry. Inherit removes the entry so
// a service default applies; Local writes an explicit __local__ override.
let relayPickerDomain = null;
let relayPickerTrigger = null;

function closeRelayPicker() {
  document.getElementById('relay-picker-menu')?.classList.add('hidden');
  if (relayPickerTrigger) relayPickerTrigger.setAttribute('aria-expanded', 'false');
  relayPickerDomain = null;
  relayPickerTrigger = null;
}

function openRelayPicker(domain, trigger) {
  const menu = document.getElementById('relay-picker-menu');
  if (!menu) return;
  if (relayPickerDomain === domain && !menu.classList.contains('hidden')) {
    closeRelayPicker();
    return;
  }
  const key = String(domain || '').toLowerCase().replace(/\.$/, '');
  const rules = currentConfig.rules || {};
  const stored = (rules.domain_agents || {})[key] || '';
  const serviceId = serviceOfDomain(key);
  const serviceRelayId = serviceId ? ((rules.service_agents || {})[serviceId] || '') : '';
  const serviceRelay = serviceRelayId ? agentForId(serviceRelayId) : null;
  const inheritLabel = serviceRelayId
    ? `Inherit service — ${serviceRelay ? serviceRelay.name : serviceRelayId}`
    : 'Inherit service — This Server';
  const rows = [
    `<button type="button" class="proxied-bulk-option" role="option" data-relay-id="__inherit__">
      ${(serviceRelay?.country || currentConfig?.server?.country)
        ? countryFlagHTML(serviceRelay?.country || currentConfig.server.country, 'agent-flag relay-flag')
        : '<i data-feather="corner-up-left" class="w-3.5 h-3.5"></i>'}
      <span class="truncate">${escapeHTML(inheritLabel)}${stored === '' ? ' · selected' : ''}</span>
    </button>`,
    `<button type="button" class="proxied-bulk-option" role="option" data-relay-id="__local__">
      ${currentConfig?.server?.country ? countryFlagHTML(currentConfig.server.country, 'agent-flag relay-flag') : '<i data-feather="server" class="w-3.5 h-3.5"></i>'}
      <span class="truncate">This Server${stored === '__local__' ? ' · selected' : ''}</span>
    </button>`,
  ];
  const group = (label) => `<div class="proxied-bulk-group">${escapeHTML(label)}</div>`;
  if (agentsCache.length) {
    rows.push(group('Relay Agents'));
    for (const a of agentsCache) {
      rows.push(`<button type="button" class="proxied-bulk-option" role="option" data-relay-id="${escapeHTML(a.id)}">
        ${agentFlagHTML(a, 'agent-flag relay-flag')}
        <span class="truncate">${escapeHTML(a.name || a.id)} · ${escapeHTML(a.country || '')}${agentIsOnline(a) ? '' : ' (offline)'}${stored === a.id ? ' · selected' : ''}</span>
      </button>`);
    }
  }
  const socks = egressesCache.filter((e) => e.kind === 'socks5');
  if (socks.length) {
    rows.push(group('SOCKS5'));
    for (const e of socks) {
      const value = egressValue(e);
      rows.push(`<button type="button" class="proxied-bulk-option" role="option" data-relay-id="${escapeHTML(value)}">
        ${egressFlagHTML(e, 'agent-flag relay-flag') || '<i data-feather="shuffle" class="w-3.5 h-3.5"></i>'}
        <span class="truncate">${escapeHTML(e.name || e.id)} · ${escapeHTML(e.country || '')}${egressIsOnline(e) ? '' : ' (offline)'}${stored === value ? ' · selected' : ''}</span>
      </button>`);
    }
  }
  const wireguard = egressesCache.filter((e) => e.kind === 'wireguard');
  if (wireguard.length) {
    rows.push(group('WireGuard'));
    for (const e of wireguard) {
      const value = egressValue(e);
      rows.push(`<button type="button" class="proxied-bulk-option" role="option" data-relay-id="${escapeHTML(value)}">
        ${egressFlagHTML(e, 'agent-flag relay-flag') || '<i data-feather="lock" class="w-3.5 h-3.5"></i>'}
        <span class="truncate">${escapeHTML(e.name || e.id)} · ${escapeHTML(e.country || '')}${egressIsOnline(e) ? '' : ' (offline)'}${stored === value ? ' · selected' : ''}</span>
      </button>`);
    }
  }
  if (!agentsCache.length && !socks.length && !wireguard.length) {
    rows.push('<div class="proxied-bulk-option is-disabled" role="option" aria-disabled="true"><span>No egress options yet</span></div>');
  }
  menu.innerHTML = rows.join('');
  menu.classList.remove('hidden');
  relayPickerDomain = domain;
  relayPickerTrigger = trigger;
  trigger.setAttribute('aria-expanded', 'true');
  const r = trigger.getBoundingClientRect();
  const mw = menu.offsetWidth;
  const mh = menu.offsetHeight;
  menu.style.left = `${Math.max(8, Math.min(r.left, innerWidth - mw - 8))}px`;
  menu.style.top = `${r.bottom + 4 + mh > innerHeight ? Math.max(8, r.top - mh - 4) : r.bottom + 4}px`;
}

async function assignRelayToDomain(domain, id) {
  const key = String(domain || '').toLowerCase().replace(/\.$/, '');
  const links = Object.assign({}, currentConfig.rules.domain_agents || {});
  if (id === '__inherit__') delete links[key];
  else links[key] = id;
  currentConfig.rules.domain_agents = links;
  closeRelayPicker();
  if (!(await saveRules(true))) return;
  renderProxiedList();
  const agent = id && id !== '__local__' && id !== '__inherit__' ? agentForId(id) : null;
  const egress = egressForId(id);
  showToast(
    id === '__inherit__'
      ? 'Relay now follows the service default'
      : id === '__local__'
        ? 'Relay set to This Server'
        : `Routed via ${egress ? egress.name : (agent ? agent.name : id)}`,
    'success',
  );
}

// renderProxiedList draws the Proxied Domains page. Two lists feed it: the
// enabled domains (cfg.rules.custom_proxied) the matcher actually proxies, and the
// ones the operator switched off (custom_proxied_disabled), which are kept only so
// they survive a reload. Every row carries a switch; the paused rows are dimmed and
// sorted after the live ones. Toggling a switch moves the domain between the two
// lists rather than deleting it — that move is what lifts it out of the proxy set, so
// a switched-off domain resolves normally instead of being proxied.
//
// The table mirrors the live log: a fixed set of columns, a leading checkbox for the
// bulk actions, the domain, its status, and the per-row switch and remove button. The
// ordered list can be long, so only its first proxiedVisible rows are drawn;
// proxiedHasMore records whether there is a next batch for the sentinel to fetch.
//
// A filter narrows the list before it is sliced: `ordered` is the whole list in sort
// order and `rows` is what the search (or the Add box) leaves of it. Filtering re-runs
// the slice from the top, so a match beyond the first batch is drawn rather than
// hidden behind "Load more".

// The global resolution defaults and the per-domain overrides, mirrored from
// /api/settings/resolution. Defaults live in the Upstream Resolvers card; a
// per-domain rule is attached to a row in the Proxied Domains list and can only
// exist for a name that is in that list.
let resolutionSettings = {
  transport: 'any',
  transports: ['plain', 'dot', 'doh', 'doh3', 'doq', 'dnscrypt'],
  loaded: false,
  encrypted_available: false,
  encrypted_upstreams: [],
  encrypted_only: false,
  default_upstream: '',
  default_strategy: 'adaptive',
  adaptive_interval_minutes: 60,
  adaptive_last_run: '',
  adaptive_current: '',
  adaptive_last_latency_ms: 0,
  adaptive_last_note: '',
  profiles: {},
  upstreams: []
};

let resolutionLoading = false;
let adaptiveRunInFlight = false;

function singleServerStorageKey() {
  const adminPath = window.location.pathname.split('/').filter(Boolean)[0] || 'root';
  return `digitdns:last-single-server:${adminPath}`;
}

function rememberedSingleServer() {
  try {
    return localStorage.getItem(singleServerStorageKey()) || '';
  } catch (e) {
    return '';
  }
}

function rememberSingleServer(address) {
  if (!address) return;
  try {
    localStorage.setItem(singleServerStorageKey(), address);
  } catch (e) { /* private mode / storage denied: the pin still works */ }
}
let adaptiveFlash = false;
let adaptiveFlashTimer = null;

// The DNSSEC card in Settings mirrors /api/settings/dnssec. The endpoint is
// startup-only: saving writes the "dns" record, and restart_required tells the
// operator that the running validator still uses the previous policy.
let dnssecSettings = {
  loaded: false,
  enabled: false,
  trust_anchors: [],
  default_anchors: [],
  running: false,
  restart_required: false
};
let dnssecLoading = false;

function normalizeDNSSECSettings(settings) {
  const out = settings || {};
  out.loaded = true;
  out.enabled = !!out.enabled;
  out.trust_anchors = Array.isArray(out.trust_anchors) ? out.trust_anchors : [];
  out.default_anchors = Array.isArray(out.default_anchors) ? out.default_anchors : [];
  out.running = !!out.running;
  out.restart_required = !!out.restart_required;
  return out;
}

async function loadDNSSECSettings() {
  if (dnssecLoading) return;
  dnssecLoading = true;
  try {
    const res = await fetch(api('/api/settings/dnssec'), {
      headers: { 'Authorization': `Bearer ${authToken}` }
    });
    if (!res.ok) return;
    dnssecSettings = normalizeDNSSECSettings(await res.json());
    syncDNSSECControls();
  } catch (e) { /* keep the markup defaults; the Save button still works */ }
  finally {
    dnssecLoading = false;
    // A failed read must not leave the card without controls; fall back to the
    // markup's OFF state rather than an invisible switch.
    if (!dnssecSettings.loaded) revealDNSSECControls();
  }
}

// The DNSSEC controls start hidden in the markup: the switch is unchecked until
// the saved policy arrives, and revealing it before then made it animate from
// OFF to its real position on every refresh. Hiding the wrapper keeps its space
// reserved, so the card does not jump, and the switch is only shown once it
// already carries the actual state.
function revealDNSSECControls() {
  const controls = document.getElementById('dnssec-controls');
  if (!controls) return;
  controls.style.visibility = 'visible';
  controls.classList.remove('boot-hide');
}

function syncDNSSECControls() {
  const enabled = document.getElementById('dnssec-enabled-input');
  if (enabled) enabled.checked = dnssecSettings.enabled;

  const anchors = document.getElementById('dnssec-anchors-input');
  if (anchors && document.activeElement !== anchors) {
    anchors.value = (dnssecSettings.trust_anchors || []).join('\n');
  }

  const defaults = document.getElementById('dnssec-default-anchors');
  if (defaults) defaults.textContent = (dnssecSettings.default_anchors || []).join('\n');

  // The badge reflects the RUNNING policy, not the saved checkbox: after a save
  // the two differ until the engine restarts, and that is the whole point of
  // the restart note below it.
  const badge = document.getElementById('dnssec-status-badge');
  if (badge) {
    const running = dnssecSettings.running;
    badge.textContent = running ? 'RUNNING' : 'OFF';
    badge.classList.toggle('bg-emerald-500/20', running);
    badge.classList.toggle('text-emerald-300', running);
    badge.classList.toggle('border-emerald-500/30', running);
    badge.classList.toggle('bg-slate-800/70', !running);
    badge.classList.toggle('text-slate-400', !running);
    badge.classList.toggle('border-slate-700', !running);
  }

  const note = document.getElementById('dnssec-restart-note');
  if (note) note.classList.toggle('hidden', !dnssecSettings.restart_required);

  const status = document.getElementById('dnssec-save-status');
  if (status) {
    status.textContent = dnssecSettings.restart_required ? 'Saved — restart required.' : '';
  }

  revealDNSSECControls();
}

async function saveDNSSECSettings() {
  const enabled = document.getElementById('dnssec-enabled-input');
  const anchors = document.getElementById('dnssec-anchors-input');
  const status = document.getElementById('dnssec-save-status');
  const payload = {
    enabled: !!(enabled && enabled.checked),
    trust_anchors: (anchors ? anchors.value : '')
      .split('\n')
      .map(line => line.trim())
      .filter(Boolean)
  };
  if (status) status.textContent = 'Saving…';
  try {
    const res = await fetch(api('/api/settings/dnssec'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${authToken}` },
      body: JSON.stringify(payload)
    });
    if (!res.ok) {
      if (status) status.textContent = '';
      showToast(await errorMessage(res, 'Could not save the DNSSEC settings'), 'error');
      return;
    }
    dnssecSettings = normalizeDNSSECSettings(await res.json());
    syncDNSSECControls();
    if (status) status.textContent = dnssecSettings.restart_required ? 'Saved — restart required.' : 'Applied.';
    showToast(dnssecSettings.restart_required
      ? 'DNSSEC settings saved — restart to apply'
      : 'DNSSEC settings applied', 'success');
  } catch (e) {
    if (status) status.textContent = '';
    showToast('Could not save the DNSSEC settings', 'error');
  }
}

// Proxy IPv6 is the server-side counterpart of the Android/agent path: the
// public address is stored in ServerSettings and the live switch is applied by
// the backend. The web form deliberately requires a global IPv6 literal before
// the switch can be saved on; the server validates it again.
let proxyIPv6Saving = false;

function revealProxyIPv6Controls() {
  const controls = document.getElementById('proxy-ipv6-controls');
  if (!controls) return;
  controls.style.visibility = 'visible';
  controls.classList.remove('boot-hide');
}

function renderProxyIPv6Controls() {
  const server = currentConfig?.server || {};
  const input = document.getElementById('proxy-ipv6-input');
  if (input && document.activeElement !== input) {
    input.value = server.public_ipv6 || '';
  }
  const toggle = document.getElementById('proxy-ipv6-enabled');
  if (toggle) toggle.checked = !!server.proxy_ipv6_enabled;
  const on = !!server.proxy_ipv6_enabled;
  const badge = document.getElementById('proxy-ipv6-badge');
  if (badge) {
    badge.textContent = on ? 'ON' : 'OFF';
    badge.classList.toggle('bg-emerald-500/20', on);
    badge.classList.toggle('text-emerald-300', on);
    badge.classList.toggle('border-emerald-500/30', on);
    badge.classList.toggle('bg-slate-800/70', !on);
    badge.classList.toggle('text-slate-400', !on);
    badge.classList.toggle('border-slate-700', !on);
  }
  document.getElementById('proxy-ipv6-off-note')?.classList.toggle('hidden', on);
  document.getElementById('proxy-ipv6-on-note')?.classList.toggle('hidden', !on);
  revealProxyIPv6Controls();
}

async function saveProxyIPv6Settings() {
  if (proxyIPv6Saving) return;
  const input = document.getElementById('proxy-ipv6-input');
  const toggle = document.getElementById('proxy-ipv6-enabled');
  const address = (input?.value || '').trim();
  const enabled = !!(toggle && toggle.checked);
  if (enabled && !address) {
    showToast('Enter a global IPv6 address before enabling Proxy IPv6.', 'error');
    input?.focus();
    return;
  }
  const status = document.getElementById('proxy-ipv6-status');
  const btn = document.getElementById('save-proxy-ipv6-btn');
  proxyIPv6Saving = true;
  if (btn) btn.disabled = true;
  if (status) status.textContent = 'Saving…';
  try {
    const res = await fetch(api('/api/settings'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${authToken}` },
      body: JSON.stringify({ public_ipv6: address, proxy_ipv6_enabled: enabled })
    });
    if (!res.ok) throw new Error(await errorMessage(res, 'Could not save IPv6 settings'));
    const data = await res.json();
    if (!currentConfig.server) currentConfig.server = {};
    currentConfig.server.public_ipv6 = data.public_ipv6 || '';
    currentConfig.server.proxy_ipv6_enabled = data.proxy_ipv6_enabled === true;
    renderProxyIPv6Controls();
    if (typeof renderAgents === 'function') renderAgents();
    if (status) status.textContent = 'Saved.';
    showToast(enabled ? 'Proxy IPv6 enabled' : 'Proxy IPv6 disabled', enabled ? 'success' : 'warning');
  } catch (e) {
    if (status) status.textContent = e.message || 'Could not save IPv6 settings';
    showToast(e.message || 'Could not save IPv6 settings', 'error');
  } finally {
    proxyIPv6Saving = false;
    if (btn) btn.disabled = false;
  }
}

function initProxyIPv6Events() {
  const toggle = document.getElementById('proxy-ipv6-enabled');
  if (toggle && !toggle.dataset.bound) {
    toggle.dataset.bound = '1';
    toggle.addEventListener('change', async () => {
      if (toggle.checked) return;
      const ok = await confirmAction({
        destructive: true,
        title: 'Turn off Proxy IPv6?',
        hint: 'PROXIED AAAA WILL BE WITHHELD',
        message: 'Clients will be forced onto the IPv4 relay. HTTPS/SVCB hints stay blocked, and direct domains keep their normal AAAA.',
        confirmText: 'TURN OFF',
      });
      if (!ok) toggle.checked = true;
    });
  }
  const btn = document.getElementById('save-proxy-ipv6-btn');
  if (btn && !btn.dataset.bound) {
    btn.dataset.bound = '1';
    btn.addEventListener('click', saveProxyIPv6Settings);
  }
}

// Proxy Leak Protection is a live, global policy: proxied names withhold
// HTTPS/SVCB so browsers cannot discover QUIC or origin address hints. The
// guard is recommended on, so the switch starts checked and a confirmation is
// required before turning it off.
let proxyLeakSettings = { loaded: false, enabled: true, recommended: true };
let proxyLeakLoading = false;

async function loadProxyLeakSettings() {
  if (proxyLeakLoading) return;
  proxyLeakLoading = true;
  try {
    const res = await fetch(api('/api/settings/proxy-leak'), {
      headers: { 'Authorization': `Bearer ${authToken}` }
    });
    if (!res.ok) return;
    const data = await res.json();
    proxyLeakSettings = {
      loaded: true,
      enabled: data.enabled !== false,
      recommended: data.recommended !== false
    };
    syncProxyLeakControls();
  } catch (e) { /* keep the recommended default */ }
  finally {
    proxyLeakLoading = false;
    // Same flicker rule as DNSSEC: show the controls only once they carry the
    // server's value. A failed read falls back to the recommended ON default.
    if (!proxyLeakSettings.loaded) revealProxyLeakControls();
  }
}

function revealProxyLeakControls() {
  const controls = document.getElementById('proxy-leak-controls');
  if (!controls) return;
  controls.style.visibility = 'visible';
  controls.classList.remove('boot-hide');
}

function syncProxyLeakControls() {
  const input = document.getElementById('proxy-leak-enabled-input');
  if (input) input.checked = proxyLeakSettings.enabled;

  const badge = document.getElementById('proxy-leak-badge');
  if (badge) {
    badge.textContent = proxyLeakSettings.enabled ? 'ACTIVE' : 'OFF';
    badge.classList.toggle('bg-emerald-500/20', proxyLeakSettings.enabled);
    badge.classList.toggle('text-emerald-300', proxyLeakSettings.enabled);
    badge.classList.toggle('border-emerald-500/30', proxyLeakSettings.enabled);
    badge.classList.toggle('bg-slate-800/70', !proxyLeakSettings.enabled);
    badge.classList.toggle('text-slate-400', !proxyLeakSettings.enabled);
    badge.classList.toggle('border-slate-700', !proxyLeakSettings.enabled);
  }

  const offNote = document.getElementById('proxy-leak-off-note');
  if (offNote) offNote.classList.toggle('hidden', proxyLeakSettings.enabled);

  revealProxyLeakControls();
}

async function saveProxyLeakSettings(enabled, message) {
  try {
    const res = await fetch(api('/api/settings/proxy-leak'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${authToken}` },
      body: JSON.stringify({ enabled })
    });
    if (!res.ok) {
      showToast(await errorMessage(res, 'Could not save proxy leak protection'), 'error');
      return false;
    }
    const data = await res.json();
    proxyLeakSettings = {
      loaded: true,
      enabled: data.enabled === true,
      recommended: data.recommended !== false
    };
    syncProxyLeakControls();
    if (message) showToast(message, enabled ? 'success' : 'warning');
    return true;
  } catch (e) {
    showToast('Could not save proxy leak protection', 'error');
    return false;
  }
}

async function loadResolutionSettings() {
  if (resolutionLoading) return;
  resolutionLoading = true;
  try {
    const res = await fetch(api('/api/settings/resolution'), {
      headers: { 'Authorization': `Bearer ${authToken}` }
    });
    if (!res.ok) return;
    resolutionSettings = normalizeResolutionSettings(await res.json());
    syncResolutionControls();
    refreshProxiedListIfVisible();
  } catch (e) { /* keep the defaults; the dialog still works */ }
  finally { resolutionLoading = false; }
}

// The resolver transport chips and the Client Transports switches are one
// backend policy. A chip save rebinds listeners, so refresh the card that shows
// their live state instead of waiting for the next full config poll.
async function refreshTransportCard() {
  if (!currentConfig || !authToken) return;
  try {
    const res = await fetch(api('/api/settings/transports'), {
      headers: { 'Authorization': `Bearer ${authToken}` }
    });
    if (!res.ok) return;
    currentConfig.transports = normalizeTransportSettings({ transports: await res.json() });
    // A resolver-chip save changes the applied transport policy outside the
    // Client Transports form. Drop any staged draft so the switches show the
    // server's new state instead of the pre-chip draft.
    transportDraft = null;
    renderTransportSettings();
  } catch (e) { /* the next config poll will catch up */ }
}

// normalizeStrategyValue maps retired spellings onto the four strategies the
// panel offers. "fastest" and the old implicit empty value are Hedged now; an
// unknown value also falls back to Hedged rather than leaving the select blank.
function normalizeStrategyValue(value) {
  const v = String(value || '').trim().toLowerCase();
  if (v === 'adaptive' || v === 'sequential' || v === 'best') return v;
  // Empty is the legacy "never chose" value and now means the adaptive default;
  // "fastest" is the retired race-every-resolver mode and reads as Hedged.
  if (v === '') return 'adaptive';
  return 'hedged';
}

function normalizeAdaptiveInterval(value) {
  const n = Number(value);
  return [5, 15, 60, 360, 1440].includes(n) ? n : 60;
}

// normalizeResolutionSettings fills the fields an older server may omit, so the
// controls never bind to undefined.
function normalizeResolutionSettings(settings) {
  const out = settings || {};
  out.loaded = true;
  out.transport = out.transport || (out.encrypted_only ? 'encrypted' : 'any');
  out.encrypted_available = !!out.encrypted_available;
  out.encrypted_upstreams = out.encrypted_upstreams || [];
  out.profiles = out.profiles || {};
  out.upstreams = out.upstreams || [];
  out.default_upstream = out.default_upstream || '';
  out.default_strategy = normalizeStrategyValue(out.default_strategy);
  out.transports = normalizeTransports(out.transports, out.transport);
  out.adaptive_interval_minutes = normalizeAdaptiveInterval(out.adaptive_interval_minutes);
  out.adaptive_last_run = out.adaptive_last_run || '';
  out.adaptive_current = out.adaptive_current || '';
  out.adaptive_last_latency_ms = Number(out.adaptive_last_latency_ms) || 0;
  out.adaptive_last_note = out.adaptive_last_note || '';
  return out;
}

// The server's encrypted set is authoritative, but a stale settings snapshot
// must not hide a live DoT/DoH/DoQ resolver. The scheme fallback keeps the
// transport picker and the dimming honest when the list lags.
function looksEncrypted(address) {
  const a = String(address || '').trim().toLowerCase();
  return ['tls://', 'https://', 'quic://', 'h3://', 'sdns://'].some(p => a.startsWith(p));
}

function upstreamIsEncrypted(address) {
  return (resolutionSettings.encrypted_upstreams || []).includes(address) || looksEncrypted(address);
}

const ALL_TRANSPORTS = ['plain', 'dot', 'doh', 'doh3', 'doq', 'dnscrypt'];

function protocolsForTransport(transport) {
  if (transport === 'encrypted') return ['dot', 'doh', 'doh3', 'doq', 'dnscrypt'];
  if (transport === 'regular') return ['plain'];
  return [...ALL_TRANSPORTS];
}

function normalizeTransports(value, legacyTransport) {
  const list = Array.isArray(value) ? value : [];
  const seen = new Set(list.map(v => String(v || '').toLowerCase()).filter(v => ALL_TRANSPORTS.includes(v)));
  if (!seen.size) return protocolsForTransport(legacyTransport);
  return ALL_TRANSPORTS.filter(p => seen.has(p));
}

function upstreamProtocolKey(address) {
  const a = String(address || '').trim().toLowerCase();
  if (a.startsWith('tls://')) return 'dot';
  if (a.startsWith('https://')) return 'doh';
  if (a.startsWith('h3://')) return 'doh3';
  if (a.startsWith('quic://')) return 'doq';
  if (a.startsWith('sdns://')) return 'dnscrypt';
  return 'plain';
}

function upstreamProtocolEnabled(address) {
  return (resolutionSettings.transports || ALL_TRANSPORTS).includes(upstreamProtocolKey(address));
}

// Provider marks use the bundled images, never a remote favicon request. The
// address patterns cover the public resolvers the preset catalogue ships with.
function providerIdForUpstream(address) {
  const a = String(address || '').trim().toLowerCase();
  if (a.includes('cloudflare') || a.includes('1.1.1.1') || a.includes('1.0.0.1') || a.includes('one.one.one.one')) return 'cloudflare';
  if (a.includes('google') || a.includes('8.8.8.8') || a.includes('8.8.4.4')) return 'google';
  if (a.includes('quad9') || a.includes('9.9.9.9') || a.includes('149.112.112.112')) return 'quad9';
  if (a.includes('adguard') || a.includes('94.140.14.14') || a.includes('94.140.15.15')) return 'adguard';
  if (a.includes('controld') || a.includes('control d') || a.includes('freedns') || a.includes('76.76.2.0') || a.includes('76.76.10.0')) return 'controld';
  if (a.includes('dns.sb') || a.includes('dot.sb') || a.includes('doh.dns.sb') || a.includes('185.222.222.222') || a.includes('45.11.45.11')) return 'dnssb';
  if (a.includes('opendns') || a.includes('208.67.222.222') || a.includes('208.67.220.220')) return 'opendns';
  if (a.includes('yandex') || a.includes('77.88.8.8') || a.includes('77.88.8.1')) return 'yandex';
  if (a.includes('nextdns') || a.includes('45.90.28.0') || a.includes('45.90.30.0')) return 'nextdns';
  if (a.includes('cleanbrowsing') || a.includes('185.228.168.9') || a.includes('185.228.169.9')) return 'cleanbrowsing';
  return '';
}

function providerFaviconMarkup(address, extraClass = '') {
  const id = providerIdForUpstream(address);
  const cls = `resolver-favicon${extraClass ? ' ' + extraClass : ''}`;
  if (id) {
    return `<img class="${escapeHTML(cls)}" src="${escapeHTML(api(`/img/providers/${encodeURIComponent(id)}.png`))}" alt="">`;
  }
  return `<span class="${escapeHTML(cls)} resolver-favicon-generic" aria-hidden="true">${FEATHER_GLOBE_SVG}</span>`;
}

// availableUpstreams prefers the settings list and falls back to the last
// stats payload, so the pickers work even if the settings call failed.
function availableUpstreams() {
  // Merge rather than choose: a settings snapshot can predate a resolver that
  // the stats poll already sees, and both the pickers and the dimming must know
  // about every live resolver.
  const merged = [...(resolutionSettings.upstreams || [])];
  for (const u of (lastUpstreamsPayload || []).map(x => x.address)) {
    if (!merged.includes(u)) merged.push(u);
  }
  return merged;
}

function hasEncryptedUpstream() {
  return availableUpstreams().some(upstreamIsEncrypted);
}

function hasRegularUpstream() {
  return availableUpstreams().some(u => !upstreamIsEncrypted(u));
}

function transportExcludes(address, transport) {
  const mode = transport || 'any';
  if (mode === 'encrypted') return !upstreamIsEncrypted(address);
  if (mode === 'regular') return upstreamIsEncrypted(address);
  return false;
}

function transportLabel(mode) {
  if (mode === 'encrypted') return 'Encrypted Only';
  if (mode === 'regular') return 'Regular DNS';
  return 'Any Transport';
}

// activePin is the Default upstream only when the transport allows it. The pool
// ignores an ineligible pin too, so it must not dim the rest of the list.
function activePin(transport) {
  const pinned = resolutionSettings.default_upstream || '';
  if (!pinned || !upstreamProtocolEnabled(pinned) || transportExcludes(pinned, transport)) return '';
  return pinned;
}

function upstreamDimmed(address, transport) {
  if (!upstreamProtocolEnabled(address)) return true;
  if (transportExcludes(address, transport)) return true;
  const pin = activePin(transport);
  return !!pin && pin !== address;
}

// syncAdaptiveControls keeps the interval row and its status line in the grid at
// all times. Only the enabled state and the text change, so selecting Adaptive
// does not push the card taller or make the controls below it jump.
function syncAdaptiveControls() {
  const adaptive = normalizeStrategyValue(resolutionSettings.default_strategy) === 'adaptive';
  const single = !!resolutionSettings.default_upstream;
  const interval = normalizeAdaptiveInterval(resolutionSettings.adaptive_interval_minutes);

  document.querySelectorAll('.resolver-chip-block').forEach(block => {
    block.classList.toggle('is-disabled', single);
  });
  document.querySelectorAll('#resolution-strategy-chips [data-strategy]').forEach(button => {
    button.disabled = single;
    button.setAttribute('aria-pressed', button.dataset.strategy === normalizeStrategyValue(resolutionSettings.default_strategy) ? 'true' : 'false');
  });
  document.querySelectorAll('#resolution-interval-chips [data-interval]').forEach(button => {
    button.disabled = single || !adaptive;
    button.setAttribute('aria-pressed', Number(button.dataset.interval) === interval ? 'true' : 'false');
  });

  const run = document.getElementById('resolution-adaptive-run');
  if (run) {
    run.disabled = single || !adaptive || adaptiveRunInFlight;
    run.classList.toggle('is-running', adaptiveRunInFlight);
    run.classList.toggle('is-success', adaptiveFlash && !single);
    run.setAttribute('aria-busy', adaptiveRunInFlight ? 'true' : 'false');
  }
  const forceLabel = document.getElementById('resolution-adaptive-force-label');
  if (forceLabel) {
    forceLabel.classList.toggle('resolution-adaptive-disabled', single || !adaptive);
  }

  const note = document.getElementById('resolution-adaptive-note');
  if (!note) return;
  if (single || !adaptive) {
    // A single upstream bypasses the strategy/interval/transport controls;
    // keep the reserved line blank rather than adding more text.
    note.textContent = '';
    note.title = '';
    return;
  }

  const current = resolutionSettings.adaptive_current || '';
  const lastRun = resolutionSettings.adaptive_last_run || '';
  if (!lastRun) {
    note.textContent = 'No benchmark yet.';
    note.title = '';
    return;
  }
  const when = new Date(lastRun);
  const stamp = Number.isNaN(when.getTime()) ? lastRun : when.toLocaleString();
  note.textContent = `Last run ${stamp} · using ${current || 'no healthy resolver selected'}`;
  note.title = '';
}

function syncTransportChips() {
  const transports = normalizeTransports(resolutionSettings.transports, resolutionSettings.transport);
  const single = !!resolutionSettings.default_upstream;
  document.querySelectorAll('#resolution-transport-chips [data-transport]').forEach(button => {
    const on = transports.includes(button.dataset.transport);
    button.setAttribute('aria-pressed', on ? 'true' : 'false');
    // The last enabled chip cannot be turned off: the pool would have no
    // candidates. A single upstream bypasses the protocol filter entirely.
    button.disabled = single || (on && transports.length === 1);
  });
}

function syncSingleUpstream() {
  const button = document.getElementById('resolution-single-upstream');
  const icon = document.getElementById('resolution-single-icon');
  const label = document.getElementById('resolution-single-label');
  const toggle = document.getElementById('resolution-single-switch');
  const mode = document.querySelector('.resolver-mode-switch');
  const pinned = resolutionSettings.default_upstream || '';
  const available = availableUpstreams();
  if (pinned) rememberSingleServer(pinned);
  if (icon) icon.innerHTML = providerFaviconMarkup(pinned);
  if (label) label.textContent = pinned || 'Pool (automatic)';
  if (toggle) {
    toggle.checked = !!pinned;
    toggle.disabled = available.length === 0;
  }
  if (mode) mode.classList.toggle('is-disabled', available.length === 0);
  if (button) {
    button.disabled = !pinned;
    button.classList.toggle('is-dimmed', !pinned);
    button.title = pinned
      ? 'Choose the resolver used instead of the pool'
      : 'Turn on Single Server to choose one resolver';
  }
}

const PROTOCOL_CHIP_LABELS = {
  plain: 'Plain',
  dot: 'DoT',
  doh: 'DoH',
  doh3: 'DoH3',
  doq: 'DoQ',
  dnscrypt: 'DNSCrypt',
};

function renderSingleUpstreamMenu() {
  const menu = document.getElementById('resolution-single-menu');
  if (!menu) return;
  const pinned = resolutionSettings.default_upstream || '';
  const upstreams = availableUpstreams();
  if (pinned && !upstreams.includes(pinned)) upstreams.push(pinned);
  const items = [];
  for (const address of upstreams) {
    const protocol = upstreamProtocolKey(address);
    items.push(`<button type="button" class="resolver-single-item" role="option" aria-selected="${escapeHTML(address === pinned ? 'true' : 'false')}" data-upstream="${escapeHTML(address)}">
      ${providerFaviconMarkup(address)}
      <span class="resolver-single-label">${escapeHTML(address)}</span>
      <span class="resolver-single-proto proto-${escapeHTML(protocol)}">${escapeHTML(PROTOCOL_CHIP_LABELS[protocol] || protocol)}</span>
    </button>`);
  }
  menu.innerHTML = items.join('');
}

function setSingleUpstreamMenu(open) {
  const menu = document.getElementById('resolution-single-menu');
  const button = document.getElementById('resolution-single-upstream');
  if (!menu || !button) return;
  if (open) renderSingleUpstreamMenu();
  menu.classList.toggle('hidden', !open);
  button.setAttribute('aria-expanded', open ? 'true' : 'false');
}

function closeSingleUpstreamMenu() {
  setSingleUpstreamMenu(false);
}

// syncResolutionControls repaints every chip and the Single Upstream picker.
// It also forces the resolver list to repaint, because a changed protocol or pin
// changes which rows are dimmed and which one is the current Adaptive choice.
function syncResolutionControls() {
  syncTransportChips();
  syncAdaptiveControls();
  syncSingleUpstream();
  if (lastUpstreamsPayload) {
    lastUpstreamsSig = '';
    renderUpstreams(lastUpstreamsPayload);
  }
}

// The Domains table is expensive to rebuild and invisible while another tab is
// showing. Background resolution refreshes (the adaptive last-run poll) should
// not rewrite it; switchTab('proxied') and the resolution save path call it when
// the table is actually on screen.
function refreshProxiedListIfVisible() {
  if (activeDashTab === 'proxied') renderProxiedList();
}

// saveResolutionSettings posts the current state with the given overrides and
// mirrors the server's answer back into the controls.
async function saveResolutionSettings(overrides, message) {
  // Saving from an unloaded snapshot would post an empty profile map and wipe
  // the operator's per-domain rules.
  if (!resolutionSettings.loaded) {
    showToast('Could not load the resolution settings — reload the page', 'error');
    return false;
  }
  const payload = Object.assign({
    transport: resolutionSettings.transport || 'any',
    transports: normalizeTransports(resolutionSettings.transports, resolutionSettings.transport),
    default_upstream: resolutionSettings.default_upstream || '',
    default_strategy: normalizeStrategyValue(resolutionSettings.default_strategy),
    adaptive_interval_minutes: normalizeAdaptiveInterval(resolutionSettings.adaptive_interval_minutes),
    profiles: resolutionSettings.profiles || {}
  }, overrides || {});
  try {
    const res = await fetch(api('/api/settings/resolution'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${authToken}` },
      body: JSON.stringify(payload)
    });
    if (!res.ok) {
      showToast(await errorMessage(res, 'Could not save the resolution settings'), 'error');
      return false;
    }
    resolutionSettings = normalizeResolutionSettings(await res.json());
    syncResolutionControls();
    refreshProxiedListIfVisible();
    void refreshTransportCard();
    if (message) showToast(message, 'success');
    return true;
  } catch (e) {
    showToast('Could not save the resolution settings', 'error');
    return false;
  }
}

// Force one Adaptive benchmark/selection cycle instead of waiting for the
// interval. The server runs the probe synchronously and returns the updated
// settings, so the card can show the new resolver immediately.
async function runAdaptiveNow() {
  if (adaptiveRunInFlight) return;
  if (normalizeStrategyValue(resolutionSettings.default_strategy) !== 'adaptive') return;
  adaptiveRunInFlight = true;
  adaptiveFlash = false;
  if (adaptiveFlashTimer) clearTimeout(adaptiveFlashTimer);
  syncAdaptiveControls();
  try {
    const res = await fetch(api('/api/settings/resolution/run-adaptive'), {
      method: 'POST',
      headers: { 'Authorization': `Bearer ${authToken}` }
    });
    if (!res.ok) {
      showToast(await errorMessage(res, 'Could not run the adaptive check'), 'error');
      return;
    }
    resolutionSettings = normalizeResolutionSettings(await res.json());
    adaptiveFlash = true;
    syncResolutionControls();
    // Repaint the resolver rows now: the highlight and FASTEST badge depend on
    // adaptive_current, and waiting for the next stats poll made Force look like
    // it had not switched.
    if (Array.isArray(lastUpstreamsPayload)) renderUpstreams(lastUpstreamsPayload);
    refreshProxiedListIfVisible();
    showToast('Adaptive check complete', 'success');
    if (adaptiveFlashTimer) clearTimeout(adaptiveFlashTimer);
    adaptiveFlashTimer = setTimeout(() => {
      adaptiveFlash = false;
      syncAdaptiveControls();
    }, 1600);
  } catch (e) {
    showToast('Could not run the adaptive check', 'error');
  } finally {
    adaptiveRunInFlight = false;
    syncAdaptiveControls();
  }
}

// The transport option markup shared by the dialog. Availability is expressed
// by disabling, never by an "unavailable" label.
function transportOptionsMarkup(selected) {
  const sel = (v) => (selected === v ? 'selected' : '');
  const enc = hasEncryptedUpstream();
  const reg = hasRegularUpstream();
  return `<option value="any" ${sel('any')}>Any Transport</option>`
    + `<option value="encrypted" ${sel('encrypted')} ${enc ? '' : 'disabled'}>Encrypted Only (DoT · DoH · DoQ)</option>`
    + `<option value="regular" ${sel('regular')} ${reg ? '' : 'disabled'}>Regular DNS (UDP · TCP)</option>`;
}

// openResolutionDialog edits the rule for one proxied domain. The domain is
// fixed: the list that opened it owns the rule.
function openResolutionDialog(domain) {
  const saved = resolutionSettings.profiles[domain] || {};
  // Fastest is retired; a saved legacy rule reads as Hedged in this dialog.
  const savedStrategy = saved.strategy === 'fastest' ? 'hedged' : (saved.strategy || '');
  // The resolver decides the transport. An old rule with a conflicting pair is
  // shown in the consistent form the pool can actually honour.
  const savedTransport = saved.upstream
    ? (upstreamIsEncrypted(saved.upstream) ? 'encrypted' : 'regular')
    : (saved.transport || (saved.encrypted_only === true ? 'encrypted'
      : saved.encrypted_only === false ? 'any' : 'inherit'));
  const transportOptions = ['<option value="inherit" ' + (savedTransport === 'inherit' ? 'selected' : '') + '>Inherit Global</option>']
    .concat(transportOptionsMarkup(savedTransport))
    .join('');

  const backdrop = document.createElement('div');
  // A light scrim, not a blackout: the page stays readable behind the dialog.
  backdrop.className = 'fixed inset-0 z-50 flex items-center justify-center bg-slate-950/25 px-4';
  backdrop.innerHTML = `
    <div class="rounded-2xl bg-slate-900 p-5 w-full max-w-md border border-cyan-500/40 shadow-2xl shadow-cyan-500/10" role="dialog" aria-modal="true">
      <h2 class="text-base font-bold text-white tracking-wide font-heading">Override Resolving</h2>
      <p class="text-[11px] text-slate-400 mt-0.5 mb-3 font-mono break-all">${escapeHTML(domain)}</p>
      <p class="text-[11px] text-slate-500 mb-3">Overrides the global defaults for this proxied domain. It applies when the name is not being spoofed (paused) or for query types other than A/AAAA.</p>
      <div class="space-y-2.5 text-xs">
        <label class="block">
          <span class="block text-[11px] font-semibold text-slate-400 mb-1">Upstream</span>
          <select id="resolution-upstream" class="w-full bg-slate-950 border border-slate-700 text-xs text-slate-200 rounded-lg px-2.5 py-1.5 focus:outline-none focus:border-cyan-400 font-mono"></select>
        </label>
        <p id="resolution-dialog-pin-note" class="hidden text-[10px] text-amber-400">The selected upstream is excluded by this transport and will be ignored.</p>
        <label class="block">
          <span class="block text-[11px] font-semibold text-slate-400 mb-1">Strategy</span>
          <select id="resolution-strategy" title="Hedged starts the next resolver after 50 ms; Best Known uses the fastest measured resolver" class="w-full bg-slate-950 border border-slate-700 text-xs text-slate-200 rounded-lg px-2.5 py-1.5 focus:outline-none focus:border-cyan-400 font-mono">
            <option value="" ${savedStrategy ? '' : 'selected'}>Inherit Global</option>
            <option value="sequential" ${savedStrategy === 'sequential' ? 'selected' : ''}>In Order</option>
            <option value="hedged" ${savedStrategy === 'hedged' ? 'selected' : ''}>Hedged</option>
            <option value="best" ${savedStrategy === 'best' ? 'selected' : ''}>Best Known</option>
          </select>
        </label>
        <label class="block">
          <span class="block text-[11px] font-semibold text-slate-400 mb-1">Transports</span>
          <select id="resolution-dialog-transport" class="w-full bg-slate-950 border border-slate-700 text-xs text-slate-200 rounded-lg px-2.5 py-1.5 focus:outline-none focus:border-cyan-400 font-mono">${transportOptions}</select>
        </label>
      </div>
      <div class="flex gap-2 mt-5">
        <button id="resolution-cancel" class="flex-1 py-2 rounded-lg bg-slate-800 border border-slate-700 text-slate-300 text-sm font-semibold hover:bg-slate-700 transition">Cancel</button>
        <button id="resolution-save" class="flex-1 py-2 rounded-lg bg-cyan-500 hover:bg-cyan-400 text-slate-950 text-sm font-bold font-heading transition">Save</button>
      </div>
    </div>`;
  document.body.appendChild(backdrop);
  const transportSelect = backdrop.querySelector('#resolution-dialog-transport');
  const upstreamSelect = backdrop.querySelector('#resolution-upstream');
  let chosenUpstream = saved.upstream || '';
  const dialogUpstreams = () => {
    const list = availableUpstreams();
    // Keep a saved pin visible even if it has been removed from the pool.
    return chosenUpstream && !list.includes(chosenUpstream) ? list.concat(chosenUpstream) : list;
  };
  const effectiveTransport = () => transportSelect.value === 'inherit'
    ? (resolutionSettings.transport || 'any')
    : transportSelect.value;
  const updateDialogPinNote = () => {
    const ignored = !!chosenUpstream && transportExcludes(chosenUpstream, effectiveTransport());
    backdrop.querySelector('#resolution-dialog-pin-note')?.classList.toggle('hidden', !ignored);
  };
  const renderDialogUpstreams = () => {
    const options = ['<option value="">Inherit Global</option>']
      .concat(dialogUpstreams().map(u =>
        `<option value="${escapeHTML(u)}" ${chosenUpstream === u ? 'selected' : ''}>${escapeHTML(u)}</option>`));
    upstreamSelect.innerHTML = options.join('');
    upstreamSelect.value = chosenUpstream;
    updateDialogPinNote();
  };
  // The two fields stay consistent in both directions: a chosen resolver sets
  // its transport, and a chosen transport selects a resolver of that type. A
  // contradictory pair can no longer be saved.
  upstreamSelect.addEventListener('change', () => {
    chosenUpstream = upstreamSelect.value;
    if (chosenUpstream) {
      transportSelect.value = upstreamIsEncrypted(chosenUpstream) ? 'encrypted' : 'regular';
    }
    updateDialogPinNote();
  });
  transportSelect.addEventListener('change', () => {
    const choice = transportSelect.value;
    if (choice === 'encrypted' || choice === 'regular') {
      const wantEncrypted = choice === 'encrypted';
      if (!chosenUpstream || upstreamIsEncrypted(chosenUpstream) !== wantEncrypted) {
        chosenUpstream = availableUpstreams().find(u => upstreamIsEncrypted(u) === wantEncrypted) || '';
        renderDialogUpstreams();
      }
    }
    updateDialogPinNote();
  });
  renderDialogUpstreams();

  const close = () => backdrop.remove();
  backdrop.addEventListener('click', (e) => { if (e.target === backdrop) close(); });
  const onKey = (e) => { if (e.key === 'Escape') { document.removeEventListener('keydown', onKey); close(); } };
  document.addEventListener('keydown', onKey);
  backdrop.querySelector('#resolution-cancel').onclick = () => { document.removeEventListener('keydown', onKey); close(); };
  backdrop.querySelector('#resolution-save').onclick = async () => {
    const upstream = backdrop.querySelector('#resolution-upstream').value;
    const strategy = backdrop.querySelector('#resolution-strategy').value;
    const transport = backdrop.querySelector('#resolution-dialog-transport').value;
    const profile = {};
    if (upstream) profile.upstream = upstream;
    if (strategy) profile.strategy = strategy;
    if (transport && transport !== 'inherit') profile.transport = transport;
    const profiles = Object.assign({}, resolutionSettings.profiles);
    if (Object.keys(profile).length === 0) delete profiles[domain]; else profiles[domain] = profile;
    const ok = await saveResolutionSettings({ profiles }, 'Resolution rule saved');
    if (ok) { document.removeEventListener('keydown', onKey); close(); }
  };
}

// proxiedPatternIndex maps every stored rule pattern to the status that wins when a
// name appears in more than one list. The wildcard chips read it: a row has to know
// whether *.host, *.parent or both are active, and in which list.
function proxiedPatternIndex() {
  const rules = currentConfig?.rules || {};
  const index = new Map();
  const add = (list, status) => {
    if (!Array.isArray(list)) return;
    for (const raw of list) {
      const pattern = String(raw || '').trim().toLowerCase().replace(/\.$/, '');
      if (pattern) index.set(pattern, status);
    }
  };
  // Low precedence first, so the same name in two lists reads like a row does.
  add(rules.custom_proxied_disabled, 'paused');
  add(rules.custom_proxied, 'proxy');
  add(rules.custom_blocked, 'blocked');
  add(rules.custom_direct, 'direct');
  return index;
}

// wildcardTypesFor names the wildcards that apply to one row. A wildcard is relative
// to the row: *.example.com is the host wildcard of example.com and the parent
// wildcard of api.example.com. Both can be active at once.
function wildcardTypesFor(domain, index) {
  const wildcard = domain.startsWith('*.');
  const base = wildcard ? domain.slice(2) : domain;
  const labels = base.split('.').filter(Boolean);
  const types = [];
  if (labels.length >= 2) {
    const host = '*.' + base;
    // A plain rule covers every subdomain already, so *.host in the same list adds
    // nothing; only show it when it lives in a different list.
    const redundant = !wildcard && index.has(base) && index.get(host) === index.get(base);
    if (index.has(host) && !redundant) {
      types.push({ type: '*.host', pattern: host, status: index.get(host) });
    }
  }
  if (labels.length >= 3) {
    const parent = '*.' + labels.slice(1).join('.');
    if (index.has(parent)) types.push({ type: '*.parent', pattern: parent, status: index.get(parent) });
  }
  return types;
}

// wildcardLabelFor is the second line under a domain: the wildcard type active for
// that row, or "exact" when there is none. The title carries the stored pattern and
// its list, so the short label cannot be mistaken for the rule itself.
function wildcardLabelFor(domain, index) {
  const types = wildcardTypesFor(domain, index);
  if (!types.length) {
    return { text: 'exact', title: 'No wildcard rule for this name', wildcard: false };
  }
  const title = types.map(({ pattern, status }) => {
    const statusLabel = (DOMAIN_STATUS_META[status] || DOMAIN_STATUS_META.direct).label.toLowerCase();
    return `${pattern} — ${statusLabel}`;
  }).join(' · ');
  return { text: types.map(({ type }) => type).join(' · '), title, wildcard: true };
}

function renderProxiedList() {
  const container = document.getElementById('custom-proxied-rows');
  if (!container) return;
  closeServicePicker();
  closeRelayPicker();
  proxiedPruneSelection();
  updateDomainFilterChips();
  const needle = proxiedFilterNeedle();
  const ordered = proxiedFilteredRows();
  // The service filter is a view over the same in-memory rules, but it has to be
  // applied before the count and the pagination slice - otherwise the header says
  // "3 domains" while one row is visible and "Load more" grows a hidden list.
  const serviceScoped = activeService
    ? ordered.filter(({ domain }) => {
        const sid = serviceOfDomain(domain);
        return activeService === '__none__' ? !sid : sid === activeService;
      })
    : ordered;
  const total = serviceScoped.length;
  const serviceScope = activeService === '__none__'
    ? 'No service'
    : (activeService ? serviceName(activeService) : '');
  const countEl = document.getElementById('proxied-count');
  const moreBtn = document.getElementById('proxied-load-more');
  const statusLabel = proxiedStatusFilter
    ? { proxy: 'proxied', blocked: 'blocked', paused: 'paused', direct: 'direct' }[proxiedStatusFilter]
    : '';

  container.innerHTML = '';
  if (total === 0) {
    // Three empty states: no rules at all, no matches for the search, or none in
    // the selected status chip. They mean different things to the operator.
    const empty = needle
      ? `No domains match “${escapeHTML(needle)}”`
      : serviceScope
        ? `No ${statusLabel ? statusLabel + ' ' : ''}domains in ${escapeHTML(serviceScope)} yet.`
        : proxiedStatusFilter
          ? `No ${statusLabel} domains yet.`
          : 'No domains yet.';
    container.innerHTML = `<tr><td colspan="5" class="py-8 text-center text-slate-500 text-xs">${empty}</td></tr>`;
    proxiedHasMore = false;
    if (countEl) {
      countEl.textContent = needle
        ? '0 matches'
        : serviceScope
          ? `0 domains in ${serviceScope}`
          : (proxiedStatusFilter ? `0 ${statusLabel} domains` : '');
    }
    if (moreBtn) moreBtn.classList.add('hidden');
    updateProxiedBulkBar();
    return;
  }

  const rows = serviceScoped.slice(0, proxiedVisible);
  proxiedHasMore = rows.length < total;
  if (countEl) {
    if (needle) {
      countEl.textContent = proxiedHasMore
        ? `Showing ${rows.length} of ${total} matches`
        : `${total} match${total === 1 ? '' : 'es'}`;
    } else if (proxiedStatusFilter) {
      countEl.textContent = (proxiedHasMore
        ? `Showing ${rows.length} of ${total} ${statusLabel} domains`
        : `${total} ${statusLabel} domain${total === 1 ? '' : 's'}`) +
        (serviceScope ? ` in ${serviceScope}` : '');
    } else {
      countEl.textContent = (proxiedHasMore
        ? `Showing ${rows.length} of ${total}`
        : `${total} domain${total === 1 ? '' : 's'}`) +
        (serviceScope ? ` in ${serviceScope}` : '');
    }
  }
  if (moreBtn) moreBtn.classList.toggle('hidden', !proxiedHasMore);

  renderServices();
  const wildcardIndex = proxiedPatternIndex();
  rows.forEach(({ domain, status }) => {
    const meta = DOMAIN_STATUS_META[status] || DOMAIN_STATUS_META.direct;
    const row = document.createElement('tr');
    row.className = 'hover:bg-slate-800/40 transition' + (status === 'paused' ? ' is-paused' : '');
    // The colour classes come from a fixed status table, never from operator data;
    // they are the two entries the escaping sweep allows by name.
    const domainClass = meta.domainClass;
    const statusClass = meta.badge;
    const wildcardInfo = wildcardLabelFor(domain, wildcardIndex);
    const wildcardClass = wildcardInfo.wildcard ? 'text-purple-300' : 'text-slate-500';
    const statusBadge = `<button type="button" class="proxied-route-btn badge ${statusClass} inline-flex items-center gap-1" data-domain="${escapeHTML(domain)}" title="Proxy, block or direct ${escapeHTML(domain)}" aria-haspopup="menu" aria-expanded="false">${meta.label}</button>`;

    // Keep the same actions on every status. Paused is still a proxy state (the
    // switch resumes it and the resolution rule is valid); blocked and direct
    // keep the controls visible but disabled instead of removing them.
    const canAdjust = status === 'proxy' || status === 'paused';
    const resolutionTitle = canAdjust
      ? 'Override Resolving'
      : 'Resolution overrides are available for proxied or paused domains';
    const resolutionButton = `<button class="resolution-proxied inline-flex items-center justify-center w-7 h-7 shrink-0 rounded-lg text-slate-400 bg-slate-900 border border-slate-700 hover:text-cyan-300 hover:border-cyan-500/40 transition disabled:opacity-50 disabled:cursor-not-allowed" data-val="${escapeHTML(domain)}" ${canAdjust ? '' : 'disabled'} title="${escapeHTML(resolutionTitle)}" aria-label="${escapeHTML(resolutionTitle)}"><i data-feather="settings" class="w-4 h-4"></i></button>`;
    const pauseTitle = canAdjust ? 'Pause proxying' : 'Pause is only available for proxied domains';
    const stateLabel = status === 'proxy'
      ? '<span class="text-[10px] text-emerald-400 font-semibold">On</span>'
      : '<span class="text-[10px] text-slate-400 font-semibold">Off</span>';
    const pauseInner = `<span class="text-[10px] text-slate-500 font-semibold">Pause</span>
          <label class="switch shrink-0">
            <input type="checkbox" class="toggle-proxied" data-val="${escapeHTML(domain)}" ${status === 'proxy' ? 'checked' : ''} ${canAdjust ? '' : 'disabled'} aria-label="Toggle proxying for ${escapeHTML(domain)}">
            <span class="slider"></span>
          </label>
          ${stateLabel}`;
    const pauseControl = canAdjust
      ? `<div class="inline-flex items-center gap-2" title="${escapeHTML(pauseTitle)}">${pauseInner}</div>`
      : `<div class="pause-unavailable inline-flex items-center gap-2" title="${escapeHTML(pauseTitle)}">${pauseInner}</div>`;

    row.innerHTML = `
      <td class="py-2.5 px-3 text-center">
        <input type="checkbox" class="proxied-select-check proxied-check" data-val="${escapeHTML(domain)}" ${proxiedSelected.has(domain) ? 'checked' : ''} aria-label="Select ${escapeHTML(domain)}">
      </td>
      <td class="py-2.5 px-3 text-start" title="${escapeHTML(domain)}">
        <div class="flex flex-col min-w-0">
          <span class="font-mono ${domainClass} truncate">${escapeHTML(domain)}</span>
          <span class="text-[10px] font-mono truncate ${wildcardClass}" title="${escapeHTML(wildcardInfo.title)}">${escapeHTML(wildcardInfo.text)}</span>
        </div>
      </td>
      <td class="py-2.5 px-3 text-start proxied-service-cell"></td>
      <td class="py-2.5 px-3 text-start proxied-relay-cell"></td>
      <td class="py-2.5 px-3 text-center">
        <div class="flex items-center justify-center gap-3">
          ${resolutionButton}
          ${pauseControl}
          <button class="remove-proxied inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-slate-900 hover:bg-red-500/10 text-red-400 border border-red-500/30 text-[11px] font-semibold transition" data-val="${escapeHTML(domain)}" aria-label="Remove ${escapeHTML(domain)}">Remove</button>
        </div>
      </td>
      <td class="py-2.5 px-3 text-center">${statusBadge}</td>
    `;
    // Filled here rather than in the template: the select needs its change
    // listener, and a native select avoids inventing another popover.
    const svcCell = row.children[2];
    if (svcCell) {
      const current = serviceOfDomain(domain);
      const label = current ? ((currentConfig.rules.services || {})[current] || current) : 'No service';
      const mark = current ? serviceMarkHTML(current, label) : BULK_NONE_MARK;
      const trigger = document.createElement('button');
      trigger.type = 'button';
      trigger.className = 'proxied-service-trigger';
      trigger.dataset.serviceDomain = domain;
      trigger.setAttribute('aria-haspopup', 'listbox');
      trigger.setAttribute('aria-expanded', 'false');
      trigger.setAttribute('aria-label', `Service for ${escapeHTML(domain)}`);
      trigger.innerHTML = mark +
        '<span class="proxied-service-name"></span>' +
        '<i data-feather="chevron-down" class="w-3 h-3"></i>';
      trigger.querySelector('.proxied-service-name').textContent = label;
      trigger.addEventListener('click', (e) => {
        e.stopPropagation();
        openServicePicker(domain, trigger);
      });
      svcCell.appendChild(trigger);
    }
    const relayCell = row.children[3];
    if (relayCell) {
      const relay = effectiveRelay(domain);
      const trigger = document.createElement('button');
      trigger.type = 'button';
      trigger.className = 'proxied-relay-trigger';
      trigger.dataset.relayDomain = domain;
      trigger.setAttribute('aria-haspopup', 'listbox');
      trigger.setAttribute('aria-expanded', 'false');
      trigger.setAttribute('aria-label', `Relay for ${escapeHTML(domain)}`);
      trigger.title = 'Change the relay for this domain';
      if (relay.local) {
        const localFlag = countryFlagHTML(currentConfig?.server?.country, 'agent-flag relay-flag');
        trigger.innerHTML = (localFlag || '<i data-feather="server" class="w-3.5 h-3.5"></i>') +
          '<span class="proxied-relay-name"></span>' +
          '<i data-feather="chevron-down" class="w-3 h-3"></i>';
        trigger.querySelector('.proxied-relay-name').textContent = 'This Server';
        if (currentConfig?.server?.country) {
          trigger.title = `This Server — ${countryName(currentConfig.server.country)}`;
        }
      } else if (relay.egress) {
        const e = relay.egress;
        const isOnline = egressIsOnline(e);
        trigger.innerHTML = (e.country ? egressFlagHTML(e, 'agent-flag relay-flag') : '<i data-feather="shuffle" class="w-3.5 h-3.5"></i>') +
          '<span class="proxied-relay-name"></span>' +
          (isOnline ? '' : '<span class="proxied-relay-offline">offline</span>') +
          '<i data-feather="chevron-down" class="w-3 h-3"></i>';
        trigger.querySelector('.proxied-relay-name').textContent = e.name || 'Egress';
        trigger.title = `${e.name || 'Egress'} — ${egressKindLabel(e.kind)}${isOnline ? '' : ' (offline)'}`;
        if (!isOnline) trigger.classList.add('is-offline');
      } else {
        const agent = relay.agent;
        const isOnline = agent ? agentIsOnline(agent) : false;
        const name = agent ? (agent.name || agent.id) : (relay.id || 'Agent');
        trigger.innerHTML = (agent ? agentFlagHTML(agent, 'agent-flag relay-flag') : '') +
          '<span class="proxied-relay-name"></span>' +
          (isOnline ? '' : '<span class="proxied-relay-offline">offline</span>') +
          '<i data-feather="chevron-down" class="w-3 h-3"></i>';
        trigger.querySelector('.proxied-relay-name').textContent = name;
        if (!isOnline) trigger.classList.add('is-offline');
      }
      trigger.addEventListener('click', (e) => {
        e.stopPropagation();
        openRelayPicker(domain, trigger);
      });
      relayCell.appendChild(trigger);
    }

    row.addEventListener('click', (e) => {
      const target = e.target;
      if (target && (target.closest('input, button, label, .switch'))) return;
      const cb = row.querySelector('.proxied-select-check');
      if (cb) { cb.checked = !cb.checked; cb.dispatchEvent(new Event('change', { bubbles: true })); }
    });
    row.style.cursor = 'pointer';
    container.appendChild(row);
  });
  safeFeatherReplace();
  updateProxiedBulkBar();
}

// Size the table viewport to the space left below the toolbar and above the
// footer. The toolbar and the table's sticky header then stay on screen while
// the rows scroll in their own container. Desktop only: a phone's natural page
// scroll is better than a 180 px letterbox.
function sizeProxiedScroll() {
  const wrap = document.getElementById('custom-proxied-list');
  if (!wrap || wrap.offsetParent === null || window.innerWidth < 1024) {
    if (wrap) wrap.style.maxHeight = '';
    return;
  }
  const top = wrap.getBoundingClientRect().top + window.scrollY;
  const footer = document.querySelector('footer');
  const footerH = footer && getComputedStyle(footer).display !== 'none' ? footer.offsetHeight : 0;
  wrap.style.maxHeight = `${Math.max(220, Math.round(window.innerHeight - top - footerH - 64))}px`;
}

// proxiedShowMore appends the next batch. It is the one growth path, shared by
// the sentinel observer and the fallback button, so the two cannot drift apart.
function proxiedShowMore() {
  if (!proxiedHasMore) return;
  proxiedVisible += PROXIED_BATCH;
  renderProxiedList();
  proxiedFillViewport();
}

// proxiedFillViewport keeps pulling batches while the sentinel is still on
// screen. A batch of 10 rows is shorter than the 24rem viewport, so after one
// append there is nothing left to scroll and the observer — which fires on a
// change of intersection — would never fire again. The loop closes that gap, and
// the guard stops a pathological size from spinning: it costs at most 50 paints.
function proxiedFillViewport() {
  const root = document.getElementById('custom-proxied-list');
  const sentinel = document.getElementById('proxied-sentinel');
  if (!root || !sentinel) return;
  // Only meaningful while the Proxied tab is on screen. A hidden panel has a
  // zero-height rect, which would otherwise read as "the sentinel is already
  // visible" and pull every batch in one go.
  const rootRect = root.getBoundingClientRect();
  if (rootRect.height === 0) return;
  let guard = 0;
  while (proxiedHasMore && guard < 50 && sentinel.getBoundingClientRect().top <= rootRect.bottom) {
    proxiedVisible += PROXIED_BATCH;
    renderProxiedList();
    guard++;
  }
}

// setupProxiedObserver watches the sentinel that sits below the rewritten rows,
// inside the scroll container. It is built once, at init: the sentinel is static,
// so the observer never has to be rebuilt as the list redraws.
function setupProxiedObserver() {
  const root = document.getElementById('custom-proxied-list');
  const sentinel = document.getElementById('proxied-sentinel');
  if (!root || !sentinel || typeof IntersectionObserver === 'undefined') return;
  proxiedObserver = new IntersectionObserver((entries) => {
    if (entries.some((e) => e.isIntersecting)) proxiedShowMore();
  }, { root: root, rootMargin: '160px' });
  proxiedObserver.observe(sentinel);
}

// proxiedResetSlice re-slices from the top after the sort order changes, then
// refills the viewport so the control's effect is visible without a scroll.
function proxiedResetSlice() {
  proxiedVisible = PROXIED_BATCH;
  renderProxiedList();
  proxiedFillViewport();
}

// proxiedFilterChanged re-slices from the top as the filter is typed. Without
// resetting proxiedVisible, a match that sorts past the first batch would stay hidden
// even though the count says it exists; refilling the viewport keeps the scroll region
// full instead of leaving a short list above a large blank.
function proxiedFilterChanged() {
  proxiedVisible = PROXIED_BATCH;
  renderProxiedList();
  proxiedFillViewport();
}

// proxiedBulkSetEnabled applies Pause (enable=false) or Resume (enable=true) to every
// selected domain. Pause is the same move the per-row switch makes: the domain stays in
// the list but leaves the proxy set, so it resolves normally. The selected domains are
// removed from both lists first, then added to the target one, so the operation is a
// move and not a duplicate.
function proxiedBulkSetEnabled(enable) {
  if (!currentConfig || !currentConfig.rules || proxiedSelected.size === 0) return;
  const selectedRows = proxiedOrderedRows().filter((r) => proxiedSelected.has(r.domain));
  // Pause/resume is a proxied-state action; a selection that includes blocked or
  // direct rows is left to the Remove button and the per-row status menu.
  if (selectedRows.some((r) => r.status !== 'proxy' && r.status !== 'paused')) return;
  const rules = currentConfig.rules;
  const enabled = Array.isArray(rules.custom_proxied) ? rules.custom_proxied : [];
  const disabled = Array.isArray(rules.custom_proxied_disabled) ? rules.custom_proxied_disabled : [];
  const restEnabled = enabled.filter((d) => !proxiedSelected.has(d));
  const restDisabled = disabled.filter((d) => !proxiedSelected.has(d));
  if (enable) {
    proxiedSelected.forEach((d) => { if (!restEnabled.includes(d)) restEnabled.push(d); });
    rules.custom_proxied = restEnabled;
    rules.custom_proxied_disabled = restDisabled;
  } else {
    proxiedSelected.forEach((d) => { if (!restDisabled.includes(d)) restDisabled.push(d); });
    rules.custom_proxied = restEnabled;
    rules.custom_proxied_disabled = restDisabled;
  }
  const n = proxiedSelected.size;
  proxiedSelected = new Set();
  renderConfig(currentConfig);
  saveRules();
  showToast(`${enable ? 'Resumed' : 'Paused'} ${n} domain${n === 1 ? '' : 's'}`, 'success');
}

// proxiedBulkRemove deletes the selected domains from both lists for good. It asks
// first, because a long list can hide the rows a bulk selection captured and the tick
// marks are small; the message names the count so the dialog is a real decision.
async function proxiedBulkRemove() {
  if (!currentConfig || !currentConfig.rules || proxiedSelected.size === 0) return;
  const n = proxiedSelected.size;
  const ok = await confirmAction({
    destructive: true,
    title: `Remove ${n} domain${n === 1 ? '' : 's'}?`,
    hint: 'THIS CANNOT BE UNDONE',
    message: `${n} selected domain${n === 1 ? '' : 's'} will be deleted from the managed lists. A proxied domain stops being spoofed to the SNI proxy immediately and resolves normally.`,
    confirmText: 'REMOVE',
  });
  if (!ok) return;
  const rules = currentConfig.rules;
  rules.custom_proxied = (rules.custom_proxied || []).filter((d) => !proxiedSelected.has(d));
  rules.custom_proxied_disabled = (rules.custom_proxied_disabled || []).filter((d) => !proxiedSelected.has(d));
  rules.custom_blocked = (rules.custom_blocked || []).filter((d) => !proxiedSelected.has(d));
  rules.custom_direct = (rules.custom_direct || []).filter((d) => !proxiedSelected.has(d));
  proxiedSelected = new Set();
  renderConfig(currentConfig);
  saveRules();
  showToast(`Removed ${n} domain${n === 1 ? '' : 's'}`, 'success');
}

// ---- Named custom policy groups ---------------------------------------------

let customGroupsCache = [];

async function loadCustomGroups() {
  const container = document.getElementById('custom-groups-list');
  if (!container) return;
  try {
    const res = await fetch(api('/api/custom-groups'), {
      headers: { 'Authorization': `Bearer ${authToken}` }
    });
    if (!res.ok) throw new Error('load failed');
    const data = await res.json();
    customGroupsCache = data.groups || [];
    renderCustomGroups();
  } catch (e) {
    container.innerHTML = '<div class="text-red-400 text-xs py-1">Failed to load custom groups</div>';
  }
}

function renderCustomGroups() {
  const container = document.getElementById('custom-groups-list');
  if (!container) return;
  container.innerHTML = '';
  if (!customGroupsCache.length) {
    container.innerHTML = '<div class="text-slate-500 text-xs py-1">No custom groups yet</div>';
    return;
  }
  const actionColor = { proxy: 'text-cyan-300', direct: 'text-emerald-300', block: 'text-red-300' };
  customGroupsCache.forEach((g) => {
    const row = document.createElement('div');
    row.className = 'flex items-center justify-between py-2 px-3 rounded-lg bg-slate-950/60 border border-slate-800 text-xs';
    const state = g.enabled ? '' : ' <span class="text-slate-500">(disabled)</span>';
    const actionCls = actionColor[g.action] || 'text-slate-400';
    // The uploaded icon rides in the left block and removes itself when the
    // group has none — one request per row, self-deleting on 404.
    const iconHTML = `
      <span data-icon-slot class="w-6 h-6 rounded bg-slate-900 border border-slate-800 flex items-center justify-center shrink-0">
        <i data-feather="folder" class="w-3 h-3 text-slate-600"></i>
      </span>`;
    row.innerHTML = `
      <div class="flex items-center gap-2 min-w-0 flex-1">
        ${iconHTML}
        <div class="min-w-0">
          <div class="font-bold text-white truncate">${escapeHTML(g.name)}${state}</div>
          <div class="font-mono ${actionCls}">${escapeHTML(g.action)} · ${g.domains.length} domain(s)</div>
        </div>
      </div>
      <div class="flex items-center gap-1.5 shrink-0">
        <button class="edit-group-btn px-2.5 py-1 rounded bg-slate-900 hover:bg-slate-800 text-slate-300 border border-slate-700 transition" data-id="${escapeHTML(g.id)}">Edit</button>
        <button class="delete-group-btn inline-flex items-center justify-center w-6 h-6 rounded text-slate-500 hover:text-red-400 hover:bg-red-500/10 transition" data-id="${escapeHTML(g.id)}" aria-label="Delete group ${escapeHTML(g.name)}">
          <i data-feather="trash-2" class="w-3.5 h-3.5"></i>
        </button>
      </div>
    `;
    const slot = row.querySelector('[data-icon-slot]');
    const img = new Image();
    img.className = 'w-4 h-4';
    img.alt = '';
    img.decoding = 'async';
    img.onload = () => { slot.innerHTML = ''; slot.appendChild(img); };
    img.onerror = () => {}; // keep the Feather folder placeholder
    img.src = api(`/api/custom-groups/${encodeURIComponent(g.id)}/icon`) + '?t=' + Date.now();
    container.appendChild(row);
  });
  safeFeatherReplace();
}

function openCustomGroupEditor(group) {
  const ed = document.getElementById('custom-group-editor');
  if (!ed) return;
  document.getElementById('custom-group-id').value = group ? group.id : '';
  document.getElementById('custom-group-name').value = group ? group.name : '';
  document.getElementById('custom-group-action').value = group ? group.action : 'proxy';
  document.getElementById('custom-group-domains').value = group ? group.domains.join('\n') : '';
  document.getElementById('custom-group-enabled').checked = group ? group.enabled : true;
  document.getElementById('custom-group-status').textContent = '';
  resetGroupIconEditor(group ? group.id : null);
  ed.classList.remove('hidden');
}

// ---- Group icon upload ------------------------------------------------------
// One pending file per editor session; it is sent to the server right away for
// an existing group, or held until Save creates the group for a new one.
let pendingGroupIconFile = null;

function resetGroupIconEditor(groupId) {
  pendingGroupIconFile = null;
  const file = document.getElementById('custom-group-icon-file');
  if (file) file.value = '';
  refreshGroupIconPreview(groupId);
}

function refreshGroupIconPreview(groupId) {
  const preview = document.getElementById('custom-group-icon-preview');
  const removeBtn = document.getElementById('custom-group-icon-remove');
  if (!preview) return;
  preview.innerHTML = '';
  if (pendingGroupIconFile) {
    // Client-side preview of the chosen file; the server re-validates on save.
    const url = URL.createObjectURL(pendingGroupIconFile);
    const img = new Image();
    img.className = 'w-6 h-6';
    img.alt = '';
    img.onload = () => URL.revokeObjectURL(url);
    img.src = url;
    preview.appendChild(img);
    if (removeBtn) removeBtn.classList.remove('hidden');
    return;
  }
  if (groupId) {
    const img = new Image();
    img.className = 'w-6 h-6';
    img.alt = '';
    img.onload = () => {
      preview.appendChild(img);
      if (removeBtn) removeBtn.classList.remove('hidden');
    };
    img.onerror = () => showGroupIconPlaceholder();
    img.src = api(`/api/custom-groups/${encodeURIComponent(groupId)}/icon`) + '?t=' + Date.now();
    return;
  }
  showGroupIconPlaceholder();
  if (removeBtn) removeBtn.classList.add('hidden');
}

function showGroupIconPlaceholder() {
  const preview = document.getElementById('custom-group-icon-preview');
  if (!preview) return;
  preview.innerHTML = '<i data-feather="image" class="w-4 h-4 text-slate-600"></i>';
  safeFeatherReplace();
}

async function uploadPendingGroupIcon(groupId) {
  if (!pendingGroupIconFile) return true;
  const res = await fetch(api(`/api/custom-groups/${encodeURIComponent(groupId)}/icon`), {
    method: 'PUT',
    headers: { 'Authorization': 'Bearer ' + authToken },
    body: pendingGroupIconFile
  });
  if (!res.ok) {
    showToast(await errorMessage(res, 'Icon was rejected'), 'error');
    return false;
  }
  return true;
}

async function saveCustomGroup() {
  const id = document.getElementById('custom-group-id').value.trim();
  const status = document.getElementById('custom-group-status');
  const body = {
    name: document.getElementById('custom-group-name').value.trim(),
    action: document.getElementById('custom-group-action').value,
    domains: document.getElementById('custom-group-domains').value
      .split('\n').map((d) => d.trim().toLowerCase()).filter(Boolean),
    enabled: document.getElementById('custom-group-enabled').checked
  };
  const path = id ? `/api/custom-groups/${encodeURIComponent(id)}` : '/api/custom-groups';
  const method = id ? 'PUT' : 'POST';
  status.textContent = 'Saving…';
  try {
    const res = await fetch(api(path), {
      method,
      headers: { 'Authorization': `Bearer ${authToken}`, 'Content-Type': 'application/json' },
      body: JSON.stringify(body)
    });
    const data = await res.json().catch(() => ({}));
    if (!res.ok) {
      status.textContent = data.error || 'Save failed';
      return;
    }
    // The group exists now — flush a pending icon, but don't lose the save if
    // the icon is refused: the group is stored, the modal just stays open with
    // the reason.
    const savedId = data.id || id;
    if (pendingGroupIconFile && savedId) {
      if (!(await uploadPendingGroupIcon(savedId))) {
        status.textContent = 'Group saved, but the icon was rejected';
        pendingGroupIconFile = null;
        refreshGroupIconPreview(savedId);
        document.getElementById('custom-group-id').value = savedId;
        loadCustomGroups();
        return;
      }
    }
    document.getElementById('custom-group-editor').classList.add('hidden');
    showToast('Custom group saved', 'success');
    loadCustomGroups();
  } catch (e) {
    status.textContent = 'Network error';
  }
}

async function deleteCustomGroup(id) {
  try {
    const res = await fetch(api(`/api/custom-groups/${encodeURIComponent(id)}`), {
      method: 'DELETE',
      headers: { 'Authorization': `Bearer ${authToken}` }
    });
    if (res.ok) {
      showToast('Custom group deleted', 'success');
      loadCustomGroups();
    }
  } catch (e) { /* ignore */ }
}

function renderCustomRecords(records) {
  const container = document.getElementById('custom-records-list');
  if (!container) return;
  container.innerHTML = '';
  if (!records || Object.keys(records).length === 0) {
    container.innerHTML = '<div class="text-slate-500 text-xs py-1">No DNS rewrites yet</div>';
    return;
  }
  for (const [dom, ip] of Object.entries(records)) {
    const row = document.createElement('div');
    row.className = 'flex items-center justify-between py-1.5 px-3 rounded-lg bg-slate-950/60 border border-slate-800 text-xs';
    row.innerHTML = `
      <span class="font-mono text-emerald-300">${escapeHTML(dom)} &rarr; ${escapeHTML(ip)}</span>
      <button class="remove-record inline-flex items-center justify-center w-6 h-6 shrink-0 rounded text-slate-500 hover:text-red-400 hover:bg-red-500/10 transition" data-dom="${escapeHTML(dom)}" aria-label="Remove record ${escapeHTML(dom)}">
        <i data-feather="x" class="w-3.5 h-3.5"></i>
      </button>
    `;
    container.appendChild(row);
  }
  safeFeatherReplace();
}

// =======================================================
// SAVE RULES API
// =======================================================
async function saveRules(silent = false) {
  if (!currentConfig) return;

  const payload = {
    custom_proxied: currentConfig.rules.custom_proxied,
    custom_proxied_disabled: currentConfig.rules.custom_proxied_disabled || [],
    custom_blocked: currentConfig.rules.custom_blocked,
    custom_direct: currentConfig.rules.custom_direct,
    custom_records: currentConfig.rules.custom_records,
    custom_domain_added: currentConfig.rules.custom_domain_added || {},
    services: currentConfig.rules.services || {},
    domain_services: currentConfig.rules.domain_services || {},
    service_icons: currentConfig.rules.service_icons || {},
    domain_agents: currentConfig.rules.domain_agents || {},
    service_agents: currentConfig.rules.service_agents || {}
  };

  // A per-domain toggle saves silently; only the explicit Save button asks for
  // confirmation. Compare only the fields this request will write, so the
  // server-owned stamps do not appear as phantom changes.
  if (!silent) {
    const before = {};
    for (const key of Object.keys(payload)) before[key] = currentConfig.rules[key];
    if (!(await showDiffConfirm('Save routing rules?', before, payload, 'Save rules'))) return false;
  }

  try {
    const res = await fetch(api('/api/config/rules'), {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${authToken}`
      },
      body: JSON.stringify(payload)
    });

    if (res.ok) {
      currentConfig.rules = payload;
      if (!silent) showToast('Routing rules saved & active!', 'success');
      return true;
    }
    showToast(await errorMessage(res, 'Failed to save routing rules'), 'error');
    return false;
  } catch (e) {
    showToast('Failed to save routing rules', 'error');
    return false;
  }
}

// =======================================================
// STATS POLLING
// =======================================================

// STATS_POLL_MS is the beat of the home tab. Every tile on it — Query Rate, RAM Cache,
// SNI Proxy, CPU Usage, RAM Usage, Live Traffic, Resolver Latency, Upstream Racers — is
// painted from one /api/stats response, so this interval *is* how live the page is. It
// was 2000 while the backend only resampled memory and CPU every fifth second; both
// halves now run at one second, and neither is the bottleneck for the other.
//
// setInterval is deliberately not used, for two reasons that bite at one second and did
// not at two:
//
//   A fetch can outlast the interval. setInterval does not care — it fires again
//   regardless. On a link where /api/stats takes 1.4 s, and a VPS answering a
//   home connection is exactly that link, polls overlap, queue behind each other, and
//   the tiles end up painted from whichever response happens to land last. Scheduling
//   the next poll only after the previous one settles makes overlap structurally
//   impossible and lets the real cadence degrade to whatever the link sustains, instead
//   of pretending to be 1 Hz while running four requests deep.
//
//   A hidden tab should not poll at all. A dashboard left open in a background tab over
//   an afternoon is fourteen thousand requests nobody looks at, on both ends of the one
//   scarce resource here. Polling stops on hide and resumes with an *immediate* refresh
//   rather than a wait, so coming back to the tab shows current numbers, not a second
//   of stale ones followed by a jump.
const STATS_POLL_MS = 1000;

// Off the home tab the tiles are not on screen, but the sidebar badges still need to
// stay truthful. Five seconds keeps them current without paying the 1 Hz cost of a
// dashboard nobody is looking at — the Logs tab is left open for long stretches, and
// it was polling stats every second the whole time.
const STATS_POLL_IDLE_MS = 5000;

// While the poll is failing, back off instead of hammering a daemon that is already
// unhappy once a second. The ceiling is low on purpose: a resolver that comes back up
// should light the badge again within a few seconds, not after a minute of silence.
const STATS_POLL_MAX_BACKOFF_MS = 8000;

let statsPollTimer = null;
// Tracked separately from the timer handle because a poll in flight has no timer: the
// handle is cleared on entry and only reassigned after the fetch settles. Without this
// flag, a visibilitychange landing inside that window would see "no timer scheduled",
// start a second chain, and quietly double the request rate for the rest of the session.
let statsPollInFlight = false;

async function pollStatsOnce() {
  if (statsPollInFlight) return;
  statsPollTimer = null;
  // Hidden tabs are resumed by the visibilitychange listener below, not by a timer.
  if (document.visibilityState === 'hidden') return;

  statsPollInFlight = true;
  try {
    await updateStats();
  } finally {
    statsPollInFlight = false;
  }

  // Checked again after the await: the tab may have been hidden while the request was
  // in flight, and arming a timer here would leave a background tab polling on.
  if (document.visibilityState === 'hidden') return;

  // The backoff reads statsFailures, which markStatsHealth already maintains for the
  // LIVE ENGINE badge. Deriving both from one counter keeps the page from claiming to
  // be live while polling every eight seconds, or the reverse.
  const base = activeDashTab === 'dashboard' ? STATS_POLL_MS : STATS_POLL_IDLE_MS;
  const delay = statsFailures > 0
    ? Math.min(base * 2 ** statsFailures, STATS_POLL_MAX_BACKOFF_MS)
    : base;

  statsPollTimer = setTimeout(pollStatsOnce, delay);
}

function startStatsPolling() {
  statsFull = null;
  statsPollCount = 0;
  // Called once, from the post-login boot path. The listener is registered here rather
  // than at module scope so that a page which never logs in never installs it.
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'hidden') {
      clearTimeout(statsPollTimer);
      statsPollTimer = null;
      stopAgentPolling();
      // The live stream stays open: an operator watching the log in one tab
      // while browsing in another must keep seeing the queries that happen in
      // between. pushStreamQuery already skips DOM work while hidden.
      return;
    }
    // If the stream dropped while the tab was hidden, its backoff timer may
    // have been throttled; reconnect as soon as the page is visible again.
    if (!sseSource || sseSource.readyState === EventSource.CLOSED) startLiveStream();
    if (activeDashTab === 'agents') startAgentPolling();
    if (statsPollTimer === null) pollStatsOnce();
    // Returning to a visible page while the stream tab is showing: the SSE
    // buffer kept growing hidden (per-query DOM work is skipped then — see
    // pushStreamQuery), so this is where the backlog becomes rows. Without
    // it the table stays stale until the next query happens to arrive.
    if (activeDashTab === 'stream') {
      if (!isStreamPaused) renderQueryStream();
      if (streamFallbackTimer) streamFallbackPoll();
      else if (Date.now() - streamLastEventAt >= 3000) startStreamFallback();
    }
  });

  pollStatsOnce();
}

async function updateStats() {
  if (!authToken) return;
  try {
    const needFull = !statsFull || (statsPollCount++ % 5 === 0);
    const res = await fetch(api('/api/stats') + (needFull ? '' : '?compact=1'), {
      headers: { 'Authorization': `Bearer ${authToken}` }
    });
    if (res.status === 401) {
      // The deploy restarts the daemon and sessions are in memory: the page
      // must say so instead of showing stale tiles with a dead live log.
      markStatsHealth(false, 'Session expired');
      stopLiveStream();
      stopAgentPolling();
      showLoginModal();
      return;
    }
    if (!res.ok) {
      markStatsHealth(false, `The server answered ${res.status}`);
      return;
    }

    let data = await res.json();
    if (needFull) {
      statsFull = data;
    } else {
      // Live fields override the last full document; slow fields (CPU, RAM,
      // system memory, goroutines, agent counters) stay from the last full poll.
      data = { ...statsFull, ...data };
    }

    const setTxt = (id, txt) => {
      const el = document.getElementById(id);
      // textContent, not innerText: reading innerText forces a layout, and this
      // runs on every one-second poll even when the value never changes. The
      // comparison still avoids invalidating the glass backdrop, but now it is
      // a string compare rather than a forced reflow.
      if (el && el.textContent !== txt) el.textContent = txt;
    };

    // Every tile below used to read `data.field.toFixed(1)` straight. The endpoint
    // does send all of these today — handleStats builds a map literal with no
    // omitempty — but the failure mode if one ever goes missing is invisible: the
    // TypeError lands in the catch below, and every tile *after* the failing line
    // keeps whatever it showed two seconds ago, forever, while the page looks alive.
    // num() makes one missing field cost one dash instead of the whole update.
    const num = (v, digits) => (typeof v === 'number' && isFinite(v)) ? v.toFixed(digits) : '—';
    const count = (v) => (typeof v === 'number' && isFinite(v)) ? v.toLocaleString() : '—';

    setTxt('stat-qps', num(data.qps, 1));
    setTxt('stat-total-queries', count(data.total_queries));
    setTxt('stat-cache-ratio', typeof data.cache_hit_ratio === 'number' ? num(data.cache_hit_ratio, 1) + '%' : '—');
    setTxt('stat-cache-entries', count(data.cache_entries));
    setTxt('stat-proxy-active', count(data.active_proxy_conns));
    setTxt('stat-proxy-total', count(data.total_proxy_conns));
    setTxt('stat-cpu-percent', typeof data.cpu_usage_percent === 'number' ? num(data.cpu_usage_percent, 1) + '%' : '—');
    setTxt('stat-cpu-cores', count(data.num_cpu));
    setTxt('stat-memory', typeof data.ram_usage_mb === 'number' ? num(data.ram_usage_mb, 1) : '—');
    // Machine-wide load (internal/sysmetrics): the whole server, not just the
    // daemon process. Negative values mean the platform cannot answer — dash.
    const sysCPU = typeof data.system_cpu_percent === 'number' && data.system_cpu_percent >= 0
      ? num(data.system_cpu_percent, 1) + '%' : '—';
    setTxt('stat-cpu-sys', sysCPU);
    const sysMemOK = typeof data.system_mem_used_mb === 'number' && data.system_mem_used_mb >= 0;
    setTxt('stat-mem-sys-used', sysMemOK ? num(data.system_mem_used_mb, 0) : '—');
    setTxt('stat-mem-sys-total', sysMemOK ? num(data.system_mem_total_mb, 0) : '—');
    setTxt('stat-mem-sys-pct', sysMemOK ? num(data.system_mem_percent, 0) : '—');
    setTxt('stat-speed-in', num(data.speed_in_kbps, 1));
    setTxt('stat-speed-out', num(data.speed_out_kbps, 1));
    setTxt('stat-traffic-total', formatBytes(data.total_bytes_transferred));
    renderRateLimit(data.rate_limit_qps, data.rate_limited);
    renderLatency(data.latency_uncached, data.latency);
    renderCacheHealth(data);
    renderProxyDrops(data);

    // Render upstreams list
    renderUpstreams(data.upstreams);

    // Fold the same health data into the global banner; this only repaints when
    // the set of degraded resolvers changes.

    // Push QPS point to chart
    pushChartData(data.qps);

    markStatsHealth(true);
  } catch (e) {
    // A dropped poll is normal — a reload, a restart, a phone changing networks —
    // so the badge only changes after two in a row. What must not happen is the old
    // behaviour: swallow it and leave the page claiming to be live.
    markStatsHealth(false, 'The dashboard cannot reach the daemon');
  }
}

// statsFailures counts consecutive failed polls so one blip does not repaint the badge.
let statsFailures = 0;
// The last full stats document. The 1 Hz poll asks for compact live fields and
// merges them over this; every fifth poll refreshes the slow fields (CPU, RAM,
// system memory, goroutines, agent counters).
let statsFull = null;
let statsPollCount = 0;
// The last painted health state. A healthy poll used to rewrite the badge class,
// dot class and title every second even when nothing changed; this makes the
// healthy path a single string compare.
let statsHealthState = '';

// markStatsHealth is the only thing that makes the numbers on the overview page
// trustworthy: without it a daemon that died two minutes ago is indistinguishable
// from a healthy one, because every tile simply keeps its last value.
function markStatsHealth(ok, reason) {
  const badge = document.getElementById('live-engine-badge');
  const dot = document.getElementById('live-engine-dot');
  const label = document.getElementById('live-engine-label');

  if (ok) {
    statsFailures = 0;
    if (statsHealthState === 'ok') return;
    statsHealthState = 'ok';
    if (badge) {
      badge.className = 'text-[10px] font-mono text-cyan-400 px-2.5 py-0.5 rounded-full bg-cyan-500/10 border border-cyan-500/30 flex items-center gap-1.5';
      badge.title = 'The dashboard is receiving live telemetry';
    }
    if (dot) dot.className = 'pulse-dot';
    if (label) label.textContent = 'LIVE ENGINE';
    return;
  }

  statsFailures++;
  if (statsFailures < 2) return;
  const failureState = 'bad:' + (reason || '');
  if (statsHealthState === failureState) return;
  statsHealthState = failureState;

  if (badge) {
    badge.className = 'text-[10px] font-mono text-amber-400 px-2.5 py-0.5 rounded-full bg-amber-500/10 border border-amber-500/30 flex items-center gap-1.5';
    badge.title = `${reason || 'Telemetry stopped'} — every number on this page is from the last successful poll`;
  }
  if (dot) dot.className = 'pulse-dot is-stale';
  if (label) label.textContent = 'STALE — NO TELEMETRY';
}

function formatBytes(bytes) {
  // Called with data.total_bytes_transferred and with a client's traffic counter, and
  // an absent counter used to render "NaN KB" here. Bytes below a kilobyte get their
  // own branch so a subscriber who has used nothing reads "0 B" rather than "0.0 KB".
  if (typeof bytes !== 'number' || !isFinite(bytes) || bytes < 0) return '—';
  if (bytes < 1024) return Math.round(bytes) + ' B';
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
  if (bytes < 1024 * 1024 * 1024) return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
  return (bytes / (1024 * 1024 * 1024)).toFixed(2) + ' GB';
}

// The per-source rate limiter drops a UDP flood without answering it, so the only
// way an operator can tell it fired is if the panel says so. Both numbers are
// shown together: "42 dropped" means nothing without the limit it was measured
// against, and a limit with no drops is the reassuring case worth displaying.
function renderRateLimit(qps, dropped) {
  const qpsEl = document.getElementById('stat-ratelimit-qps');
  const dropEl = document.getElementById('stat-ratelimit-dropped');
  if (!qpsEl && !dropEl) return;

  const n = dropped || 0;

  if (qpsEl) {
    if (!qps || qps <= 0) {
      qpsEl.textContent = 'off';
      qpsEl.className = 'text-slate-500';
      qpsEl.title = 'Per-source rate limiting is disabled (access.rate_limit_qps = 0)';
    } else {
      qpsEl.textContent = `${qps}/s`;
      qpsEl.className = 'text-slate-400';
      qpsEl.title = `Limiting at ${qps} qps per source`;
    }
  }

  if (dropEl) {
    dropEl.textContent = n.toLocaleString();
    dropEl.className = n > 0 ? 'text-amber-400 font-bold' : 'text-slate-400';
    dropEl.title = n > 0
      ? `${n.toLocaleString()} queries dropped or refused for exceeding ${qps} qps per source since start`
      : 'Nothing dropped for exceeding the per-source rate limit so far';
  }
}

// Latency needs percentiles, not an average: at a healthy hit rate the mean is
// dominated by cache hits and reads under a millisecond while every real
// resolution crawls. `miss` excludes the cache hits — it is the distribution that
// moves when an upstream degrades — and `all` is every answer the resolver served.
function renderLatency(miss, all) {
  const setTxt = (id, txt) => {
    const el = document.getElementById(id);
    if (el && el.textContent !== txt) el.textContent = txt;
  };
  // Sub-10 ms readings need two decimals to be worth showing at all; past that
  // the tenth of a millisecond is noise.
  const fmt = (v) => (typeof v === 'number' && isFinite(v))
    ? (v < 10 ? v.toFixed(2) : v.toFixed(1))
    : '—';

  const haveMiss = !!(miss && miss.count > 0);
  setTxt('stat-lat-p50', haveMiss ? fmt(miss.p50_ms) : '—');
  setTxt('stat-lat-p95', haveMiss ? fmt(miss.p95_ms) : '—');
  setTxt('stat-lat-p99', haveMiss ? fmt(miss.p99_ms) : '—');
  setTxt('stat-lat-max', haveMiss ? fmt(miss.max_ms) : '—');

  const badge = document.getElementById('stat-lat-count');
  if (badge) {
    if (haveMiss) {
      badge.textContent = `${miss.count.toLocaleString()} resolved`;
      badge.title = 'Queries answered by an upstream rather than from cache, since the daemon started';
    } else if (all && all.count > 0) {
      // A real and reassuring state, not an error: nothing has needed an upstream.
      badge.textContent = 'every answer from cache';
      badge.title = 'No query has needed an upstream resolver yet';
    } else {
      badge.textContent = 'no data yet';
      badge.title = 'The resolver has not answered a query yet';
    }
  }

  // p50 carries the colour because it is the everyday experience. The thresholds
  // are the resolver's own service time, not a game's ping: past ~60 ms the
  // upstream path is the bottleneck, not this daemon.
  const p50El = document.getElementById('stat-lat-p50');
  if (p50El) {
    let tone = 'text-emerald-400';
    if (haveMiss && miss.p50_ms >= 60) tone = 'text-red-400';
    else if (haveMiss && miss.p50_ms >= 25) tone = 'text-amber-400';
    p50El.className = `text-lg sm:text-xl font-extrabold font-mono ${tone}`;
  }

  const allEl = document.getElementById('stat-lat-all');
  if (allEl) {
    allEl.textContent = (all && all.count > 0)
      ? `p50 ${fmt(all.p50_ms)} · p95 ${fmt(all.p95_ms)} · p99 ${fmt(all.p99_ms)} ms over ${all.count.toLocaleString()}`
      : '—';
  }
}

// Serve-stale answers a dead upstream instantly from an expired entry, which is
// precisely why it hides the outage: clients keep getting fast replies from a
// cache nothing is refilling. stale_served climbing alone is the feature working;
// climbing beside refresh_failed is the warning that names are about to go dark.
// refresh_started is carried in the tooltip rather than the line, because it is
// the denominator: "18 failed" is alarming on its own and unremarkable against
// 40,000 attempts, and the badge has no room for both numbers.
function renderCacheHealth(data) {
  const el = document.getElementById('stat-cache-health');
  if (!el) return;

  const served = data.stale_served || 0;
  const started = data.refresh_started || 0;
  const failed = data.refresh_failed || 0;
  const dropped = data.refresh_dropped || 0;

  if (!served && !started && !failed && !dropped) {
    el.textContent = 'Cache refresh: healthy';
    el.className = 'text-slate-500';
    el.title = 'No background refresh has run yet, no stale answer has been served';
    return;
  }

  // Share of attempts that failed. Only meaningful once something was attempted;
  // a failure with no attempt recorded would be a counter bug, not a bad upstream.
  const failRate = started > 0 ? (failed / started) * 100 : 0;

  el.textContent = `Cache refresh: ${served.toLocaleString()} stale · ${failed.toLocaleString()} failed · ${dropped.toLocaleString()} skipped`;
  el.className = failed > 0 ? 'text-amber-400 font-bold' : 'text-slate-400';
  if (failed > 0) {
    el.title = `${failed.toLocaleString()} of ${started.toLocaleString()} background refreshes failed (${failRate.toFixed(1)}%) while ${served.toLocaleString()} stale answer(s) were served. Clients are getting instant replies from a cache nothing is refilling — check the upstream resolvers before the grace window closes.`;
  } else if (dropped > 0) {
    el.title = `${started.toLocaleString()} background refresh(es) succeeded, ${dropped.toLocaleString()} were skipped because the refresh worker pool was saturated. Skipped renewals are not errors; they simply expire normally.`;
  } else {
    el.title = `${started.toLocaleString()} background refresh(es) run, none failed. ${served.toLocaleString()} expired entry/entries were served instantly while being renewed.`;
  }
}

// A connection can reach the SNI proxy and never become a relay, in which case it
// appears in none of the relay figures on the card above. Two ways that happens:
//
//   refused    — the target was blocked, resolved back to this host, or the relay
//                table was full. Expected, and mostly means the guards work.
//   unreadable — the opening bytes named no destination, so there was nowhere to
//                dial. A port scanner looks exactly like this, so a handful is
//                background noise on any public IP. A number that climbs with
//                real traffic does not: it means a name is being answered with
//                this server's address while the client then speaks something
//                the relay cannot read a destination out of — UDP-only, or TCP on
//                a port this relay does not accept on, or TCP that is not TLS or
//                HTTP. The client sees a connection that opens and dies, and
//                cannot tell why, because the substitution happened in DNS.
//
// So unreadable is shown even at zero-refused, and it is the one that gets the
// warning colour.
function renderProxyDrops(data) {
  const refusedEl = document.getElementById('stat-proxy-refused');
  const unreadableEl = document.getElementById('stat-proxy-unreadable');
  if (!refusedEl && !unreadableEl) return;

  const refused = data.relays_refused || 0;
  const malformed = data.relays_refused_malformed || 0;
  const blocked = data.relays_refused_blocked || 0;
  const selfDial = data.relays_refused_self || 0;
  const quota = data.relays_refused_quota || 0;
  const tableFull = data.relays_refused_table_full || 0;
  const resolveFailed = data.relays_refused_resolve || 0;
  const flood = data.relays_refused_flood || 0;
  const unreadable = data.relays_unreadable || 0;

  // Flood attempts are the penalty box counting every retry from a source
  // already blocked for malformed probes. One scanner can produce tens of
  // thousands of them, which says nothing about relay health, so the headline
  // reports only actionable refusals. The tooltip keeps the full breakdown.
  const actionableRefused = Math.max(refused - flood, 0);

  const breakdown = `Actionable refusals ${actionableRefused.toLocaleString()} (blocked ${blocked.toLocaleString()}, self ${selfDial.toLocaleString()}, quota ${quota.toLocaleString()}, table full ${tableFull.toLocaleString()}, resolve failed ${resolveFailed.toLocaleString()}) · flood attempts ${flood.toLocaleString()} (excluded) · malformed probes ${malformed.toLocaleString()} (excluded)`;

  if (refusedEl) {
    refusedEl.textContent = actionableRefused.toLocaleString();
    refusedEl.className = actionableRefused > 0 ? 'text-slate-300 font-bold' : 'text-slate-400';
    refusedEl.title = breakdown;
  }

  if (unreadableEl) {
    unreadableEl.textContent = unreadable.toLocaleString();
    unreadableEl.className = unreadable > 0 ? 'text-amber-400 font-bold' : 'text-slate-400';
    unreadableEl.title = unreadable > 0
      ? `${unreadable.toLocaleString()} connection(s) arrived without naming a destination — no TLS SNI and no HTTP Host header. A few are port scans. A rising count means a proxied name is sending clients here for traffic this relay cannot carry; check the server log for the sampled "unreadable destination" lines, which name the port and the first bytes.`
      : 'Every connection that reached the proxy named a destination and was relayed';
  }
}

// The trash icon as inline SVG. renderUpstreams runs once per stats poll
// (1 Hz), and the old row markup used a data-feather <i> plus a
// document-wide icon sweep — on a 1-core VPS that sweep was the single
// largest recurring cost on the dashboard, walking every element of every
// tab once a second. The SVG is byte-identical to what the icon library
// would substitute, so nothing changes visually.
const FEATHER_TRASH_SVG = '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="w-3 h-3"><polyline points="3 6 5 6 21 6"></polyline><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"></path></svg>';

const FEATHER_GLOBE_SVG = '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="w-3 h-3"><circle cx="12" cy="12" r="10"></circle><line x1="2" y1="12" x2="22" y2="12"></line><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"></path></svg>';

// Signature of the last rendered upstream set: the poll repaints the list
// unconditionally and most ticks change nothing (latency only moves when the
// racer re-measures). Skipping identical payloads removes the innerHTML
// rebuild + the SVG re-parse for every one of those ticks. Built by
// concatenation because the row template below interpolates the escaped
// address into innerHTML; a bare template here would trip the raw-
// interpolation scan for exactly the value it exists to protect.
let lastUpstreamsSig = '';
// Keyed rows for the resolver card. The list is polled at 1 Hz and latency
// changes every time; rebuilding the DOM on each poll was the largest remaining
// dashboard cost. New addresses get a row, existing rows update in place, and
// rows are moved only when the order actually changes.
let upstreamRowMap = new Map();
// The last stats payload, kept so a policy change can repaint the list without
// waiting for the next 1 Hz poll.
let lastUpstreamsPayload = null;
// The last address set rendered, so the settings fallback is rebuilt once per
// pool change rather than on every 1 Hz repaint.
let lastUpstreamsAddresses = '';
// Throttles the settings re-fetch when a stats poll and the settings list agree
// only after a moment (adding an upstream writes both, but not atomically).
let lastResolutionSyncAt = 0;

// Public-provider browser state. The catalogue is fetched once and then cached;
// the add/delete endpoints remain the only writers.
let upstreamPresetProviders = null;
let upstreamPresetLoaded = false;
let upstreamPresetLoading = false;
let upstreamPresetError = '';
let upstreamPresetExpanded = new Set();
let upstreamPresetAdded = new Set();
let upstreamPresetAdding = '';
let upstreamPresetOpen = false;
// Stats polls that were already in flight when a preset was added or removed
// can land with the old pool. Ignore that snapshot briefly so the tick does not
// flip back and forth while the mutation settles.
let upstreamPresetMutationUntil = 0;

const UPSTREAM_PROTO_CLASSES = {
  UDP: 'proto-udp',
  TCP: 'proto-tcp',
  DOT: 'proto-dot',
  DOH: 'proto-doh',
  DOQ: 'proto-doq',
  DOH3: 'proto-doh3',
  DNSCRYPT: 'proto-dnscrypt',
};

function upstreamProtoClass(protocol) {
  return UPSTREAM_PROTO_CLASSES[String(protocol || '').toUpperCase()] || 'proto-other';
}

// The server validates the typed value; this is only the coloured label shown
// before the POST, so an unknown scheme stays neutral instead of claiming UDP.
function upstreamCustomProtocol(address) {
  const v = String(address || '').trim().toLowerCase();
  if (v.startsWith('sdns://')) return 'DNSCrypt';
  if (v.startsWith('tls://')) return 'DoT';
  if (v.startsWith('https://')) return 'DoH';
  if (v.startsWith('quic://') || v.startsWith('doq://')) return 'DoQ';
  if (v.startsWith('h3://') || v.startsWith('doh3://')) return 'DoH3';
  if (v.startsWith('tcp://')) return 'TCP';
  return 'UDP';
}

function renderUpstreamPresetPanel() {
  const panel = document.getElementById('upstream-preset-panel');
  const list = document.getElementById('upstream-preset-list');
  const input = document.getElementById('new-upstream-input');
  if (!panel || !list) return;
  panel.classList.toggle('hidden', !upstreamPresetOpen);
  if (input) input.setAttribute('aria-expanded', upstreamPresetOpen ? 'true' : 'false');
  if (!upstreamPresetOpen) return;

  if (upstreamPresetLoading && !upstreamPresetLoaded) {
    list.innerHTML = '<div class="upstream-preset-loading">Loading providers…</div>';
    return;
  }
  if (upstreamPresetError && !upstreamPresetLoaded) {
    list.innerHTML = `<div class="upstream-preset-empty">${escapeHTML(upstreamPresetError)} <button type="button" data-preset-retry class="font-bold text-cyan-300">Retry</button></div>`;
    return;
  }

  const providers = upstreamPresetProviders || [];
  const query = (input ? input.value : '').trim();
  const q = query.toLowerCase();
  let html = '';
  const exactPreset = q && providers.some(p => (p.servers || []).some(s => s.address.toLowerCase() === q));

  if (query && !exactPreset) {
    const proto = upstreamCustomProtocol(query);
    html += `<button type="button" class="upstream-preset-server upstream-preset-custom" data-custom="1">
      <span class="upstream-preset-addr" title="${escapeHTML(query)}">Use custom: ${escapeHTML(query)}</span>
      <span class="upstream-preset-proto ${escapeHTML(upstreamProtoClass(proto))}">${escapeHTML(protocolLabel(proto))}</span>
      <span class="upstream-preset-action">+</span>
    </button>`;
  }

  let shown = 0;
  providers.forEach(provider => {
    const nameMatch = !q || provider.name.toLowerCase().includes(q) || String(provider.id || '').toLowerCase().includes(q);
    let servers = Array.isArray(provider.servers) ? provider.servers : [];
    if (q && !nameMatch) {
      servers = servers.filter(s => s.address.toLowerCase().includes(q)
        || String(s.label || '').toLowerCase().includes(q)
        || String(s.protocol || '').toLowerCase().includes(q));
    }
    if (!servers.length) return;
    shown++;
    const expanded = !!q || upstreamPresetExpanded.has(provider.id);
    html += `<button type="button" class="upstream-preset-provider" data-provider="${escapeHTML(provider.id)}" aria-expanded="${escapeHTML(expanded ? 'true' : 'false')}">
      <img class="upstream-preset-logo" src="${escapeHTML(api(`/img/providers/${encodeURIComponent(provider.id)}.png`))}" alt="">
      <span class="upstream-preset-provider-name">${escapeHTML(provider.name)}</span>
      <span class="upstream-preset-count">${servers.length}</span>
      <span class="upstream-preset-chevron" aria-hidden="true">›</span>
    </button>`;
    if (!expanded) return;
    html += '<div class="upstream-preset-servers">';
    servers.forEach(server => {
      const added = upstreamPresetAdded.has(server.address);
      const adding = upstreamPresetAdding === server.address;
      const title = added
        ? `${server.address} — click to remove from the upstream list`
        : server.address;
      html += `<button type="button" class="upstream-preset-server${escapeHTML(added ? ' is-added' : '')}" data-addr="${escapeHTML(server.address)}" title="${escapeHTML(title)}"${adding ? ' disabled' : ''}>
        <span class="upstream-preset-addr">${escapeHTML(server.label || server.address)}</span>
        <span class="upstream-preset-proto ${escapeHTML(upstreamProtoClass(server.protocol))}">${escapeHTML(protocolLabel(server.protocol))}</span>
        <span class="upstream-preset-action">${adding ? '…' : added ? '✓' : '+'}</span>
      </button>`;
    });
    html += '</div>';
    if (provider.note) {
      html += `<div class="upstream-preset-note">${escapeHTML(provider.note)}</div>`;
    }
  });

  if (!shown) {
    html += `<div class="upstream-preset-empty">${query ? 'No matching provider or server.' : 'No providers available.'}</div>`;
  }
  list.innerHTML = html;
}

async function loadUpstreamPresets() {
  if (upstreamPresetLoading || upstreamPresetLoaded) return;
  upstreamPresetLoading = true;
  upstreamPresetError = '';
  renderUpstreamPresetPanel();
  try {
    const res = await fetch(api('/api/upstreams/presets'), {
      headers: { 'Authorization': `Bearer ${authToken}` }
    });
    if (!res.ok) throw new Error('provider list unavailable');
    const data = await res.json();
    upstreamPresetProviders = Array.isArray(data.providers) ? data.providers : [];
    upstreamPresetLoaded = true;
    if (upstreamPresetProviders.length && upstreamPresetExpanded.size === 0) {
      upstreamPresetExpanded.add(upstreamPresetProviders[0].id);
    }
  } catch (e) {
    upstreamPresetError = 'Could not load the provider list. You can still type a resolver address.';
  } finally {
    upstreamPresetLoading = false;
    renderUpstreamPresetPanel();
  }
}

// addUpstreamAddress is shared by the custom Add button, Enter, and every
// preset row. Preset adds keep the browser open and the filter text intact so
// several endpoints can be added in one visit.
async function removeUpstreamAddress(raw) {
  const addr = String(raw || '').trim();
  if (!addr || upstreamPresetAdding) return false;
  const wasAdded = upstreamPresetAdded.has(addr);
  // Optimistic: uncheck immediately, keep the row locked with "…" until the
  // server confirms. This is the half that used to look like the tick bouncing.
  upstreamPresetAdding = addr;
  upstreamPresetAdded.delete(addr);
  upstreamPresetMutationUntil = Date.now() + 2500;
  renderUpstreamPresetPanel();
  try {
    const res = await fetch(api('/api/upstreams/delete'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${authToken}` },
      body: JSON.stringify({ address: addr })
    });
    if (!res.ok) {
      if (wasAdded) upstreamPresetAdded.add(addr);
      showToast(await errorMessage(res, 'Could not remove the upstream.'), 'error');
      return false;
    }
    const body = await res.json().catch(() => ({}));
    if (Array.isArray(body.upstreams)) upstreamPresetAdded = new Set(body.upstreams);
    else upstreamPresetAdded.delete(addr);
    showToast(`Removed ${addr}`, 'info');
    loadResolutionSettings();
    updateStats();
    return true;
  } catch (e) {
    if (wasAdded) upstreamPresetAdded.add(addr);
    showToast('Failed to remove upstream', 'error');
    return false;
  } finally {
    upstreamPresetAdding = '';
    upstreamPresetMutationUntil = Date.now() + 1500;
    renderUpstreamPresetPanel();
  }
}

async function addUpstreamAddress(raw, opts = {}) {
  const input = document.getElementById('new-upstream-input');
  const addr = String(raw || '').trim();
  if (!addr) return false;

  // A bare provider name is not a resolver. Typing "quad9" and pressing Add
  // used to become "quad9:53", an unresolvable host that kept the health banner
  // permanently red. Point the operator at that provider's server list instead.
  const bareName = /^[a-z0-9-]+$/i.test(addr);
  const providerMatch = bareName && !opts.preset
    ? (upstreamPresetProviders || []).find(p =>
        String(p.id || '').toLowerCase() === addr.toLowerCase()
        || String(p.name || '').toLowerCase() === addr.toLowerCase())
    : null;
  if (providerMatch) {
    showToast(`Choose a server under ${providerMatch.name}, or enter a full DNS address.`, 'info');
    return false;
  }

  const wasAdded = upstreamPresetAdded.has(addr);
  // Optimistic check, then "…" until the add is confirmed. The mutation grace
  // window stops an old stats payload from re-rendering the previous state.
  upstreamPresetAdding = addr;
  upstreamPresetAdded.add(addr);
  upstreamPresetMutationUntil = Date.now() + 2500;
  renderUpstreamPresetPanel();
  try {
    const res = await fetch(api('/api/upstreams/add'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${authToken}` },
      body: JSON.stringify({ address: addr })
    });
    if (!res.ok) {
      let reason = 'Invalid upstream address';
      try {
        const body = await res.json();
        if (body && body.error) reason = body.error;
      } catch (_) { /* non-JSON error body */ }
      if (res.status === 409) {
        upstreamPresetAdded.add(addr);
      } else if (!wasAdded) {
        upstreamPresetAdded.delete(addr);
      }
      showToast(reason, 'error');
      return false;
    }
    const body = await res.json().catch(() => ({}));
    if (Array.isArray(body.upstreams)) upstreamPresetAdded = new Set(body.upstreams);
    if (input && !opts.keepText) input.value = '';
    showToast(opts.preset ? `Added ${addr}` : 'Upstream resolver added & tested!', 'success');
    loadResolutionSettings();
    updateStats();
    return true;
  } catch (e) {
    if (!wasAdded) upstreamPresetAdded.delete(addr);
    showToast('Failed to add upstream', 'error');
    return false;
  } finally {
    upstreamPresetAdding = '';
    upstreamPresetMutationUntil = Date.now() + 1500;
    renderUpstreamPresetPanel();
  }
}

function initUpstreamPresetBrowser() {
  const wrap = document.getElementById('upstream-add-wrap');
  const input = document.getElementById('new-upstream-input');
  const panel = document.getElementById('upstream-preset-panel');
  const list = document.getElementById('upstream-preset-list');
  if (!wrap || !input || !panel || !list || wrap.dataset.presetsWired === '1') return;
  wrap.dataset.presetsWired = '1';

  const open = () => {
    upstreamPresetOpen = true;
    renderUpstreamPresetPanel();
    if (!upstreamPresetLoaded) void loadUpstreamPresets();
  };
  input.addEventListener('focus', open);
  input.addEventListener('input', open);
  input.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') {
      upstreamPresetOpen = false;
      renderUpstreamPresetPanel();
      input.blur();
    } else if (e.key === 'Enter' && input.value.trim()) {
      e.preventDefault();
      void addUpstreamAddress(input.value, { preset: true, keepText: true });
    } else if (e.key === 'ArrowDown') {
      const first = list.querySelector('.upstream-preset-server:not(:disabled)');
      if (first) {
        e.preventDefault();
        first.focus();
      }
    }
  });

  panel.addEventListener('click', (e) => {
    const retry = e.target.closest('[data-preset-retry]');
    if (retry) {
      upstreamPresetLoaded = false;
      upstreamPresetError = '';
      void loadUpstreamPresets();
      return;
    }
    const providerBtn = e.target.closest('[data-provider]');
    if (providerBtn) {
      const id = providerBtn.dataset.provider;
      if (upstreamPresetExpanded.has(id)) upstreamPresetExpanded.delete(id);
      else upstreamPresetExpanded.add(id);
      renderUpstreamPresetPanel();
      return;
    }
    const custom = e.target.closest('[data-custom]');
    if (custom) {
      void addUpstreamAddress(input.value, { preset: true, keepText: true });
      return;
    }
    const serverBtn = e.target.closest('[data-addr]');
    if (serverBtn && !serverBtn.disabled) {
      const address = serverBtn.dataset.addr;
      if (upstreamPresetAdded.has(address)) {
        void removeUpstreamAddress(address);
      } else {
        void addUpstreamAddress(address, { preset: true, keepText: true });
      }
    }
  });

  document.getElementById('close-upstream-presets')?.addEventListener('click', () => {
    upstreamPresetOpen = false;
    renderUpstreamPresetPanel();
  });
  document.addEventListener('click', (e) => {
    // A provider/server click re-renders the list, so by the time this bubbles
    // here its target is detached from the DOM. Treat that as an inside click,
    // not as the operator clicking away, or the panel closes after every add.
    if (upstreamPresetOpen && e.target.isConnected && !wrap.contains(e.target)) {
      upstreamPresetOpen = false;
      renderUpstreamPresetPanel();
    }
  });
}

// updateUpstreamAddrHint toggles the fade on the edges the address can still be
// scrolled towards. It runs after a render and on every scroll of the span.
function updateUpstreamAddrHint(el) {
  if (!el || !el.classList) return;
  const overflow = el.scrollWidth > el.clientWidth + 1;
  el.classList.toggle('is-overflowing', overflow);
  el.classList.toggle('can-left', overflow && el.scrollLeft > 1);
  el.classList.toggle('can-right', overflow && el.scrollLeft + el.clientWidth < el.scrollWidth - 1);
}

// initUpstreamAddressScroll wires the address fields once. Delegation keeps it
// working across the list's wholesale re-render on every stats change, and the
// handlers only act when a row really overflows, so taps and text selection on a
// short name behave exactly as before.
function initUpstreamAddressScroll() {
  const container = document.getElementById('upstreams-list');
  if (!container || container.dataset.addrScroll === '1') return;
  container.dataset.addrScroll = '1';
  let drag = null;
  container.addEventListener('pointerdown', (e) => {
    const el = e.target.closest('.upstream-addr');
    if (!el || el.scrollWidth <= el.clientWidth + 1 || e.button !== 0) return;
    drag = { el, x: e.clientX, left: el.scrollLeft, id: e.pointerId };
    el.classList.add('is-dragging');
    try { el.setPointerCapture(e.pointerId); } catch (err) {}
  });
  container.addEventListener('pointermove', (e) => {
    if (!drag) return;
    drag.el.scrollLeft = drag.left - (e.clientX - drag.x);
  });
  const finish = (e) => {
    if (!drag) return;
    drag.el.classList.remove('is-dragging');
    try { drag.el.releasePointerCapture(e.pointerId); } catch (err) {}
    drag = null;
  };
  container.addEventListener('pointerup', finish);
  container.addEventListener('pointercancel', finish);
  container.addEventListener('scroll', (e) => {
    if (e.target && e.target.classList && e.target.classList.contains('upstream-addr')) {
      updateUpstreamAddrHint(e.target);
    }
  }, true);
}

function renderUpstreams(upstreams) {
  const container = document.getElementById('upstreams-list');
  if (!container) return;
  lastUpstreamsPayload = upstreams || [];
  // A poll that started before an add/remove can finish after it. Leave the
  // optimistic tick alone until that in-flight snapshot has drained.
  if (Date.now() >= upstreamPresetMutationUntil) {
    const addedNow = new Set((upstreams || []).map(u => u.address));
    const addedChanged = addedNow.size !== upstreamPresetAdded.size
      || [...addedNow].some(addr => !upstreamPresetAdded.has(addr));
    if (addedChanged) {
      upstreamPresetAdded = addedNow;
      if (upstreamPresetOpen) renderUpstreamPresetPanel();
    }
  }

  // If the settings snapshot is unavailable, the pickers are built from this
  // live list. Rebuild them once when the pool changes.
  const addressSet = (upstreams || []).map(u => u.address).sort().join('|');
  if (addressSet !== lastUpstreamsAddresses) {
    lastUpstreamsAddresses = addressSet;
    if (!(resolutionSettings.upstreams || []).length) setTimeout(syncResolutionControls, 0);
  }

  // Keep the resolver card's settings in step with the pool. Adding the first
  // DoT/DoH upstream is what enables the encrypted choice, and the picker must
  // not keep showing the old list until a page reload.
  const polledSet = (upstreams || []).map(u => u.address).sort().join('|');
  const knownSet = (resolutionSettings.upstreams || []).slice().sort().join('|');
  if (polledSet && polledSet !== knownSet && !resolutionLoading && Date.now() - lastResolutionSyncAt > 5000) {
    lastResolutionSyncAt = Date.now();
    loadResolutionSettings();
  }
  // Adaptive's last-run/current fields change on the server's schedule, not on
  // a pool edit. Refresh them occasionally so the card does not show yesterday's
  // benchmark forever.
  if (resolutionSettings.default_strategy === 'adaptive' && !resolutionLoading &&
      Date.now() - lastResolutionSyncAt > 30000) {
    lastResolutionSyncAt = Date.now();
    loadResolutionSettings();
  }

  if (!upstreams || upstreams.length === 0) {
    if (lastUpstreamsSig !== 'empty') {
      upstreamRowMap.clear();
      container.innerHTML = '<div class="text-slate-500 text-xs py-2">No upstreams configured</div>';
      lastUpstreamsSig = 'empty';
    }
    return;
  }
  // The policy is part of the signature: a changed transport or pin dims a
  // different set of rows even when the upstreams themselves did not move.
  const policySig = [
    (resolutionSettings.transports || []).join(','),
    resolutionSettings.default_upstream || '',
    normalizeStrategyValue(resolutionSettings.default_strategy),
    resolutionSettings.adaptive_current || '',
    (resolutionSettings.encrypted_upstreams || []).join(','),
  ].join('~');
  const sig = upstreams.map(u => [
    u.address, u.latency || 0, u.errors || 0, u.transport_errors || 0,
    u.policy_errors || 0, u.circuit_open ? 1 : 0, u.last_error || '',
    u.measured ? 1 : 0, u.samples || 0, u.recent_samples || 0,
    Math.round((u.recent_error_rate || 0) * 100), u.consecutive_failures || 0,
  ].join(':')).join('|') + '|' + policySig;
  if (sig === lastUpstreamsSig) return;
  lastUpstreamsSig = sig;

  // A selected transport promotes the resolvers it allows to the top, keeping the
  // server's latency order inside each group. Any Transport leaves the list alone.
  const renderTransport = resolutionSettings.transport || 'any';
  const ordered = (renderTransport === 'encrypted' || renderTransport === 'regular')
    ? upstreams.filter(u => !transportExcludes(u.address, renderTransport))
        .concat(upstreams.filter(u => transportExcludes(u.address, renderTransport)))
    : upstreams;

  const desiredAddresses = new Set(ordered.map(u => u.address));
  for (const [addr, row] of upstreamRowMap) {
    if (!desiredAddresses.has(addr)) {
      row.remove();
      upstreamRowMap.delete(addr);
    }
  }
  // Drop the empty-state placeholder once real rows exist.
  Array.from(container.children).forEach(el => { if (!el.dataset.addr) el.remove(); });
  const desiredRows = [];

  ordered.forEach(u => {
    // u.latency is nanoseconds, and it is absent until the racer has actually measured
    // this upstream — a freshly added one, or one that has never answered. The old code
    // did `(u.latency / 1000000).toFixed(1)` unconditionally and then compared the
    // resulting *string* to 40 and 80: for a missing latency that yields "NaN", both
    // comparisons are false, and the row rendered "NaN ms" wearing the green badge that
    // means "fastest". Zero was worse, because "0.0 ms" with a green badge reads as a
    // perfect upstream when it means nobody has timed it yet.
    const latency = Number(u.latency) / 1e6;
    // The server says whether it has actually measured this upstream. Fall back
    // to the numeric check for an older server that does not send the flag.
    const measured = (typeof u.measured === 'boolean')
      ? u.measured
      : (Number.isFinite(latency) && latency > 0);

    let badgeClass = 'text-slate-400 bg-slate-500/10 border-slate-600/40';
    let dotClass = 'w-2 h-2 rounded-full bg-slate-600';
    if (measured) {
      badgeClass = 'text-emerald-400 bg-emerald-500/10 border-emerald-500/30';
      dotClass = 'w-2 h-2 rounded-full bg-emerald-400';
      if (latency > 40) {
        badgeClass = 'text-yellow-400 bg-yellow-500/10 border-yellow-500/30';
      }
      if (latency > 80) {
        badgeClass = 'text-red-400 bg-red-500/10 border-red-500/30';
      }
    }
    // "not timed yet" and "timed at 0.4 ms" have to look different, and the title is
    // where the distinction is spelled out for whoever hovers it.
    const latText = measured ? `${latency.toFixed(1)} ms` : '—';
    const latencyIsProbe = !!u.latency_is_probe;
    const recentRate = Number(u.recent_error_rate) || 0;
    const recentSamples = Number(u.recent_samples) || 0;
    const failing = (Number(u.consecutive_failures) || 0) > 0
      || (recentSamples > 0 && recentRate >= 0.1);

    let latTitle = measured
      ? (latencyIsProbe
          ? 'Median of the latest warm benchmark probes — waiting for three live exchanges'
          : `Median of the last ${Number(u.samples).toLocaleString()} live exchanges`)
      : 'Not measured yet — no answer has been timed from this upstream';
    if (measured && Number(u.latency_p95_ms) > 0) {
      latTitle += ` · p95 ${Number(u.latency_p95_ms).toFixed(1)} ms`;
    }
    const errorParts = [];
    if (u.circuit_state === 'half-open') errorParts.push('half-open — probing one request');
    else if (u.circuit_open) errorParts.push('circuit open — retrying soon');
    if (recentSamples > 0) {
      errorParts.push(`recent errors ${Math.round(recentRate * 100)}% of ${recentSamples}`);
    }
    if (u.transport_errors || u.policy_errors) {
      errorParts.push(`live transport ${u.transport_errors || 0} · policy ${u.policy_errors || 0}`);
    }
    if (u.probe_hits || u.probe_errors || u.probe_policy) {
      errorParts.push(`probes ${u.probe_hits || 0} ok · ${(u.probe_errors || 0) + (u.probe_policy || 0)} failed`);
    }
    if (u.last_error) errorParts.push(`last: ${u.last_error}`);
    if (errorParts.length) latTitle += ` · ${errorParts.join(' · ')}`;

    // No extra chip or line: the dot carries current health, and the latency
    // badge's tooltip carries the numbers and the last error. The dot is not the
    // latency colour any more: a slow-but-healthy resolver stays green, and a
    // resolver that timed out once an hour ago stops looking permanently broken.
    if (u.circuit_open) {
      dotClass = 'w-2 h-2 rounded-full bg-red-400';
    } else if (failing) {
      dotClass = 'w-2 h-2 rounded-full bg-amber-400';
    }

    // A resolver the active transport excludes, or one a valid pin bypasses, is
    // dimmed but stays removable: the list is also the place removals happen.
    const dimmed = upstreamDimmed(u.address, resolutionSettings.transport || 'any');
    const adaptiveCurrent = normalizeStrategyValue(resolutionSettings.default_strategy) === 'adaptive'
      && resolutionSettings.adaptive_current === u.address;
    let row = upstreamRowMap.get(u.address);
    if (!row) {
      row = document.createElement('div');
      row.dataset.addr = u.address;
      row.className = 'upstream-row flex items-center justify-between py-1.5 px-3 rounded-lg bg-slate-950/70 border border-slate-800 text-xs'
        + (dimmed ? ' is-dimmed' : '')
        + (adaptiveCurrent ? ' is-adaptive-current' : '');
      // The left half is `min-w-0 truncate`: a flex item's default min-width:auto
      // resolves to its content width, so a long DoH URL (https://dns.google/dns-query)
      // would refuse to shrink and crush the latency badge on the right into a
      // square. min-w-0 lets the name shrink, truncate ellipsizes it, and the
      // badge keeps shrink-0 so it never wraps.
      row.innerHTML = `
        <div class="flex items-center gap-2 min-w-0 flex-1">
          <span class="upstream-dot shrink-0 ${dotClass}" aria-hidden="true"></span>
          ${providerFaviconMarkup(u.address)}
          <div class="min-w-0">
            <!-- The address is its own horizontal scroll area, not a clipped ellipsis:
                 a DoH URL is routinely wider than this column and the operator needs
                 to read the whole host to know which resolver a latency belongs to.
                 Drag it, scroll it, or focus and use the arrow keys; the title is the
                 hover fallback. The dot and the latency/delete controls never move. -->
            <span class="upstream-addr block min-w-0 font-mono text-slate-200" tabindex="0" title="${escapeHTML(u.address)}">${escapeHTML(u.address)}</span>
            <span class="upstream-fastest-badge">FASTEST</span>
          </div>
        </div>
        <div class="flex items-center gap-2 shrink-0">
          <span class="upstream-latency-badge shrink-0 px-2 py-0.5 rounded border ${badgeClass} text-[10px] font-bold font-mono whitespace-nowrap" title="${escapeHTML(latTitle)}">${latText}</span>
          <!-- 24x24, up from the 16x16 that a p-0.5 box around a 12px icon gave: this
               deletes a resolver from the live racer set and it sat under the WCAG 2.2
               SC 2.5.8 floor on the surface where it is only ever touched. Enlarging a
               one-tap destructive control on its own would trade a target you cannot hit
               for one you hit by mistake, so the handler now asks first — see the
               confirmAction call in the click delegate. -->
          <button class="remove-upstream inline-flex items-center justify-center w-6 h-6 shrink-0 rounded text-slate-500 hover:text-red-400 hover:bg-red-500/10 transition" data-addr="${escapeHTML(u.address)}" title="Remove upstream" aria-label="Remove upstream ${escapeHTML(u.address)}">
            ${FEATHER_TRASH_SVG}
          </button>
        </div>
      `;
      upstreamRowMap.set(u.address, row);
    } else {
      row.classList.toggle('is-dimmed', dimmed);
      row.classList.toggle('is-adaptive-current', adaptiveCurrent);
      const dot = row.querySelector('.upstream-dot');
      if (dot) dot.className = 'upstream-dot shrink-0 ' + dotClass;
      const badge = row.querySelector('.upstream-latency-badge');
      if (badge) {
        badge.className = 'upstream-latency-badge shrink-0 px-2 py-0.5 rounded border ' + badgeClass + ' text-[10px] font-bold font-mono whitespace-nowrap';
        if (badge.textContent !== latText) badge.textContent = latText;
        if (badge.title !== latTitle) badge.title = latTitle;
      }
    }
    desiredRows.push(row);
  });

  const existingRows = Array.from(container.children).filter(el => el.dataset.addr);
  const sameOrder = existingRows.length === desiredRows.length
    && existingRows.every((row, i) => row === desiredRows[i]);
  if (!sameOrder) {
    for (const row of desiredRows) container.appendChild(row);
  }

  // A row is only scrollable when its address actually overflows; mark it so the
  // edge fade and the grab cursor do not appear on short names.
  requestAnimationFrame(() => {
    container.querySelectorAll('.upstream-addr').forEach(updateUpstreamAddrHint);
  });
}

// =======================================================
// LIVE QUERY STREAM (SSE)
// =======================================================
async function startLiveStream() {
  // Single-flight: boot, visibility and a manual refresh can all ask for the
  // stream at once, and minting two tickets used to leave the loser's
  // EventSource orphaned.
  if (sseConnecting) return;
  if (sseSource && sseSource.readyState !== EventSource.CLOSED) return;
  sseConnecting = true;
  const generation = ++sseGeneration;
  if (sseBackoffTimer) {
    clearTimeout(sseBackoffTimer);
    sseBackoffTimer = null;
  }
  if (sseSource) {
    try { sseSource.close(); } catch(e) {}
    sseSource = null;
  }

  // The long-lived token no longer rides the query string. The
  // dashboard exchanges it (POST, authenticated) for a one-time, 60-second
  // ticket that the EventSource URL carries; the ticket is consumed on first
  // use, so a leaked URL authorises nothing afterwards.
  //
  // A rejected mint is the one signal that the session itself is gone (not a
  // transient drop): the ticket endpoint is Bearer-gated, so a 401 there means
  // the token is dead and no amount of reconnecting will help. Stop and hand
  // off to the normal re-auth path rather than looping.
  let ticketParam = '';
  if (authToken) {
    try {
      const tr = await fetch(api('/api/auth/sse-ticket'), {
        method: 'POST',
        headers: { 'Authorization': `Bearer ${authToken}` }
      });
      if (tr.status === 401) {
        console.warn('SSE: session expired; stopping stream and re-authenticating.');
        sseConnecting = false;
        showLoginModal();
        return;
      }
      if (tr.ok) {
        const tj = await tr.json();
        ticketParam = `?ticket=${encodeURIComponent(tj.ticket)}`;
      }
    } catch (e) { /* network blip: fall through and retry on backoff */ }
  }

  // A sign-out or refresh may have superseded this attempt while the ticket
  // was being minted.
  if (generation !== sseGeneration) {
    sseConnecting = false;
    return;
  }

  // The stream URL carries only a fresh ticket; the fallback to the long-lived
  // token is gone, so a failed mint means the connect will 401. Treat that as a
  // retryable drop (the mint itself may have hit a network blip) rather than
  // opening a stream we know is unauthenticated.
  if (!ticketParam) {
    sseConnecting = false;
    scheduleStreamReconnect();
    return;
  }

  let source;
  try {
    source = new EventSource(`${api('/api/stream/queries')}${ticketParam}`);
  } catch (e) {
    sseConnecting = false;
    scheduleStreamReconnect();
    return;
  }
  sseSource = source;
  sseConnecting = false;

  source.onopen = () => {
    // A live connection is the only proof the ticket was accepted: reset the
    // backoff so the next drop reconnects promptly.
    sseBackoffMs = 0;
    streamLastEventAt = Date.now();
    stopStreamFallback();
    scheduleStreamFallbackCheck();
    refreshStreamPlaceholder();
  };

  // Handle single query event
  source.addEventListener('query', (event) => {
    markStreamActivity();
    // No pause gate here: pushStreamQuery keeps buffering and only skips the DOM
    // insert. Dropping arrivals at this point made Resume show a hole.
    try {
      pushStreamQuery(JSON.parse(event.data));
    } catch (e) {
      console.error('Error parsing live query:', e);
    }
  });

  // Handle initial history batch
  source.addEventListener('history', (event) => {
    markStreamActivity();
    try {
      const list = JSON.parse(event.data);
      if (!Array.isArray(list)) return;
      finishStreamRefresh();
      // While paused the view must not change — a reconnect used to repaint the
      // history straight through the pause, which looked like Clear had failed.
      // Buffer the snapshot instead: Resume redraws from it.
      if (isStreamPaused) {
        streamBuffer = list.slice(0, STREAM_BUFFER_MAX);
        return;
      }
      seedStreamQueries(list);
    } catch (e) {
      console.error('Error parsing query history:', e);
    }
  });

  // Another dashboard (or this one, after a reconnect) cleared the log. Drop the
  // rows without posting again, or two tabs would clear each other in a loop.
  source.addEventListener('cleared', () => {
    finishStreamRefresh();
    resetStreamView();
  });

  // Fallback for default messages
  source.onmessage = (event) => {
    markStreamActivity();
    try {
      const q = JSON.parse(event.data);
      if (Array.isArray(q)) {
        if (isStreamPaused) {
          streamBuffer = q.slice(0, STREAM_BUFFER_MAX);
        } else {
          seedStreamQueries(q);
        }
      } else {
        // Same reasoning as the named query listener: buffer while paused.
        pushStreamQuery(q);
      }
    } catch (e) {}
  };

  // EventSource's own reconnect replays the spent ticket and 401s, so it is
  // never allowed to run: close the source here and drive the retry ourselves
  // with a fresh ticket on a silent exponential backoff.
  source.onerror = () => {
    if (sseSource !== source) return; // a newer attempt already replaced us
    try { source.close(); } catch(e) {}
    sseSource = null;
    scheduleStreamReconnect();
    // Keep the log usable through the reconnect. The polling path reads the
    // server's recent-log ring and stops as soon as the EventSource delivers.
    startStreamFallback();
    refreshStreamPlaceholder();
  };
}

// scheduleStreamReconnect re-mints a ticket and re-opens the stream after a
// silent exponential backoff (1s, 2s, 4s … capped at 30s). It is single-flight:
// a pending timer is never rescheduled, so a burst of error callbacks cannot
// stack overlapping reconnects. No user-facing message is shown — the log
// simply resumes when the connection is back.
function scheduleStreamReconnect() {
  if (sseBackoffTimer) return;
  if (!authToken) return; // signed out; nothing to reconnect to
  if (sseBackoffMs === 0) {
    sseBackoffMs = SSE_BACKOFF_START_MS;
  } else {
    sseBackoffMs = Math.min(sseBackoffMs * 2, SSE_BACKOFF_MAX_MS);
  }
  sseBackoffTimer = setTimeout(() => {
    sseBackoffTimer = null;
    startLiveStream();
  }, sseBackoffMs);
}

// markStreamActivity records a real SSE frame and stops the REST fallback. A
// working EventSource is always preferred: it is pushed, not polled.
function markStreamActivity() {
  streamLastEventAt = Date.now();
  if (streamFallbackTimer) stopStreamFallback();
}

function stopStreamFallback() {
  if (streamFallbackTimer) {
    clearInterval(streamFallbackTimer);
    streamFallbackTimer = null;
    streamFallbackSig = '';
    refreshStreamPlaceholder();
  }
}

function clearStreamFallbackCheck() {
  if (streamFallbackCheck) {
    clearTimeout(streamFallbackCheck);
    streamFallbackCheck = null;
  }
}

// scheduleStreamFallbackCheck gives the EventSource a few seconds to deliver its
// opening snapshot. If it never arrives, a proxy is buffering the stream or the
// browser refused it; the recent-log endpoint keeps the log usable.
function scheduleStreamFallbackCheck() {
  clearStreamFallbackCheck();
  streamFallbackCheck = setTimeout(() => {
    streamFallbackCheck = null;
    if (Date.now() - streamLastEventAt >= 3000) startStreamFallback();
  }, 3000);
}

function startStreamFallback() {
  if (streamFallbackTimer) return;
  streamFallbackSig = '';
  streamFallbackTimer = setInterval(streamFallbackPoll, 2000);
  refreshStreamPlaceholder();
  streamFallbackPoll();
}

async function streamFallbackPoll() {
  // Poll only while the log is actually on screen. When the operator opens it,
  // the first poll fills the buffer with the server's last hundred queries.
  if (activeDashTab !== 'stream' || document.visibilityState === 'hidden') return;
  if (streamFallbackInFlight || !authToken) return;
  streamFallbackInFlight = true;
  try {
    const res = await fetch(api('/api/stream/recent'), {
      headers: { 'Authorization': `Bearer ${authToken}` }
    });
    if (res.status === 401) {
      stopLiveStream();
      showLoginModal();
      return;
    }
    if (!res.ok) return;
    const data = await res.json();
    const list = Array.isArray(data) ? data : (data.queries || []);
    if (!Array.isArray(list) || list.length === 0) return;
    // Re-rendering an unchanged ring every two seconds would make the table
    // flicker and reset its scroll for nothing.
    const sig = list.map((q) => q.id).join(',');
    if (sig === streamFallbackSig) return;
    streamFallbackSig = sig;
    if (isStreamPaused) {
      streamBuffer = list.slice(0, STREAM_BUFFER_MAX);
      return;
    }
    seedStreamQueries(list);
  } catch (e) {
    // Keep the previous frame; the next tick retries.
  } finally {
    streamFallbackInFlight = false;
  }
}

function refreshStreamPlaceholder() {
  if (streamBuffer.length === 0) renderQueryStream();
}

// stopLiveStream tears the stream down and cancels any pending reconnect, so a
// sign-out does not race a backoff timer into re-opening an authenticated route.
function stopLiveStream() {
  // Invalidate any ticket mint still in flight, so it cannot open a stream
  // after sign-out or a manual refresh.
  sseGeneration++;
  sseConnecting = false;
  clearStreamFallbackCheck();
  stopStreamFallback();
  if (sseBackoffTimer) {
    clearTimeout(sseBackoffTimer);
    sseBackoffTimer = null;
  }
  sseBackoffMs = 0;
  if (sseSource) {
    try { sseSource.close(); } catch(e) {}
    sseSource = null;
  }
  finishStreamRefresh();
}

// setStreamPaused is the single owner of the paused flag and the pause button's
// label/aria state. The refresh button needs to resume the log before reconnecting,
// and duplicating that DOM update in two handlers is how the two drift apart.
function setStreamPaused(paused) {
  if (isStreamPaused === paused) return;
  isStreamPaused = paused;
  const btn = document.getElementById('stream-pause-btn');
  if (btn) {
    btn.innerHTML = paused
      ? '<i data-feather="play" class="w-3.5 h-3.5"></i> <span>Resume</span>'
      : '<i data-feather="pause" class="w-3.5 h-3.5"></i> <span>Pause</span>';
    btn.setAttribute('aria-pressed', paused ? 'true' : 'false');
    safeFeatherReplace();
  }
  if (!paused) renderQueryStream();
  else refreshStreamPlaceholder();
}

function setStreamRefreshBusy(busy) {
  const btn = document.getElementById('stream-refresh-btn');
  if (!btn) return;
  btn.disabled = busy;
  btn.classList.toggle('is-refreshing', busy);
  btn.setAttribute('aria-busy', busy ? 'true' : 'false');
}

function finishStreamRefresh() {
  if (streamRefreshTimer) {
    clearTimeout(streamRefreshTimer);
    streamRefreshTimer = null;
  }
  setStreamRefreshBusy(false);
}

// refreshLiveStream reconnects the SSE stream, which makes the server send a fresh
// history snapshot. It resumes a paused view: a refresh that leaves the table frozen
// would look like the button did nothing.
async function refreshLiveStream() {
  if (!authToken) return;
  stopLiveStream();
  setStreamPaused(false);
  setStreamRefreshBusy(true);
  await startLiveStream();
  if (streamRefreshTimer) clearTimeout(streamRefreshTimer);
  streamRefreshTimer = setTimeout(() => {
    streamRefreshTimer = null;
    setStreamRefreshBusy(false);
    showToast('Live log refresh is taking longer than expected; reconnecting in the background.', 'warning');
  }, 6000);
}

function escapeHTML(str) {
  if (!str) return '';
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}

// streamFilterState reads the two controls once per render rather than once per row.
function streamFilterState() {
  const filterEl = document.getElementById('stream-filter');
  const searchEl = document.getElementById('stream-search');
  return {
    action: filterEl ? filterEl.value : 'ALL',
    search: searchEl ? searchEl.value.toLowerCase().trim() : '',
  };
}

// The live log and the aggregate counters treat the IPv6 sink and the HTTPS/SVCB
// sink as proxy outcomes: the client's route was Proxy even though the answer
// was synthesized locally. One predicate keeps the filter, the relay cell and
// the counters from disagreeing about the same query.
function isProxyAction(action) {
  return action === 'PROXY' ||
    action === 'PROXY_IPV6_SINK' ||
    action === 'PROXY_HTTPS_SINK';
}

// queryMatchesStream decides whether one query belongs in the current view. CACHED is handled
// apart from the others because it is not an action — it is a flag that can accompany any of
// them, so comparing it against q.action would match nothing.
function queryMatchesStream(q, f) {
  if (f.action !== 'ALL') {
    if (f.action === 'CACHED') {
      if (!q.cached) return false;
    } else if (f.action === 'DIRECT') {
      // DNS Rewrite answers are local overrides, so they belong in the Direct view.
      if (q.action !== 'DIRECT' && q.action !== 'CUSTOM') return false;
    } else if (f.action === 'PROXY') {
      // The two sink actions are proxy routes whose answer was answered locally.
      if (!isProxyAction(q.action)) return false;
    } else if (q.action !== f.action) {
      return false;
    }
  }
  if (!f.search) return true;
  const domain = (q.domain || '').toLowerCase();
  const clientIP = q.client_ip || '';
  const accountName = (q.account_name || 'Public').toLowerCase();
  return domain.includes(f.search) || clientIP.includes(f.search) || accountName.includes(f.search);
}

// Every action string the resolver can log, mapped to the badge that stands for it. This is
// a table and not a chain of ifs because the previous chain defaulted to a green DIRECT
// badge: a query that was rate limited, was refused for a blown traffic quota, or had its
// AAAA sunk to keep a proxied domain off IPv6 all read as "resolved normally, straight out to
// the internet" — the opposite of what the engine did. The values are whole literal spans
// rather than a class plus a label so that nothing is interpolated into an attribute here.
// An action the frontend has never heard of is shown verbatim in a neutral badge instead of
// being dressed up as DIRECT.
// Keep in sync with the logQuery call sites in internal/core/dns/handler.go.
const STREAM_ACTION_BADGES = {
  PROXY: '<span class="badge badge-proxy">PROXY</span>',
  PROXY_IPV6_SINK: '<span class="badge badge-proxy">PROXY · IPv6 SINK</span>',
  PROXY_HTTPS_SINK: '<span class="badge badge-proxy">PROXY · HTTPS SINK</span>',
  PROXY_FAILED: '<span class="badge badge-block">PROXY FAILED</span>',
  SERVFAIL: '<span class="badge badge-block">SERVFAIL</span>',
  BLOCK: '<span class="badge badge-block">BLOCK</span>',
  RATELIMIT: '<span class="badge badge-block">RATE LIMITED</span>',
  QUOTA: '<span class="badge badge-block">QUOTA</span>',
  CUSTOM: '<span class="badge badge-rewrite">Rewrite</span>',
  CACHED: '<span class="badge badge-direct">DIRECT</span>',
  STALE: '<span class="badge badge-cached">STALE</span>',
  DIRECT: '<span class="badge badge-direct">DIRECT</span>',
};

// Colour per protocol for the PROTO column. The shown value is the transport that
// resolved the query (upstream_protocol), falling back to the client protocol when
// nothing upstream answered. Unknown or blank values stay muted.
const PROTO_COLORS = {
  UDP: 'text-blue-400',
  TCP: 'text-sky-400',
  DOT: 'text-emerald-400',
  DOH: 'text-purple-400',
  DOQ: 'text-amber-400',
  DOH3: 'text-orange-400',
  DNSCRYPT: 'text-fuchsia-400',
};

function protocolClass(protocol) {
  return PROTO_COLORS[String(protocol || '').toUpperCase()] || 'text-slate-500';
}

// Wire values are upper-case; the UI uses the conventional mixed-case names.
function protocolLabel(protocol) {
  switch (String(protocol || '').toUpperCase()) {
    case 'DOT': return 'DoT';
    case 'DOH': return 'DoH';
    case 'DOQ': return 'DoQ';
    case 'DOH3': return 'DoH3';
    case 'DNSCRYPT': return 'DNSCrypt';
    default: return String(protocol || '').trim();
  }
}

// queryRowHTML renders one query into the eight cells of the stream table. Everything that
// came off the wire is escaped — the row reaches the DOM through innerHTML, and a domain is
// whatever a client asked this resolver to look up.
//
// The third and fifth cells (PROTO, RULE MATCHED) carry `hidden sm:table-cell` to match the
// two <th> in index.html that do the same. The pair has to stay in sync: hide a header
// without its cell and every column after it shifts one place left on a phone.
function queryRowHTML(q) {
  const domain = q.domain || '';
  const clientIP = q.client_ip || '';
  const accountName = q.account_name || 'Public';

  const action = q.action || 'DIRECT';
  // Relay is the route's exit, not part of the PROXY badge. It gets its own
  // display-only column, matching the Domains table; editing stays there. The
  // two sink actions also left through the proxy, so they keep the same exit.
  const proxied = isProxyAction(action);
  const relayName = proxied
    ? String(q.relay_egress_name || q.relay_agent_name || 'This Server').trim()
    : '';
  const relayCountry = proxied
    ? String(q.relay_country || (currentConfig?.server?.country || '')).trim()
    : '';
  const relayFlag = relayCountry && flagURL(relayCountry)
    ? `<img class="agent-flag agent-flag-sm" src="${escapeHTML(flagURL(relayCountry))}" alt="${escapeHTML(countryName(relayCountry))}" title="${escapeHTML(countryName(relayCountry))}">`
    : '';
  const relayKind = proxied && q.relay_egress_kind
    ? (q.relay_egress_kind === 'wireguard' ? 'WireGuard' : 'SOCKS5')
    : '';
  const relayFallback = proxied ? String(q.relay_fallback_reason || '').trim() : '';
  const relayTitle = [relayName, relayKind, relayCountry ? countryName(relayCountry) : '', relayFallback]
    .filter(Boolean).join(' · ');
  const relayCell = proxied
    ? `<span class="stream-relay" title="Relay: ${escapeHTML(relayTitle)}">${relayFlag}<span>${escapeHTML(relayName)}</span></span>`
    : '<span class="text-slate-600">—</span>';
  const actBadgeInner = STREAM_ACTION_BADGES[action]
    || `<span class="badge badge-unknown">${escapeHTML(action)}</span>`;
  // The badge is the trigger for the quick-action menu: it carries the row's
  // domain, and clicking it opens the one shared popover. It is a <button> so it
  // is focusable and Enter/Space reach it; the badge styling stays on the inner
  // span, so the wrapper carries no decoration of its own.
  const actBadge = `<button type="button" class="stream-action-btn" data-domain="${escapeHTML(domain)}" aria-haspopup="menu" aria-expanded="false" title="Quick actions for ${escapeHTML(domain)}">${actBadgeInner}</button>`;

  let cacheBadge = q.cached ? '<span class="badge badge-cached ms-1">RAM</span>' : '';
  const timeStr = q.timestamp ? new Date(q.timestamp).toLocaleTimeString() : new Date().toLocaleTimeString();
  const latVal = (typeof q.latency_ms === 'number') ? q.latency_ms : ((typeof q.latency === 'number') ? q.latency / 1000000 : 0.0);
  const latStr = latVal.toFixed(1) + ' ms';
  const ruleStr = q.rule_name || q.rule_matched || q.rule || 'Default Direct';
  // The PROTO column describes the resolution leg: the resolver's transport
  // when one answered, and the client's transport only when nothing upstream
  // was involved (cache, local, blocked, proxied). The client protocol stays in
  // the tooltip and in the API payload.
  const clientProto = q.protocol || 'UDP';
  const resolutionProto = (q.upstream_protocol || '').trim();
  const protoWire = resolutionProto || clientProto;
  const protoStr = protocolLabel(protoWire);
  const protoTitle = resolutionProto
    ? `Resolution: ${protocolLabel(resolutionProto)} · Client: ${protocolLabel(clientProto)}`
    : `Client: ${protocolLabel(clientProto)}`;
  const protoClass = protocolClass(protoWire);

  // clientIP is escaped like the domain beside it. It is normally a parsed address,
  // but on DoH it can originate from a forwarding header, and the row is written with
  // innerHTML — so the value is treated as data, not markup, rather than relying on
  // every producer upstream to have parsed it first.
  let clientDisplay = `<div class="flex flex-col"><span class="text-cyan-400 font-bold font-mono text-xs leading-tight">${escapeHTML(accountName)}</span><span class="text-[10px] text-slate-400 font-mono">${escapeHTML(clientIP)}</span></div>`;
  if (accountName === 'Public') {
    clientDisplay = `<div class="flex flex-col"><span class="text-slate-300 font-mono text-xs leading-tight">${escapeHTML(clientIP)}</span><span class="text-[10px] text-slate-500 font-mono">Public / Direct</span></div>`;
  }

  // Which resolver produced this answer. Empty for blocked/proxied/local and
  // "cache" for a RAM hit; shown under the domain so it is visible at every
  // width, including phones where the RULE column is hidden.
  const upstream = (q.upstream || '').trim();
  const upstreamLine = upstream
    ? `<div class="text-[10px] text-slate-500 truncate" title="${escapeHTML(upstream)}">via ${escapeHTML(upstream)}</div>`
    : '';
  const matchedPattern = String(q.matched_pattern || '').trim();
  // On a phone the RULE column is hidden, so the block source has to live under
  // the domain. It is also the answer to "why was this blocked?" on every width:
  // Custom Block, or the exact adblock list that matched, plus the pattern.
  const blockDetail = action === 'BLOCK'
    ? `<div class="text-[10px] text-red-300 truncate" title="${escapeHTML(ruleStr + (matchedPattern ? ' · ' + matchedPattern : ''))}">Blocked by ${escapeHTML(ruleStr)}${matchedPattern ? ` · ${escapeHTML(matchedPattern)}` : ''}</div>`
    : upstreamLine;

  return `
    <td class="stream-col-time py-2.5 px-2 sm:px-3 text-slate-400 whitespace-nowrap">${timeStr}</td>
    <td class="stream-col-client py-2.5 px-2 sm:px-3">${clientDisplay}</td>
    <td class="stream-col-proto py-2.5 px-2 sm:px-3 ${protoClass} font-bold hidden sm:table-cell" title="${escapeHTML(protoTitle)}">${escapeHTML(protoStr)}</td>
    <td class="stream-col-domain py-2.5 px-2 sm:px-3">
      <div class="text-cyan-300 font-semibold truncate" title="${escapeHTML(domain)}">${escapeHTML(domain)}</div>
      ${blockDetail}
    </td>
    <td class="stream-col-rule py-2.5 px-2 sm:px-3 text-slate-400 hidden sm:table-cell">${escapeHTML(ruleStr)}</td>
    <td class="stream-col-relay py-2.5 px-2 sm:px-3">${relayCell}</td>
    <td class="stream-col-action py-2.5 px-2 sm:px-3">${actBadge}${cacheBadge}</td>
    <td class="stream-col-latency py-2.5 px-2 sm:px-3 text-end text-emerald-400 font-mono">${latStr}</td>
  `;
}

// newQueryRow builds the <tr> for one query. The class list is shared by every data row, and
// the absence of stream-placeholder on it is what tells renderQueryStream and the append path
// apart from the two placeholder states.
function newQueryRow(q) {
  const row = document.createElement('tr');
  row.className = 'hover:bg-slate-800/40 transition border-b border-slate-800/40';
  row.innerHTML = queryRowHTML(q);
  return row;
}

// The two placeholder rows. Both carry stream-placeholder, which is how they are recognised —
// the old code matched tbody.children[0].innerText against the string 'Listening', so
// rewording the copy in index.html would have left the placeholder stuck above the first real
// query forever, with nothing anywhere to say why.
function streamListeningRow() {
  let text = 'Listening for live DNS queries...';
  if (isStreamPaused) {
    text = 'Live log paused. Press Resume to continue.';
  } else if (streamFallbackTimer) {
    text = 'Live log connected via polling — waiting for queries...';
  } else if (!sseSource) {
    text = 'Connecting to the live log...';
  }
  return `
  <tr class="stream-placeholder">
    <td colspan="8" class="py-12 text-center text-slate-500">
      <div class="flex flex-col items-center gap-2">
        <div class="pulse-dot"></div>
        <span>${text}</span>
      </div>
    </td>
  </tr>`;
}

// The no-match state names the term and the number of queries held, because "no queries have
// arrived" and "none of the 200 I am holding match this" are opposite situations: the first is
// a resolver nobody is using, the second is a filter to clear. They used to look identical —
// an empty table — and a filtered-out arrival even consumed the Listening placeholder on its
// way to being dropped, so the panel went blank and stayed blank.
function streamNoMatchRow(f) {
  const bits = [];
  if (f.action !== 'ALL') bits.push(`action <span class="text-slate-300">${escapeHTML(f.action)}</span>`);
  if (f.search) bits.push(`&ldquo;<span class="text-slate-300">${escapeHTML(f.search)}</span>&rdquo;`);
  // Unreachable today — with neither control set, every query matches and this row is not
  // rendered — but it keeps the sentence grammatical if that ever stops being true.
  const what = bits.length ? bits.join(' and ') : 'the current filter';
  const held = streamBuffer.length;
  const lead = held === 1
    ? 'The one query received so far does not match'
    : `None of the last ${held} queries match`;
  return `
    <tr class="stream-placeholder">
      <td colspan="8" class="py-12 text-center text-slate-500">
        <div class="flex flex-col items-center gap-1.5">
          <div class="font-bold text-slate-400 font-heading">No Match</div>
          <p class="text-xs">${lead} ${what}.</p>
          <button class="clear-stream-filter-btn mt-1.5 text-[11px] font-bold text-cyan-400 border border-cyan-500/30 bg-cyan-500/10 px-3 py-1.5 rounded-lg hover:bg-cyan-500/20 transition">
            Show all queries
          </button>
        </div>
      </td>
    </tr>`;
}

// renderQueryStream redraws the table from the buffer. This is what the filter and the search
// box call, so a change applies to the queries already received instead of only to the next
// one to arrive.
function renderQueryStream() {
  const tbody = document.getElementById('stream-tbody');
  if (!tbody) return;

  // A full redraw rebuilds the table from the buffer, so whatever the eviction list
  // held is already accounted for and must not be replayed against the new rows.
  streamPendingDropped = [];

  if (streamBuffer.length === 0) {
    tbody.innerHTML = streamListeningRow();
    streamRendered = [];
    return;
  }

  const f = streamFilterState();
  const rows = streamBuffer.filter((q) => queryMatchesStream(q, f)).sort(compareStreamQueries);
  if (rows.length === 0) {
    tbody.innerHTML = streamNoMatchRow(f);
    streamRendered = [];
    return;
  }

  // One reflow rather than one per row: on a busy resolver this runs on every keystroke.
  const frag = document.createDocumentFragment();
  for (const q of rows) {
    frag.appendChild(newQueryRow(q));
  }
  tbody.innerHTML = '';
  tbody.appendChild(frag);
  streamRendered = rows;
}

// insertStreamQuery places one arriving query into the table without rebuilding the
// rest. pushStreamQuery used to call renderQueryStream() on every animation frame, so
// under load the whole 200-row table was torn down and rebuilt up to 60 times a second:
// that is the flicker an operator sees as "the list is flashing". Inserting the single
// new row leaves every existing row — and its layout, focus and scroll — untouched.
//
// The insert index is the query's position in the filtered, sorted buffer, which is the
// order the DOM mirrors. For the default order that is always the top; for the others it
// is found by a linear scan of the (at most STREAM_BUFFER_MAX) buffered rows.
function insertStreamQuery(q) {
  const tbody = document.getElementById('stream-tbody');
  if (!tbody) return;

  const f = streamFilterState();
  if (!queryMatchesStream(q, f)) return;
  if (tbody.querySelector('.stream-placeholder')) {
    renderQueryStream();
    return;
  }

  // The slot is found among the rows already drawn, not among the whole buffer: a
  // batch arriving in one frame is all in the buffer before any of it is in the DOM,
  // so a buffer index would count rows this pass has not inserted yet and land the
  // row one or more places too far down. streamRendered is exactly the drawn rows.
  let lo = 0;
  let hi = streamRendered.length;
  while (lo < hi) {
    const mid = (lo + hi) >> 1;
    if (compareStreamQueries(streamRendered[mid], q) <= 0) lo = mid + 1;
    else hi = mid;
  }
  tbody.insertBefore(newQueryRow(q), tbody.children[lo] || null);
  streamRendered.splice(lo, 0, q);
}

// ---- Live-log quick actions --------------------------------------------------
//
// Clicking a row's ACTION badge opens one shared menu (declared in index.html,
// because app.js never creates element ids). The menu names the domain and offers
// a scope — the exact host, its own wildcard, or the parent wildcard — plus four
// actions that rewrite the routing lists. Applied through saveRules, so a quick
// action is the same write as editing the Proxied or Rules tab by hand.
let streamActionDomain = '';
let streamActionScope = 'exact';
// Patterns written by a scope click in the current menu session. They let the next
// click undo the previous conversion even when the origin domain no longer appears
// in any list (the *.parent case).
let streamActionApplied = [];

// streamManagedPatterns is every domain pattern the routing lists already hold,
// across all four statuses. The quick-action menu reads it to open on the scope
// that is actually configured instead of resetting to exact on every open.
function streamManagedPatterns() {
  const rules = (currentConfig && currentConfig.rules) || {};
  const patterns = new Set();
  for (const key of ['custom_proxied', 'custom_proxied_disabled', 'custom_blocked', 'custom_direct']) {
    for (const d of rules[key] || []) {
      const pattern = String(d || '').trim().toLowerCase().replace(/\.$/, '');
      if (pattern) patterns.add(pattern);
    }
  }
  return patterns;
}

// The last scope the operator picked for a name. Reopening the menu keeps that
// choice instead of snapping back to exact; it is session state, not a saved rule.
const streamActionScopeMemory = new Map();

// streamScopeTargets lists every pattern a scope writes.
//
// - exact  : the name itself (a wildcard row stays its wildcard)
// - *.host : subdomains of the name only; on a wildcard row that is the row itself
// - host+* : the name AND its subdomains (two entries, so the host stays covered)
// - *.parent: the parent's subtree, which already contains the name
//
// Nothing is written for a scope that would name a bare TLD, and host+* is not
// offered on a row that is already a wildcard (it would duplicate *.host).
function streamScopeTargets(domain, scope) {
  const raw = String(domain || '').trim().toLowerCase().replace(/^\.+/, '');
  if (!raw) return [];
  const wildcard = raw.startsWith('*.');
  const base = wildcard ? raw.slice(2) : raw;
  const labels = base.split('.').filter(Boolean);
  // Exact is the host itself. On a wildcard row that means dropping the "*.", so
  // selecting Exact converts *.a.b.c back to a.b.c instead of being a no-op.
  if (scope === 'exact') return [base];
  if (scope === 'host') return labels.length >= 2 ? ['*.' + base] : [];
  if (scope === 'parent') {
    return labels.length >= 3 ? ['*.' + labels.slice(1).join('.')] : [];
  }
  return [];
}

// The four rule lists, in the precedence the Domains rows use. A scope change keeps
// the list the name is already in, so the status is preserved.
const STREAM_ACTION_LISTS = [
  ['custom_proxied', 'proxy'],
  ['custom_proxied_disabled', 'paused'],
  ['custom_blocked', 'blocked'],
  ['custom_direct', 'direct'],
];

// replacePatterns rewrites one rule list in place: the new patterns take the slot of
// the first removed one, so a scope change never moves the row to the end of the
// list (and therefore never to the top under "Added (newest)"). With nothing to
// remove the patterns are appended, which is the live-log case.
function replacePatterns(list, remove, add) {
  const first = list.findIndex((d) => remove.includes(d) || add.includes(d));
  const keep = (d) => !remove.includes(d) && !add.includes(d);
  const kept = list.filter(keep);
  if (first < 0) return kept.concat(add);
  const at = list.slice(0, first).filter(keep).length;
  return [...kept.slice(0, at), ...add, ...kept.slice(at)];
}

// streamRowStamp returns the added-at stamp already stored for a row, from any of its
// patterns or its base, so a rename keeps the original position in "Added" sorting.
function streamRowStamp(domain) {
  const added = (currentConfig?.rules?.custom_domain_added) || {};
  const base = domain.startsWith('*.') ? domain.slice(2) : domain;
  for (const pattern of [...streamRowPatterns(domain), base]) {
    if (added[pattern]) return added[pattern];
  }
  return '';
}

// streamRowPatterns is what "this row's rule" means: a wildcard row owns itself, a
// plain row owns its exact entry and its host wildcard. Switching scope replaces
// these, so choosing *.parent cannot leave *.host behind.
function streamRowPatterns(domain) {
  const raw = String(domain || '').trim().toLowerCase();
  if (!raw) return [];
  const wildcard = raw.startsWith('*.');
  const base = wildcard ? raw.slice(2) : raw;
  return wildcard ? [raw] : [base, '*.' + base];
}

// streamActionListKey finds the list the clicked name is stored in, or null when the
// name has no rule yet. A live-log host may only have its wildcard stored, so any of
// the row's patterns counts.
function streamActionListKey(domain) {
  const rules = currentConfig && currentConfig.rules;
  if (!rules || !domain) return null;
  const rowPatterns = streamRowPatterns(domain);
  for (const [key, status] of STREAM_ACTION_LISTS) {
    if (rowPatterns.some((p) => (rules[key] || []).includes(p))) return { key, status };
  }
  return null;
}

// applyStreamScope rewrites an existing rule in place when the operator picks a scope.
// Selecting *.host used to only arm the next action, so the line under the domain
// stayed "exact" and the change looked unsaved. Now the row's own patterns are
// replaced in the same list and the subtitle updates immediately. A name with no rule
// is left alone: there the scope stays a modifier for Proxy/Block/Direct.
function applyStreamScope(domain, scope) {
  const rules = currentConfig && currentConfig.rules;
  // After *.parent the origin domain is gone from every list; the pattern the last
  // click wrote is where the rule now lives, so use it to find the status.
  const current = streamActionListKey(domain) ||
    (streamActionApplied.length ? streamActionListKey(streamActionApplied[0]) : null);
  if (!rules || !current) return false;
  const targets = streamScopeTargets(domain, scope);
  if (!targets.length) return false;
  const rowPatterns = streamRowPatterns(domain);
  const stamp = streamRowStamp(domain) || streamRowStamp(streamActionApplied[0] || '');
  const remove = [...new Set([...rowPatterns, ...streamActionApplied, ...targets])];
  for (const [key] of STREAM_ACTION_LISTS) {
    if (key === current.key) continue;
    rules[key] = (rules[key] || []).filter((x) => !remove.includes(x));
  }
  rules[current.key] = replacePatterns(rules[current.key] || [], remove, targets);
  const added = rules.custom_domain_added || {};
  for (const pattern of rowPatterns) delete added[pattern];
  if (stamp) for (const target of targets) added[target] = stamp;
  rules.custom_domain_added = added;
  streamActionApplied = targets;
  saveRules();
  renderConfig(currentConfig);
  renderQueryStream();
  showToast(`Scope: ${targets.join(' + ')}`, 'success');
  return true;
}

// streamActionTarget is the single-pattern form, kept for callers that only need
// the primary target (host+* returns the exact host first).
function streamActionTarget(domain, scope) {
  return streamScopeTargets(domain, scope)[0] || '';
}

// currentStreamScope picks the scope whose pattern is already in the routing
// lists. host+* wins when both its entries exist, then exact, then the host
// wildcard, then the parent wildcard.
function currentStreamScope(domain, patterns) {
  const raw = String(domain || '').trim().toLowerCase().replace(/^\.+/, '');
  if (!raw) return 'exact';
  const wildcard = raw.startsWith('*.');
  const base = wildcard ? raw.slice(2) : raw;
  const labels = base.split('.').filter(Boolean);
  if (!wildcard) {
    // A plain rule already covers the host and every subdomain, so it is the
    // broadest thing that can be stored for this row.
    if (patterns.has(base)) return 'exact';
    if (labels.length >= 2 && patterns.has('*.' + base)) return 'host';
  } else if (patterns.has(raw)) {
    // The wildcard row's own rule is the host wildcard.
    return 'host';
  }
  if (labels.length >= 3 && patterns.has('*.' + labels.slice(1).join('.'))) return 'parent';
  return 'exact';
}

// One-line explanation for each scope; the exact rule covers subdomains too, which
// the bare label cannot say on its own.
const STREAM_SCOPE_HINTS = {
  exact: 'host + subdomains',
  host: 'subdomains only',
  parent: 'parent subtree',
};

// updateStreamActionScopes disables the scopes that would produce nothing. That is
// both the "which wildcard am I applying" hint and the fix for selecting *.parent
// on a bare host, which used to fail only after the action was chosen.
function updateStreamActionScopes(domain) {
  streamActionScopeButtons().forEach((b) => {
    const targets = streamScopeTargets(domain, b.dataset.scope || 'exact');
    b.disabled = targets.length === 0;
    const hint = STREAM_SCOPE_HINTS[b.dataset.scope] || '';
    b.title = targets.length
      ? `Apply to ${targets.join(' + ')} (${hint})`
      : 'Not available for this domain';
    if (!targets.length) {
      b.classList.remove('is-active');
      b.setAttribute('aria-checked', 'false');
    }
  });
}

// updateStreamActionPreview shows the exact patterns the chosen scope writes.
// The scope labels are short on purpose: without this, *.host on
// s-imp.rmp.rakuten.com looks like it covers that host, while it writes
// *.s-imp.rmp.rakuten.com and covers its subdomains only.
function updateStreamActionPreview() {
  const el = document.getElementById('stream-action-target');
  if (!el) return;
  const targets = streamScopeTargets(streamActionDomain, streamActionScope);
  if (!targets.length) {
    el.textContent = 'No pattern for this scope';
    el.title = '';
    return;
  }
  const hint = STREAM_SCOPE_HINTS[streamActionScope] || '';
  el.textContent = `applies to ${targets.join(' + ')} (${hint})`;
  el.title = targets.join(' + ');
}

function streamActionScopeButtons() {
  const menu = document.getElementById('stream-action-menu');
  return menu ? menu.querySelectorAll('.stream-action-scope') : [];
}

function openStreamActionMenu(trigger) {
  const menu = document.getElementById('stream-action-menu');
  if (!menu) return;
  streamActionDomain = trigger.dataset.domain || '';
  streamActionApplied = [];
  const patterns = streamManagedPatterns();
  // Keep the last scope picked for this name; fall back to the one its rules imply.
  const remembered = streamActionScopeMemory.get(streamActionDomain);
  streamActionScope = remembered && streamScopeTargets(streamActionDomain, remembered).length
    ? remembered
    : currentStreamScope(streamActionDomain, patterns);
  const domainEl = document.getElementById('stream-action-domain');
  if (domainEl) domainEl.textContent = streamActionDomain;
  updateStreamActionScopes(streamActionDomain);
  // The chosen scope can be disabled on a row whose base has no parent; fall back
  // to exact rather than opening with nothing selected.
  if (!streamScopeTargets(streamActionDomain, streamActionScope).length) streamActionScope = 'exact';
  streamActionScopeButtons().forEach((b) => {
    const active = b.dataset.scope === streamActionScope && !b.disabled;
    b.classList.toggle('is-active', active);
    b.setAttribute('aria-checked', active ? 'true' : 'false');
  });
  updateStreamActionPreview();
  document.querySelectorAll('.stream-action-btn[aria-expanded="true"], .proxied-route-btn[aria-expanded="true"]')
    .forEach((b) => b.setAttribute('aria-expanded', 'false'));
  trigger.setAttribute('aria-expanded', 'true');
  menu.classList.remove('hidden');
  positionStreamActionMenu(trigger, menu);
}

// positionStreamActionMenu anchors the menu under the badge, flipping above it and
// clamping to the viewport so a badge in the last row or at the right edge still
// opens a menu that is fully on screen.
function positionStreamActionMenu(trigger, menu) {
  const r = trigger.getBoundingClientRect();
  const mw = menu.offsetWidth;
  const mh = menu.offsetHeight;
  let left = Math.min(r.left, window.innerWidth - mw - 8);
  let top = r.bottom + 6;
  if (top + mh > window.innerHeight - 8) top = Math.max(8, r.top - mh - 6);
  menu.style.left = Math.max(8, left) + 'px';
  menu.style.top = top + 'px';
}

function closeStreamActionMenu() {
  const menu = document.getElementById('stream-action-menu');
  if (menu) menu.classList.add('hidden');
  document.querySelectorAll('.stream-action-btn[aria-expanded="true"], .proxied-route-btn[aria-expanded="true"]')
    .forEach((b) => b.setAttribute('aria-expanded', 'false'));
}

// The log badge an applied action should show, so the row reflects the choice
// immediately instead of waiting for the next matching query.
const STREAM_ACTION_LOG_BADGE = { proxy: 'PROXY', block: 'BLOCK', direct: 'DIRECT' };

function applyStreamAction(action) {
  if (!currentConfig || !currentConfig.rules || !streamActionDomain) return;
  const targets = streamScopeTargets(streamActionDomain, streamActionScope);
  if (!targets.length) {
    showToast('That scope has no specific domain to match', 'error');
    closeStreamActionMenu();
    return;
  }
  streamActionScopeMemory.set(streamActionDomain, streamActionScope);
  const rules = currentConfig.rules;
  const lists = {
    custom_proxied: Array.isArray(rules.custom_proxied) ? rules.custom_proxied : [],
    custom_proxied_disabled: Array.isArray(rules.custom_proxied_disabled) ? rules.custom_proxied_disabled : [],
    custom_blocked: Array.isArray(rules.custom_blocked) ? rules.custom_blocked : [],
    custom_direct: Array.isArray(rules.custom_direct) ? rules.custom_direct : [],
  };
  // Every action starts from a clean slate: a domain lives in exactly one list,
  // so switching it from blocked to proxied is a move, not an addition to both.
  const rowPatterns = streamRowPatterns(streamActionDomain);
  const stamp = streamRowStamp(streamActionDomain) || streamRowStamp(streamActionApplied[0] || '');
  const remove = [...new Set([...rowPatterns, ...streamActionApplied, ...targets])];
  const targetKey = { proxy: 'custom_proxied', block: 'custom_blocked', direct: 'custom_direct' }[action] || '';
  for (const key of Object.keys(lists)) {
    if (key === targetKey) continue;
    lists[key] = lists[key].filter((x) => !remove.includes(x));
  }
  if (!targetKey) return;
  lists[targetKey] = replacePatterns(lists[targetKey], remove, targets);

  rules.custom_proxied = lists.custom_proxied;
  rules.custom_proxied_disabled = lists.custom_proxied_disabled;
  rules.custom_blocked = lists.custom_blocked;
  rules.custom_direct = lists.custom_direct;
  streamActionApplied = [];
  const added = rules.custom_domain_added || {};
  for (const pattern of rowPatterns) delete added[pattern];
  if (stamp) for (const target of targets) added[target] = stamp;
  rules.custom_domain_added = added;

  // Rewrite the buffered rows for this exact host so the badge the operator just
  // acted on (and any sibling row for the same host) shows the new action at once.
  const logBadge = STREAM_ACTION_LOG_BADGE[action];
  if (logBadge) {
    streamBuffer.forEach((q) => {
      if ((q.domain || '') === streamActionDomain) q.action = logBadge;
    });
  }

  const label = { proxy: 'Proxied', block: 'Blocked', direct: 'Set direct' }[action] || 'Updated';
  // Only a wildcard-only target leaves the clicked host out, so say that once here
  // as well as in the menu preview: otherwise the original row keeps resolving and
  // the action reads as a no-op. host+* includes the host, so it needs no warning.
  const scopeNote = targets.every((t) => t.startsWith('*.')) ? ' (subdomains only)' : '';
  const target = targets.join(' + ');
  saveRules();
  renderConfig(currentConfig);
  renderQueryStream();
  showToast(`${label} ${target}${scopeNote}`, 'success');
  closeStreamActionMenu();
}

function onStreamActionMenuClick(e) {
  const scopeBtn = e.target.closest('.stream-action-scope');
  if (scopeBtn) {
    streamActionScope = scopeBtn.dataset.scope || 'exact';
    streamActionScopeMemory.set(streamActionDomain, streamActionScope);
    streamActionScopeButtons().forEach((b) => {
      const active = b === scopeBtn;
      b.classList.toggle('is-active', active);
      b.setAttribute('aria-checked', active ? 'true' : 'false');
    });
    updateStreamActionPreview();
    // Rows that already have a rule change scope on the spot; nothing is written for
    // a name with no rule until an action button is pressed.
    applyStreamScope(streamActionDomain, streamActionScope);
    return;
  }
  const actionBtn = e.target.closest('.stream-action-item');
  if (actionBtn) applyStreamAction(actionBtn.dataset.action || '');
}


// per-second DOM work on it: the SSE stream keeps buffering on every tab,
// but its rows are only painted while the stream view (the sidebar's Logs tab, id "stream") is actually showing.
let activeDashTab = 'dashboard';

// pushStreamQuery records one arriving query and, if it belongs in the current view, puts it
// on top without redrawing the rest.
//
// Perf: the buffer write is cheap and always runs; the DOM work is
// what made the dashboard crawl on a 1-core VPS — one <tr> build + insert per
// DNS query at wire rate, from every tab, in every foreground state. Two
// gates now stand before it: the Logs tab must be the active tab (buffering
// continues elsewhere, and the view repaints from the buffer on switch), and
// the document must be visible. A rAF coalescer collapses bursts that arrive
// inside one frame into a single batch of insertStreamQuery calls.
let streamPendingRender = false;
// streamPendingQueries collects the queries that arrived inside one frame. The old
// coalescer threw them away and rebuilt the whole table once; this one keeps them so
// each can be inserted as a single row (see insertStreamQuery).
let streamPendingQueries = [];
function pushStreamQuery(q) {
  if (!q) return;

  streamBuffer.unshift(q);
  const dropped = streamBuffer.length > STREAM_BUFFER_MAX ? streamBuffer.pop() : null;

  if (document.visibilityState === 'hidden' || activeDashTab !== 'stream') return;
  if (isStreamPaused) return;

  // Recorded only now, past the gates: a hidden or paused view does no incremental
  // inserts, so its drops have no row to remove and would only pile up until the
  // next full render — which rebuilds from the buffer anyway. Recording them here
  // keeps the list bounded to the rows a live frame can actually be mirroring.
  if (dropped) streamPendingDropped.push(dropped);
  streamPendingQueries.push(q);
  if (streamPendingRender) return;
  streamPendingRender = true;
  const epoch = streamEpoch;
  requestAnimationFrame(() => {
    // A Clear since this frame was queued owns the flags and the buffer now;
    // inserting the captured batch would repaint rows the operator dismissed.
    if (epoch !== streamEpoch) return;
    streamPendingRender = false;
    const batch = streamPendingQueries;
    streamPendingQueries = [];
    if (document.visibilityState === 'hidden' || activeDashTab !== 'stream' || isStreamPaused) return;
    const tbody = document.getElementById('stream-tbody');
    if (!tbody) return;
    // The placeholder (listening / no-match row) is replaced wholesale by a
    // full render; a table still holding one must not have rows inserted
    // above it, which is why the class is looked up before the repaint.
    if (tbody.querySelector('.stream-placeholder') || tbody.children.length === 0) {
      renderQueryStream();
      return;
    }
    for (const pending of batch) insertStreamQuery(pending);
    // Evicted arrivals lose their row here, once the inserts have placed the new
    // ones, so the table never outgrows the buffer it is mirroring.
    for (const gone of streamPendingDropped) {
      const at = streamRendered.indexOf(gone);
      if (at === -1) continue;
      streamRendered.splice(at, 1);
      if (tbody.children[at]) tbody.removeChild(tbody.children[at]);
    }
    streamPendingDropped = [];
  });
}

// seedStreamQueries replaces the buffer with a history batch. The server sends newest first,
// which is the order the buffer keeps.
function seedStreamQueries(list) {
  streamBuffer = list.slice(0, STREAM_BUFFER_MAX);
  renderQueryStream();
}

// resetStreamView drops everything the live log is holding locally and repaints the
// placeholder. Every piece of pending state is cleared, not just the buffer: a batch
// coalesced for the next animation frame carries query objects that are no longer in
// the buffer, and inserting them after a Clear is exactly how "Clear" used to leave a
// handful of rows behind.
function resetStreamView() {
  streamEpoch++;
  streamBuffer = [];
  streamRendered = [];
  streamPendingQueries = [];
  streamPendingDropped = [];
  streamPendingRender = false;
  streamFallbackSig = '';
  renderQueryStream();
}

// clearQueryStream clears the server's ring first, then the view. The POST matters:
// a client-only clear was undone by the next reconnect, when the server replayed the
// history it still held. A failed request still clears the view (the operator asked
// for it) but says so, because the entries will come back on the next connect.
async function clearQueryStream() {
  resetStreamView();
  try {
    const res = await fetch(api('/api/stream/clear'), {
      method: 'POST',
      headers: { 'Authorization': `Bearer ${authToken}` }
    });
    if (!res.ok) {
      showToast(await errorMessage(res, 'Could not clear the server log'), 'error');
    }
  } catch (e) {
    showToast('Could not clear the server log', 'error');
  }
}

// =======================================================
// CHART.JS TELEMETRY GRAPH
// =======================================================
// QPS_CHART_POINTS is both the number of samples the chart holds and, because
// pushChartData is called exactly once per poll, the width of its window in seconds.
// It is tied to STATS_POLL_MS: change one and the other stops meaning what it says.
const QPS_CHART_POINTS = 60;
// Chart.js repaints the whole canvas for each update. The data still advances at
// the poll's 1 Hz, but the paint is capped at roughly 1.5 s: two points per
// redraw is visually indistinguishable on a 60-second line and halves the
// canvas work on the low-power VPS this panel usually runs on.
const QPS_CHART_REDRAW_MS = 1500;
let qpsChartLastDraw = 0;

// Chart.js paints onto a canvas, and a canvas is the one surface in this panel that CSS
// cannot re-colour: every stroke is a literal string baked in at construction time. So the
// four colours here used to be the four places light mode had no effect at all. Three were
// merely wrong — neon #00f0ff on a white card is a bright smear rather than a line — and one
// was a genuine defect: grid lines of rgba(255,255,255,0.05) are white on white, so the y
// axis in light mode had no gridlines whatsoever and the graph read as a shape with no scale.
//
// The values come from the same custom properties the rest of the panel is built on, read off
// <html> after [data-theme] has changed, so there is one definition of "cyan" per theme rather
// than a canvas-shaped copy of it. The channel tokens hold space-separated RGB — `0 240 255` —
// which is what lets one read serve both an opaque rgb() and the two translucent stops.
function cssVar(name, fallback) {
  try {
    const v = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
    return v || fallback;
  } catch (e) {
    return fallback;
  }
}

// chartPalette resolves the four colours for the theme in force right now. The gradient needs
// the 2d context because a CanvasGradient belongs to the context that made it — it cannot be
// built once and reused after a theme flip, which is why this returns a factory rather than a
// value and why applyChartTheme has to call it again rather than mutating a stored stop.
function chartPalette(ctx) {
  const cyan = cssVar('--c-cyan-400', '0 240 255');
  const gradient = ctx.createLinearGradient(0, 0, 0, 220);
  gradient.addColorStop(0, 'rgba(' + cyan.replace(/\s+/g, ',') + ', 0.45)');
  gradient.addColorStop(1, 'rgba(' + cyan.replace(/\s+/g, ',') + ', 0)');
  return {
    line: 'rgb(' + cyan + ')',
    fill: gradient,
    // --border-color is already a full rgba() and already flips: a translucent sky in dark,
    // a translucent slate in light. Reusing it means the gridlines match the hairline on
    // every card around the chart instead of approximating it.
    grid: cssVar('--border-color', 'rgba(255,255,255,0.05)'),
    tick: 'rgb(' + cssVar('--c-slate-400', '100 116 139') + ')'
  };
}

// Re-colour in place on a theme change. update('none') skips the animation, which matters
// here: the whole chart's colours change at once, and animating that reads as a flash.
//
// The context is looked up from the DOM rather than off qpsChart.ctx — Chart.js does expose
// it, but the canvas is the thing that is actually needed and initChart already proves that
// lookup works, so there is no version-specific property standing between a theme flip and a
// legible graph.
function applyChartTheme() {
  if (!qpsChart) return;
  try {
    const canvas = document.getElementById('qpsChart');
    if (!canvas) return;
    const pal = chartPalette(canvas.getContext('2d'));
    const ds = qpsChart.data.datasets[0];
    ds.borderColor = pal.line;
    ds.backgroundColor = pal.fill;
    qpsChart.options.scales.y.grid.color = pal.grid;
    qpsChart.options.scales.y.ticks.color = pal.tick;
    qpsChart.update('none');
  } catch (e) { /* a chart that fails to re-colour is still a readable chart */ }
}

document.addEventListener('digitdns:theme', applyChartTheme);
document.addEventListener('digitdns:theme', applyHistoryChartTheme);

// Chart.js is 201 KB and the dashboard's first screen is useful before it
// paints. Load it on demand, once, and let both charts share the same promise.
let chartLibPromise = null;
function ensureChartLib() {
  if (typeof Chart !== 'undefined') return Promise.resolve(true);
  if (!chartLibPromise) {
    chartLibPromise = new Promise((resolve) => {
      const script = document.createElement('script');
      script.src = ADMIN_BASE + '/js/chart.umd.min.js';
      script.async = true;
      script.onload = () => resolve(typeof Chart !== 'undefined');
      script.onerror = () => resolve(false);
      document.head.appendChild(script);
    });
  }
  return chartLibPromise;
}

async function initChart() {
  try {
    const canvas = document.getElementById('qpsChart');
    if (!canvas) return;
    if (!(await ensureChartLib())) {
      console.warn('Chart.js could not be loaded');
      return;
    }
    const ctx = canvas.getContext('2d');

    const pal = chartPalette(ctx);

    // One point per poll, and the poll is now 1 Hz — so the point count is the chart's
    // window in seconds. 25 points was a fifty-second window while polling every two
    // seconds; keeping 25 would have silently halved it to twenty-five. Sixty is a
    // minute, which is a span worth reading and is what the panel's subtitle claims.
    const labels = Array.from({ length: QPS_CHART_POINTS }, () => '');
    const data = Array.from({ length: QPS_CHART_POINTS }, () => 0);

    qpsChart = new Chart(ctx, {
      type: 'line',
      data: {
        labels: labels,
        datasets: [{
          label: 'QPS',
          data: data,
          borderColor: pal.line,
          borderWidth: 2,
          backgroundColor: pal.fill,
          fill: true,
          tension: 0.4,
          pointRadius: 0
        }]
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        // update('none') already skips the tween, but the option is what
        // keeps Chart.js from arming its animator at all — the cheaper of
        // the two on the 1-core VPS this dashboard runs on.
        animation: false,
        plugins: { legend: { display: false } },
        scales: {
          x: { display: false },
          y: {
            beginAtZero: true,
            grid: { color: pal.grid },
            ticks: {
              color: pal.tick,
              font: { size: 10, family: 'JetBrains Mono' },
              // Chart.js defaults to toLocaleString(), which renders decimal
              // commas on most non-English systems while every other number in
              // the panel uses a dot. Format explicitly instead.
              callback: (value) => (Number.isInteger(value) ? String(value) : Number(value).toFixed(1)),
            }
          }
        }
      }
    });
    qpsChartLastDraw = 0;
  } catch (e) {
    console.warn('Chart init error:', e);
  }
}

function pushChartData(val) {
  // The chart lives on the Dash view; painting it while another tab is
  // showing spends a canvas render per second on a picture nobody sees.
  // The poll keeps running (the tiles read the same response), so the
  // chart simply resumes on return.
  if (activeDashTab !== 'dashboard') return;
  if (!qpsChart) {
    initChart();
    if (!qpsChart) return;
  }
  // Chart.js draws no point for null and keeps the line's scale honest; pushing an
  // undefined or a NaN instead makes the axis collapse and the last few seconds of
  // real load disappear with it.
  const point = (typeof val === 'number' && isFinite(val) && val >= 0) ? val : null;
  try {
    const d = qpsChart.data.datasets[0].data;
    d.shift();
    d.push(point);
    const now = performance.now();
    if (qpsChartLastDraw === 0 || now - qpsChartLastDraw >= QPS_CHART_REDRAW_MS) {
      qpsChart.update('none');
      qpsChartLastDraw = now;
    }
  } catch (e) {}
}

// =======================================================
// HISTORICAL STATS (90-day aggregates)
// =======================================================
const HISTORY_RANGES = ['1h', '24h', '7d', '30d', '90d'];
const HISTORY_TOP_DIMENSIONS = [
  { id: 'history-top-domains', dimension: 'domain', empty: 'No domains in this range.' },
  { id: 'history-top-blocked', dimension: 'blocked_domain', empty: 'No blocked queries in this range.' },
  { id: 'history-top-clients', dimension: 'client', empty: 'No client history in this range.' },
  { id: 'history-top-upstreams', dimension: 'upstream', empty: 'No upstream history in this range.' },
  { id: 'history-top-protocols', dimension: 'protocol', empty: 'No protocol history in this range.' },
  { id: 'history-top-relay', dimension: 'relay_country', empty: 'No relayed queries in this range.' },
];

function historyColor(name, fallback) {
  return 'rgb(' + cssVar(name, fallback) + ')';
}

function historyFill(name, fallback, alpha) {
  return 'rgba(' + cssVar(name, fallback).replace(/\s+/g, ',') + ',' + alpha + ')';
}

function historyPalette() {
  return {
    allowed: historyColor('--c-cyan-400', '0 240 255'),
    allowedFill: historyFill('--c-cyan-400', '0 240 255', 0.18),
    blocked: historyColor('--c-red-400', '248 113 113'),
    blockedFill: historyFill('--c-red-400', '248 113 113', 0.18),
    proxied: historyColor('--c-purple-400', '192 132 252'),
    proxiedFill: historyFill('--c-purple-400', '192 132 252', 0.18),
    grid: cssVar('--border-color', 'rgba(255,255,255,0.05)'),
    tick: 'rgb(' + cssVar('--c-slate-400', '100 116 139') + ')',
  };
}

function applyHistoryChartTheme() {
  if (!historyChart) return;
  const p = historyPalette();
  try {
    const ds = historyChart.data.datasets;
    ds[0].borderColor = p.allowed;
    ds[0].backgroundColor = p.allowedFill;
    ds[1].borderColor = p.blocked;
    ds[1].backgroundColor = p.blockedFill;
    ds[2].borderColor = p.proxied;
    ds[2].backgroundColor = p.proxiedFill;
    historyChart.options.scales.y.grid.color = p.grid;
    historyChart.options.scales.y.ticks.color = p.tick;
    historyChart.options.scales.x.ticks.color = p.tick;
    if (historyChart.options.plugins.legend.labels) {
      historyChart.options.plugins.legend.labels.color = p.tick;
    }
    historyChart.update('none');
  } catch (e) { /* a chart that fails to re-colour is still readable */ }
}


async function ensureHistoryChart() {
  if (historyChart) return historyChart;
  const canvas = document.getElementById('historyChart');
  if (!canvas) return null;
  if (!(await ensureChartLib())) return null;
  const p = historyPalette();
  try {
    historyChart = new Chart(canvas.getContext('2d'), {
      type: 'line',
      data: {
        labels: [],
        datasets: [
          {
            label: 'Allowed',
            data: [],
            borderColor: p.allowed,
            backgroundColor: p.allowedFill,
            borderWidth: 2,
            fill: true,
            stack: 'traffic',
            tension: 0.3,
            pointRadius: 0,
          },
          {
            label: 'Blocked',
            data: [],
            borderColor: p.blocked,
            backgroundColor: p.blockedFill,
            borderWidth: 2,
            fill: true,
            stack: 'traffic',
            tension: 0.3,
            pointRadius: 0,
          },
          {
            label: 'Proxied',
            data: [],
            borderColor: p.proxied,
            backgroundColor: p.proxiedFill,
            borderWidth: 2,
            fill: true,
            stack: 'traffic',
            tension: 0.3,
            pointRadius: 0,
          },
        ],
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        animation: false,
        interaction: { mode: 'index', intersect: false },
        plugins: {
          legend: {
            display: true,
            labels: {
              color: p.tick,
              boxWidth: 8,
              boxHeight: 8,
              usePointStyle: true,
              font: { size: 10, family: 'JetBrains Mono' },
            },
          },
        },
        scales: {
          x: {
            grid: { display: false },
            ticks: {
              color: p.tick,
              maxTicksLimit: 8,
              maxRotation: 0,
              font: { size: 10, family: 'JetBrains Mono' },
            },
          },
          y: {
            stacked: true,
            beginAtZero: true,
            grid: { color: p.grid },
            ticks: {
              color: p.tick,
              font: { size: 10, family: 'JetBrains Mono' },
              callback: (value) => (Number.isInteger(value) ? String(value) : Number(value).toFixed(1)),
            },
          },
        },
      },
    });
  } catch (e) {
    console.warn('History chart init error:', e);
    historyChart = null;
  }
  return historyChart;
}

function historyLabel(start, range, interval) {
  const d = new Date(start);
  if (range === '7d') {
    return d.toLocaleDateString(undefined, { weekday: 'short', day: 'numeric' });
  }
  if (range === '30d' || range === '90d' || interval === 'day') {
    return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
  }
  return d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
}

async function renderHistoryChart(series) {
  const chart = await ensureHistoryChart();
  if (!chart) return;
  const range = series.range || historyRange;
  const points = Array.isArray(series.points) ? series.points : [];
  // A single bucket has no line segment to draw, so the same three series are
  // rendered as one stacked bar; Chart.js supports a per-dataset type switch.
  const single = points.length === 1;
  let labels = points.map((p) => historyLabel(p.start, range, series.interval));
  let allowed = points.map((p) => Math.max(0, (p.queries || 0) - (p.blocked || 0) - (p.proxied || 0)));
  let blocked = points.map((p) => p.blocked || 0);
  let proxied = points.map((p) => p.proxied || 0);

  // The 1h range always spans the full hour. The API returns only the started
  // five-minute slots, so the rest of the axis is filled here and the datasets
  // stay null after "now" rather than dropping a misleading line to zero.
  if (range === '1h' && points.length) {
    const slots = 12;
    const startMs = new Date(points[0].start).getTime();
    if (Number.isFinite(startMs)) {
      labels = Array.from({ length: slots }, (_, i) =>
        historyLabel(new Date(startMs + i * 5 * 60 * 1000).toISOString(), range, '5m'));
      const pad = (values) => values.concat(Array(Math.max(0, slots - values.length)).fill(null));
      allowed = pad(allowed);
      blocked = pad(blocked);
      proxied = pad(proxied);
    }
  }

  chart.data.labels = labels;
  chart.data.datasets[0].data = allowed;
  chart.data.datasets[1].data = blocked;
  chart.data.datasets[2].data = proxied;
  chart.data.datasets.forEach((dataset) => {
    dataset.type = single ? 'bar' : 'line';
    dataset.fill = !single;
    dataset.tension = single ? 0 : 0.3;
    dataset.pointRadius = single ? 4 : 0;
    dataset.borderWidth = single ? 1 : 2;
    dataset.borderSkipped = false;
    dataset.barPercentage = 0.6;
    dataset.categoryPercentage = 0.5;
    dataset.borderRadius = 3;
  });
  // Only offset the axis when there really is one category; a padded 1h axis
  // must keep its first slot at the left edge.
  chart.options.scales.x.offset = single && labels.length === 1;
  chart.options.scales.x.ticks.maxTicksLimit = range === '7d' ? 7 : 6;
  try {
    chart.update('none');
    chart.resize();
  } catch (e) {}
}

function historyNumber(value) {
  return (typeof value === 'number' && isFinite(value)) ? value.toLocaleString() : '—';
}

function historyPercent(value) {
  return (typeof value === 'number' && isFinite(value)) ? `${value.toFixed(1)}%` : '—';
}

function renderHistorySummary(summary) {
  const set = (id, text) => {
    const el = document.getElementById(id);
    if (el) el.textContent = text;
  };
  set('history-total', historyNumber(summary.queries));
  set('history-total-sub', `${historyNumber(summary.buckets)} buckets`);
  set('history-blocked', historyPercent(summary.blocked_pct));
  set('history-blocked-sub', `${historyNumber(summary.blocked)} queries`);
  set('history-proxied', historyPercent(summary.proxied_pct));
  set('history-proxied-sub', `${historyNumber(summary.proxied)} queries`);
  set('history-cache', historyPercent(summary.cache_hit_pct));
  set('history-cache-sub', `${historyNumber(summary.cached)} hits`);
  set('history-clients', historyNumber(summary.active_clients));
  const clientsSub = document.getElementById('history-clients-sub');
  if (clientsSub) {
    clientsSub.textContent = summary.active_clients_approx ? 'top-N approx.' : 'HLL distinct';
  }
  set('history-latency', typeof summary.latency_avg_ms === 'number'
    ? summary.latency_avg_ms.toFixed(1) : '—');
}

function renderHistoryTop(id, data, empty, dimension) {
  const el = document.getElementById(id);
  if (!el) return;
  const previousList = el.querySelector('.history-top-list');
  const scrollTop = previousList ? previousList.scrollTop : 0;
  let items = Array.isArray(data.items) ? data.items : [];
  if (id === 'history-top-upstreams' && historyTopUpstreamByProvider) {
    items = groupUpstreamsByProvider(items);
  }
  if (!items.length) {
    el.innerHTML = `<div class="history-top-list"><div class="py-2 text-slate-500">${escapeHTML(empty)}</div></div>`;
    return;
  }
  const max = Math.max(...items.map((item) => item.count || 0), 1);
  const rows = items.map((item) => {
    const value = String(item.value || 'unknown');
    const count = historyNumber(item.count);
    const width = Math.max(1, Math.round(((item.count || 0) / max) * 100));
    const relay = dimension === 'relay_country';
    const localRelay = relay && value.toUpperCase() === 'LOCAL';
    const flagCode = localRelay ? (currentConfig?.server?.country || '') : value;
    const flag = relay ? countryFlagHTML(flagCode, 'agent-flag relay-flag') : '';
    const label = relay ? relayCountryLabel(value) : dimension === 'protocol' ? protocolLabel(value) : value;
    const valueClass = dimension === 'protocol' ? protocolClass(value) : 'text-slate-300';
    const providerLogo = dimension === 'upstream'
      ? providerFaviconMarkup(item.provider || value, 'history-top-provider-logo')
      : '';
    const valueMarkup = `${providerLogo}${flag}<span class="truncate font-mono ${escapeHTML(valueClass)}" title="${escapeHTML(label)}">${escapeHTML(label)}</span>`;
    // Relay rows carry the flag and name, the number of queries proxied
    // through that relay, and a share bar. The count is labelled for assistive
    // tech because the card title alone does not make the number self-evident.
    const countTitle = relay ? 'Queries proxied through this relay' : '';
    const countMarkup = `<span class="shrink-0 font-mono text-[10px] text-slate-400" title="${escapeHTML(countTitle)}">${count}</span>`;
    return `<div class="history-top-row">
      <div class="flex items-center justify-between gap-2">
        <span class="flex items-center gap-1.5 min-w-0">${valueMarkup}</span>
        ${countMarkup}
      </div>
      <div class="mt-0.5 h-1 rounded-full bg-slate-800 overflow-hidden"><div class="h-full bg-cyan-500/20" style="width:${width}%"></div></div>
    </div>`;
  }).join('');
  const other = data.other_count
    ? `<div class="history-top-other">Other: ${historyNumber(data.other_count)}</div>`
    : '';
  el.innerHTML = `<div class="history-top-list">${rows}</div>${other}`;
  // A 30 s refresh must not yank an operator back to the top of a list they
  // were reading; only the entry viewport scrolls.
  const nextList = el.querySelector('.history-top-list');
  if (nextList) nextList.scrollTop = scrollTop;
}

const PROVIDER_NAMES = {
  cloudflare: 'Cloudflare', google: 'Google', quad9: 'Quad9', adguard: 'AdGuard',
  controld: 'Control D', dnssb: 'DNS.SB', opendns: 'OpenDNS', yandex: 'Yandex',
  nextdns: 'NextDNS', cleanbrowsing: 'CleanBrowsing',
};

// groupUpstreamsByProvider turns per-server history rows into provider totals.
// Unknown/custom resolvers are one "Other" bucket rather than a dozen rows that
// all say nothing about which provider answered.
function groupUpstreamsByProvider(items) {
  const totals = new Map();
  for (const item of items) {
    const address = String(item.value || 'unknown');
    const id = providerIdForUpstream(address);
    const key = id || 'other';
    const entry = totals.get(key) || {
      value: id ? (PROVIDER_NAMES[id] || id) : 'Other',
      count: 0,
      provider: id || '',
    };
    entry.count += Number(item.count || 0);
    totals.set(key, entry);
  }
  return [...totals.values()].sort((a, b) => b.count - a.count);
}

function setHistoryTopUpstreamGrouping(byProvider) {
  historyTopUpstreamByProvider = !!byProvider;
  const btn = document.getElementById('history-top-upstreams-toggle');
  if (btn) btn.setAttribute('aria-pressed', historyTopUpstreamByProvider ? 'true' : 'false');
  const top = historyTopData.upstream || { items: [] };
  renderHistoryTop('history-top-upstreams', top, 'No upstream history in this range.', 'upstream');
}

function relayCountryLabel(value) {
  const code = String(value || '').toUpperCase();
  if (code === 'LOCAL') return 'Local';
  return countryName(code) || code || 'unknown';
}

function setHistoryError(message) {
  const el = document.getElementById('history-error');
  if (!el) return;
  el.textContent = message || '';
  el.classList.toggle('hidden', !message);
}

function setHistoryStatus(message) {
  const el = document.getElementById('history-status');
  if (el) el.textContent = message;
}

function setHistoryRange(range) {
  if (!HISTORY_RANGES.includes(range)) return;
  historyRange = range;
  document.querySelectorAll('[data-history-range]').forEach((btn) => {
    const active = btn.dataset.historyRange === range;
    btn.setAttribute('aria-pressed', active ? 'true' : 'false');
    btn.className = active
      ? 'px-2.5 py-1 rounded-lg text-[11px] font-semibold border border-cyan-500/40 bg-cyan-500/10 text-cyan-300 transition'
      : 'px-2.5 py-1 rounded-lg text-[11px] font-semibold border border-slate-700 bg-slate-900 text-slate-300 hover:bg-slate-800 transition';
  });
  const badge = document.getElementById('history-chart-range');
  if (badge) badge.textContent = range;
  loadHistory(true);
}

function scheduleHistoryRefresh() {
  if (historyTimer) {
    clearTimeout(historyTimer);
    historyTimer = null;
  }
  if (!authToken || activeDashTab !== 'dashboard' || document.visibilityState === 'hidden') return;
  historyTimer = setTimeout(() => {
    historyTimer = null;
    loadHistory(true);
  }, 60000);
}

async function historyFetch(path) {
  const res = await fetch(api(path), {
    headers: { 'Authorization': `Bearer ${authToken}` },
  });
  if (res.status === 401) {
    // The gate is the whole answer. Marking this as a 401 lets loadHistory keep
    // the panel quiet instead of painting "Session expired" behind the login
    // overlay, where it stayed until the next reload.
    const err = new Error('Session expired');
    err.status = 401;
    showLoginModal();
    throw err;
  }
  if (!res.ok) {
    const err = new Error(await errorMessage(res, 'Could not load traffic history'));
    err.status = res.status;
    throw err;
  }
  return res.json();
}

async function loadHistory(force = false) {
  if (!authToken || activeDashTab !== 'dashboard' || document.visibilityState === 'hidden') return;
  if (historyInFlight && !force) return;
  const seq = ++historySeq;
  historyInFlight = true;
  const range = historyRange;
  const interval = range === '1h' ? '5m' : (range === '24h' || range === '7d') ? 'hour' : 'day';
  const query = `range=${encodeURIComponent(range)}`;
  setHistoryStatus('Loading history…');
  try {
    // One document instead of eight requests. The six ranked tables, the series
    // and the summary all come from the same aggregate store, so splitting them
    // only added round trips — seven extra on every refresh, on every client.
    const data = await historyFetch(
      `/api/v3/stats/history?${query}&interval=${encodeURIComponent(interval)}&limit=10`);
    if (seq !== historySeq) return;
    renderHistorySummary(data.summary);
    await renderHistoryChart(data.series);
    historyTopData = data.tops || {};
    HISTORY_TOP_DIMENSIONS.forEach((entry) => {
      const top = (data.tops && data.tops[entry.dimension]) || { items: [] };
      renderHistoryTop(entry.id, top, entry.empty, entry.dimension);
    });
    setHistoryError('');
    setHistoryStatus(`Updated ${new Date().toLocaleTimeString()} · ${range} aggregate`);
    scheduleHistoryRefresh();
  } catch (e) {
    if (seq !== historySeq) return;
    if (e.status === 401) {
      // Signed out: showLoginModal() has already raised the gate. Writing the
      // auth failure into the card put a message behind the overlay that was
      // revealed briefly when the next login hid it before reloading.
      setHistoryError('');
      setHistoryStatus('');
      return;
    }
    setHistoryError(e.status === 503
      ? 'Historical stats are unavailable on this instance.'
      : (e.message || 'Could not load traffic history.'));
    setHistoryStatus('History refresh failed');
    scheduleHistoryRefresh();
  } finally {
    if (seq === historySeq) historyInFlight = false;
  }
}

async function exportHistory(format) {
  if (!authToken) return;
  const selected = format === 'json' ? 'json' : 'csv';
  try {
    const res = await fetch(api(`/api/v3/stats/export?range=${encodeURIComponent(historyRange)}&format=${selected}`), {
      headers: { 'Authorization': `Bearer ${authToken}` },
    });
    if (!res.ok) throw new Error(await errorMessage(res, 'Could not export history'));
    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = `digitdns-stats-${historyRange}.${selected}`;
    document.body.appendChild(link);
    link.click();
    link.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
    showToast(`History exported as ${selected.toUpperCase()}.`, 'success');
  } catch (e) {
    showToast(e.message || 'Could not export history.', 'error');
  }
}

function initHistory() {
  if (historyInitialized) return;
  historyInitialized = true;
  document.querySelectorAll('[data-history-range]').forEach((btn) => {
    btn.addEventListener('click', () => setHistoryRange(btn.dataset.historyRange));
  });
  document.getElementById('history-export-csv')?.addEventListener('click', () => exportHistory('csv'));
  document.getElementById('history-export-json')?.addEventListener('click', () => exportHistory('json'));
  document.getElementById('history-refresh-btn')?.addEventListener('click', () => loadHistory(true));
  document.getElementById('history-top-upstreams-toggle')?.addEventListener('click', () => {
    setHistoryTopUpstreamGrouping(!historyTopUpstreamByProvider);
  });
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'hidden') {
      if (historyTimer) {
        clearTimeout(historyTimer);
        historyTimer = null;
      }
      return;
    }
    if (activeDashTab === 'dashboard') loadHistory();
  });
  loadHistory();
}

let statsSettingsInitialized = false;

function setStatsSettingsStatus(message) {
  const el = document.getElementById('stats-settings-status');
  if (el) el.textContent = message || '';
}

async function loadStatsSettings() {
  if (!authToken) return;
  try {
    const res = await fetch(api('/api/stats/settings'), {
      headers: { 'Authorization': `Bearer ${authToken}` },
    });
    if (!res.ok) throw new Error(await errorMessage(res, 'Could not load history settings'));
    const settings = await res.json();
    const set = (id, value) => {
      const el = document.getElementById(id);
      if (el) el.value = value;
    };
    const check = (id, value) => {
      const el = document.getElementById(id);
      if (el) el.checked = !!value;
    };
    check('stats-enabled-input', settings.enabled);
    check('stats-anonymize-input', settings.anonymize_clients);
    set('stats-retention-input', settings.retention_days);
    set('stats-top-domains-input', settings.top_domains);
    set('stats-top-clients-input', settings.top_clients);
    const excluded = Array.isArray(settings.exclude_clients) ? settings.exclude_clients.join('\n') : '';
    set('stats-exclude-clients-input', excluded);
    setStatsSettingsStatus('Restart required after changing these values.');
  } catch (e) {
    setStatsSettingsStatus(e.message || 'Could not load history settings.');
  }
}

async function saveStatsSettings() {
  const enabled = document.getElementById('stats-enabled-input')?.checked;
  const anonymize = document.getElementById('stats-anonymize-input')?.checked;
  const retention = Number(document.getElementById('stats-retention-input')?.value);
  const topDomains = Number(document.getElementById('stats-top-domains-input')?.value);
  const topClients = Number(document.getElementById('stats-top-clients-input')?.value);
  const excludeClients = (document.getElementById('stats-exclude-clients-input')?.value || '')
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean);
  if (!Number.isInteger(retention) || retention < 7 || retention > 365) {
    showToast('Retention must be between 7 and 365 days.', 'error');
    return;
  }
  if (!Number.isInteger(topDomains) || topDomains < 100 || topDomains > 10000) {
    showToast('Top domains must be between 100 and 10,000.', 'error');
    return;
  }
  if (!Number.isInteger(topClients) || topClients < 50 || topClients > 5000) {
    showToast('Top clients must be between 50 and 5,000.', 'error');
    return;
  }
  if (excludeClients.length > 500) {
    showToast('Excluded clients cannot contain more than 500 entries.', 'error');
    return;
  }
  const btn = document.getElementById('save-stats-settings-btn');
  if (btn) btn.disabled = true;
  setStatsSettingsStatus('Saving…');
  try {
    const res = await fetch(api('/api/stats/settings'), {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        enabled,
        retention_days: retention,
        top_domains: topDomains,
        top_clients: topClients,
        anonymize_clients: anonymize,
        exclude_clients: excludeClients,
      }),
    });
    if (!res.ok) throw new Error(await errorMessage(res, 'Could not save history settings'));
    setStatsSettingsStatus('Saved. Restart the daemon to apply.');
    showToast('History settings saved. Restart required.', 'success');
  } catch (e) {
    setStatsSettingsStatus(e.message || 'Could not save history settings.');
    showToast(e.message || 'Could not save history settings.', 'error');
  } finally {
    if (btn) btn.disabled = false;
  }
}

function initStatsSettings() {
  if (statsSettingsInitialized) return;
  statsSettingsInitialized = true;
  document.getElementById('save-stats-settings-btn')?.addEventListener('click', saveStatsSettings);
  loadStatsSettings();
}

// ── Ad blocking ───────────────────────────────────────────────────────────────
// The engine lives in the daemon. This UI only edits policy and list sources;
// downloads and compilation happen in background goroutines, never in a request
// or on the DNS path.
let filteringInitialized = false;
let filteringPollTimer = null;
let filteringPresetData = [];
let filteringPresetCounts = {};
let filteringInstalledLists = [];
let filteringUpdating = new Set();
let filteringAdding = new Set();
let filteringCounting = false;
let filteringCountTimer = null;
let filteringSettings = null;

function setFilteringStatus(message) {
  const el = document.getElementById('filtering-settings-status');
  if (el) el.textContent = message || '';
}

function setFilteringError(message) {
  const el = document.getElementById('filtering-error');
  if (!el) return;
  el.textContent = message || '';
  el.classList.toggle('hidden', !message);
}

function filterDate(iso) {
  if (!iso || String(iso).startsWith('0001')) return 'never';
  const d = new Date(iso);
  return isNaN(d.getTime()) ? 'never' : d.toLocaleString();
}

async function loadFiltering() {
  if (!authToken || !document.getElementById('filtering-card')) return;
  try {
    const res = await fetch(api('/api/filtering'), {
      headers: { 'Authorization': `Bearer ${authToken}` },
    });
    if (!res.ok) throw new Error(await errorMessage(res, 'Could not load ad blocking status'));
    renderFiltering(await res.json());
    setFilteringError('');
  } catch (e) {
    setFilteringError(e.message || 'Could not load ad blocking status.');
  }
}

function renderFiltering(status) {
  const settings = status.settings || {};
  filteringSettings = settings;
  const set = (id, value) => {
    const el = document.getElementById(id);
    if (el) el.value = value;
  };
  const check = (id, value) => {
    const el = document.getElementById(id);
    if (el) el.checked = !!value;
  };
  check('filtering-enabled-input', settings.enabled);
  set('filtering-mode-select', settings.block_mode || 'null_ip');
  set('filtering-ttl-input', settings.block_ttl || 10);
  set('filtering-interval-input', settings.update_interval_hours || 24);
  set('filtering-allowlist-input', Array.isArray(settings.allowlist) ? settings.allowlist.join('\n') : '');

  const lists = Array.isArray(status.lists) ? status.lists : [];
  filteringInstalledLists = lists;
  filteringPresetData = Array.isArray(status.presets) ? status.presets : filteringPresetData;
  filteringPresetCounts = {};
  for (const count of (Array.isArray(status.preset_counts) ? status.preset_counts : [])) {
    if (count && count.preset_id) filteringPresetCounts[count.preset_id] = count;
  }
  filteringCounting = !!status.preset_counts_running;
  filteringUpdating = new Set(Array.isArray(status.updating) ? status.updating : []);

  const enabledCount = lists.filter((l) => l.enabled).length;
  const totalDomains = typeof status.total_domains === 'number'
    ? status.total_domains
    : lists.reduce((sum, l) => sum + (l.domain_count || l.rule_count || 0), 0);
  const summary = document.getElementById('filtering-summary');
  if (summary) {
    summary.textContent = `${historyNumber(totalDomains)} unique domains · ${enabledCount}/${lists.length} active`;
  }
  renderFilteringPresets();
  if (filteringCounting) scheduleFilteringCountPoll();
}

const FILTERING_PRESET_GROUPS = [
  ['base', 'General protection'],
  ['ads', 'Supplementary ads & trackers'],
  ['security', 'Malware, phishing & scams'],
  ['telemetry', 'Device & vendor telemetry'],
  ['extras', 'Optional extras'],
];

function renderFilteringPresets() {
  const container = document.getElementById('filtering-presets');
  if (!container) return;

  const installedByURL = new Map();
  const installedByPreset = new Map();
  for (const list of filteringInstalledLists) {
    if (list.url) installedByURL.set(list.url, list);
    if (list.preset_id) installedByPreset.set(list.preset_id, list);
  }

  // Resolve every preset to its installed record in one pass, so the custom
  // group only contains rows that are not already represented above.
  const installedForPreset = new Map();
  const represented = new Set();
  for (const preset of filteringPresetData) {
    const list = installedByPreset.get(preset.id) || installedByURL.get(preset.url);
    if (list) {
      installedForPreset.set(preset.id, list);
      if (list.id) represented.add(list.id);
    }
  }

  const query = (document.getElementById('filtering-preset-search')?.value || '').trim().toLowerCase();
  const matches = (values) => !query || values.filter(Boolean).join(' ').toLowerCase().includes(query);

  const allCustom = filteringInstalledLists.filter((list) => !represented.has(list.id));

  // The lists actually in force come first: an operator opening this tab needs
  // to see what is blocking without scrolling past the whole catalogue. Enabled
  // rows are then skipped in their own groups so nothing appears twice.
  const activePresetRows = filteringPresetData
    .map((preset) => ({ preset, list: installedForPreset.get(preset.id) }))
    .filter(({ preset, list }) => list?.enabled &&
      matches([preset.name, preset.description, preset.license, preset.warning, list.name, list.url]))
    .map(({ preset, list }) => presetRow(preset, list));
  const activeCustomRows = allCustom
    .filter((list) => list.enabled && matches([list.name, list.url, list.license]))
    .map(customListRow);
  const activeGroup = (activePresetRows.length || activeCustomRows.length)
    ? `<div class="filtering-preset-group filtering-preset-active"><h5>Active</h5>${activePresetRows.join('')}${activeCustomRows.join('')}</div>`
    : '';

  const groups = FILTERING_PRESET_GROUPS.map(([key, label]) => {
    const items = filteringPresetData
      .filter((p) => p.group === key)
      .filter((p) => {
        const list = installedForPreset.get(p.id);
        if (list?.enabled) return false;
        return matches([p.name, p.description, p.license, p.warning, list?.name, list?.url]);
      });
    if (!items.length) return '';
    return `<div class="filtering-preset-group"><h5>${escapeHTML(label)}</h5>${items.map((p) => presetRow(p, installedForPreset.get(p.id))).join('')}</div>`;
  }).join('');

  const customLists = allCustom.filter((list) => !list.enabled && matches([list.name, list.url, list.license]));
  const customGroup = customLists.length
    ? `<div class="filtering-preset-group"><h5>Custom lists</h5>${customLists.map(customListRow).join('')}</div>`
    : '';

  container.innerHTML = activeGroup + groups + customGroup || '<div class="filter-list-empty">No blocklists match that search.</div>';

  const summary = document.getElementById('filtering-preset-summary');
  if (summary) {
    summary.textContent = `${filteringPresetData.length} presets · ${installedForPreset.size} installed · ${allCustom.length} custom`;
  }
  safeFeatherReplace();
}

function filteringListMeta(list, updating) {
  return [
    `${historyNumber(list.domain_count || list.rule_count || 0)} domains`,
    `${historyNumber(list.allowed_count || 0)} allow`,
    `${historyNumber(list.skipped || 0)} skipped`,
    list.license ? escapeHTML(list.license) : '',
    updating ? 'updating…' : `updated ${escapeHTML(filterDate(list.last_updated))}`,
  ].filter(Boolean).join(' · ');
}

function presetLinks(preset) {
  const name = preset.name || 'list';
  const home = preset.homepage
    ? `<a class="filter-link" href="${escapeHTML(preset.homepage)}" target="_blank" rel="noopener noreferrer" title="Open project homepage" aria-label="Open ${escapeHTML(name)} homepage"><i data-feather="home" class="w-3.5 h-3.5"></i></a>`
    : '';
  const file = preset.url
    ? `<a class="filter-link" href="${escapeHTML(preset.url)}" target="_blank" rel="noopener noreferrer" title="Open the whole list file" aria-label="Open ${escapeHTML(name)} file"><i data-feather="external-link" class="w-3.5 h-3.5"></i></a>`
    : '';
  return home + file;
}

function installedActions(list) {
  const updating = filteringUpdating.has(list.id);
  const name = list.name || 'list';
  const home = list.homepage
    ? `<a class="filter-link" href="${escapeHTML(list.homepage)}" target="_blank" rel="noopener noreferrer" title="Open list homepage" aria-label="Open ${escapeHTML(name)} homepage"><i data-feather="home" class="w-3.5 h-3.5"></i></a>`
    : '';
  const file = list.url
    ? `<a class="filter-link" href="${escapeHTML(list.url)}" target="_blank" rel="noopener noreferrer" title="Open the whole list file" aria-label="Open ${escapeHTML(name)} file"><i data-feather="external-link" class="w-3.5 h-3.5"></i></a>`
    : '';
  return `${home}${file}
    <button type="button" class="panel-btn panel-btn-neutral" data-filter-update ${updating ? 'disabled' : ''} title="Update now" aria-label="Update ${escapeHTML(name)}">
      <i data-feather="refresh-cw" class="w-3.5 h-3.5"></i>
    </button>
    <button type="button" class="panel-btn panel-btn-danger" data-filter-delete ${updating ? 'disabled' : ''} title="Delete list" aria-label="Delete ${escapeHTML(name)}">
      <i data-feather="trash-2" class="w-3.5 h-3.5"></i>
    </button>`;
}

function presetRow(preset, installedList) {
  const presetID = preset.id || '';
  const isInstalled = !!installedList;
  const updating = isInstalled && filteringUpdating.has(installedList.id);
  const adding = !isInstalled && filteringAdding.has(presetID);
  const active = isInstalled && !!installedList.enabled;
  const badges = [];
  if (preset.recommended) badges.push('<span class="filtering-preset-badge">RECOMMENDED</span>');
  if (isInstalled) {
    const badgeClass = active ? 'filtering-preset-badge is-active' : 'filtering-preset-badge is-off';
    badges.push(`<span class="${escapeHTML(badgeClass)}">${active ? 'ACTIVE' : 'DISABLED'}</span>`);
  } else if (adding) {
    badges.push('<span class="filtering-preset-badge">ADDING…</span>');
  }
  const warning = preset.warning ? `<div class="filtering-preset-warning">${escapeHTML(preset.warning)}</div>` : '';
  const description = preset.description ? `<div class="filtering-preset-desc">${escapeHTML(preset.description)}</div>` : '';
  const license = !isInstalled && preset.license ? `<div class="filtering-preset-desc">License: ${escapeHTML(preset.license)}</div>` : '';
  const countEntry = filteringPresetCounts[presetID];
  let domains = '';
  if (isInstalled) {
    // The installed meta line already carries domains, allow/skipped and the
    // licence; a second "Domains:" line only made the row taller.
    domains = '';
  } else if (countEntry && countEntry.domain_count > 0) {
    domains = `<div class="filtering-preset-desc">Domains: ${historyNumber(countEntry.domain_count)}</div>`;
  } else if (filteringCounting) {
    domains = '<div class="filtering-preset-desc">Domains: counting…</div>';
  } else if (countEntry && countEntry.last_error) {
    domains = '<div class="filtering-preset-desc">Domains: unavailable</div>';
  }
  const meta = isInstalled ? `<div class="filtering-preset-desc">${filteringListMeta(installedList, updating)}</div>` : '';
  const error = isInstalled && installedList.last_error
    ? `<div class="filter-list-error">${escapeHTML(installedList.last_error)}</div>`
    : '';
  const inputID = `preset-${presetID}`;
  const rowClass = `${active ? 'is-selected ' : ''}${isInstalled ? 'is-installed' : ''}`.trim();
  const rowAttrs = isInstalled
    ? `data-preset-id="${escapeHTML(presetID)}" data-filter-id="${escapeHTML(installedList.id || '')}" data-filter-name="${escapeHTML(installedList.name || preset.name || '')}"`
    : `data-preset-id="${escapeHTML(presetID)}"`;
  const inputAttrs = isInstalled ? 'data-filter-toggle' : 'data-preset-add';
  return `<div class="filtering-preset-row ${escapeHTML(rowClass)}" ${rowAttrs}>
    <input id="${escapeHTML(inputID)}" type="checkbox" name="${escapeHTML(inputID)}" class="filtering-preset-check"
      ${inputAttrs} ${active ? 'checked' : ''} ${(updating || adding) ? 'disabled' : ''}>
    <label class="filtering-preset-main" for="${escapeHTML(inputID)}">
      <div class="filtering-preset-title">${escapeHTML(preset.name)} ${badges.join(' ')}</div>
      ${description}
      ${warning}
      ${license}
      ${domains}
      ${meta}
      ${error}
    </label>
    <div class="filtering-preset-links">${isInstalled ? installedActions(installedList) : presetLinks(preset)}</div>
  </div>`;
}

function customListRow(list) {
  const id = list.id || '';
  const updating = filteringUpdating.has(id);
  const active = !!list.enabled;
  const inputID = `custom-${id}`;
  const badgeClass = active ? 'filtering-preset-badge is-active' : 'filtering-preset-badge is-off';
  const badge = `<span class="${escapeHTML(badgeClass)}">${active ? 'ACTIVE' : 'DISABLED'}</span>`;
  const source = list.url
    ? `<div class="filter-list-url" title="${escapeHTML(list.url)}">${escapeHTML(list.url)}</div>`
    : '<div class="filter-list-url">Uploaded file</div>';
  const error = list.last_error ? `<div class="filter-list-error">${escapeHTML(list.last_error)}</div>` : '';
  const rowClass = active ? 'is-selected is-installed' : 'is-installed';
  return `<div class="filtering-preset-row ${escapeHTML(rowClass)}" data-filter-id="${escapeHTML(id)}" data-filter-name="${escapeHTML(list.name || 'Unnamed list')}">
    <input id="${escapeHTML(inputID)}" type="checkbox" name="${escapeHTML(inputID)}" class="filtering-preset-check"
      data-filter-toggle ${active ? 'checked' : ''} ${updating ? 'disabled' : ''}>
    <label class="filtering-preset-main" for="${escapeHTML(inputID)}">
      <div class="filtering-preset-title">${escapeHTML(list.name || 'Unnamed list')} ${badge}</div>
      ${source}
      <div class="filtering-preset-desc">${filteringListMeta(list, updating)}</div>
      ${error}
    </label>
    <div class="filtering-preset-links">${installedActions(list)}</div>
  </div>`;
}

function scheduleFilteringRefresh() {
  if (filteringPollTimer) clearTimeout(filteringPollTimer);
  filteringPollTimer = setTimeout(() => { loadFiltering(); }, 2500);
}

function scheduleFilteringCountPoll() {
  if (filteringCountTimer) return;
  filteringCountTimer = setTimeout(() => {
    filteringCountTimer = null;
    loadFiltering();
  }, 5000);
}

async function saveFilteringSettings() {
  const enabled = document.getElementById('filtering-enabled-input')?.checked;
  const mode = document.getElementById('filtering-mode-select')?.value;
  const ttl = Number(document.getElementById('filtering-ttl-input')?.value);
  const interval = Number(document.getElementById('filtering-interval-input')?.value);
  const allowlist = (document.getElementById('filtering-allowlist-input')?.value || '')
    .split(/\r?\n/).map((line) => line.trim()).filter(Boolean);
  if (!Number.isInteger(ttl) || ttl < 1 || ttl > 3600) {
    showToast('Block TTL must be between 1 and 3600 seconds.', 'error');
    return;
  }
  if (!Number.isInteger(interval) || interval < 1 || interval > 168) {
    showToast('Update interval must be between 1 and 168 hours.', 'error');
    return;
  }
  const payload = {
    enabled: !!enabled,
    block_mode: mode,
    block_ttl: ttl,
    update_interval_hours: interval,
    allowlist,
  };
  // Normalise the stored form before diffing: the server returns null for an
  // empty allowlist while the form sends [], and a null-to-empty-list line is
  // noise, not a change the operator made.
  const beforeFiltering = {
    enabled: !!(filteringSettings && filteringSettings.enabled),
    block_mode: (filteringSettings && filteringSettings.block_mode) || 'null_ip',
    block_ttl: (filteringSettings && filteringSettings.block_ttl) || 10,
    update_interval_hours: (filteringSettings && filteringSettings.update_interval_hours) || 24,
    allowlist: Array.isArray(filteringSettings && filteringSettings.allowlist)
      ? filteringSettings.allowlist
      : [],
  };
  if (!(await showDiffConfirm('Save ad-blocking settings?', beforeFiltering, payload, 'Save settings'))) {
    return;
  }
  const btn = document.getElementById('filtering-save-settings-btn');
  if (btn) btn.disabled = true;
  setFilteringStatus('Saving…');
  try {
    const res = await fetch(api('/api/filtering/settings'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${authToken}` },
      body: JSON.stringify(payload),
    });
    if (!res.ok) throw new Error(await errorMessage(res, 'Could not save ad blocking settings'));
    renderFiltering(await res.json());
    setFilteringStatus('Saved.');
    showToast('Ad blocking settings saved.', 'success');
  } catch (e) {
    setFilteringStatus(e.message || 'Could not save ad blocking settings.');
    showToast(e.message || 'Could not save ad blocking settings.', 'error');
  } finally {
    if (btn) btn.disabled = false;
  }
}

async function addFilteringList(name, url, content, homepage = '') {
  if (!url && !content) {
    showToast('Enter a list URL or choose a file.', 'error');
    return;
  }
  setFilteringError('');
  try {
    const res = await fetch(api('/api/filtering/lists'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${authToken}` },
      body: JSON.stringify({ name, url, homepage, content }),
    });
    if (!res.ok) throw new Error(await errorMessage(res, 'Could not add blocklist'));
    document.getElementById('filtering-url-input').value = '';
    document.getElementById('filtering-name-input').value = '';
    const home = document.getElementById('filtering-homepage-input');
    if (home) home.value = '';
    showToast('Blocklist added; compiling in the background.', 'success');
    scheduleFilteringRefresh();
  } catch (e) {
    setFilteringError(e.message || 'Could not add blocklist.');
    showToast(e.message || 'Could not add blocklist.', 'error');
  }
}

async function addFilteringPresets(ids) {
  if (!ids.length) {
    showToast('Tick a preset to add it.', 'error');
    return;
  }
  const status = document.getElementById('filtering-preset-status');
  if (status) status.textContent = 'Adding preset…';
  for (const id of ids) filteringAdding.add(id);
  renderFilteringPresets();
  try {
    const res = await fetch(api('/api/filtering/presets'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${authToken}` },
      body: JSON.stringify({ ids }),
    });
    if (!res.ok) throw new Error(await errorMessage(res, 'Could not add presets'));
    const result = await res.json();
    const added = Array.isArray(result.added) ? result.added.length : 0;
    const skipped = Array.isArray(result.skipped) ? result.skipped.length : 0;
    const failed = Array.isArray(result.failed) ? result.failed : [];
    const parts = [`${added} added`];
    if (skipped) parts.push(`${skipped} already installed`);
    if (failed.length) parts.push(`${failed.length} failed`);
    if (status) status.textContent = parts.join(' · ');
    showToast(`Presets: ${parts.join(', ')}.`, failed.length ? 'warning' : 'success');
    if (failed.length) {
      setFilteringError(failed.map((f) => `${f.name || f.id}: ${f.reason}`).join('; '));
    }
    await loadFiltering();
    scheduleFilteringRefresh();
  } catch (e) {
    if (status) status.textContent = e.message || 'Could not add presets.';
    showToast(e.message || 'Could not add presets.', 'error');
    await loadFiltering();
  } finally {
    for (const id of ids) filteringAdding.delete(id);
    renderFilteringPresets();
  }
}

async function updateFilteringList(id) {
  try {
    const res = await fetch(api(`/api/filtering/lists/${encodeURIComponent(id)}/update`), {
      method: 'POST',
      headers: { 'Authorization': `Bearer ${authToken}` },
    });
    if (!res.ok) throw new Error(await errorMessage(res, 'Could not update blocklist'));
    showToast('Updating blocklist…', 'info');
    scheduleFilteringRefresh();
  } catch (e) {
    showToast(e.message || 'Could not update blocklist.', 'error');
  }
}

async function toggleFilteringList(id, enabled) {
  try {
    const res = await fetch(api(`/api/filtering/lists/${encodeURIComponent(id)}`), {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${authToken}` },
      body: JSON.stringify({ enabled }),
    });
    if (!res.ok) throw new Error(await errorMessage(res, 'Could not change blocklist'));
    scheduleFilteringRefresh();
  } catch (e) {
    showToast(e.message || 'Could not change blocklist.', 'error');
    loadFiltering();
  }
}

async function deleteFilteringList(id, name) {
  if (!await confirmAction({
    title: 'Delete blocklist?',
    message: `Remove “${name}” and its downloaded rules?`,
    confirmText: 'DELETE',
    destructive: true,
  })) return;
  try {
    const res = await fetch(api(`/api/filtering/lists/${encodeURIComponent(id)}`), {
      method: 'DELETE',
      headers: { 'Authorization': `Bearer ${authToken}` },
    });
    if (!res.ok) throw new Error(await errorMessage(res, 'Could not delete blocklist'));
    showToast('Blocklist deleted.', 'success');
    scheduleFilteringRefresh();
  } catch (e) {
    showToast(e.message || 'Could not delete blocklist.', 'error');
  }
}

async function refreshAllFiltering() {
  try {
    const res = await fetch(api('/api/filtering/refresh'), {
      method: 'POST',
      headers: { 'Authorization': `Bearer ${authToken}` },
    });
    if (!res.ok) throw new Error(await errorMessage(res, 'Could not refresh blocklists'));
    showToast('Refreshing enabled blocklists…', 'info');
    scheduleFilteringRefresh();
  } catch (e) {
    showToast(e.message || 'Could not refresh blocklists.', 'error');
  }
}

// testFilterRule asks the server which rule would decide a name. It is
// deliberately read-only: the matcher is consulted, nothing is added or rebuilt,
// so an operator can probe a suspicious domain without changing policy.
async function testFilterRule() {
  const input = document.getElementById('filter-test-input');
  const out = document.getElementById('filter-test-result');
  const btn = document.getElementById('filter-test-btn');
  if (!out || !input) return;
  const domain = input.value.trim().replace(/\.$/, '');
  if (!domain) {
    out.classList.remove('hidden');
    out.textContent = 'Type a domain to test.';
    out.className = 'text-xs text-slate-400 mt-1';
    return;
  }
  out.classList.remove('hidden');
  out.textContent = 'Testing…';
  out.className = 'text-xs text-slate-400 mt-1';
  if (btn) btn.disabled = true;
  try {
    const res = await fetch(api('/api/filter/test?domain=' + encodeURIComponent(domain)), {
      headers: { 'Authorization': `Bearer ${authToken}` },
      cache: 'no-store',
    });
    if (!res.ok) throw new Error(await errorMessage(res, 'The rule test failed.'));
    const r = await res.json();
    const action = String(r.action || 'DIRECT');
    const tone = action === 'BLOCK' ? 'text-rose-300'
      : action === 'PROXY' ? 'text-cyan-300'
      : 'text-emerald-300';
    const pattern = String(r.pattern || '');
    const how = pattern
      ? (pattern.startsWith('*.') ? `parent wildcard ${pattern}` : `exact entry ${pattern}`)
      : 'no rule matched';
    out.className = 'text-xs mt-1 flex flex-wrap items-center gap-2';
    out.innerHTML =
      `<span class="font-mono text-slate-400">${escapeHTML(domain)}</span>` +
      `<span class="text-slate-600">→</span>` +
      `<span class="font-bold ${tone}">${escapeHTML(action)}</span>` +
      `<span class="text-slate-300">${escapeHTML(String(r.rule || 'Direct'))}</span>` +
      `<span class="font-mono text-slate-500">${escapeHTML(how)}</span>`;
  } catch (e) {
    out.className = 'text-xs mt-1 text-amber-300';
    out.textContent = e.message || 'The rule test failed.';
  } finally {
    if (btn) btn.disabled = false;
  }
}

function initFiltering() {
  if (filteringInitialized) return;
  filteringInitialized = true;
  document.getElementById('filtering-save-settings-btn')?.addEventListener('click', saveFilteringSettings);
  document.getElementById('filtering-refresh-all-btn')?.addEventListener('click', refreshAllFiltering);
  document.getElementById('filtering-show-custom-btn')?.addEventListener('click', () => {
    document.getElementById('filtering-custom-panel')?.classList.toggle('hidden');
  });
  document.getElementById('filtering-preset-search')?.addEventListener('input', renderFilteringPresets);
  document.getElementById('filtering-recommended-base-btn')?.addEventListener('click', () => {
    addFilteringPresets(['hagezi-pro']);
  });
  document.getElementById('filtering-add-url-btn')?.addEventListener('click', () => {
    addFilteringList(
      document.getElementById('filtering-name-input')?.value || '',
      document.getElementById('filtering-url-input')?.value || '',
      '',
      document.getElementById('filtering-homepage-input')?.value || '',
    );
  });
  document.getElementById('filter-test-btn')?.addEventListener('click', testFilterRule);
  document.getElementById('filter-test-input')?.addEventListener('keydown', (event) => {
    if (event.key === 'Enter') {
      event.preventDefault();
      testFilterRule();
    }
  });
  document.getElementById('filtering-file-input')?.addEventListener('change', async (event) => {
    const file = event.target.files && event.target.files[0];
    if (!file) return;
    if (file.size > 16 * 1024 * 1024) {
      showToast('That file is larger than 16 MB.', 'error');
      event.target.value = '';
      return;
    }
    try {
      const content = await file.text();
      await addFilteringList(
        document.getElementById('filtering-name-input')?.value || file.name,
        '',
        content,
        document.getElementById('filtering-homepage-input')?.value || '',
      );
    } catch (e) {
      showToast('Could not read that file.', 'error');
    }
    event.target.value = '';
  });
  const roster = document.getElementById('filtering-presets');
  roster?.addEventListener('change', (event) => {
    const input = event.target.closest('[data-preset-add], [data-filter-toggle]');
    const row = event.target.closest('[data-preset-id], [data-filter-id]');
    if (!input || !row) return;
    if (input.hasAttribute('data-filter-toggle')) {
      toggleFilteringList(row.dataset.filterId, input.checked);
      return;
    }
    // One-click add: ticking an uninstalled preset installs it immediately.
    if (input.checked) {
      addFilteringPresets([row.dataset.presetId]);
    } else {
      renderFilteringPresets();
    }
  });
  roster?.addEventListener('click', (event) => {
    const row = event.target.closest('[data-filter-id]');
    if (!row) return;
    if (event.target.closest('[data-filter-update]')) {
      updateFilteringList(row.dataset.filterId);
      return;
    }
    if (event.target.closest('[data-filter-delete]')) {
      deleteFilteringList(row.dataset.filterId, row.dataset.filterName || 'this list');
    }
  });
}

// =======================================================
// EVENT LISTENERS & SPA ROUTING
// =======================================================
/* ── Services ─────────────────────────────────────────────────────────────────
 * A service is a label that groups domains so they can be found. It carries no
 * action and never touches routing: the worst a wrong label can do is make a
 * domain harder to find. Stored as two maps in the rules payload -
 * services[id] = name, domain_services[domain] = id - which is the same shape
 * custom_records and custom_domain_added already use.
 */
let activeService = '';   // '' = all, '__none__' = unlabelled

function serviceName(id) {
  const services = (currentConfig && currentConfig.rules && currentConfig.rules.services) || {};
  return services[id] || '';
}

function serviceOfDomain(domain) {
  const links = (currentConfig && currentConfig.rules && currentConfig.rules.domain_services) || {};
  return links[String(domain || '').toLowerCase()] || '';
}

function serviceCounts() {
  const rules = (currentConfig && currentConfig.rules) || {};
  const links = rules.domain_services || {};
  // Count what the table actually shows: every pattern in the four lists, once.
  const all = new Set();
  for (const key of ['custom_proxied', 'custom_proxied_disabled', 'custom_blocked', 'custom_direct']) {
    for (const d of (rules[key] || [])) all.add(String(d).toLowerCase());
  }
  const counts = {};
  let none = 0;
  for (const d of all) {
    const id = links[d];
    if (!id) { none++; continue; }
    counts[id] = (counts[id] || 0) + 1;
  }
  return { counts, none, total: all.size };
}

function serviceHue(name) {
  let h = 0;
  for (const ch of String(name || '')) h = (h * 31 + ch.charCodeAt(0)) % 360;
  return h;
}

// The icon is stored with the service: a bare domain is turned into a favicon
// URL, anything else is used as given. Both are rendered as an <img> with the
// letter mark behind it, so a dead URL degrades instead of leaving a hole.
function serviceIconURL(raw) {
  const v = String(raw || '').trim();
  if (!v) return '';
  if (/^https?:\/\//i.test(v)) return v;
  if (!/^[a-z0-9.-]+\.[a-z]{2,}$/i.test(v)) return '';
  return `https://icons.duckduckgo.com/ip3/${encodeURIComponent(v)}.ico`;
}

// Icon-library suggestions. The search goes through the dashboard's own proxy
// (same origin, so connect-src stays 'self'); the chosen icon is stored as its
// normal https SVG URL, which both the panel and the Android app already render.
const ICON_REPO_BASE = 'https://api.iconify.design';
const ICON_CHOICE_COLOR = '%2394a3b8';
let iconSearchTimer = null;
let iconSearchSeq = 0;

// Sets that carry their own palette: no colour parameter, or the logo/emoji
// would be flattened to one tint. The rest are monochrome and get the neutral
// slate so they stay visible on both themes.
const ICON_COLORFUL_PREFIXES = new Set(['logos', 'flat-color-icons', 'twemoji', 'skill-icons', 'devicon']);

function iconRepoURL(id) {
  if (!/^[a-z0-9-]+:[a-z0-9-]+$/.test(String(id || ''))) return '';
  const [prefix, name] = id.split(':');
  const base = `${ICON_REPO_BASE}/${encodeURIComponent(prefix)}/${encodeURIComponent(name)}.svg`;
  return ICON_COLORFUL_PREFIXES.has(prefix) ? base : `${base}?color=${ICON_CHOICE_COLOR}`;
}

function hideIconResults() {
  document.getElementById('service-icon-results')?.classList.add('hidden');
}

function renderIconResults(ids) {
  const box = document.getElementById('service-icon-results');
  if (!box) return;
  const urls = (Array.isArray(ids) ? ids : [])
    .map(iconRepoURL)
    .filter(Boolean)
    .slice(0, 12);
  if (!urls.length) {
    box.innerHTML = '';
    box.classList.add('hidden');
    return;
  }
  box.innerHTML = urls.map((url) =>
    `<button type="button" class="service-icon-choice" data-icon-url="${escapeHTML(url)}" title="Use this icon">
       <img src="${escapeHTML(url)}" alt="" width="22" height="22">
     </button>`).join('');
  box.classList.remove('hidden');
}

async function searchServiceIcons(raw) {
  const q = String(raw || '').trim();
  // A URL or a domain is an explicit choice; only unknown words are searched.
  if (q.length < 2 || serviceIconURL(q)) { hideIconResults(); return; }
  const seq = ++iconSearchSeq;
  try {
    const res = await fetch(api(`/api/icons/search?q=${encodeURIComponent(q)}`), {
      headers: { 'Authorization': `Bearer ${authToken}` },
    });
    if (!res.ok) { if (seq === iconSearchSeq) hideIconResults(); return; }
    const data = await res.json();
    if (seq !== iconSearchSeq) return;
    renderIconResults(data && data.icons);
  } catch (e) {
    if (seq === iconSearchSeq) hideIconResults();
  }
}

function renderServices() {
  const list = document.getElementById('services-list');
  if (!list || !currentConfig || !currentConfig.rules) return;
  const services = currentConfig.rules.services || {};
  const { counts, none, total } = serviceCounts();

  const marks = currentConfig.rules.service_icons || {};
  const row = (id, label, count) => {
    const hue = serviceHue(label);
    const markStyle = `background:hsl(${hue} 45% 22%);color:hsl(${hue} 70% 70%)`;
    const cls = 'service-row w-full text-start px-2 py-1.5 rounded text-[11px]' +
      (id === activeService ? ' is-active' : '');
    return `
    <div role="button" tabindex="0" class="${escapeHTML(cls)}"
      data-service="${escapeHTML(id)}">
      ${id && id !== '__none__'
        ? (marks[id]
            ? `<img class="service-mark" src="${escapeHTML(marks[id])}" alt="" data-letter="${escapeHTML((label || '?').trim().charAt(0).toUpperCase())}" data-hue="${escapeHTML(String(hue))}">`
            : `<span class="service-mark" style="${escapeHTML(markStyle)}">${escapeHTML((label || '?').trim().charAt(0).toUpperCase())}</span>`)
        : '<span class="service-mark" aria-hidden="true">·</span>'}
      <span class="service-name truncate">${label}</span>
      ${id && id !== '__none__'
        ? `<span class="service-row-actions flex items-center gap-1">
             <button type="button" data-service-edit="${escapeHTML(id)}" title="Rename" aria-label="Rename ${escapeHTML(label)}" class="text-slate-500 hover:text-cyan-300"><i data-feather="edit-2" class="w-3 h-3"></i></button>
             <button type="button" data-service-del="${escapeHTML(id)}" title="Delete" aria-label="Delete ${escapeHTML(label)}" class="text-slate-500 hover:text-red-400"><i data-feather="trash-2" class="w-3 h-3"></i></button>
           </span>`
        : ''}
      <span class="service-count text-[10px] font-mono text-slate-500">${count}</span>
    </div>`;
  };

  const entries = Object.entries(services).sort((a, b) => a[1].localeCompare(b[1]));
  // No virtual "All domains" entry: with no service selected the table already
  // shows everything, and an explicit row for it competed with the real groups.
  // "No service" stays last because it is the remainder, not a group among them.
  list.innerHTML =
    entries.map(([id, name]) => row(id, escapeHTML(name), counts[id] || 0)).join('') +
    row('__none__', 'No service', none);

  // A favicon that 404s or is blocked leaves an <img> with no pixels. Replace it
  // with the same letter mark used when no icon was given, so a service always
  // has an identity. addEventListener, not onerror: the CSP forbids inline handlers.
  list.querySelectorAll('img.service-mark').forEach((img) => {
    img.addEventListener('error', () => {
      const fallback = document.createElement('span');
      fallback.className = 'service-mark';
      fallback.textContent = img.dataset.letter || '?';
      const hue = img.dataset.hue || '210';
      fallback.style.background = `hsl(${hue} 45% 22%)`;
      fallback.style.color = `hsl(${hue} 70% 70%)`;
      img.replaceWith(fallback);
    }, { once: true });
  });
  renderBulkServiceOptions();
  renderBulkEgressOptions();
  safeFeatherReplace();
  requestAnimationFrame(sizeProxiedScroll);
}

function setActiveService(id) {
  // Tapping the already-selected service clears the filter, since there is no
  // separate "All domains" row to go back to.
  const next = id || '';
  activeService = activeService === next ? '' : next;
  renderServices();
  renderProxiedList();
}

function assignService(domain, id) {
  const rules = currentConfig.rules;
  const links = Object.assign({}, rules.domain_services || {});
  const key = String(domain || '').toLowerCase();
  if (id) links[key] = id; else delete links[key];
  rules.domain_services = links;
  saveRules();
  renderServices();
  renderProxiedList();
}

// Which service the form is editing. Empty means "creating a new one", so the
// same two fields serve both flows instead of a second dialog.
let serviceEditing = '';

function openServiceForm(id) {
  const f = document.getElementById('add-service-form');
  if (!f) return;
  serviceEditing = id || '';
  const nameEl = document.getElementById('service-name');
  const iconEl = document.getElementById('service-icon');
  const services = (currentConfig.rules && currentConfig.rules.services) || {};
  const icons = (currentConfig.rules && currentConfig.rules.service_icons) || {};
  if (nameEl) nameEl.value = serviceEditing ? (services[serviceEditing] || '') : '';
  if (iconEl) iconEl.value = serviceEditing ? (icons[serviceEditing] || '') : '';
  const relayEl = document.getElementById('service-relay');
  if (relayEl) relayEl.value = serviceEditing ? ((currentConfig.rules.service_agents || {})[serviceEditing] || '') : '';
  renderServiceRelayControl();
  hideIconResults();
  document.getElementById('save-service-btn').textContent = serviceEditing ? 'Save' : 'Add';
  f.classList.remove('hidden');
  nameEl?.focus();
}

async function createService() {
  const nameEl = document.getElementById('service-name');
  const iconEl = document.getElementById('service-icon');
  const name = (nameEl?.value || '').trim();
  if (!name) { nameEl?.focus(); return; }

  const rules = currentConfig.rules;
  const services = Object.assign({}, rules.services || {});
  // Renaming keeps the id, so every domain already labelled stays labelled: the
  // id is the identity, the name is only what it is called.
  const id = serviceEditing || (name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '') || 'service');
  services[id] = name;
  rules.services = services;

  const iconURL = serviceIconURL(iconEl?.value || '');
  const icons = Object.assign({}, rules.service_icons || {});
  if (iconURL) icons[id] = iconURL; else delete icons[id];
  rules.service_icons = icons;

  const serviceRelay = document.getElementById('service-relay')?.value || '';
  const serviceAgents = Object.assign({}, rules.service_agents || {});
  if (serviceRelay) serviceAgents[id] = serviceRelay; else delete serviceAgents[id];
  rules.service_agents = serviceAgents;

  await saveRules();
  renderServices();
  // The per-row selects are built from the same map, so they need repainting too;
  // otherwise a new service is missing from them until the next reload.
  renderProxiedList();
  document.getElementById('add-service-form')?.classList.add('hidden');
  hideIconResults();
  serviceEditing = '';
  if (nameEl) nameEl.value = '';
  if (iconEl) iconEl.value = '';
}

async function deleteService(id) {
  if (!id) return;
  const rules = currentConfig.rules;
  const name = (rules.services || {})[id] || id;
  if (!window.confirm(`Delete "${name}"? Domains keep their routing; they just lose the label.`)) return;

  const services = Object.assign({}, rules.services || {});
  delete services[id];
  rules.services = services;

  const icons = Object.assign({}, rules.service_icons || {});
  delete icons[id];
  rules.service_icons = icons;

  // Clear the labels here as well as on the server: the save prunes them, but the
  // in-memory copy drives the table and would otherwise keep showing a service
  // that no longer exists until the next reload.
  const links = Object.assign({}, rules.domain_services || {});
  for (const [d, sid] of Object.entries(links)) {
    if (sid === id) delete links[d];
  }
  rules.domain_services = links;

  const serviceAgents = Object.assign({}, rules.service_agents || {});
  delete serviceAgents[id];
  rules.service_agents = serviceAgents;

  await saveRules();
  if (activeService === id) activeService = '';
  renderServices();
  renderProxiedList();
}

function initServices() {
  document.getElementById('add-service-btn')?.addEventListener('click', () => openServiceForm(''));
  document.getElementById('service-icon')?.addEventListener('input', (e) => {
    clearTimeout(iconSearchTimer);
    hideIconResults();
    const value = e.target.value;
    iconSearchTimer = setTimeout(() => searchServiceIcons(value), 350);
  });
  document.getElementById('service-icon-results')?.addEventListener('click', (e) => {
    const choice = e.target.closest('[data-icon-url]');
    if (!choice) return;
    const iconEl = document.getElementById('service-icon');
    if (iconEl) iconEl.value = choice.dataset.iconUrl;
    hideIconResults();
  });
  document.getElementById('save-service-btn')?.addEventListener('click', createService);
  document.getElementById('cancel-service-btn')?.addEventListener('click', () => {
    document.getElementById('add-service-form')?.classList.add('hidden');
  });
  document.getElementById('service-relay-trigger')?.addEventListener('click', (e) => {
    e.stopPropagation();
    openServiceRelayPicker(e.currentTarget);
  });
  document.getElementById('service-relay-menu')?.addEventListener('click', (e) => {
    const row = e.target.closest('[data-service-relay-id]');
    if (!row) return;
    const input = document.getElementById('service-relay');
    if (input) input.value = row.dataset.serviceRelayId;
    renderServiceRelayControl();
    closeServiceRelayPicker();
  });
  document.addEventListener('click', (e) => {
    if (!e.target.closest('#service-relay-menu') && !e.target.closest('#service-relay-trigger')) {
      closeServiceRelayPicker();
    }
  });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') closeServiceRelayPicker();
  });
  window.addEventListener('scroll', closeServiceRelayPicker, true);
  window.addEventListener('resize', closeServiceRelayPicker);
  document.getElementById('services-list')?.addEventListener('keydown', (e) => {
    if (e.key !== 'Enter' && e.key !== ' ') return;
    const row = e.target.closest('[data-service]');
    if (!row || e.target !== row) return;
    e.preventDefault();
    setActiveService(row.dataset.service);
  });
  initDashboardEditor();
  document.getElementById('services-list')?.addEventListener('click', (e) => {
    const edit = e.target.closest('[data-service-edit]');
    if (edit) { openServiceForm(edit.dataset.serviceEdit); return; }
    const del = e.target.closest('[data-service-del]');
    if (del) { deleteService(del.dataset.serviceDel); return; }
    const b = e.target.closest('[data-service]');
    if (b) setActiveService(b.dataset.service);
  });
}


// ── Relay agents ─────────────────────────────────────────────────────────────
// A relay agent is an optional egress point in another country. Domains can be
// assigned directly, or inherit a service's default. The agent itself has no
// certificate in this design: it reads SNI and forwards the encrypted stream.
let agentsCache = [];
// The agent cards carry live online/last-seen/sync state, so the tab refreshes
// them on a slow beat while it is visible. It is not a 1 Hz dashboard poll: the
// data changes at heartbeat speed.
let agentPollTimer = null;
// The agent whose inline edit form is open, so its card can stay highlighted.
let agentEditingId = '';
let agentCountrySelectReady = false;

// Pinned first because these are the exits operators ask for most. The rest is
// the ISO list the bundled flags cover; names come from Intl.DisplayNames.
const AGENT_PINNED_COUNTRIES = ['gb', 'us', 'de', 'by', 'nl', 'fr', 'ca', 'au', 'jp', 'pl'];
const AGENT_COUNTRY_CODES = `ad ae af ag ai al am ao aq ar as at au aw ax az ba bb bd be bf bg bh bi bj bl bm bn bo bq br bs bt bv bw by bz ca cc cd cf cg ch ci ck cl cm cn co cp cr cu cv cw cx cy cz de dg dj dk dm do dz ec ee eg eh er es et eu fi fj fk fm fo fr ga gb gd ge gf gg gh gi gl gm gn gp gq gr gs gt gu gw gy hk hm hn hr ht hu ic id ie il im in io iq ir is it je jm jo jp ke kg kh ki km kn kp kr kw ky kz la lb lc li lk lr ls lt lu lv ly ma mc md me mf mg mh mk ml mm mn mo mp mq mr ms mt mu mv mw mx my mz na nc ne nf ng ni nl no np nr nu nz om pa pc pe pf pg ph pk pl pm pn pr ps pt pw py qa re ro rs ru rw sa sb sc sd se sg sh si sj sk sl sm sn so sr ss st sv sx sy sz tc td tf tg th tj tk tl tm tn to tr tt tv tw tz ua ug um un us uy uz va vc ve vg vi vn vu wf ws xk xx ye yt za zm zw`.trim().split(/\s+/);

function agentIsOnline(a) {
  if (!a || !a.enabled || !a.last_seen || String(a.last_seen).startsWith('0001')) return false;
  // The server computes this with the same 90 s window the DNS router uses.
  // Fall back to the old value only for an older hub.
  if (typeof a.online === 'boolean') return a.online;
  return Date.now() - new Date(a.last_seen).getTime() < 90000;
}

// agentSyncLabel turns the server's applied-revision age into the short string
// shown on the card. A missing revision means the agent has not reported one
// (older build) — not that it is broken.
function agentSyncLabel(a) {
  if (!a || !a.catalogue_revision) return '—';
  const age = Number(a.sync_age_seconds);
  if (!Number.isFinite(age) || age < 0) return '—';  if (age < 5) return 'just now';
  if (age < 60) return `${Math.floor(age)}s ago`;
  if (age < 3600) return `${Math.floor(age / 60)}m ago`;
  return `${Math.floor(age / 3600)}h ago`;
}

function agentRelativeAge(iso) {
  if (!iso || String(iso).startsWith('0001')) return 'never';
  const secs = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 1000));
  if (secs < 60) return secs + 's ago';
  if (secs < 3600) return Math.round(secs / 60) + 'm ago';
  if (secs < 86400) return Math.round(secs / 3600) + 'h ago';
  return Math.round(secs / 86400) + 'd ago';
}

function humanBytes(n) {
  n = Number(n) || 0;
  if (n < 1024) return n + ' B';
  const units = ['KiB', 'MiB', 'GiB', 'TiB'];
  let i = -1;
  do { n /= 1024; i++; } while (n >= 1024 && i < units.length - 1);
  return n.toFixed(1) + ' ' + units[i];
}

function countryFlagHTML(code, cls) {
  const url = flagURL(code);
  if (!url) return '';
  const klass = cls || 'agent-flag';
  return `<img class="${escapeHTML(klass)}" src="${escapeHTML(url)}" alt="${escapeHTML(countryName(code))}" title="${escapeHTML(countryName(code))}">`;
}

function agentFlagHTML(a, cls) {
  if (!a || !a.country) return '';
  return countryFlagHTML(a.country, cls);
}

function agentForId(id) {
  return agentsCache.find((a) => a.id === id) || null;
}

// Hub-side egresses (SOCKS5 and WireGuard) are a separate list from agents.
// Assignment values use the "egress:<id>" prefix; existing agent IDs are left
// untouched so saved rules keep working.
let egressesCache = [];
let egressCountrySelectReady = false;
let egressEditingId = '';

function egressIsOnline(e) {
  return !!(e && e.enabled && e.health === 'online');
}

function egressForId(value) {
  const raw = String(value || '');
  if (!raw.startsWith('egress:')) return null;
  const id = raw.slice('egress:'.length);
  return egressesCache.find((e) => e.id === id) || null;
}

function egressKindLabel(kind) {
  return kind === 'wireguard' ? 'WireGuard' : 'SOCKS5';
}

function egressFlagHTML(e, cls) {
  return e && e.country ? countryFlagHTML(e.country, cls) : '';
}

function egressValue(e) {
  return e && e.id ? 'egress:' + e.id : '';
}

function matchRelayPattern(assignments, domain) {
  const d = String(domain || '').toLowerCase().replace(/\.$/, '');
  let best = '';
  let bestScore = -1;
  for (const [pattern, value] of Object.entries(assignments || {})) {
    if (!value) continue;
    let p = String(pattern || '').toLowerCase().replace(/\.$/, '');
    if (!p) continue;
    let matches = false;
    let score = 0;
    if (p.startsWith('*.')) {
      const base = p.slice(2);
      if (base && d.endsWith('.' + base)) { matches = true; score = base.length + 1; }
    } else if (d === p) {
      matches = true; score = p.length + 1000;
    } else if (d.endsWith('.' + p)) {
      matches = true; score = p.length;
    }
    if (matches && score > bestScore) { best = value; bestScore = score; }
  }
  return best;
}

function effectiveRelay(domain) {
  const rules = (currentConfig && currentConfig.rules) || {};
  const direct = matchRelayPattern(rules.domain_agents || {}, domain);
  if (direct && direct !== '__inherit__') {
    if (direct === '__local__') return { local: true, source: 'domain' };
    if (direct.startsWith('egress:')) {
      const egress = egressForId(direct);
      return { local: false, id: direct, egress, kind: 'egress', source: 'domain' };
    }
    return { local: false, id: direct, agent: agentForId(direct), source: 'domain' };
  }
  const service = serviceOfDomain(domain);
  const sid = service ? ((rules.service_agents || {})[service] || '') : '';
  if (sid) {
    if (sid.startsWith('egress:')) {
      const egress = egressForId(sid);
      return { local: false, id: sid, egress, kind: 'egress', source: 'service', service };
    }
    return { local: false, id: sid, agent: agentForId(sid), source: 'service', service };
  }
  return { local: true, source: 'local' };
}


function assignedDomainsForAgent(id) {
  const rules = (currentConfig && currentConfig.rules) || {};
  const out = new Set();
  for (const [d, aid] of Object.entries(rules.domain_agents || {})) {
    if (aid === id) out.add(d);
  }
  const services = new Set(Object.entries(rules.service_agents || {})
    .filter(([, aid]) => aid === id)
    .map(([sid]) => sid));
  for (const [d, sid] of Object.entries(rules.domain_services || {})) {
    if (!services.has(sid)) continue;
    const override = (rules.domain_agents || {})[d];
    if (override && override !== id) continue;
    out.add(d);
  }
  return out;
}

function updateAgentSummary() {
  const count = agentsCache.length;
  const upCount = agentsCache.filter(agentIsOnline).length;
  const assigned = new Set();
  let traffic = 0;
  for (const a of agentsCache) {
    for (const d of assignedDomainsForAgent(a.id)) assigned.add(d);
    traffic += Number(a.bytes_in || 0) + Number(a.bytes_out || 0);
  }
  const set = (id, value) => { const el = document.getElementById(id); if (el) el.textContent = value; };
  set('agent-count', String(count));
  set('agent-online-count', String(upCount));
  set('agent-domain-count', String(assigned.size));
  set('agent-traffic', humanBytes(traffic));
}

async function loadAgents() {
  const list = document.getElementById('agents-list');
  // The route pickers and the Egress Options page read both lists, so every
  // agent refresh also refreshes the hub-side egresses. Failures are rendered
  // by loadEgresses itself and do not block the agent list.
  await loadEgresses();
  try {
    const res = await fetch(api('/api/agents'), { headers: { 'Authorization': `Bearer ${authToken}` } });
    if (res.status === 401) {
      stopAgentPolling();
      showLoginModal();
      return;
    }
    if (!res.ok) throw new Error(await errorMessage(res, 'Could not load agents'));
    const data = await res.json();
    agentsCache = Array.isArray(data.agents) ? data.agents : [];
    renderAgents();
    renderServiceRelayControl();
    updateAgentSummary();
    if (currentConfig && document.getElementById('custom-proxied-rows')) renderProxiedList();
  } catch (e) {
    if (list) list.innerHTML = `<p class="text-xs text-red-400 py-2">Could not load agents: ${escapeHTML(e.message)}</p>`;
  }
}

function startAgentPolling() {
  if (agentPollTimer) return;
  agentPollTimer = setInterval(() => {
    if (activeDashTab !== 'agents' || document.visibilityState === 'hidden' || !authToken) return;
    // A card being edited owns its inputs until the operator saves or cancels.
    if (agentEditingId) return;
    loadAgents();
  }, 10000);
}

function stopAgentPolling() {
  if (agentPollTimer) {
    clearInterval(agentPollTimer);
    agentPollTimer = null;
  }
}

// Reload one card meaningfully: the agent snapshot gives traffic, heartbeat and
// ping, while /api/config gives the assignment maps that drive the Domains
// count. Loading only the agent list would update traffic and leave a stale
// count, which is the exact confusion this button exists to remove.
async function refreshAgentCard(id, button) {
  if (!id) return;
  if (button) {
    button.classList.add('is-refreshing');
    button.disabled = true;
  }
  try {
    const [agentRes, configRes] = await Promise.all([
      fetch(api('/api/agents'), { headers: { 'Authorization': `Bearer ${authToken}` } }),
      fetch(api('/api/config'), { headers: { 'Authorization': `Bearer ${authToken}` } }),
    ]);
    if (!agentRes.ok) throw new Error(await errorMessage(agentRes, 'Could not refresh the agent'));
    if (!configRes.ok) throw new Error(await errorMessage(configRes, 'Could not refresh relay assignments'));
    const agentData = await agentRes.json();
    const cfg = await configRes.json();
    agentsCache = Array.isArray(agentData.agents) ? agentData.agents : [];
    if (currentConfig) {
      currentConfig.rules = cfg.rules || currentConfig.rules;
      currentConfig.dashboard_layout = cfg.dashboard_layout ?? currentConfig.dashboard_layout;
    } else {
      currentConfig = cfg;
    }
    renderAgents();
    updateAgentSummary();
    renderServiceRelayControl();
    if (currentConfig && document.getElementById('custom-proxied-rows')) renderProxiedList();
    showToast('Agent refreshed', 'success');
  } catch (e) {
    showToast(e.message || 'Could not refresh the agent', 'error');
  } finally {
    if (button) {
      button.classList.remove('is-refreshing');
      button.disabled = false;
    }
    safeFeatherReplace();
  }
}

function agentPingBadge(a) {
  const measured = Number(a.ping_ms) > 0 && a.ping_at && !String(a.ping_at).startsWith('0001');
  if (!measured) {
    return {
      cls: 'text-slate-400 bg-slate-500/10 border-slate-600/40',
      text: 'ping —',
      title: 'No connection test yet',
    };
  }
  const ms = Number(a.ping_ms);
  let cls = 'text-emerald-400 bg-emerald-500/10 border-emerald-500/30';
  if (ms > 40) cls = 'text-yellow-400 bg-yellow-500/10 border-yellow-500/30';
  if (ms > 80) cls = 'text-red-400 bg-red-500/10 border-red-500/30';
  return {
    cls,
    text: `ping ${ms.toFixed(1)} ms`,
    title: `Last test ${agentRelativeAge(a.ping_at)}`,
  };
}

function renderAgents() {
  const list = document.getElementById('agents-list');
  if (!list) return;
  updateAgentSummary();
  if (!agentsCache.length) {
    list.innerHTML = `
      <div class="agent-empty text-center py-8 border border-slate-800 rounded-lg">
        <i data-feather="globe" class="w-6 h-6 mx-auto text-slate-600 mb-2"></i>
        <p class="text-xs font-bold text-slate-300 font-heading">No relay agents yet</p>
        <p class="text-[11px] text-slate-500 mt-1">Add one on a server in another country, then assign domains or services to it.</p>
      </div>`;
    safeFeatherReplace();
    return;
  }
  list.innerHTML = agentsCache.map((a) => {
    const isOnline = agentIsOnline(a);
    const status = !a.enabled ? 'disabled' : (isOnline ? 'online' : 'offline');
    const stateClass = status === 'online' ? 'is-online' : (status === 'offline' ? 'is-offline' : 'is-disabled');
    const cardClass = 'agent-card glass-panel panel-card p-4 sm:p-5 flex flex-col justify-between space-y-3'
      + (status === 'offline' ? ' is-offline' : '')
      + (status === 'disabled' ? ' is-disabled' : '')
      + (agentEditingId === a.id ? ' is-editing' : '');
    const ip = a.public_ip || 'not reported';
    const ipv6 = a.public_ipv6 || 'not reported';
    const globalIPv6 = !!currentConfig?.server?.proxy_ipv6_enabled;
    const ipv6Badge = !a.ipv6_enabled
      ? '<span class="agent-badge is-disabled">IPv6 OFF</span>'
      : (!globalIPv6
        ? '<span class="agent-badge is-disabled" title="Global Proxy IPv6 is off">IPv6 GLOBAL OFF</span>'
        : (a.public_ipv6
          ? '<span class="agent-badge is-online">IPv6 ON</span>'
          : '<span class="agent-badge is-offline">IPv6 WAIT</span>'));
    const traffic = humanBytes((a.bytes_in || 0) + (a.bytes_out || 0));
    const assigned = assignedDomainsForAgent(a.id).size;
    const rules = currentConfig?.rules || {};
    const services = Object.entries(rules.service_agents || {})
      .filter(([, agentID]) => agentID === a.id)
      .map(([serviceID]) => (rules.services || {})[serviceID] || serviceID);
    const ping = agentPingBadge(a);
    return `
      <div class="${escapeHTML(cardClass)}">
        <div>
          <div class="flex items-start justify-between gap-2 mb-2 pb-2 border-b border-slate-800/80">
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2 flex-wrap">
                ${agentFlagHTML(a)}
                <h4 class="agent-name truncate">${escapeHTML(a.name || a.id)}</h4>
                <span class="agent-badge ${escapeHTML(stateClass)}">${escapeHTML(status)}</span>
                ${ipv6Badge}
                ${agentEditingId === a.id ? '<span class="agent-badge is-editing">editing</span>' : ''}
              </div>
              <div class="agent-meta mt-0.5 truncate">${escapeHTML(a.country || '—')}${a.country ? ' · ' + escapeHTML(countryName(a.country)) : ''} · ${escapeHTML(a.id)}</div>
            </div>
            <div class="flex items-center gap-1 shrink-0">
              <button type="button" class="agent-icon-btn" data-agent-refresh="${escapeHTML(a.id)}" title="Refresh agent" aria-label="Refresh agent ${escapeHTML(a.name || a.id)}">
                <i data-feather="refresh-cw" class="w-3.5 h-3.5"></i>
              </button>
              <button type="button" class="agent-icon-btn" data-agent-edit="${escapeHTML(a.id)}" title="Edit agent" aria-label="Edit agent ${escapeHTML(a.name || a.id)}">
                <i data-feather="edit-2" class="w-3.5 h-3.5"></i>
              </button>
              <button type="button" class="agent-icon-btn" data-agent-toggle="${escapeHTML(a.id)}" data-enable="${escapeHTML(a.enabled ? '0' : '1')}" title="${escapeHTML(a.enabled ? 'Disable agent' : 'Enable agent')}" aria-label="${escapeHTML(a.enabled ? 'Disable agent' : 'Enable agent')}">
                <i data-feather="${escapeHTML(a.enabled ? 'pause' : 'play')}" class="w-3.5 h-3.5"></i>
              </button>
              <button type="button" class="agent-icon-btn agent-icon-btn-danger" data-agent-delete="${escapeHTML(a.id)}" title="Delete agent" aria-label="Delete agent ${escapeHTML(a.name || a.id)}">
                <i data-feather="trash-2" class="w-3.5 h-3.5"></i>
              </button>
            </div>
          </div>

          <div class="agent-card-body">
            <div class="agent-card-values">
              <div class="flex items-center justify-between gap-2 text-[10px] font-mono bg-slate-950/80 px-2.5 py-1.5 rounded-lg border border-slate-800/80">
                <span class="text-slate-500 font-semibold shrink-0">PUBLIC IP</span>
                <span class="text-cyan-300 min-w-0 flex-1 truncate text-right select-all" title="${escapeHTML(ip)}">${escapeHTML(ip)}</span>
                ${a.public_ip ? `<button type="button" class="agent-icon-btn agent-icon-btn-inline" data-agent-copy-ip="${escapeHTML(a.public_ip)}" title="Copy public IP" aria-label="Copy public IP"><i data-feather="copy" class="w-3 h-3"></i></button>` : ''}
              </div>
              <div class="flex items-center justify-between gap-2 text-[10px] font-mono bg-slate-950/80 px-2.5 py-1.5 rounded-lg border border-slate-800/80">
                <span class="text-slate-500 font-semibold shrink-0">PUBLIC IPV6</span>
                <span class="${a.public_ipv6 ? 'text-cyan-300' : 'text-slate-500'} min-w-0 flex-1 truncate text-right select-all" title="${escapeHTML(ipv6)}">${escapeHTML(ipv6)}</span>
                ${a.public_ipv6 ? `<button type="button" class="agent-icon-btn agent-icon-btn-inline" data-agent-copy-ip="${escapeHTML(a.public_ipv6)}" title="Copy public IPv6" aria-label="Copy public IPv6"><i data-feather="copy" class="w-3 h-3"></i></button>` : ''}
              </div>
              <div class="flex items-center justify-between gap-2 text-[10px] font-mono bg-slate-950/80 px-2.5 py-1.5 rounded-lg border border-slate-800/80">
                <span class="text-slate-500 font-semibold shrink-0">AGENT KEY</span>
                <span class="text-cyan-300 min-w-0 flex-1 truncate text-right">${escapeHTML(a.id)}:••••••</span>
                <button type="button" class="agent-icon-btn agent-icon-btn-inline" data-agent-copy-key="${escapeHTML(a.id)}" title="Copy agent key" aria-label="Copy agent key"><i data-feather="copy" class="w-3 h-3"></i></button>
              </div>
              <div class="agent-meta agent-card-traffic">${traffic} relayed${services.length ? ' · Services: ' + escapeHTML(services.join(', ')) : ' · No service default'}</div>
            </div>

            <div class="agent-card-side">
              <div class="agent-side-stat"><span>Version</span><strong title="${escapeHTML(a.version || '—')}">${escapeHTML(a.version || '—')}</strong></div>
              <div class="agent-side-stat"><span>Last Seen</span><strong>${escapeHTML(agentRelativeAge(a.last_seen))}</strong></div>
              <div class="agent-side-stat"><span>Applied</span><strong title="${escapeHTML(a.catalogue_revision ? 'Applied ' + a.catalogue_revision + ' · ' + agentRelativeAge(a.catalogue_synced_at) : 'No revision reported yet')}">${escapeHTML(agentSyncLabel(a))}</strong></div>
              <div class="agent-side-stat is-domains"><span>Domains</span><strong>${assigned}</strong></div>
            </div>
          </div>
        </div>

        <div class="flex items-center justify-between gap-2 pt-2 border-t border-slate-800/80">
          <button type="button" class="panel-btn panel-btn-sm panel-btn-neutral" data-agent-test="${escapeHTML(a.id)}" title="Open a TCP connection to the relay port">
            <i data-feather="activity" class="w-3.5 h-3.5"></i> Test connection
          </button>
          <span class="shrink-0 px-2 py-0.5 rounded border ${escapeHTML(ping.cls)} text-[10px] font-bold font-mono whitespace-nowrap" title="${escapeHTML(ping.title)}">${escapeHTML(ping.text)}</span>
        </div>
      </div>`;
  }).join('');
  safeFeatherReplace();
}


let serviceRelayPickerTrigger = null;

function renderServiceRelayControl() {
  const input = document.getElementById('service-relay');
  if (!input) return;
  let id = input.value || '';
  const egress = egressForId(id);
  const egressMissing = id.startsWith('egress:') && !egress;
  if (id && !agentForId(id) && !egress && !egressMissing) {
    id = '';
    input.value = '';
  }
  const agent = id ? agentForId(id) : null;
  const name = document.getElementById('service-relay-name');
  const mark = document.getElementById('service-relay-mark');
  const label = egress ? (egress.name || egress.id)
    : (agent ? (agent.name || agent.id) : (egressMissing ? 'Egress (unavailable)' : 'This Server'));
  if (name) name.textContent = label;
  if (mark) {
    if (egress) {
      mark.innerHTML = egressFlagHTML(egress, 'agent-flag agent-flag-sm') ||
        `<i data-feather="${escapeHTML(egress.kind === 'wireguard' ? 'lock' : 'shuffle')}" class="w-3.5 h-3.5"></i>`;
    } else if (egressMissing) {
      mark.innerHTML = '<i data-feather="alert-triangle" class="w-3.5 h-3.5"></i>';
    } else {
      mark.innerHTML = agent
        ? agentFlagHTML(agent, 'agent-flag agent-flag-sm')
        : countryFlagHTML(currentConfig?.server?.country, 'agent-flag agent-flag-sm');
    }
  }
  const trigger = document.getElementById('service-relay-trigger');
  if (trigger) {
    trigger.title = egress
      ? `${egress.name || egress.id} — ${egressKindLabel(egress.kind)}${egress.country ? ' · ' + egress.country : ''}`
      : (agent
        ? `${agent.name || agent.id} — ${agent.country || ''}`
        : `This Server — ${countryName(currentConfig?.server?.country) || 'local'}`);
  }
}

function closeServiceRelayPicker() {
  document.getElementById('service-relay-menu')?.classList.add('hidden');
  if (serviceRelayPickerTrigger) serviceRelayPickerTrigger.setAttribute('aria-expanded', 'false');
  serviceRelayPickerTrigger = null;
}

function openServiceRelayPicker(trigger) {
  const menu = document.getElementById('service-relay-menu');
  if (!menu) return;
  if (!menu.classList.contains('hidden')) { closeServiceRelayPicker(); return; }
  const input = document.getElementById('service-relay');
  const current = input?.value || '';
  const localFlag = countryFlagHTML(currentConfig?.server?.country, 'agent-flag');
  const rows = [
    `<button type="button" class="proxied-bulk-option" role="option" data-service-relay-id="">
      ${localFlag || '<i data-feather="server" class="w-3.5 h-3.5"></i>'}
      <span>This Server${current === '' ? ' · selected' : ''}</span>
    </button>`,
  ];
  const group = (label) => `<div class="proxied-bulk-group">${escapeHTML(label)}</div>`;
  if (agentsCache.length) {
    rows.push(group('Relay Agents'));
    for (const a of agentsCache) {
      rows.push(`<button type="button" class="proxied-bulk-option" role="option" data-service-relay-id="${escapeHTML(a.id)}">
        ${agentFlagHTML(a)}
        <span class="truncate">${escapeHTML(a.name || a.id)} · ${escapeHTML(a.country || '')}${current === a.id ? ' · selected' : ''}</span>
      </button>`);
    }
  }
  for (const kind of ['socks5', 'wireguard']) {
    const items = egressesCache.filter((e) => e.kind === kind);
    if (!items.length) continue;
    rows.push(group(egressKindLabel(kind)));
    for (const e of items) {
      const value = egressValue(e);
      rows.push(`<button type="button" class="proxied-bulk-option" role="option" data-service-relay-id="${escapeHTML(value)}">
        ${egressFlagHTML(e) || `<i data-feather="${escapeHTML(kind === 'wireguard' ? 'lock' : 'shuffle')}" class="w-3.5 h-3.5"></i>`}
        <span class="truncate">${escapeHTML(e.name || e.id)} · ${escapeHTML(e.country || '')}${current === value ? ' · selected' : ''}</span>
      </button>`);
    }
  }
  if (!agentsCache.length && !egressesCache.length) {
    rows.push('<div class="proxied-bulk-option is-disabled" role="option" aria-disabled="true"><span>No egress options yet</span></div>');
  }
  menu.innerHTML = rows.join('');
  menu.classList.remove('hidden');
  serviceRelayPickerTrigger = trigger;
  trigger.setAttribute('aria-expanded', 'true');
  const r = trigger.getBoundingClientRect();
  const mw = menu.offsetWidth;
  const mh = menu.offsetHeight;
  menu.style.left = `${Math.max(8, Math.min(r.left, innerWidth - mw - 8))}px`;
  menu.style.top = `${r.bottom + 4 + mh > innerHeight ? Math.max(8, r.top - mh - 4) : r.bottom + 4}px`;
}

function initAgentCountrySelect() {
  const select = document.getElementById('agent-country');
  if (!select || agentCountrySelectReady) return;
  agentCountrySelectReady = true;
  const pinned = AGENT_PINNED_COUNTRIES.filter((c, i, all) => AGENT_COUNTRY_CODES.includes(c) && all.indexOf(c) === i);
  const rest = AGENT_COUNTRY_CODES.filter((c) => !pinned.includes(c));
  const opts = (codes) => codes.map((c) =>
    `<option value="${escapeHTML(c)}">${escapeHTML(countryName(c))} (${escapeHTML(c.toUpperCase())})</option>`).join('');
  select.innerHTML = `<optgroup label="Frequently used">${opts(pinned)}</optgroup><optgroup label="All countries">${opts(rest)}</optgroup>`;
  select.value = 'de';
  updateAgentCountryFlag();
}

function updateAgentCountryFlag() {
  const select = document.getElementById('agent-country');
  const img = document.getElementById('agent-country-flag');
  if (!select || !img) return;
  if (select.value) paintCountryFlag(img, select.value);
  else img.classList.add('hidden');
}

function openAgentForm(agent) {
  initAgentCountrySelect();
  const form = document.getElementById('agent-form');
  if (!form) return;
  agentEditingId = agent ? agent.id : '';
  renderAgents();
  document.getElementById('agent-editing').value = agent ? agent.id : '';
  document.getElementById('agent-name').value = agent ? (agent.name || '') : '';
  document.getElementById('agent-country').value = agent && agent.country ? agent.country.toLowerCase() : 'de';
  document.getElementById('agent-public-ip').value = agent ? (agent.public_ip || '') : '';
  const ipv6Input = document.getElementById('agent-public-ipv6');
  const ipv6Toggle = document.getElementById('agent-ipv6-enabled');
  if (ipv6Input) {
    ipv6Input.value = agent ? (agent.public_ipv6 || '') : '';
    ipv6Input.disabled = !agent;
  }
  if (ipv6Toggle) {
    ipv6Toggle.checked = !!(agent && agent.ipv6_enabled);
    ipv6Toggle.disabled = !agent;
  }
  const ipv6Note = document.getElementById('agent-ipv6-note');
  if (ipv6Note) {
    if (!agent) {
      ipv6Note.textContent = 'Create the agent first; IPv6 is optional and its first heartbeat fills the address.';
    } else if (!currentConfig?.server?.proxy_ipv6_enabled) {
      ipv6Note.textContent = 'Optional for this existing agent; IPv4 keeps working. Global Proxy IPv6 is off; the agent stops IPv6 on its next heartbeat.';
    } else {
      ipv6Note.textContent = 'Optional for this existing agent; IPv4 keeps working. The agent applies this on its next heartbeat (within 30 seconds).';
    }
  }
  const keyEl = document.getElementById('agent-key');
  if (keyEl) keyEl.value = agent ? 'Stored encrypted — rotate to replace' : 'Generated when you create the agent';
  const rotate = document.getElementById('rotate-agent-key-btn');
  if (rotate) rotate.disabled = !agent;
  document.getElementById('save-agent-btn').textContent = agent ? 'Save changes' : 'Create agent';
  document.getElementById('agent-form-status').textContent = '';
  document.getElementById('agent-token-box')?.classList.add('hidden');
  updateAgentCountryFlag();
  form.classList.remove('hidden');
  form.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
  document.getElementById('agent-name')?.focus();
}

async function saveAgentForm() {
  initAgentCountrySelect();
  const status = document.getElementById('agent-form-status');
  const editing = document.getElementById('agent-editing').value;
  const name = (document.getElementById('agent-name').value || '').trim();
  const country = (document.getElementById('agent-country').value || '').toUpperCase();
  const publicIP = (document.getElementById('agent-public-ip').value || '').trim();
  const publicIPv6 = (document.getElementById('agent-public-ipv6')?.value || '').trim();
  const ipv6Enabled = !!(document.getElementById('agent-ipv6-enabled')?.checked);
  if (!/^[A-Z]{2}$/.test(country)) { if (status) status.textContent = 'Choose a country.'; return; }
  if (status) status.textContent = editing ? 'Saving…' : 'Creating agent…';
  try {
    if (editing) {
      const res = await fetch(api('/api/agents/' + encodeURIComponent(editing)), {
        method: 'PUT',
        headers: { 'Authorization': `Bearer ${authToken}`, 'Content-Type': 'application/json' },
        body: JSON.stringify({
          name,
          country,
          public_ip: publicIP,
          public_ipv6: publicIPv6,
          ipv6_enabled: ipv6Enabled
        })
      });
      if (!res.ok) throw new Error(await errorMessage(res, 'Could not save the agent'));
      showToast('Agent saved.', 'success');
      document.getElementById('agent-form').classList.add('hidden');
      agentEditingId = '';
      await loadAgents();
      return;
    }
    const res = await fetch(api('/api/agents'), {
      method: 'POST',
      headers: { 'Authorization': `Bearer ${authToken}`, 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, country })
    });
    const data = await res.json();
    if (!res.ok) throw new Error(data.error || ('HTTP ' + res.status));
    const keyEl = document.getElementById('agent-key');
    if (keyEl) keyEl.value = data.key || '';
    showAgentKey(data.key, data.agent && data.agent.name);
    document.getElementById('agent-form').classList.add('hidden');
    agentEditingId = '';
    if (status) status.textContent = '';
    showToast('Agent key created.', 'success');
    await loadAgents();
  } catch (e) {
    if (status) status.textContent = e.message;
  }
}

let agentPlatformChoice = 'auto';

function renderAgentCommand() {
  const key = document.getElementById('agent-token-value')?.textContent || '';
  const hub = window.location.origin + ADMIN_BASE;
  const platform = agentPlatformChoice === 'auto' ? '' : `  --platform linux/${agentPlatformChoice}`;
  // Always track the published agent image; both the Docker run command and
  // the Compose stack force a fresh pull so a stale local tag cannot win.
  const image = 'ghcr.io/digitrob/digitdns-agent:latest';
  const command = document.getElementById('agent-token-command');
  if (!command) return;
  const continuation = ' ' + String.fromCharCode(92) + '\n';
  command.textContent = [
    'docker run -d --name digitdns-agent --pull always',
    platform,
    '  --network host --restart unless-stopped',
    '  -v /opt/digitdns-agent:/data',
    `  ${image}`,
    "  -hub '" + hub + "'",
    "  -key '" + key + "'",
    '  -listen :80,:443',
    '  -state /data/agent-state.json',
  ].filter(Boolean).join(continuation);
  renderAgentCompose(image, platform, hub, key);
}

function renderAgentCompose(image, platform, hub, key) {
  const compose = document.getElementById('agent-compose');
  if (!compose) return;
  const q = (value) => JSON.stringify(String(value));
  const lines = [
    'services:',
    '  digitdns-agent:',
    `    image: ${image}`,
    '    pull_policy: always',
    '    container_name: digitdns-agent',
  ];
  if (platform) lines.push(`    platform: linux/${agentPlatformChoice}`);
  lines.push(
    '    network_mode: host',
    '    restart: unless-stopped',
    '    volumes:',
    '      - /opt/digitdns-agent:/data',
    '    command:',
    '      - "-hub"',
    `      - ${q(hub)}`,
    '      - "-key"',
    `      - ${q(key)}`,
    '      - "-listen"',
    `      - ${q(':80,:443')}`,
    '      - "-state"',
    `      - ${q('/data/agent-state.json')}`,
  );
  compose.textContent = lines.join('\n');
}

function setAgentPlatform(platform) {
  agentPlatformChoice = ['amd64', 'arm64'].includes(platform) ? platform : 'auto';
  document.querySelectorAll('[data-agent-platform]').forEach((btn) => {
    const active = btn.dataset.agentPlatform === agentPlatformChoice;
    btn.setAttribute('aria-pressed', active ? 'true' : 'false');
    btn.classList.toggle('is-active', active);
  });
  renderAgentCommand();
}

function showAgentKey(key, name) {
  const label = name ? ` for ${name}` : '';
  document.getElementById('agent-token-value').textContent = key;
  setAgentPlatform('auto');
  document.getElementById('agent-token-box').classList.remove('hidden');
  const box = document.getElementById('agent-token-box');
  const title = box && box.querySelector('p');
  if (title) title.textContent = 'Agent key' + label;
}

async function rotateAgentKey(id) {
  if (!id) return;
  const res = await fetch(api('/api/agents/' + encodeURIComponent(id) + '/rotate-key'), {
    method: 'POST',
    headers: { 'Authorization': `Bearer ${authToken}` }
  });
  if (!res.ok) { showToast(await errorMessage(res, 'Could not rotate the key.'), 'error'); return; }
  const data = await res.json();
  const agent = agentForId(id);
  const keyEl = document.getElementById('agent-key');
  if (keyEl) keyEl.value = data.key || '';
  showAgentKey(data.key, agent && agent.name);
  showToast('Agent key rotated. The old key no longer works.', 'success');
}

async function copyClipboard(value, message) {
  const text = String(value || '');
  try {
    await navigator.clipboard.writeText(text);
    showToast(message, 'success');
    return;
  } catch (e) { /* fall through to the textarea path */ }
  try {
    const area = document.createElement('textarea');
    area.value = text;
    area.setAttribute('readonly', '');
    area.style.position = 'fixed';
    area.style.top = '-1000px';
    area.style.opacity = '0';
    document.body.appendChild(area);
    area.select();
    const ok = document.execCommand('copy');
    area.remove();
    if (ok) { showToast(message, 'success'); return; }
  } catch (e) { /* report below */ }
  showToast('Could not copy to the clipboard.', 'error');
}

async function copyAgentKey(id) {
  const res = await fetch(api('/api/agents/' + encodeURIComponent(id) + '/key'), {
    headers: { 'Authorization': `Bearer ${authToken}` }
  });
  if (!res.ok) {
    showToast(await errorMessage(res, 'The agent key is not available; rotate it to create a copy.'), 'error');
    return;
  }
  const data = await res.json();
  await copyClipboard(data.key, 'Agent key copied.');
}

async function testAgentConnection(id) {
  showToast('Testing connection…', 'info');
  let data = {};
  let status = 0;
  try {
    const res = await fetch(api('/api/agents/' + encodeURIComponent(id) + '/test'), {
      method: 'POST',
      headers: { 'Authorization': `Bearer ${authToken}` }
    });
    status = res.status;
    data = await res.json().catch(() => ({}));
  } catch (e) {
    data = { error: e.message };
  }
  await loadAgents();
  if (status === 200 && data.ok) {
    const a = agentForId(id);
    const ping = Number(data.ping_ms || 0).toFixed(1);
    showToast(`Online — ping ${ping} ms${a ? ' · last heartbeat ' + agentRelativeAge(a.last_seen) : ''}`, 'success');
    return;
  }
  showToast(`Test failed — ${data.error || 'agent unreachable'}`, 'error');
}

async function toggleAgent(id, enable) {
  const res = await fetch(api('/api/agents/' + encodeURIComponent(id) + (enable ? '/enable' : '/disable')), {
    method: 'POST',
    headers: { 'Authorization': `Bearer ${authToken}` }
  });
  if (!res.ok) { showToast(await errorMessage(res, 'Could not update the agent.'), 'error'); return; }
  showToast(enable ? 'Agent enabled.' : 'Agent disabled.', 'success');
  await loadAgents();
}

async function deleteAgent(id) {
  const ok = await confirmAction({
    destructive: true,
    title: 'Delete this relay agent?',
    hint: 'ITS DOMAINS FALL BACK TO THE LOCAL RELAY',
    message: 'The box keeps running but can no longer sync. Domains assigned directly or through a service fall back to this server.',
    confirmText: 'DELETE AGENT',
  });
  if (!ok) return;
  const res = await fetch(api('/api/agents/' + encodeURIComponent(id)), {
    method: 'DELETE',
    headers: { 'Authorization': `Bearer ${authToken}` }
  });
  if (!res.ok) { showToast(await errorMessage(res, 'Could not delete the agent.'), 'error'); return; }
  showToast('Agent deleted.', 'success');
  await loadAgents();
}


function initAgentPage() {
  initAgentCountrySelect();
  initEgressPage();
  const add = document.getElementById('add-agent-btn');
  const cancel = document.getElementById('cancel-agent-btn');
  const save = document.getElementById('save-agent-btn');
  add?.addEventListener('click', () => openAgentForm(null));
  cancel?.addEventListener('click', () => {
    document.getElementById('agent-form')?.classList.add('hidden');
    agentEditingId = '';
    renderAgents();
  });
  save?.addEventListener('click', saveAgentForm);
  document.getElementById('rotate-agent-key-btn')?.addEventListener('click', () => {
    const id = document.getElementById('agent-editing').value;
    if (id) rotateAgentKey(id);
  });
  document.getElementById('agent-country')?.addEventListener('change', updateAgentCountryFlag);
  document.getElementById('copy-agent-token-btn')?.addEventListener('click', () => {
    const value = document.getElementById('agent-token-value')?.textContent || '';
    copyClipboard(value, 'Key copied.');
  });
  document.getElementById('copy-agent-command-btn')?.addEventListener('click', () => {
    const value = document.getElementById('agent-token-command')?.textContent || '';
    copyClipboard(value, 'Docker run command copied.');
  });
  document.getElementById('copy-agent-compose-btn')?.addEventListener('click', () => {
    const value = document.getElementById('agent-compose')?.textContent || '';
    copyClipboard(value, 'Portainer stack copied.');
  });
  document.querySelectorAll('[data-agent-platform]').forEach((btn) => {
    btn.addEventListener('click', () => setAgentPlatform(btn.dataset.agentPlatform));
  });
  document.getElementById('agents-list')?.addEventListener('click', (e) => {
    const refresh = e.target.closest('[data-agent-refresh]');
    if (refresh) { refreshAgentCard(refresh.dataset.agentRefresh, refresh); return; }
    const edit = e.target.closest('[data-agent-edit]');
    if (edit) { openAgentForm(agentForId(edit.dataset.agentEdit)); return; }
    const toggle = e.target.closest('[data-agent-toggle]');
    if (toggle) { toggleAgent(toggle.dataset.agentToggle, toggle.dataset.enable === '1'); return; }
    const test = e.target.closest('[data-agent-test]');
    if (test) { testAgentConnection(test.dataset.agentTest); return; }
    const copyKey = e.target.closest('[data-agent-copy-key]');
    if (copyKey) { copyAgentKey(copyKey.dataset.agentCopyKey); return; }
    const copyIP = e.target.closest('[data-agent-copy-ip]');
    if (copyIP) { copyClipboard(copyIP.dataset.agentCopyIp, 'Public IP copied.'); return; }
    const del = e.target.closest('[data-agent-delete]');
    if (del) deleteAgent(del.dataset.agentDelete);
  });
}


// ── Hub-side egress options ──────────────────────────────────────────────────
// SOCKS5 and WireGuard are where the hub relay dials out. Unlike a relay agent,
// the client never connects to them: DNS still answers with this server, and
// only the origin dial changes. Both kinds share one list, one modal and one
// set of health/traffic badges.

async function loadEgresses() {
  try {
    const res = await fetch(api('/api/egresses'), { headers: { 'Authorization': `Bearer ${authToken}` } });
    if (res.status === 401) return;
    if (!res.ok) throw new Error(await errorMessage(res, 'Could not load egress options'));
    const data = await res.json();
    egressesCache = Array.isArray(data.egresses) ? data.egresses : [];
    renderEgressOptions();
  } catch (e) {
    const socks = document.getElementById('socks5-list');
    const wg = document.getElementById('wireguard-list');
    const msg = `<p class="text-xs text-red-400 py-2">Could not load egress options: ${escapeHTML(e.message)}</p>`;
    if (socks) socks.innerHTML = msg;
    if (wg) wg.innerHTML = msg;
  }
}

function updateEgressSummary() {
  const set = (id, value) => { const el = document.getElementById(id); if (el) el.textContent = value; };
  const enabled = egressesCache.filter((e) => e.enabled);
  const healthy = enabled.filter((e) => e.health === 'online').length;
  const failed = enabled.filter((e) => e.health === 'offline').length;
  const failClosed = enabled.filter((e) => e.fail_closed).length;
  const traffic = egressesCache.reduce((sum, e) => sum + Number(e.bytes_in || 0) + Number(e.bytes_out || 0), 0);
  set('egress-healthy-count', String(healthy));
  set('egress-failed-count', String(failed));
  set('egress-traffic', humanBytes(traffic));
  set('egress-failclosed-count', String(failClosed));
}

function renderEgressOptions() {
  renderEgressList('socks5');
  renderEgressList('wireguard');
  updateEgressSummary();
  renderServiceRelayControl();
  renderBulkEgressOptions();
  safeFeatherReplace();
}

function renderEgressList(kind) {
  const list = document.getElementById(kind === 'socks5' ? 'socks5-list' : 'wireguard-list');
  if (!list) return;
  const items = egressesCache.filter((e) => e.kind === kind);
  if (!items.length) {
    list.innerHTML = kind === 'socks5'
      ? `<div class="egress-empty">
           <i data-feather="shuffle" class="w-5 h-5 mx-auto text-slate-600 mb-2"></i>
           <p>No SOCKS5 egress yet — add one to route selected domains through a proxy.</p>
         </div>`
      : `<div class="egress-empty">
           <i data-feather="lock" class="w-5 h-5 mx-auto text-slate-600 mb-2"></i>
           <p>No WireGuard egress yet — paste a <span class="font-mono">.conf</span> to route selected domains through a tunnel.</p>
         </div>`;
    safeFeatherReplace();
    return;
  }
  list.innerHTML = items.map(renderEgressSubcard).join('');
  safeFeatherReplace();
}

function renderEgressSubcard(e) {
  const status = !e.enabled ? 'disabled' : (e.health === 'online' ? 'online' : (e.health === 'offline' ? 'offline' : 'unknown'));
  const stateClass = status === 'online' ? 'is-online' : (status === 'offline' ? 'is-offline' : 'is-disabled');
  const cardClass = 'egress-subcard' + (status === 'offline' ? ' is-offline' : '') + (status === 'disabled' ? ' is-disabled' : '');
  const latency = Number(e.latency_ms) > 0 ? `${Number(e.latency_ms).toFixed(0)} ms` : '—';
  const traffic = humanBytes(Number(e.bytes_in || 0) + Number(e.bytes_out || 0));
  const detail = e.kind === 'socks5'
    ? (e.address || 'no address')
    : (e.config_set ? 'Configuration saved' : 'No configuration');
  const failClosed = e.fail_closed
    ? `<span class="agent-badge egress-badge-failclosed" title="If this egress is offline, matching domains fail instead of falling back to this server">fail-closed</span>`
    : '';
  const error = e.last_error
    ? `<div class="egress-error" title="${escapeHTML(e.last_error)}">${escapeHTML(e.last_error)}</div>`
    : '';
  const icon = e.kind === 'wireguard' ? 'lock' : 'shuffle';
  return `
    <div class="${escapeHTML(cardClass)}">
      <div class="egress-head">
        <div class="min-w-0 flex-1">
          <div class="egress-title">
            ${egressFlagHTML(e, 'agent-flag')}
            <span class="egress-name">${escapeHTML(e.name || e.id)}</span>
            <span class="agent-badge ${escapeHTML(stateClass)}">${escapeHTML(status)}</span>
            ${failClosed}
            <span class="egress-kind"><i data-feather="${escapeHTML(icon)}" class="w-3 h-3"></i>${escapeHTML(egressKindLabel(e.kind))}</span>
          </div>
          <div class="egress-meta mt-1" title="${escapeHTML(detail)}">${escapeHTML(detail)}${e.country ? ' · ' + escapeHTML(e.country) : ''}</div>
        </div>
        <div class="flex items-center gap-1 shrink-0">
          <button type="button" class="agent-icon-btn" data-egress-toggle="${escapeHTML(e.id)}" data-enable="${escapeHTML(e.enabled ? '0' : '1')}" title="${escapeHTML(e.enabled ? 'Disable egress' : 'Enable egress')}" aria-label="${escapeHTML(e.enabled ? 'Disable egress' : 'Enable egress')}">
            <i data-feather="${escapeHTML(e.enabled ? 'pause' : 'play')}" class="w-3.5 h-3.5"></i>
          </button>
          <button type="button" class="agent-icon-btn agent-icon-btn-danger" data-egress-delete="${escapeHTML(e.id)}" title="Delete egress" aria-label="Delete egress ${escapeHTML(e.name || e.id)}">
            <i data-feather="trash-2" class="w-3.5 h-3.5"></i>
          </button>
        </div>
      </div>
      <div class="egress-stats">
        <div class="egress-stat"><span>Latency</span><strong>${escapeHTML(latency)}</strong></div>
        <div class="egress-stat"><span>Traffic</span><strong>${escapeHTML(traffic)}</strong></div>
        <div class="egress-stat"><span>Last seen</span><strong>${escapeHTML(egressRelativeAge(e.last_seen))}</strong></div>
      </div>
      ${error}
      <div class="egress-actions">
        <button type="button" class="panel-btn panel-btn-sm panel-btn-neutral" data-egress-test="${escapeHTML(e.id)}">
          <i data-feather="activity" class="w-3.5 h-3.5"></i> Test
        </button>
        <button type="button" class="panel-btn panel-btn-sm panel-btn-neutral" data-egress-edit="${escapeHTML(e.id)}">
          <i data-feather="edit-2" class="w-3.5 h-3.5"></i> Edit
        </button>
      </div>
    </div>`;
}

function egressRelativeAge(value) {
  if (!value || String(value).startsWith('0001')) return '—';
  const then = new Date(value).getTime();
  if (!Number.isFinite(then) || then < Date.UTC(2000, 0, 1)) return '—';
  const seconds = Math.max(0, Math.round((Date.now() - then) / 1000));
  if (seconds < 60) return `${seconds}s ago`;
  if (seconds < 3600) return `${Math.round(seconds / 60)}m ago`;
  if (seconds < 86400) return `${Math.round(seconds / 3600)}h ago`;
  return `${Math.round(seconds / 86400)}d ago`;
}

function initEgressCountrySelect() {
  const select = document.getElementById('egress-country');
  if (!select || egressCountrySelectReady) return;
  egressCountrySelectReady = true;
  const pinned = AGENT_PINNED_COUNTRIES.filter((c, i, all) => AGENT_COUNTRY_CODES.includes(c) && all.indexOf(c) === i);
  const rest = AGENT_COUNTRY_CODES.filter((c) => !pinned.includes(c));
  const opts = (codes) => codes.map((c) =>
    `<option value="${escapeHTML(c)}">${escapeHTML(countryName(c))} (${escapeHTML(c.toUpperCase())})</option>`).join('');
  select.innerHTML = `<optgroup label="Frequently used">${opts(pinned)}</optgroup><optgroup label="All countries">${opts(rest)}</optgroup>`;
  select.value = 'nl';
}

function updateEgressCountryFlag() {
  const select = document.getElementById('egress-country');
  const img = document.getElementById('egress-country-flag');
  if (!select || !img) return;
  if (select.value) paintCountryFlag(img, select.value);
  else img.classList.add('hidden');
}

function openEgressForm(kind, item) {
  initEgressCountrySelect();
  const modal = document.getElementById('egress-modal');
  if (!modal) return;
  egressEditingId = item ? item.id : '';
  document.getElementById('egress-id').value = item ? item.id : '';
  document.getElementById('egress-kind').value = kind;
  document.getElementById('egress-modal-title').textContent = item
    ? `Edit ${egressKindLabel(kind)} egress`
    : `Add ${egressKindLabel(kind)} egress`;
  document.getElementById('egress-modal-subtitle').textContent = kind === 'wireguard'
    ? 'Route selected domains through a userspace tunnel'
    : 'Route selected domains through a SOCKS5 proxy';
  document.getElementById('egress-socks5-fields').classList.toggle('hidden', kind !== 'socks5');
  document.getElementById('egress-wireguard-fields').classList.toggle('hidden', kind !== 'wireguard');

  document.getElementById('egress-name').value = item ? (item.name || '') : '';
  const country = document.getElementById('egress-country');
  country.value = item && item.country ? item.country.toLowerCase() : (currentConfig?.server?.country || 'nl').toLowerCase();
  updateEgressCountryFlag();

  const password = document.getElementById('egress-password');
  password.value = '';
  password.placeholder = item && item.password_set ? 'Saved — leave blank to keep' : 'Optional';
  document.getElementById('egress-username').value = item ? (item.username || '') : '';
  document.getElementById('egress-address').value = item ? (item.address || '') : '';
  document.getElementById('egress-resolve').value = item && item.resolve === 'local' ? 'local' : 'remote';

  const config = document.getElementById('egress-config');
  config.value = '';
  const configFile = document.getElementById('egress-config-file');
  if (configFile) configFile.value = '';
  const configFileName = document.getElementById('egress-config-file-name');
  if (configFileName) configFileName.textContent = '';
  config.placeholder = item && item.config_set
    ? 'Saved configuration — paste a new one to replace it'
    : '[Interface]\nPrivateKey = ...\nAddress = 10.9.0.2/32\n\n[Peer]\nPublicKey = ...\nEndpoint = vpn.example.com:51820\nAllowedIPs = 0.0.0.0/0';
  document.getElementById('egress-fail-closed').checked = item ? !!item.fail_closed : true;
  document.getElementById('egress-enabled').checked = item ? !!item.enabled : true;
  document.getElementById('egress-form-status').textContent = '';

  modal.classList.remove('hidden');
  safeFeatherReplace();
  document.getElementById('egress-name')?.focus();
}

function closeEgressForm() {
  document.getElementById('egress-modal')?.classList.add('hidden');
  egressEditingId = '';
}

async function saveEgressForm() {
  const statusEl = document.getElementById('egress-form-status');
  const setStatus = (text, error) => {
    if (!statusEl) return;
    statusEl.textContent = text || '';
    statusEl.className = 'text-[11px] ' + (error ? 'text-red-400' : 'text-slate-400');
  };
  const id = document.getElementById('egress-id').value;
  const kind = document.getElementById('egress-kind').value === 'wireguard' ? 'wireguard' : 'socks5';
  const name = (document.getElementById('egress-name').value || '').trim();
  if (!name) { setStatus('Give the egress a name.', true); return; }

  const payload = {
    kind,
    name,
    country: (document.getElementById('egress-country').value || '').trim(),
    fail_closed: !!document.getElementById('egress-fail-closed').checked,
    enabled: !!document.getElementById('egress-enabled').checked,
  };
  if (kind === 'socks5') {
    const address = (document.getElementById('egress-address').value || '').trim();
    if (!address) { setStatus('Enter the proxy address as host:port.', true); return; }
    payload.address = address;
    payload.username = (document.getElementById('egress-username').value || '').trim();
    payload.resolve = document.getElementById('egress-resolve').value === 'local' ? 'local' : 'remote';
    const password = document.getElementById('egress-password').value || '';
    if (password) payload.password = password;
  } else {
    const config = (document.getElementById('egress-config').value || '').trim();
    if (config) payload.config = config;
    if (!id && !config) { setStatus('Paste a WireGuard configuration.', true); return; }
  }

  const btn = document.getElementById('save-egress-btn');
  if (btn) btn.disabled = true;
  setStatus('Saving…');
  try {
    const res = await fetch(api(id ? '/api/egresses/' + encodeURIComponent(id) : '/api/egresses'), {
      method: id ? 'PUT' : 'POST',
      headers: { 'Authorization': `Bearer ${authToken}`, 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    if (!res.ok) throw new Error(await errorMessage(res, 'Could not save the egress'));
    await loadEgresses();
    closeEgressForm();
    showToast(id ? 'Egress updated.' : 'Egress added.', 'success');
  } catch (e) {
    setStatus(e.message || 'Could not save the egress', true);
  } finally {
    if (btn) btn.disabled = false;
  }
}

async function testEgress(id, button) {
  if (!id) return;
  if (button) { button.disabled = true; button.classList.add('is-refreshing'); }
  showToast('Testing egress…', 'info');
  try {
    const res = await fetch(api('/api/egresses/' + encodeURIComponent(id) + '/test'), {
      method: 'POST',
      headers: { 'Authorization': `Bearer ${authToken}` },
    });
    const data = await res.json().catch(() => ({}));
    await loadEgresses();
    if (res.ok && data.ok) {
      showToast(`Egress online — ${Number(data.latency_ms || 0).toFixed(0)} ms`, 'success');
    } else {
      showToast(`Egress test failed — ${data.error || 'unreachable'}`, 'error');
    }
  } catch (e) {
    showToast(e.message || 'Egress test failed', 'error');
  } finally {
    if (button) { button.disabled = false; button.classList.remove('is-refreshing'); }
    safeFeatherReplace();
  }
}

async function toggleEgress(id, enable) {
  const res = await fetch(api('/api/egresses/' + encodeURIComponent(id) + (enable ? '/enable' : '/disable')), {
    method: 'POST',
    headers: { 'Authorization': `Bearer ${authToken}` },
  });
  if (!res.ok) { showToast(await errorMessage(res, 'Could not update the egress.'), 'error'); return; }
  showToast(enable ? 'Egress enabled.' : 'Egress disabled.', 'success');
  await loadEgresses();
}

async function deleteEgress(id) {
  const item = egressesCache.find((e) => e.id === id);
  const ok = await confirmAction({
    destructive: true,
    title: 'Delete this egress?',
    hint: 'ITS DOMAINS FALL BACK TO THE SERVICE OR LOCAL RELAY',
    message: `${item ? (item.name || item.id) : 'This egress'} stops carrying traffic immediately. Domains assigned to it fall back unless a service default applies.`,
    confirmText: 'DELETE EGRESS',
  });
  if (!ok) return;
  const res = await fetch(api('/api/egresses/' + encodeURIComponent(id)), {
    method: 'DELETE',
    headers: { 'Authorization': `Bearer ${authToken}` },
  });
  if (!res.ok) { showToast(await errorMessage(res, 'Could not delete the egress.'), 'error'); return; }
  showToast('Egress deleted.', 'success');
  await loadEgresses();
}

function handleEgressListClick(e) {
  const test = e.target.closest('[data-egress-test]');
  if (test) { testEgress(test.dataset.egressTest, test); return; }
  const edit = e.target.closest('[data-egress-edit]');
  if (edit) {
    const item = egressesCache.find((x) => x.id === edit.dataset.egressEdit);
    if (item) openEgressForm(item.kind, item);
    return;
  }
  const toggle = e.target.closest('[data-egress-toggle]');
  if (toggle) { toggleEgress(toggle.dataset.egressToggle, toggle.dataset.enable === '1'); return; }
  const del = e.target.closest('[data-egress-delete]');
  if (del) deleteEgress(del.dataset.egressDelete);
}

function initEgressPage() {
  initEgressCountrySelect();
  document.getElementById('add-socks5-btn')?.addEventListener('click', () => openEgressForm('socks5', null));
  document.getElementById('add-wireguard-btn')?.addEventListener('click', () => openEgressForm('wireguard', null));
  document.getElementById('refresh-egresses-btn')?.addEventListener('click', () => loadAgents());
  document.getElementById('close-egress-btn')?.addEventListener('click', closeEgressForm);
  document.getElementById('cancel-egress-btn')?.addEventListener('click', closeEgressForm);
  document.getElementById('save-egress-btn')?.addEventListener('click', saveEgressForm);
  document.getElementById('test-egress-btn')?.addEventListener('click', () => {
    const id = document.getElementById('egress-id').value;
    if (!id) {
      const status = document.getElementById('egress-form-status');
      if (status) { status.textContent = 'Save the egress first, then test it.'; status.className = 'text-[11px] text-slate-400'; }
      return;
    }
    testEgress(id, document.getElementById('test-egress-btn'));
  });
  document.getElementById('egress-country')?.addEventListener('change', updateEgressCountryFlag);
  document.getElementById('egress-config-file')?.addEventListener('change', async (e) => {
    const file = e.target.files && e.target.files[0];
    const nameEl = document.getElementById('egress-config-file-name');
    if (!file) {
      if (nameEl) nameEl.textContent = '';
      return;
    }
    if (file.size > 256 * 1024) {
      showToast('That configuration is too large (limit 256 KiB).', 'error');
      e.target.value = '';
      return;
    }
    try {
      const text = await file.text();
      const area = document.getElementById('egress-config');
      if (area) area.value = text;
      if (nameEl) nameEl.textContent = file.name;
    } catch (err) {
      showToast('Could not read that configuration file.', 'error');
    }
  });
  document.getElementById('socks5-list')?.addEventListener('click', handleEgressListClick);
  document.getElementById('wireguard-list')?.addEventListener('click', handleEgressListClick);
  document.getElementById('egress-modal')?.addEventListener('click', (e) => {
    if (e.target === e.currentTarget) closeEgressForm();
  });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && !document.getElementById('egress-modal')?.classList.contains('hidden')) closeEgressForm();
  });
}

const tabRoutes = {
  'dashboard': '/home',
  'proxied': '/proxied',
  'filters': '/filters',
  'agents': '/agents',
  'clients': '/clients',
  'stream': '/logs',
  'systemlog': '/system-log',
  'api': '/api',
  'rules': '/settings',
  'connect': '/guide'
};

const routeTabs = {
  '/home': 'dashboard',
  '/dashboard': 'dashboard',
  '/panel': 'dashboard',
  '/login': 'dashboard',
  '/proxied': 'proxied',
  '/filters': 'filters',
  '/agents': 'agents',
  '/clients': 'clients',
  '/logs': 'stream',
  '/stream': 'stream',
  '/system-log': 'systemlog',
  '/api': 'api',
  '/settings': 'rules',
  '/guide': 'connect',
  '/connect': 'connect'
};

function switchTab(target, updateUrl = true, loadData = true) {
  activeDashTab = target;
  document.documentElement?.setAttribute('data-boot-tab', target);
  document.querySelectorAll('.sidebar-nav-item, .nav-tab, .agent-nav-link').forEach(t => {
    if (t.dataset.tab === target) {
      t.classList.add('active');
    } else {
      t.classList.remove('active');
    }
  });

  document.querySelectorAll('.mobile-nav-item').forEach(t => {
    if (t.dataset.tab === target) {
      t.classList.add('active', 'text-cyan-400');
      t.classList.remove('text-slate-400');
    } else {
      t.classList.remove('active', 'text-cyan-400');
      t.classList.add('text-slate-400');
    }
  });

  document.querySelectorAll('.tab-content').forEach(c => c.classList.add('hidden'));
  const targetEl = document.getElementById(`tab-${target}`);
  if (targetEl) {
    targetEl.classList.remove('hidden');
    if (loadData) {
      if (target === 'clients') {
        loadClients();
      }
      // Returning to the Logs view repaints the stream from the buffer: the
      // per-query DOM work was skipped while another tab was showing (see
      // pushStreamQuery), so this is where the queued history becomes rows.
      if (target === 'stream') {
        renderQueryStream();
        // If the EventSource has been silent for a while (or a proxy is
        // buffering it), the REST fallback fills the table on tab open.
        if (streamFallbackTimer) streamFallbackPoll();
        else if (Date.now() - streamLastEventAt >= 3000) startStreamFallback();
      }
      // The Proxied list fills as the region scrolls, but a panel that was hidden
      // while it rendered has no height to measure against, so the first batch is
      // topped up once the tab is actually on screen.
      if (target === 'proxied') {
        proxiedFillViewport();
        requestAnimationFrame(sizeProxiedScroll);
        // Agents enrol and go offline outside this tab, so refresh the cache
        // before the relay column is read. Resolution profiles drive the gear
        // dialog, so they are loaded with the list rather than on every boot.
        loadAgents();
        loadResolutionSettings();
      }
      if (target === 'agents') {
        loadAgents();
        startAgentPolling();
      } else {
        stopAgentPolling();
      }
      if (target === 'filters') {
        loadFiltering();
      }
      if (target === 'systemlog') {
        loadSystemLog();
      }
      if (target === 'rules') {
        // Refresh the saved-vs-running comparison every time Settings opens,
        // so a restart performed elsewhere in the panel clears the pending note.
        loadDNSSECSettings();
        loadProxyLeakSettings();
        loadResolutionSettings();
      }
      if (target === 'dashboard') {
        loadHistory();
        // Returning from a slower tab: refresh the tiles now rather than waiting
        // out the idle interval.
        if (statsPollTimer) {
          clearTimeout(statsPollTimer);
          statsPollTimer = null;
        }
        pollStatsOnce();
        requestAnimationFrame(() => { if (historyChart) historyChart.resize(); });
      } else if (historyTimer) {
        clearTimeout(historyTimer);
        historyTimer = null;
      }
    }
  }

  if (updateUrl && tabRoutes[target]) {
    // The path pushed to history lives below the admin namespace, so the URL a
    // bookmark or a reload comes back with is one the server actually serves
    // (BuildHandler strips <admin-path>/dash before matching it here).
    const newPath = DASH_BASE + tabRoutes[target];
    if (window.location.pathname !== newPath) {
      window.history.pushState({ tab: target }, '', newPath);
    }
  }

  safeFeatherReplace();
  document.documentElement?.classList.remove('boot-pending');
}

function handleRouteFromURL(loadData = true) {
  // The admin prefix and the /dash mount point are stripped before the table is
  // consulted, so the same keys work whatever path the install was given. A path
  // that matches nothing falls back to the dashboard, which is the older
  // behaviour for an unknown route and still the least surprising one.
  const stripped = window.location.pathname.toLowerCase().replace(/\/$/, '');
  const withoutBase = ADMIN_BASE && stripped.toLowerCase().startsWith(ADMIN_BASE + '/dash')
    ? stripped.slice((ADMIN_BASE + '/dash').length) || '/'
    : stripped;
  const path = withoutBase || '/home';
  const targetTab = routeTabs[path] || 'dashboard';
  switchTab(targetTab, false, loadData);
}

window.addEventListener('popstate', () => {
  handleRouteFromURL();
});

function renderAPIKeyDisplay() {
  const apiKeyDisp = document.getElementById('api-key-display');
  if (!apiKeyDisp || !currentConfig?.server?.api_key) return;
  const rawKey = currentConfig.server.api_key;
  if (isAPIKeyMasked) {
    apiKeyDisp.innerText = '•'.repeat(36);
  } else {
    apiKeyDisp.innerText = rawKey;
  }
}

function updateCodeSnippets(pubIP, apiKey) {
  // The snippets are generated from the live
  // configuration — real web port and scheme, not hardcoded 8080/http — and
  // they speak the API's actual contract: the create field is `days`
  // (`expires_days` was never read, silently producing lifetime accounts) and
  // the response is the bare client view, whose `token` is what a subscriber's
  // register link is built from. No `success`/`data.register_url` wrapper —
  // the old snippets treated a successful provisioning as a failure.
  const tls = currentConfig?.tls || {};
  const scheme = tls.panel_https ? 'https' : 'http';
  const port = currentConfig?.server?.web_port || window.location.port || 8080;
  // Domain first: with a configured domain the API is addressed by the name the
  // certificate covers, never the bare IP — an integration copy-pasted from
  // here has to keep working the day the server moves. The IP remains the
  // no-domain fallback, matching dashLoginURL on the Go side.
  const host = (tls.domain && String(tls.domain).trim()) || pubIP;
  const isDefaultPort = (scheme === 'https' && Number(port) === 443) || (scheme === 'http' && Number(port) === 80);
  const origin = `${scheme}://${host}${isDefaultPort ? '' : ':' + port}`;
  // v2 is the contract new integrations are documented against; the v1
  // surface was removed at its published sunset (1 July 2026), so the
  // copy-paste examples a reseller starts from use the current contract.
  const apiBase = `${origin}${ADMIN_BASE}/api/v2`;

  const curlEl = document.getElementById('snippet-curl');
  if (curlEl) {
    curlEl.innerText = `# 1. Check Engine Health
curl -s "${apiBase}/status" \\
  -H "X-API-Key: ${apiKey}"

# 2. Create User Account (30 Days). v2 answers 201 with the client object;
#    unknown fields are refused (a typo errors instead of being dropped).
curl -s -X POST "${apiBase}/clients" \\
  -H "X-API-Key: ${apiKey}" \\
  -H "Content-Type: application/json" \\
  -d '{"display_name":"Customer-VIP","validity_days":30}'`;
  }

  const pyEl = document.getElementById('snippet-python');
  if (pyEl) {
    pyEl.innerText = `import requests

API_URL = "${apiBase}"
ORIGIN = "${origin}"
HEADERS = {"X-API-Key": "${apiKey}"}

# Create subscriber account on plan purchase
def create_smartdns_user(username, days=30):
    res = requests.post(f"{API_URL}/clients", headers=HEADERS, json={
        "display_name": username,
        "validity_days": days
    })
    res.raise_for_status()          # 201 with the client object
    client = res.json()
    return f"{ORIGIN}/ip/{client['token']}"`;
  }

  const nodeEl = document.getElementById('snippet-nodejs');
  if (nodeEl) {
    nodeEl.innerText = `const axios = require('axios');

const ORIGIN = '${origin}';
const client = axios.create({
  baseURL: '${apiBase}',
  headers: { 'X-API-Key': '${apiKey}' }
});

// Example Telegram Bot Handler
async function onBuySubscription(ctx, username) {
  // 201 Created answers the client object; its token builds the 1-click
  // register link. ORIGIN is the const above — this runs in Node, where
  // window.location does not exist.
  const { data, status } = await client.post('/clients', { display_name: username, validity_days: 30 });
  if (status === 201) {
    ctx.reply(\`SmartDNS Created! Register IP: \${ORIGIN}/ip/\${data.token}\`);
  }
}`;
  }
}
function initAPIEvents() {
  // Show / Hide Key Toggle
  const toggleKeyBtn = document.getElementById('toggle-api-key-visibility');
  if (toggleKeyBtn) {
    toggleKeyBtn.onclick = () => {
      isAPIKeyMasked = !isAPIKeyMasked;
      renderAPIKeyDisplay();
      toggleKeyBtn.innerHTML = isAPIKeyMasked ? '<i data-feather="eye" class="w-4 h-4"></i>' : '<i data-feather="eye-off" class="w-4 h-4 text-cyan-400"></i>';
      safeFeatherReplace();
    };
  }

  // Copy API Key Button
  const copyKeyBtn = document.getElementById('copy-api-key-btn');
  if (copyKeyBtn) {
    copyKeyBtn.onclick = () => {
      if (currentConfig?.server?.api_key) {
        copyText(currentConfig.server.api_key, copyKeyBtn);
      }
    };
  }

  // Regenerate Key Button
  const regenKeyBtn = document.getElementById('regenerate-api-key-btn');
  if (regenKeyBtn) {
    regenKeyBtn.onclick = async () => {
      const ok = await confirmAction({
        destructive: true,
        title: 'Regenerate Master API Key?',
        hint: 'EVERY INTEGRATION BREAKS IMMEDIATELY',
        message: 'The current key stops working the moment the new one is issued. Every external bot, billing hook and script still holding the old key will start getting 401s until you update it by hand.',
        confirmText: 'REGENERATE KEY',
      });
      if (!ok) return;
      // Rotation is a credential change, so the current password is asked for
      // unconditionally — a hijacked session must not be able to rotate the
      // master key, whether or not 2FA is on.
      const currentPassword = await promptForValue({
        title: 'Current Password',
        message: 'Confirm your current admin password to authorise the rotation.',
        label: 'CURRENT PASSWORD',
        placeholder: 'current password',
        mask: true,
      });
      if (!currentPassword) return;
      // When 2FA is on the rotation also requires a current code (B-03
      // remediation): the server reads both from this JSON body.
      let totpCode = '';
      if (currentConfig?.auth?.totp_enabled) {
        totpCode = await promptForValue({
          title: 'Two-Factor Code',
          message: 'Enter the 6-digit code from your authenticator app to authorise the rotation.',
          placeholder: '6-digit code',
        });
        if (!totpCode) return;
      }
      try {
        const res = await fetch(api('/api/settings/regenerate-api-key'), {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${authToken}`
          },
          body: JSON.stringify({ regenerate: true, current_password: currentPassword, code: (totpCode || '').trim() })
        });
        if (res.ok) {
          const data = await res.json();
          const newKey = data.api_key || data.data?.api_key;
          if (newKey) {
            if (!currentConfig) currentConfig = { server: {} };
            if (!currentConfig.server) currentConfig.server = {};
            currentConfig.server.api_key = newKey;
            isAPIKeyMasked = false; // unmask on fresh regeneration so user can see it
            renderAPIKeyDisplay();
            updateCodeSnippets(currentConfig.server.public_ip || '127.0.0.1', newKey);
            showToast('Master API Key regenerated successfully!', 'success');
          } else {
            showToast('Failed to regenerate API key', 'error');
          }
        } else {
          showToast(await errorMessage(res, 'Failed to regenerate API key'), 'error');
        }
      } catch (err) {
        showToast('Error regenerating API key', 'error');
      }
    };
  }

  // API exposure selector (Localhost / Public / Custom CIDR).
  const saveApiBind = async (targetBind, label) => {
    try {
      const res = await fetch(api('/api/settings'), {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${authToken}`
        },
        body: JSON.stringify({ api_bind: targetBind })
      });
      if (!res.ok) {
        showToast(await errorMessage(res, 'Failed to update API bind configuration'), 'error');
        return false;
      }
      const data = await res.json().catch(() => null);
      if (currentConfig?.server) {
        currentConfig.server.api_bind = (data && data.api_bind) || targetBind;
        // A single save clears the list server-side, so the page must drop it too
        // or it would keep showing rules the gate no longer enforces.
        currentConfig.server.api_bind_list = [];
      }
      renderConfig(currentConfig);
      showToast(label, 'success');
      return true;
    } catch (err) {
      showToast('Error communicating with server', 'error');
      return false;
    }
  };

  // saveApiBindList posts the multi-CIDR rule (v3.1.0). An empty array is a
  // deliberate save meaning "no list, fall back to the single rule", which the
  // server reads back as api_bind = 127.0.0.1.
  const saveApiBindList = async (list, label) => {
    try {
      const res = await fetch(api('/api/settings'), {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${authToken}`
        },
        body: JSON.stringify({ api_bind_list: list })
      });
      if (!res.ok) {
        showToast(await errorMessage(res, 'Failed to update API access list'), 'error');
        return false;
      }
      const data = await res.json().catch(() => null);
      if (currentConfig?.server) {
        currentConfig.server.api_bind_list = (data && data.api_bind_list) || list;
        currentConfig.server.api_bind = (data && data.api_bind) || '127.0.0.1';
      }
      renderConfig(currentConfig);
      showToast(label, 'success');
      return true;
    } catch (err) {
      showToast('Error communicating with server', 'error');
      return false;
    }
  };

  // The list textarea's current entries: one per line, blank lines dropped. The
  // server validates every entry, so nothing is checked here beyond emptiness —
  // a typo comes back as a 400 the operator can see, rather than a silent drop.
  const readBindListInput = () => (document.getElementById('api-bind-list-input')?.value || '')
    .split(/\r?\n/).map((s) => s.trim()).filter(Boolean);

  document.querySelectorAll('.api-bind-opt').forEach(btn => {
    btn.addEventListener('click', async () => {
      const bind = btn.getAttribute('data-bind');
      if (bind === '__custom__') {
        const wrap = document.getElementById('api-bind-custom-wrap');
        wrap?.classList.remove('hidden');
        // Seed the textarea from the single rule in force so converting to a list
        // starts from what the gate actually uses rather than a blank box.
        const listInput = document.getElementById('api-bind-list-input');
        if (listInput && !listInput.value.trim()) {
          const current = currentConfig?.server?.api_bind;
          if (current && current !== '127.0.0.1' && current !== '0.0.0.0') listInput.value = current;
        }
        listInput?.focus();
        return;
      }
      if (bind === '0.0.0.0' && (!currentConfig?.tls?.domain || !currentConfig.tls.domain.trim())) {
        showToast('Cannot expose API to 0.0.0.0: Public API requires an active custom domain and HTTPS configured in Settings to protect credentials.', 'error');
        return;
      }
      await saveApiBind(bind, bind === '0.0.0.0'
        ? 'Public REST API enabled — external callers with a valid key are now accepted (0.0.0.0)'
        : 'REST API restricted to localhost (127.0.0.1)');
    });
  });

  // "Add" appends the single-entry input to the textarea, then clears it. It does
  // not save: the operator assembles the whole list and presses "Apply list", so a
  // half-typed multi-address rule cannot become the live gate.
  const addBindBtn = document.getElementById('api-bind-add');
  if (addBindBtn) {
    addBindBtn.addEventListener('click', () => {
      const entry = (document.getElementById('api-bind-custom-input')?.value || '').trim();
      if (!entry) {
        showToast('Enter an IP address or CIDR block (e.g. 100.64.0.0/10)', 'error');
        return;
      }
      const listInput = document.getElementById('api-bind-list-input');
      if (!listInput) return;
      const entries = readBindListInput();
      if (!entries.includes(entry)) entries.push(entry);
      listInput.value = entries.join('\n');
      const customInput = document.getElementById('api-bind-custom-input');
      if (customInput) customInput.value = '';
      listInput.focus();
    });
  }

  const customSave = document.getElementById('api-bind-custom-save');
  if (customSave) {
    customSave.addEventListener('click', async () => {
      const list = readBindListInput();
      if (!list.length) {
        // An empty list is a valid save: it clears the multi-CIDR rule and drops
        // the gate back to the single api_bind field, which the editor has kept in
        // step. Say what will happen rather than refusing.
        await saveApiBindList([], 'API access list cleared — the single rule applies again');
        return;
      }
      await saveApiBindList(list, `API exposure restricted to ${list.length} rule${list.length === 1 ? '' : 's'}`);
    });
  }

  // Dashboard session idle window (Phase D): load + save through /api/settings.
  const idleInput = document.getElementById('session-idle-minutes');
  const idleSave = document.getElementById('session-idle-save');
  if (idleInput && idleSave) {
    idleSave.addEventListener('click', async () => {
      const raw = Number(idleInput.value);
      if (!Number.isFinite(raw) || raw < 5 || raw > 1440) {
        showToast('Enter a value between 5 and 1440 minutes', 'error');
        return;
      }
      idleSave.disabled = true;
      try {
        const res = await fetch(api('/api/settings'), {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${authToken}`
          },
          body: JSON.stringify({ session_idle_minutes: Math.round(raw) })
        });
        if (!res.ok) {
          showToast(await errorMessage(res, 'Failed to save the session timeout'), 'error');
          return;
        }
        const data = await res.json().catch(() => null);
        if (currentConfig?.server && data?.server) {
          currentConfig.server.session_idle_minutes = data.server.session_idle_minutes;
        }
        if (data?.server?.session_idle_minutes) {
          idleInput.value = data.server.session_idle_minutes;
        }
        showToast('Dashboard session timeout applied', 'success');
      } catch (err) {
        showToast('Error communicating with server', 'error');
      } finally {
        idleSave.disabled = false;
      }
    });
  }

  // Snippet Tabs Switching
  document.querySelectorAll('.api-snippet-tab').forEach(tab => {
    tab.onclick = () => {
      const snip = tab.dataset.snippet;
      document.querySelectorAll('.api-snippet-tab').forEach(t => t.classList.remove('is-active'));
      tab.classList.add('is-active');

      document.querySelectorAll('.api-snippet-content').forEach(c => c.classList.add('hidden'));
      document.getElementById(`snippet-${snip}`)?.classList.remove('hidden');
    };
  });

  window.initSubscriptionSettings?.();
  initAdminPathControls();
  // The 2FA panel lives in its own ES module (js/modules/twofa.js); it
  // registers this hook on window when the module executes.
  window.initTwoFactorControls?.();
}

// =======================================================
// HIDDEN ADMIN PATH DISPLAY + REGENERATION
// =======================================================

function initAdminPathControls() {
  const display = document.getElementById('admin-path-display');
  const regenBtn = document.getElementById('regen-admin-path-btn');
  const warn = document.getElementById('admin-path-warn');

  window.renderAdminPath = function () {
    if (!display) return;
    const p = currentConfig?.server?.admin_path;
    display.textContent = p ? `/${p}/dash/` : '/…';
  };

  if (!regenBtn) return;
  regenBtn.addEventListener('click', () => {
    if (warn) warn.classList.remove('hidden');
    // The confirm dialog is the operator's explicit second step on top of the
    // server's {"confirm":true} gate: two independent confirmations for an
    // action that invalidates every session and bookmark at once.
    if (!window.confirm('Regenerate the hidden admin path? Every bookmark, integration and session under the old path stops working immediately, and you will be signed out.')) {
      return;
    }
    regenBtn.disabled = true;
    (async () => {
      try {
        const res = await fetch(api('/api/settings/regenerate-admin-path'), {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${authToken}` },
          body: JSON.stringify({ confirm: true })
        });
        if (!res.ok) {
          showToast(await errorMessage(res, 'Failed to regenerate the admin path'), 'error');
          return;
        }
        const data = await res.json().catch(() => ({}));
        const newPath = (data && data.admin_path) || '';
        // The old token died with the session wipe; carry nothing into the new
        // namespace but the browser itself.
        localStorage.removeItem('digitdns_token');
        authToken = '';
        if (newPath) {
          showToast('Admin path regenerated. Sign in at the new address.', 'success');
          window.location.href = ADMIN_BASE === `/${newPath}`
            ? `${DASH_BASE}/`
            : `${window.location.origin}/${newPath}/dash/login`;
        } else {
          window.location.reload();
        }
      } catch (err) {
        showToast('Could not reach the server.', 'error');
      } finally {
        regenBtn.disabled = false;
      }
    })();
  });
}


// ── Shell controls: engine switch, sidebar fold, system log ─────────────────
let engineEnabledState = true;

function applyEngineUI(enabled) {
  engineEnabledState = !!enabled;
  for (const [btnID, labelID] of [
    ['engine-toggle-btn', 'engine-label'],
    ['engine-toggle-mobile', 'engine-label-mobile'],
  ]) {
    const btn = document.getElementById(btnID);
    const label = document.getElementById(labelID);
    if (btn) {
      btn.classList.toggle('is-on', engineEnabledState);
      btn.classList.toggle('is-off', !engineEnabledState);
      btn.setAttribute('aria-pressed', engineEnabledState ? 'true' : 'false');
      btn.title = engineEnabledState
        ? 'DNS policy engine is on'
        : 'DNS policy engine is off — all queries resolve directly';
    }
    if (label) label.textContent = engineEnabledState ? 'Engine ON' : 'Engine OFF';
  }
}

async function loadEngineState() {
  try {
    const res = await fetch(api('/api/engine'), { headers: { 'Authorization': `Bearer ${authToken}` } });
    if (!res.ok) return;
    const data = await res.json();
    applyEngineUI(data.enabled !== false);
  } catch (e) { /* the sidebar keeps its last known state */ }
}

async function toggleEngine() {
  const next = !engineEnabledState;
  const btn = document.getElementById('engine-toggle-btn');
  const mobileBtn = document.getElementById('engine-toggle-mobile');
  if (btn) btn.disabled = true;
  if (mobileBtn) mobileBtn.disabled = true;
  try {
    const res = await fetch(api('/api/engine'), {
      method: 'POST',
      headers: { 'Authorization': `Bearer ${authToken}`, 'Content-Type': 'application/json' },
      body: JSON.stringify({ enabled: next }),
    });
    if (!res.ok) throw new Error(await errorMessage(res, 'Could not change the engine state'));
    const data = await res.json();
    applyEngineUI(data.enabled !== false);
    showToast(
      data.enabled === false
        ? 'Engine off — every query now resolves directly, with no rules or proxying.'
        : 'Engine on — DNS rules, proxying and blocking are active.',
      data.enabled === false ? 'info' : 'success',
    );
  } catch (e) {
    showToast(e.message || 'Could not change the engine state', 'error');
  } finally {
    if (btn) btn.disabled = false;
    if (mobileBtn) mobileBtn.disabled = false;
    safeFeatherReplace();
  }
}

function applySidebarCollapsed(collapsed) {
  const on = !!collapsed;
  document.documentElement.classList.toggle('sidebar-collapsed', on);
  try { localStorage.setItem('digitdns_sidebar_collapsed', on ? '1' : '0'); } catch (e) {}
  const btn = document.getElementById('sidebar-collapse-btn');
  if (btn) {
    btn.setAttribute('aria-pressed', on ? 'true' : 'false');
    btn.title = on ? 'Expand sidebar' : 'Collapse sidebar';
    btn.setAttribute('aria-label', btn.title);
  }
}

function initShellControls() {
  let collapsed = false;
  try { collapsed = localStorage.getItem('digitdns_sidebar_collapsed') === '1'; } catch (e) {}
  applySidebarCollapsed(collapsed);
  document.getElementById('engine-toggle-btn')?.addEventListener('click', toggleEngine);
  document.getElementById('engine-toggle-mobile')?.addEventListener('click', toggleEngine);
  document.getElementById('sidebar-collapse-btn')?.addEventListener('click', () => {
    applySidebarCollapsed(!document.documentElement.classList.contains('sidebar-collapsed'));
  });
  loadEngineState();
}

let systemLogText = '';

async function loadSystemLog() {
  const view = document.getElementById('system-log-view');
  if (view) view.textContent = 'Loading…';
  try {
    const res = await fetch(api('/api/system-log?limit=2000'), {
      headers: { 'Authorization': `Bearer ${authToken}` },
    });
    if (!res.ok) throw new Error(await errorMessage(res, 'Could not load the system log'));
    const data = await res.json();
    const lines = Array.isArray(data.lines) ? data.lines : [];
    systemLogText = lines.join('\n');
    if (view) view.textContent = systemLogText || 'No log lines captured yet.';
  } catch (e) {
    systemLogText = '';
    if (view) view.textContent = e.message || 'Could not load the system log.';
  }
}

function copySystemLog() {
  if (!systemLogText) {
    showToast('The system log is empty.', 'info');
    return;
  }
  copyClipboard(systemLogText, 'System log copied.');
}

async function exportSystemLog() {
  try {
    const res = await fetch(api('/api/system-log/export'), {
      headers: { 'Authorization': `Bearer ${authToken}` },
    });
    if (!res.ok) throw new Error(await errorMessage(res, 'Could not export the system log'));
    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    const stamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
    link.download = `digitdns-system-${stamp}.log`;
    document.body.appendChild(link);
    link.click();
    link.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
    showToast('System log exported.', 'success');
  } catch (e) {
    showToast(e.message || 'Could not export the system log', 'error');
  }
}

function initSystemLogPage() {
  document.getElementById('system-log-refresh-btn')?.addEventListener('click', loadSystemLog);
  document.getElementById('system-log-copy-btn')?.addEventListener('click', copySystemLog);
  document.getElementById('system-log-export-btn')?.addEventListener('click', exportSystemLog);
}

function initEventListeners() {
  initUpstreamAddressScroll();
  initUpstreamPresetBrowser();
  initServices();
  initAgentPage();
  initShellControls();
  initSystemLogPage();
  initOnboarding();
  initDiffModal();
  // Tabs Navigation (Sidebar & Mobile Bottom Bar)
  document.querySelectorAll('.sidebar-nav-item, .nav-tab, .mobile-nav-item, .agent-nav-link').forEach(tab => {
    tab.onclick = (e) => {
      e.preventDefault();
      const target = tab.dataset.tab;
      if (target) switchTab(target);
    };
  });

  // Copy Server IP Sidebar Button
  const copyBtn = document.getElementById('copy-ip-btn');
  if (copyBtn) {
    copyBtn.onclick = () => {
      const ip = document.getElementById('header-public-ip')?.innerText || '127.0.0.1';
      copyText(ip, copyBtn);
    };
  }

  // Restart Core Engine Action
  const triggerRestart = async () => {
    const ok = await confirmAction({
      destructive: true,
      title: 'Restart the Core Engine?',
      hint: 'IN-FLIGHT QUERIES AND RELAYS ARE DROPPED',
      message: 'All policies are reloaded from the database. Listeners go down and come back up, so queries and relays in flight at that moment are lost and clients retry.',
      confirmText: 'RESTART ENGINE',
    });
    if (!ok) return;
    try {
      const res = await fetch(api('/api/server/restart'), {
        method: 'POST',
        headers: { 'Authorization': `Bearer ${authToken}` }
      });
      if (res.ok) {
        showToast('Core Engine restarted & rules reloaded!', 'success');
        updateStats();
      } else {
        // A restart that was refused used to look exactly like one that worked:
        // no toast either way.
        showToast(await errorMessage(res, 'Failed to restart engine'), 'error');
      }
    } catch (e) {
      showToast('Failed to restart engine', 'error');
    }
  };

  const restartBtn = document.getElementById('restart-engine-btn');
  if (restartBtn) restartBtn.onclick = triggerRestart;
  const mobileRestartBtn = document.getElementById('mobile-restart-btn');
  if (mobileRestartBtn) mobileRestartBtn.onclick = triggerRestart;

  // Diagnostics Suite Handlers
  const diagModal = document.getElementById('diagnostics-modal');
  const openDiag = () => {
    if (diagModal) {
      diagModal.classList.remove('hidden');
      // Load the operator's server list first, then run the probe against it.
      // The run reads the same record server-side, so an in-flight save is
      // already reflected; the load is what fills the editable list on screen.
      loadDiagTargets().finally(() => runFullDiagnostics());
    }
  };

  const openDiagBtn = document.getElementById('open-diagnostics-btn');
  if (openDiagBtn) openDiagBtn.onclick = openDiag;
  const mobileOpenDiagBtn = document.getElementById('mobile-open-diag');
  if (mobileOpenDiagBtn) mobileOpenDiagBtn.onclick = openDiag;

  const closeDiagBtn = document.getElementById('close-diagnostics-btn');
  if (closeDiagBtn) closeDiagBtn.onclick = () => diagModal?.classList.add('hidden');

  const rerunDiagBtn = document.getElementById('rerun-diagnostics-btn');
  if (rerunDiagBtn) rerunDiagBtn.onclick = () => runFullDiagnostics();

  const diagTargetForm = document.getElementById('diag-target-form');
  if (diagTargetForm) {
    diagTargetForm.onsubmit = (e) => {
      e.preventDefault();
      addDiagTarget();
    };
  }

  // Mobile Logout Button
  const mobileLogout = document.getElementById('mobile-logout-btn');
  if (mobileLogout) {
    mobileLogout.onclick = () => {
      signOut();
    };
  }

  // Add Custom Upstream
  const addUpstreamBtn = document.getElementById('add-upstream-btn');
  if (addUpstreamBtn) {
    addUpstreamBtn.onclick = async () => {
      const input = document.getElementById('new-upstream-input');
      if (input) await addUpstreamAddress(input.value);
    };
  }

  // Issue SSL Certificate
  //
  // bindLEButton wires one "Let's Encrypt" button to the shared ACME flow.
  // All three surfaces (panel, subscription, DoH/DoT) drive the same
  // single-flighted issuance; the purpose field tells the daemon which
  // record and listener the certificate belongs to. While a run is in
  // flight the progress bar polls GET /api/tls/acme/status every 500 ms and
  // renders stage + percentage; on completion the config is re-fetched so
  // every section (and the client cards) reflects the new domain, and on
  // failure the error toast carries the daemon's reason.
  window.bindLEButton = function (btn, domainInput, purpose, progressId) {
    if (!btn) return;
    const barWrap = document.getElementById(progressId);
    const bar = document.getElementById(`${progressId}-bar`);
    const barText = document.getElementById(`${progressId}-text`);

    function showProgress(state) {
      if (barWrap) barWrap.classList.remove('hidden');
      if (bar) bar.style.width = `${Math.max(0, Math.min(100, state.progress || 0))}%`;
      if (barText) {
        const stageLine = state.detail || state.stage || 'working…';
        barText.textContent = `${state.progress || 0}% — ${stageLine}`;
      }
    }
    function hideProgress() {
      if (barWrap) barWrap.classList.add('hidden');
    }

    async function pollUntilDone() {
      for (;;) {
        await new Promise(r => setTimeout(r, 500));
        let state = null;
        try {
          const res = await fetch(api('/api/tls/acme/status'), {
            headers: { 'Authorization': `Bearer ${authToken}` }
          });
          if (res.ok) state = await res.json();
        } catch (e) { /* transient poll failure: keep polling */ }
        if (!state || !state.running) return state;
        showProgress(state);
      }
    }

    btn.onclick = async () => {
      const dom = domainInput ? domainInput.value.trim() : '';
      const email = (document.getElementById('ssl-email-input')?.value || '').trim();
      if (!dom && purpose === 'panel') {
        showToast('Please enter a domain name', 'error');
        return;
      }
      btn.disabled = true;
      try {
        const res = await fetch(api('/api/tls/issue'), {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${authToken}` },
          body: JSON.stringify({ domain: dom, email, purpose })
        });
        const body = await res.json().catch(() => ({}));
        if (!res.ok) {
          showToast((body && body.error) || 'The request was refused.', 'error');
          return;
        }
        if (body.issuing) {
          showToast(body.detail || 'Requesting the certificate…', 'info');
          const final = await pollUntilDone();
          if (final && final.error) {
            showToast('Issuance failed: ' + (final.error || final.last_detail || 'see the daemon log'), 'error');
          } else if (final && final.progress === 100) {
            showToast(body.detail && body.detail.includes('already on this server')
              ? body.detail : 'Certificate issued and applied.', 'success');
          } else {
            showToast(final && final.last_detail ? final.last_detail : 'Issuance finished — check the daemon log.', 'warning');
          }
        } else {
          // Sync apply (cert already on disk) or the clear gesture.
          showToast(body.detail || 'Applied.', 'success');
        }
        // Refresh everything the domain touches: the config (which carries
        // the records and origins), then the sections rendered from it.
        await loadConfig();
        if (typeof window.renderSubscriptionSettings === 'function') window.renderSubscriptionSettings();
        if (typeof loadClients === 'function') loadClients();
        const dotInput = document.getElementById('dot-domain-input');
        if (dotInput && currentConfig && currentConfig.tls) dotInput.value = currentConfig.tls.dot_domain || '';
      } catch (e) {
        showToast('Could not reach the server, so it is not known whether the request was accepted.', 'error');
      } finally {
        hideProgress();
        btn.disabled = false;
      }
    };
  };

  // ── Certificate source manager ───────────────────────────────────────────
  // The advanced path for all three TLS surfaces: Let's Encrypt HTTP-01,
  // wildcard DNS-01 through Cloudflare, or a PEM pair the operator already
  // holds. The one-click Let's Encrypt buttons above stay untouched.
  function certConfigFor(purpose) {
    const all = (currentConfig && currentConfig.certificates) || {};
    return all[purpose] || null;
  }
  function certDefaultDomain(purpose) {
    const tls = (currentConfig && currentConfig.tls) || {};
    const sub = (currentConfig && currentConfig.subscription) || {};
    if (purpose === 'panel') return (tls.domain || '').trim();
    if (purpose === 'subscription') return (sub.domain || '').trim();
    return (tls.dot_domain || tls.domain || '').trim();
  }
  function updateCertFields(fillDefaults) {
    const source = document.getElementById('cert-source')?.value || 'acme-http01';
    const dns = document.getElementById('cert-dns-fields');
    const paths = document.getElementById('cert-path-fields');
    const upload = document.getElementById('cert-upload-fields');
    if (dns) dns.classList.toggle('hidden', source !== 'acme-dns01');
    if (paths) paths.classList.toggle('hidden', source !== 'files-path');
    if (upload) upload.classList.toggle('hidden', source !== 'files-upload');

    const purpose = document.getElementById('cert-purpose')?.value || 'dot';
    const domain = document.getElementById('cert-domain');
    if (fillDefaults && domain && !domain.value.trim()) {
      const fallback = certDefaultDomain(purpose);
      if (fallback) domain.value = fallback;
    }
    const cfg = certConfigFor(purpose);
    if (fillDefaults && cfg) {
      const zone = document.getElementById('cert-dns-zone');
      const certPath = document.getElementById('cert-path');
      const keyPath = document.getElementById('cert-key-path');
      if (zone && !zone.value.trim()) zone.value = cfg.dns_zone || '';
      if (certPath && !certPath.value.trim()) certPath.value = cfg.cert_path || '';
      if (keyPath && !keyPath.value.trim()) keyPath.value = cfg.key_path || '';
      if (domain && !domain.value.trim()) domain.value = cfg.domain || '';
    }
    if (fillDefaults && source === 'acme-dns01') {
      const name = document.getElementById('cert-name');
      if (name && !name.value.trim() && domain && domain.value.trim()) {
        const parts = domain.value.trim().split('.');
        if (parts.length >= 2) name.value = '*.' + parts.slice(-2).join('.');
      }
    }
  }
  function readFileText(file) {
    return new Promise((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => resolve(String(reader.result || ''));
      reader.onerror = () => reject(reader.error || new Error('read failed'));
      reader.readAsText(file);
    });
  }
  async function pollCertRun(statusEl) {
    for (;;) {
      await new Promise(r => setTimeout(r, 700));
      let state = null;
      try {
        const res = await fetch(api('/api/tls/acme/status'), {
          headers: { 'Authorization': `Bearer ${authToken}` }
        });
        if (res.ok) state = await res.json();
      } catch (e) { /* transient poll failure: keep polling */ }
      if (!state || !state.running) return state;
      if (statusEl) statusEl.textContent = `${state.progress || 0}% — ${state.detail || state.stage || 'working…'}`;
    }
  }
  function bindCertificateManager() {
    const purposeEl = document.getElementById('cert-purpose');
    const sourceEl = document.getElementById('cert-source');
    const applyBtn = document.getElementById('cert-apply-btn');
    const statusEl = document.getElementById('cert-apply-status');
    if (!purposeEl || !sourceEl || !applyBtn) return;
    purposeEl.onchange = () => { updateCertFields(true); };
    sourceEl.onchange = () => { updateCertFields(true); };
    updateCertFields(true);
    window.syncCertManagerFields = updateCertFields;

    function bindCertFileLabel(inputId, nameId, fallback) {
      const input = document.getElementById(inputId);
      const name = document.getElementById(nameId);
      if (!input || !name) return;
      input.addEventListener('change', () => {
        name.textContent = input.files && input.files[0] ? input.files[0].name : fallback;
      });
    }
    bindCertFileLabel('cert-file-input', 'cert-file-name', 'fullchain.pem');
    bindCertFileLabel('cert-key-input', 'cert-key-name', 'privkey.pem');

    applyBtn.onclick = async () => {
      const purpose = purposeEl.value || 'dot';
      const source = sourceEl.value || 'acme-http01';
      const domain = (document.getElementById('cert-domain')?.value || '').trim();
      if (!domain) { showToast('Enter the hostname this surface serves.', 'error'); return; }
      applyBtn.disabled = true;
      if (statusEl) statusEl.textContent = 'working…';
      try {
        if (source.startsWith('acme-')) {
          const challenge = source === 'acme-dns01' ? 'dns-01' : 'http-01';
          const certName = challenge === 'dns-01'
            ? ((document.getElementById('cert-name')?.value || '').trim() || domain)
            : domain;
          const body = {
            domain, purpose, challenge, cert_name: certName,
            email: (document.getElementById('ssl-email-input')?.value || '').trim()
          };
          if (challenge === 'dns-01') {
            body.dns_provider = 'cloudflare';
            body.dns_zone = (document.getElementById('cert-dns-zone')?.value || '').trim();
            body.dns_token = (document.getElementById('cert-dns-token')?.value || '').trim();
            if (!body.dns_token) {
              showToast('A Cloudflare API token is required for DNS-01.', 'error');
              if (statusEl) statusEl.textContent = 'token required';
              return;
            }
          }
          const res = await fetch(api('/api/tls/issue'), {
            method: 'POST',
            headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${authToken}` },
            body: JSON.stringify(body)
          });
          const out = await res.json().catch(() => ({}));
          if (!res.ok) {
            showToast(out.error || 'The certificate request was refused.', 'error');
            if (statusEl) statusEl.textContent = out.error || 'refused';
            return;
          }
          if (out.issuing) {
            showToast(out.detail || 'Requesting the certificate…', 'info');
            const final = await pollCertRun(statusEl);
            if (final && final.error) {
              showToast('Issuance failed: ' + final.error, 'error');
              if (statusEl) statusEl.textContent = final.error;
              return;
            }
          }
          showToast(out.detail || 'Certificate issued and applied.', 'success');
        } else {
          const body = { purpose, source: 'files', domain };
          if (source === 'files-upload') {
            const certFile = document.getElementById('cert-file-input')?.files?.[0];
            const keyFile = document.getElementById('cert-key-input')?.files?.[0];
            if (!certFile || !keyFile) {
              showToast('Choose both the certificate and the private key.', 'error');
              if (statusEl) statusEl.textContent = 'choose both files';
              return;
            }
            body.cert_pem = await readFileText(certFile);
            body.key_pem = await readFileText(keyFile);
            body.allow_self_signed = !!document.getElementById('cert-upload-self-signed')?.checked;
          } else {
            body.cert_path = (document.getElementById('cert-path')?.value || '').trim();
            body.key_path = (document.getElementById('cert-key-path')?.value || '').trim();
            body.allow_self_signed = !!document.getElementById('cert-allow-self-signed')?.checked;
            if (!body.cert_path || !body.key_path) {
              showToast('Enter both the certificate and key paths.', 'error');
              if (statusEl) statusEl.textContent = 'paths required';
              return;
            }
          }
          const res = await fetch(api('/api/tls/cert'), {
            method: 'POST',
            headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${authToken}` },
            body: JSON.stringify(body)
          });
          const out = await res.json().catch(() => ({}));
          if (!res.ok) {
            showToast(out.error || 'The certificate was refused.', 'error');
            if (statusEl) statusEl.textContent = out.error || 'refused';
            return;
          }
          showToast(out.detail || 'Certificate installed and applied.', 'success');
        }
        if (statusEl) statusEl.textContent = 'applied';
        await loadConfig();
        renderTLSCertDetails(currentConfig);
        if (typeof window.renderSubscriptionSettings === 'function') window.renderSubscriptionSettings();
        updateCertFields(false);
      } catch (e) {
        showToast('Could not reach the server, so it is not known whether the request was accepted.', 'error');
      } finally {
        applyBtn.disabled = false;
      }
    };
  }

  // The panel + DoH/DoT buttons live in app.js's own settings scope; the
  // subscription button binds itself from its module (twofa.js).
  window.bindLEButton(document.getElementById('issue-ssl-btn'), document.getElementById('ssl-domain-input'), 'panel', 'panel-le-progress');
  window.bindLEButton(document.getElementById('issue-dot-ssl-btn'), document.getElementById('dot-domain-input'), 'dot', 'dot-le-progress');
  bindCertificateManager();
  const dotDomainInput = document.getElementById('dot-domain-input');
  if (dotDomainInput && currentConfig && currentConfig.tls) dotDomainInput.value = currentConfig.tls.dot_domain || '';

  // The DoH/DoT auto-renew toggle saves the moment it is clicked — the card
  // has no Save button, and the pattern matches the session-idle toggle: a
  // preference, not a form field. A failed save flips the checkbox back so
  // the UI never claims a state the daemon refused.
  // Certificate auto-renew toggles: DoH/DoT and the admin panel. Both save the
  // moment they are clicked; a failed save flips the checkbox back so the UI
  // never claims a state the daemon refused.
  function bindTlsAutoRenew(inputId, purpose, label) {
    const input = document.getElementById(inputId);
    if (!input) return;
    input.onchange = async () => {
      const enabled = input.checked;
      input.disabled = true;
      try {
        const res = await fetch(api('/api/tls/auto-renew'), {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${authToken}` },
          body: JSON.stringify({ enabled, purpose })
        });
        const body = await res.json().catch(() => ({}));
        if (!res.ok) {
          input.checked = !enabled;
          input.disabled = false;
          showToast((body && body.error) || 'The auto-renew preference could not be saved.', 'error');
          return;
        }
        showToast(enabled ? `${label} auto-renew enabled.` : `${label} auto-renew disabled.`, 'success');
        await loadConfig();
        if (typeof window.renderSubscriptionSettings === 'function') window.renderSubscriptionSettings();
      } catch (e) {
        input.checked = !enabled;
        input.disabled = false;
        showToast('Could not reach the server.', 'error');
      }
    };
  }
  bindTlsAutoRenew('dot-auto-renew-input', 'dot', 'DoH/DoT');
  bindTlsAutoRenew('panel-auto-renew-input', 'panel', 'Panel');

  // Flush Cache
  const flushBtn = document.getElementById('flush-cache-btn');
  if (flushBtn) {
    flushBtn.onclick = async () => {
      try {
        await fetch(api('/api/cache/flush'), {
          method: 'POST',
          headers: { 'Authorization': `Bearer ${authToken}` }
        });
        showToast('DNS cache flushed successfully!', 'success');
        updateStats();
      } catch (e) {
        showToast('Failed to flush cache', 'error');
      }
    };
  }

  // Run Benchmark. The server probes every upstream in the background and answers
  // 202 immediately, so this cannot claim "completed" — it reports that the run
  // started, then refreshes the stats twice as the probes land (they are bounded by
  // the upstream timeout, and they all run in parallel).
  const benchBtn = document.getElementById('run-benchmark-btn');
  if (benchBtn) {
    benchBtn.onclick = async () => {
      benchBtn.disabled = true;
      try {
        const res = await fetch(api('/api/benchmark'), {
          method: 'POST',
          headers: { 'Authorization': `Bearer ${authToken}` }
        });
        if (res.status === 409) {
          showToast('A benchmark is already running', 'info');
          return;
        }
        if (!res.ok) {
          showToast('Failed to start benchmark', 'error');
          return;
        }
        showToast('Benchmark started — upstream latencies refresh as probes land', 'info');
        setTimeout(updateStats, 1500);
        setTimeout(updateStats, 4000);
      } catch (e) {
        showToast('Failed to start benchmark', 'error');
      } finally {
        setTimeout(() => { benchBtn.disabled = false; }, 4000);
      }
    };
  }

  // Custom policy groups
  document.getElementById('add-custom-group-btn')?.addEventListener('click', () => openCustomGroupEditor(null));
  document.getElementById('cancel-custom-group-btn')?.addEventListener('click', () => {
    document.getElementById('custom-group-editor')?.classList.add('hidden');
  });
  document.getElementById('save-custom-group-btn')?.addEventListener('click', saveCustomGroup);
  // Group icon: choose a file → hold it as pending; Save flushes it to
  // the server (for a new group this is the only moment the id exists).
  // Remove clears it server-side for an existing group.
  document.getElementById('custom-group-icon-file')?.addEventListener('change', (e) => {
    const file = e.target.files && e.target.files[0];
    if (!file) return;
    if (!/\.(svg|png)$/i.test(file.name) && !/^image\/(svg\+xml|png)$/.test(file.type)) {
      showToast('Pick an SVG or PNG file', 'error');
      e.target.value = '';
      return;
    }
    pendingGroupIconFile = file;
    const id = document.getElementById('custom-group-id')?.value.trim();
    refreshGroupIconPreview(id || null);
    e.target.value = '';
  });
  document.getElementById('custom-group-icon-remove')?.addEventListener('click', async () => {
    const id = document.getElementById('custom-group-id')?.value.trim();
    pendingGroupIconFile = null;
    if (id) {
      try {
        await fetch(api(`/api/custom-groups/${encodeURIComponent(id)}/icon`), {
          method: 'DELETE',
          headers: { 'Authorization': 'Bearer ' + authToken }
        });
      } catch (err) { /* the preview reset below still reflects local state */ }
    }
    refreshGroupIconPreview(id || null);
  });
  document.getElementById('custom-groups-list')?.addEventListener('click', (e) => {
    const editBtn = e.target.closest('.edit-group-btn');
    if (editBtn) {
      const g = customGroupsCache.find((x) => x.id === editBtn.dataset.id);
      if (g) openCustomGroupEditor(g);
      return;
    }
    const delBtn = e.target.closest('.delete-group-btn');
    if (delBtn) deleteCustomGroup(delBtn.dataset.id);
  });

  // Bulk import: one modal serves both lists. The target is chosen when the modal
  // opens, not when Import is pressed, so the heading and the button label can name
  // exactly what is about to change.
  let bulkImportTarget = null;
  const bulkImportModal = document.getElementById('bulk-import-modal');

  function setBulkImportStatus(msg, kind) {
    const el = document.getElementById('bulk-import-status');
    if (!el) return;
    el.textContent = msg || '';
    el.className = 'text-[11px] ' + (kind === 'error' ? 'text-red-400' : kind === 'ok' ? 'text-emerald-400' : 'text-slate-400');
  }

  function closeBulkImport() {
    if (bulkImportModal) bulkImportModal.classList.add('hidden');
  }

  function openBulkImport(target) {
    bulkImportTarget = target;
    const proxied = target === 'proxied';
    const records = target === 'records';
    const titleEl = document.getElementById('bulk-import-title');
    if (titleEl) titleEl.textContent = records ? 'Bulk Import DNS Rewrites' : proxied ? 'Bulk Import Proxied Domains' : 'Bulk Import Blocked Domains';
    const subtitleEl = document.getElementById('bulk-import-subtitle');
    if (subtitleEl) subtitleEl.textContent = records
      ? 'Each line maps one hostname to one IPv4 or IPv6 address.'
      : proxied
        ? 'Each name listed will resolve to this server through the SNI proxy.'
        : 'Each name listed will be sinkholed to 0.0.0.0.';
    const applyBtn = document.getElementById('bulk-import-apply-btn');
    if (applyBtn) applyBtn.textContent = records ? 'Import Rewrites' : proxied ? 'Import Proxied' : 'Import Blocked';
    const labelEl = document.getElementById('bulk-import-textarea-label');
    if (labelEl) labelEl.textContent = records ? 'Rewrites' : 'Domains';
    const helpEl = document.getElementById('bulk-import-help');
    if (helpEl) {
      helpEl.innerHTML = records
        ? 'One record per line as <code>hostname address</code> — separated by spaces, commas, semicolons or <code>=</code>. Lines starting with <code>#</code> are ignored. IPv4 and IPv6 are both accepted.'
        : 'One name per line, or separated by commas, semicolons or spaces. Lines starting with <code>#</code> are ignored. A leading <code>*.</code> covers every subdomain.';
    }
    const ta = document.getElementById('bulk-import-textarea');
    if (ta) {
      ta.value = '';
      ta.placeholder = records ? 'host.example 203.0.113.9\ncdn.example.net, 2001:db8::1' : 'example.com\n*.sub.example.com\nanother.example.net, third.example.org';
    }
    const fileEl = document.getElementById('bulk-import-file');
    if (fileEl) fileEl.value = '';
    setBulkImportStatus('');
    if (bulkImportModal) bulkImportModal.classList.remove('hidden');
    if (ta) ta.focus();
  }

  document.getElementById('geoip-update-btn')?.addEventListener('click', updateGeoipDatabase);
  document.getElementById('save-dnssec-btn')?.addEventListener('click', saveDNSSECSettings);
  document.getElementById('proxy-leak-enabled-input')?.addEventListener('change', async (e) => {
    const want = e.target.checked;
    if (!want) {
      const ok = await confirmAction({
        destructive: true,
        title: 'Turn off proxy leak protection?',
        hint: 'PROXIED DOMAINS MAY BYPASS THE RELAY',
        message: 'Browsers can discover QUIC and the origin’s real IPv4/IPv6 addresses through HTTPS/SVCB. Only turn this off if you know what you are doing.',
        confirmText: 'TURN OFF',
      });
      if (!ok) {
        e.target.checked = true;
        return;
      }
    }
    const saved = await saveProxyLeakSettings(
      want,
      want ? 'Proxy leak protection enabled' : 'Proxy leak protection disabled'
    );
    if (!saved) e.target.checked = !want;
  });
  initProxyIPv6Events();
  document.getElementById('resolution-strategy-chips')?.addEventListener('click', (e) => {
    const button = e.target.closest('[data-strategy]');
    if (!button) return;
    saveResolutionSettings({ default_strategy: button.dataset.strategy }, 'Strategy saved');
  });
  document.getElementById('resolution-interval-chips')?.addEventListener('click', (e) => {
    const button = e.target.closest('[data-interval]');
    if (!button || button.disabled) return;
    saveResolutionSettings({ adaptive_interval_minutes: Number(button.dataset.interval) }, 'Adaptive interval saved');
  });
  document.getElementById('resolution-transport-chips')?.addEventListener('click', (e) => {
    const button = e.target.closest('[data-transport]');
    if (!button || button.disabled) return;
    const current = normalizeTransports(resolutionSettings.transports, resolutionSettings.transport);
    const protocol = button.dataset.transport;
    const next = current.includes(protocol)
      ? current.filter(value => value !== protocol)
      : [...current, protocol];
    if (!next.length) {
      showToast('Keep at least one transport enabled', 'warning');
      return;
    }
    saveResolutionSettings({ transports: normalizeTransports(next, 'any') }, 'Transports saved');
  });
  document.getElementById('resolution-adaptive-run')?.addEventListener('click', runAdaptiveNow);
  document.getElementById('resolution-single-upstream')?.addEventListener('click', (e) => {
    e.stopPropagation();
    const menu = document.getElementById('resolution-single-menu');
    setSingleUpstreamMenu(!!menu && menu.classList.contains('hidden'));
  });
  document.getElementById('resolution-single-switch')?.addEventListener('change', async (e) => {
    const available = availableUpstreams();
    if (e.target.checked) {
      const remembered = rememberedSingleServer();
      const chosen = available.includes(remembered) ? remembered : available[0];
      if (!chosen) {
        e.target.checked = false;
        showToast('Add an upstream resolver first', 'warning');
        return;
      }
      rememberSingleServer(chosen);
      const ok = await saveResolutionSettings({ default_upstream: chosen }, 'Single server enabled');
      if (!ok) e.target.checked = false;
    } else {
      const ok = await saveResolutionSettings({ default_upstream: '' }, 'Pool mode enabled');
      if (!ok) e.target.checked = true;
    }
  });
  document.getElementById('resolution-single-menu')?.addEventListener('click', (e) => {
    const item = e.target.closest('[data-upstream]');
    if (!item) return;
    closeSingleUpstreamMenu();
    rememberSingleServer(item.dataset.upstream);
    saveResolutionSettings({ default_upstream: item.dataset.upstream }, 'Single server saved');
  });
  document.addEventListener('click', (e) => {
    if (!e.target.closest('.resolver-single-control')) closeSingleUpstreamMenu();
  });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') closeSingleUpstreamMenu();
  });
  document.getElementById('server-update-check-btn')?.addEventListener('click', checkServerUpdate);
  document.getElementById('server-update-apply-btn')?.addEventListener('click', applyServerUpdate);
  document.getElementById('server-update-copy')?.addEventListener('click', async () => {
    const command = 'docker compose pull && docker compose up -d';
    try {
      await navigator.clipboard.writeText(command);
      showToast('Update command copied.', 'success');
    } catch (e) {
      showToast('Could not copy the command.', 'error');
    }
  });
  document.querySelectorAll('input[name="server-update-mode"]').forEach((el) => {
    el.addEventListener('change', () => setContainerUpdateMode(el.value));
  });

  document.getElementById('bulk-proxied-btn')?.addEventListener('click', () => openBulkImport('proxied'));
  document.getElementById('bulk-blocked-btn')?.addEventListener('click', () => openBulkImport('blocked'));
  document.getElementById('bulk-records-btn')?.addEventListener('click', () => openBulkImport('records'));

  // Proxied-list sort and batch controls, and the sentinel that appends the next
  // batch as the region scrolls. Every one of these re-renders through
  // renderProxiedList, which reads proxiedSort / proxiedVisible rather than
  // taking arguments, so the controls only have to update the state.
  document.getElementById('proxied-sort')?.addEventListener('change', (e) => {
    proxiedSort = e.target.value || 'added-desc';
    proxiedResetSlice();
  });
  document.getElementById('proxied-load-more')?.addEventListener('click', proxiedShowMore);
  setupProxiedObserver();

  // Both boxes above the table feed the same filter: the search box is the obvious
  // one, and the Add box narrows the list too so a name that is already listed shows
  // up (and its autocomplete entry is visible) before Add is pressed.
  ['proxied-search', 'new-proxied-input'].forEach((id) => {
    document.getElementById(id)?.addEventListener('input', proxiedFilterChanged);
  });

  // Multi-select for the bulk actions. The row checkboxes are delegated on the table
  // body because renderProxiedList replaces its innerHTML on every save; the header
  // checkbox is static, so it is bound directly. The select-all set covers the rows the
  // filter currently shows, not the whole ordered list: with a search typed, ticking the
  // header must not silently capture thousands of rows the operator cannot see.
  document.getElementById('custom-proxied-rows')?.addEventListener('change', (e) => {
    const box = e.target.closest('.proxied-select-check');
    if (!box) return;
    if (box.checked) proxiedSelected.add(box.dataset.val);
    else proxiedSelected.delete(box.dataset.val);
    updateProxiedBulkBar();
  });
  document.getElementById('proxied-select-all')?.addEventListener('change', (e) => {
    const on = e.target.checked;
    proxiedSelected = on
      ? new Set(proxiedFilteredRows().map((r) => r.domain))
      : new Set();
    document.querySelectorAll('.proxied-select-check').forEach((b) => { b.checked = on; });
    updateProxiedBulkBar();
  });
  document.getElementById('proxied-bulk-pause')?.addEventListener('click', () => proxiedBulkSetEnabled(false));
  document.getElementById('proxied-bulk-resume')?.addEventListener('click', () => proxiedBulkSetEnabled(true));
  document.getElementById('proxied-bulk-remove')?.addEventListener('click', proxiedBulkRemove);
  document.getElementById('proxied-bulk-service')?.addEventListener('click', () => {
    const btn = document.getElementById('proxied-bulk-service');
    if (!btn || btn.disabled) return;
    const menu = document.getElementById('proxied-bulk-service-menu');
    if (!menu) return;
    const willOpen = menu.classList.contains('hidden');
    menu.classList.toggle('hidden');
    btn.setAttribute('aria-expanded', willOpen ? 'true' : 'false');
  });
  document.getElementById('proxied-bulk-service-menu')?.addEventListener('click', (e) => {
    const row = e.target.closest('[data-bulk-service]');
    if (!row) return;
    assignServiceToSelected(row.dataset.bulkService);
  });
  document.getElementById('proxied-bulk-egress')?.addEventListener('click', () => {
    const btn = document.getElementById('proxied-bulk-egress');
    if (!btn || btn.disabled) return;
    const menu = document.getElementById('proxied-bulk-egress-menu');
    if (!menu) return;
    const willOpen = menu.classList.contains('hidden');
    renderBulkEgressOptions();
    menu.classList.toggle('hidden');
    btn.setAttribute('aria-expanded', willOpen ? 'true' : 'false');
  });
  document.getElementById('proxied-bulk-egress-menu')?.addEventListener('click', (e) => {
    const row = e.target.closest('[data-bulk-egress]');
    if (!row) return;
    assignEgressToSelected(row.dataset.bulkEgress);
  });
  document.getElementById('service-picker-menu')?.addEventListener('click', (e) => {
    const row = e.target.closest('[data-service-id]');
    if (!row) return;
    const domain = servicePickerDomain;
    const id = row.dataset.serviceId === '__none__' ? '' : row.dataset.serviceId;
    closeServicePicker();
    if (domain) assignService(domain, id);
  });
  document.getElementById('relay-picker-menu')?.addEventListener('click', (e) => {
    const row = e.target.closest('[data-relay-id]');
    if (!row) return;
    const domain = relayPickerDomain;
    const id = row.dataset.relayId;
    if (domain) assignRelayToDomain(domain, id);
  });
  document.addEventListener('click', (e) => {
    if (!e.target.closest('.proxied-bulk-service-wrap')) {
      closeBulkServiceMenu();
      closeBulkEgressMenu();
    }
  });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') {
      closeBulkServiceMenu();
      closeBulkEgressMenu();
      closeServicePicker();
      closeRelayPicker();
    }
  });
  document.addEventListener('click', (e) => {
    if (!e.target.closest('#service-picker-menu') && !e.target.closest('.proxied-service-trigger')) {
      closeServicePicker();
    }
    if (!e.target.closest('#relay-picker-menu') && !e.target.closest('.proxied-relay-trigger')) {
      closeRelayPicker();
    }
  });
  window.addEventListener('scroll', closeServicePicker, true);
  window.addEventListener('scroll', closeRelayPicker, true);
  window.addEventListener('resize', closeServicePicker);
  window.addEventListener('resize', closeRelayPicker);
  window.addEventListener('resize', sizeProxiedScroll);

  // The × names itself through data-modal-close so Escape and a backdrop click reach
  // it; both still need their own handler or those paths click a dead button.
  document.getElementById('bulk-import-cancel-btn')?.addEventListener('click', closeBulkImport);
  document.getElementById('bulk-import-cancel-btn-bottom')?.addEventListener('click', closeBulkImport);

  document.getElementById('bulk-import-file')?.addEventListener('change', (e) => {
    const file = e.target.files && e.target.files[0];
    if (!file) return;
    // The server caps the rules payload anyway; refuse here so the operator is told
    // before a large paste is silently truncated on the wire.
    if (file.size > 1024 * 1024) {
      setBulkImportStatus('That file is larger than 1 MB — trim it or paste the list instead.', 'error');
      return;
    }
    const reader = new FileReader();
    reader.onload = () => {
      const ta = document.getElementById('bulk-import-textarea');
      if (ta) ta.value = String(reader.result || '');
      setBulkImportStatus('');
    };
    reader.onerror = () => setBulkImportStatus('Could not read that file.', 'error');
    reader.readAsText(file);
  });

  document.getElementById('bulk-import-apply-btn')?.addEventListener('click', () => {
    if (!currentConfig || !bulkImportTarget) return;
    const ta = document.getElementById('bulk-import-textarea');

    if (bulkImportTarget === 'records') {
      if (!currentConfig.rules.custom_records) currentConfig.rules.custom_records = {};
      const { accepted, updated, unchanged, invalid } = parseRecordList(ta ? ta.value : '', currentConfig.rules.custom_records);
      for (const r of accepted) currentConfig.rules.custom_records[r.domain] = r.ip;
      for (const r of updated) currentConfig.rules.custom_records[r.domain] = r.ip;
      if (accepted.length === 0 && updated.length === 0) {
        setBulkImportStatus(invalid.length ? `Nothing to import — ${invalid.length} invalid line(s).` : 'Nothing to import.', invalid.length ? 'error' : '');
        return;
      }
      saveRules();
      renderConfig(currentConfig);
      const parts = [`${accepted.length} added`];
      if (updated.length) parts.push(`${updated.length} updated`);
      if (unchanged.length) parts.push(`${unchanged.length} unchanged`);
      if (invalid.length) parts.push(`${invalid.length} invalid`);
      const summary = `DNS rewrites: ${parts.join(', ')}.`;
      if (invalid.length) {
        // Keep the modal open so the operator can see the offending lines, but the
        // good ones are already saved.
        setBulkImportStatus(summary, 'error');
        return;
      }
      closeBulkImport();
      showToast(`Imported ${accepted.length + updated.length} DNS rewrite(s)`, 'success');
      return;
    }

    const key = bulkImportTarget === 'proxied' ? 'custom_proxied' : 'custom_blocked';
    const active = Array.isArray(currentConfig.rules[key]) ? currentConfig.rules[key] : [];
    // On the proxied page a switched-off domain is still "already listed", so the
    // duplicate check sees both lists — importing it again must not silently
    // re-enable a domain the operator turned off.
    const known = bulkImportTarget === 'proxied'
      ? active.concat(Array.isArray(currentConfig.rules.custom_proxied_disabled) ? currentConfig.rules.custom_proxied_disabled : [])
      : active;
    const { accepted, duplicates, invalid } = parseDomainList(ta ? ta.value : '', known);

    const parts = [`${accepted.length} added`];
    if (duplicates.length) parts.push(`${duplicates.length} already listed`);
    if (invalid.length) parts.push(`${invalid.length} invalid`);
    const summary = parts.join(', ');

    if (accepted.length === 0) {
      setBulkImportStatus(`Nothing to import — ${summary}.`, 'error');
      return;
    }

    // Imported domains always land enabled; the operator can switch them off after.
    currentConfig.rules[key] = active.concat(accepted);
    saveRules();
    renderConfig(currentConfig);

    if (invalid.length || duplicates.length) {
      setBulkImportStatus(`Imported (${summary}). Skipped entries are ignored, not added.`, 'ok');
      return;
    }
    closeBulkImport();
    showToast(`Imported ${accepted.length} domain(s)`, 'success');
  });

  // Add Custom Proxied
  document.getElementById('add-proxied-btn')?.addEventListener('click', () => {
    const input = document.getElementById('new-proxied-input');
    const val = input ? input.value.trim().toLowerCase() : '';
    if (!val || !currentConfig) return;
    if (!currentConfig.rules.custom_proxied_disabled) currentConfig.rules.custom_proxied_disabled = [];
    // Re-adding a domain that sits in the disabled list turns it back on, rather
    // than leaving the same name in both lists.
    const wasDisabled = currentConfig.rules.custom_proxied_disabled.includes(val);
    currentConfig.rules.custom_proxied_disabled = currentConfig.rules.custom_proxied_disabled.filter(x => x !== val);
    const alreadyActive = currentConfig.rules.custom_proxied.includes(val);
    if (!alreadyActive) currentConfig.rules.custom_proxied.push(val);
    if (!alreadyActive || wasDisabled) {
      if (input) input.value = '';
      saveRules();
      renderConfig(currentConfig);
      return;
    }
    // Nothing changed: the domain is already listed and already on. Say so instead of
    // swallowing the click — the autocomplete narrows the list to it but a list of one
    // row is not an explanation on its own. The box keeps its text so the filtered row
    // stays on screen.
    showToast(`${val} is already listed`, 'info');
  });

  // Add Custom Blocked
  document.getElementById('add-blocked-btn')?.addEventListener('click', () => {
    const input = document.getElementById('new-blocked-input');
    const val = input ? input.value.trim().toLowerCase() : '';
    if (!val || !currentConfig) return;
    if (!currentConfig.rules.custom_blocked.includes(val)) {
      currentConfig.rules.custom_blocked.push(val);
      if (input) input.value = '';
      saveRules();
      renderConfig(currentConfig);
    }
  });

  // Add DoH Token
  document.getElementById('add-token-btn')?.addEventListener('click', async () => {
    const input = document.getElementById('new-token-input');
    const val = input ? input.value.trim() : '';
    if (!val || !currentConfig) return;
    if (!currentConfig.access.doh_tokens.includes(val)) {
      currentConfig.access.doh_tokens.push(val);
      if (input) input.value = '';
      await fetch(api('/api/config/access'), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${authToken}` },
        body: JSON.stringify(currentConfig.access)
      });
      showToast('DoH Token added!', 'success');
      renderConfig(currentConfig);
    }
  });

  // Client-facing encrypted transport card. Every switch and completed field
  // applies immediately; incomplete SNI-token and mTLS states stay local until
  // their required inputs are present.
  document.getElementById('doh3-enabled-input')?.addEventListener('change', (e) => {
    withTransportDraft(d => { d.doh3_enabled = !!e.target.checked; });
  });
  document.getElementById('plain-enabled-input')?.addEventListener('change', (e) => {
    withTransportDraft(d => { d.plain_dns_enabled = !!e.target.checked; });
  });
  document.getElementById('plain-port-input')?.addEventListener('change', (e) => {
    withTransportDraft(d => {
      const n = Number(e.target.value);
      d.plain_dns_port = Number.isFinite(n) && n > 0 ? n : 53;
    });
  });
  document.getElementById('doh-enabled-input')?.addEventListener('change', (e) => {
    withTransportDraft(d => { d.doh_enabled = !!e.target.checked; });
  });
  document.getElementById('doh-port-input')?.addEventListener('change', (e) => {
    withTransportDraft(d => {
      const n = Number(e.target.value);
      d.doh_port = Number.isFinite(n) && n > 0 ? n : 8443;
    });
  });
  document.getElementById('doq-enabled-input')?.addEventListener('change', (e) => {
    withTransportDraft(d => { d.doq_enabled = !!e.target.checked; });
  });
  document.getElementById('doq-auth-select')?.addEventListener('change', (e) => {
    withTransportDraft(d => { d.doq_auth = e.target.value === 'sni-token' ? 'sni-token' : 'ip'; });
  });
  document.getElementById('doq-token-domain-input')?.addEventListener('change', (e) => {
    withTransportDraft(d => { d.doq_token_domain = (e.target.value || '').trim().toLowerCase(); });
  });
  document.getElementById('doq-port-input')?.addEventListener('change', (e) => {
    withTransportDraft(d => {
      const n = Number(e.target.value);
      d.doq_port = Number.isFinite(n) && n > 0 ? n : 853;
    });
  });
  document.getElementById('add-doq-token-btn')?.addEventListener('click', () => {
    const input = document.getElementById('new-doq-token-input');
    const val = (input ? input.value : '').trim().toLowerCase();
    if (!val || !currentConfig) return;
    if (!/^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/.test(val)) {
      showToast('A DoQ token must be a single DNS label (letters, digits, hyphens).', 'error');
      return;
    }
    const draft = ensureTransportDraft(normalizeTransportSettings(currentConfig));
    if (draft.doq_tokens.includes(val)) {
      showToast(`${val} is already listed`, 'info');
      return;
    }
    if (input) input.value = '';
    withTransportDraft(d => { d.doq_tokens.push(val); });
  });

  document.getElementById('dot-enabled-input')?.addEventListener('change', (e) => {
    withTransportDraft(d => { d.dot_enabled = !!e.target.checked; });
  });
  document.getElementById('dot-port-input')?.addEventListener('change', (e) => {
    withTransportDraft(d => {
      const n = Number(e.target.value);
      d.dot_port = Number.isFinite(n) && n > 0 ? n : 853;
    });
  });
  document.getElementById('dot-auth-select')?.addEventListener('change', (e) => {
    withTransportDraft(d => { d.dot_auth = e.target.value === 'sni-token' ? 'sni-token' : 'ip'; });
  });
  document.getElementById('dot-token-domain-input')?.addEventListener('change', (e) => {
    withTransportDraft(d => { d.dot_token_domain = (e.target.value || '').trim().toLowerCase(); });
  });
  document.getElementById('add-dot-token-btn')?.addEventListener('click', () => {
    const input = document.getElementById('new-dot-token-input');
    const val = (input ? input.value : '').trim().toLowerCase();
    if (!val || !currentConfig) return;
    if (!/^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/.test(val)) {
      showToast('A DoT token must be a single DNS label (letters, digits, hyphens).', 'error');
      return;
    }
    const draft = ensureTransportDraft(normalizeTransportSettings(currentConfig));
    if (draft.dot_tokens.includes(val)) {
      showToast(`${val} is already listed`, 'info');
      return;
    }
    if (input) input.value = '';
    withTransportDraft(d => { d.dot_tokens.push(val); });
  });

  document.getElementById('dnscrypt-enabled-input')?.addEventListener('change', (e) => {
    withTransportDraft(d => { d.dnscrypt_enabled = !!e.target.checked; });
  });
  document.getElementById('dnscrypt-port-input')?.addEventListener('change', (e) => {
    withTransportDraft(d => {
      const n = Number(e.target.value);
      d.dnscrypt_port = Number.isFinite(n) && n > 0 ? n : 5443;
    });
  });
  document.getElementById('dnscrypt-provider-input')?.addEventListener('change', (e) => {
    withTransportDraft(d => { d.dnscrypt_provider_name = (e.target.value || '').trim().toLowerCase(); });
  });

  document.getElementById('mtls-enabled-input')?.addEventListener('change', (e) => {
    withTransportDraft(d => { d.mtls_enabled = !!e.target.checked; });
  });
  document.getElementById('mtls-ca-input')?.addEventListener('change', (e) => {
    withTransportDraft(d => { d.mtls_ca_pem = (e.target.value || '').trim(); });
  });

  // Add Custom Record
  document.getElementById('add-custom-record-btn')?.addEventListener('click', () => {
    const dInput = document.getElementById('custom-record-domain');
    const ipInput = document.getElementById('custom-record-ip');
    const dom = dInput ? dInput.value.trim().toLowerCase() : '';
    const ip = ipInput ? ipInput.value.trim() : '';
    if (!dom || !ip || !currentConfig) return;
    if (!currentConfig.rules.custom_records) currentConfig.rules.custom_records = {};
    currentConfig.rules.custom_records[dom] = ip;
    if (dInput) dInput.value = '';
    if (ipInput) ipInput.value = '';
    saveRules();
    renderConfig(currentConfig);
  });

  // Event Delegation for List Removals
  document.onclick = async (e) => {
    const btn = e.target.closest('button');
    if (!btn) return;

    if (btn.classList.contains('proxied-route-btn')) {
      openStreamActionMenu(btn);
      return;
    }
    if (btn.classList.contains('resolution-proxied')) {
      openResolutionDialog(btn.dataset.val);
      return;
    }
    if (btn.classList.contains('remove-upstream')) {
      const addr = btn.dataset.addr;
      // Every other destructive control on this panel asks first; this one deleted a
      // resolver on a single tap, and it is rendered inside a list that re-sorts itself
      // by measured latency on every stats tick — so the row under your thumb is not
      // guaranteed to be the row that was there when you started reaching for it. The
      // dialog also gives the operator the one fact that decides the answer: with the
      // last upstream gone there is nothing left to race, and resolution stops.
      const ok = await confirmAction({
        destructive: true,
        title: 'Remove this upstream?',
        hint: 'THE RACER SET SHRINKS IMMEDIATELY',
        message: `${addr}\n\nQueries stop being raced against this resolver at once. If it is the last one configured, nothing remains to answer from and resolution fails until another is added.`,
        confirmText: 'REMOVE UPSTREAM',
      });
      if (!ok) return;
      try {
        await fetch(api('/api/upstreams/delete'), {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${authToken}` },
          body: JSON.stringify({ address: addr })
        });
        showToast('Upstream removed', 'info');
        loadResolutionSettings();
        updateStats();
      } catch (err) {}
    } else if (btn.classList.contains('remove-proxied')) {
      if (removeDomainEverywhere(btn.dataset.val)) {
        // Render before the round-trip so the row leaves at once, then put the
        // panel back in step with the server if the save was rejected — otherwise
        // the row silently reappears on the next refresh.
        renderConfig(currentConfig);
        if (!await saveRules()) loadConfig();
      }
    } else if (btn.classList.contains('proxied-pause-btn')) {
      const val = btn.dataset.val;
      if (currentConfig) {
        currentConfig.rules.custom_proxied = currentConfig.rules.custom_proxied.filter(x => x !== val);
        if (!currentConfig.rules.custom_proxied_disabled) currentConfig.rules.custom_proxied_disabled = [];
        if (!currentConfig.rules.custom_proxied_disabled.includes(val)) currentConfig.rules.custom_proxied_disabled.push(val);
        saveRules();
        renderConfig(currentConfig);
      }
    } else if (btn.classList.contains('proxied-resume-btn')) {
      const val = btn.dataset.val;
      if (currentConfig) {
        if (!currentConfig.rules.custom_proxied_disabled) currentConfig.rules.custom_proxied_disabled = [];
        currentConfig.rules.custom_proxied_disabled = currentConfig.rules.custom_proxied_disabled.filter(x => x !== val);
        if (!currentConfig.rules.custom_proxied.includes(val)) currentConfig.rules.custom_proxied.push(val);
        saveRules();
        renderConfig(currentConfig);
      }
    } else if (btn.classList.contains('remove-blocked')) {
      const val = btn.dataset.val;
      if (currentConfig) {
        currentConfig.rules.custom_blocked = currentConfig.rules.custom_blocked.filter(x => x !== val);
        saveRules();
        renderConfig(currentConfig);
      }
    } else if (btn.classList.contains('remove-token')) {
      const val = btn.dataset.val;
      if (currentConfig) {
        currentConfig.access.doh_tokens = currentConfig.access.doh_tokens.filter(x => x !== val);
        await fetch(api('/api/config/access'), {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${authToken}` },
          body: JSON.stringify(currentConfig.access)
        });
        showToast('DoH Token removed', 'info');
        renderConfig(currentConfig);
      }
    } else if (btn.classList.contains('remove-doq-token')) {
      const val = btn.dataset.val;
      if (currentConfig) {
        withTransportDraft(d => { d.doq_tokens = d.doq_tokens.filter(x => x !== val); });
      }
    } else if (btn.classList.contains('remove-dot-token')) {
      const val = btn.dataset.val;
      if (currentConfig) {
        withTransportDraft(d => { d.dot_tokens = d.dot_tokens.filter(x => x !== val); });
      }
    } else if (btn.classList.contains('remove-record')) {
      const dom = btn.dataset.dom;
      if (currentConfig && currentConfig.rules.custom_records) {
        delete currentConfig.rules.custom_records[dom];
        saveRules();
        renderConfig(currentConfig);
      }
    }
  };

  // Per-domain proxy switches. Delegated on the list container, whose innerHTML is
  // replaced by renderProxiedList on every save — a listener bound to each row
  // would be thrown away by the next render. Flipping off does not delete the
  // domain: it moves it to custom_proxied_disabled, which the matcher never sees,
  // so it stops being proxied and resolves normally. Flipping on moves it back.
  document.getElementById('custom-proxied-list')?.addEventListener('change', (e) => {
    const box = e.target.closest('.toggle-proxied');
    if (!box || !currentConfig) return;
    const val = box.dataset.val;
    if (!currentConfig.rules.custom_proxied_disabled) currentConfig.rules.custom_proxied_disabled = [];
    if (box.checked) {
      currentConfig.rules.custom_proxied_disabled = currentConfig.rules.custom_proxied_disabled.filter(x => x !== val);
      if (!currentConfig.rules.custom_proxied.includes(val)) currentConfig.rules.custom_proxied.push(val);
    } else {
      currentConfig.rules.custom_proxied = currentConfig.rules.custom_proxied.filter(x => x !== val);
      if (!currentConfig.rules.custom_proxied_disabled.includes(val)) currentConfig.rules.custom_proxied_disabled.push(val);
    }
    saveRules();
    renderConfig(currentConfig);
  });

  // Live Stream Controls
  //
  // The filter and the search box had no listeners at all: both were read inside the append
  // path, so they applied to arriving queries and never to the rows already on screen. These
  // two lines are the whole of the fix on the control side — renderQueryStream redraws from
  // the buffer, so a change now takes effect on the last STREAM_BUFFER_MAX queries at once.
  document.getElementById('domain-filters')?.addEventListener('click', (e) => {
    const chip = e.target.closest('.domain-filter-chip');
    if (!chip) return;
    const status = chip.dataset.status;
    proxiedStatusFilter = proxiedStatusFilter === status ? null : status;
    proxiedVisible = PROXIED_BATCH;
    renderProxiedList();
  });
  document.getElementById('stream-filter')?.addEventListener('change', renderQueryStream);
  document.getElementById('stream-search')?.addEventListener('input', renderQueryStream);
  // The sort select reorders the queries already received. pushStreamQuery consults the
  // same streamSort when it inserts an arriving row, so the two stay in agreement.
  document.getElementById('stream-sort')?.addEventListener('change', (e) => {
    streamSort = e.target.value || 'time-desc';
    renderQueryStream();
  });

  // Delegated on the table body, because the button lives inside markup that is replaced on
  // every keystroke — a listener bound to the button itself would be discarded by the next
  // render.
  document.getElementById('stream-tbody')?.addEventListener('click', (e) => {
    // The ACTION badge opens the quick-action menu. Same delegation reason as the
    // clear button below: the badge is rebuilt with the row on every render.
    const badge = e.target.closest('.stream-action-btn');
    if (badge) {
      e.stopPropagation();
      openStreamActionMenu(badge);
      return;
    }
    if (!e.target.closest('.clear-stream-filter-btn')) return;
    const filterEl = document.getElementById('stream-filter');
    const searchEl = document.getElementById('stream-search');
    if (filterEl) filterEl.value = 'ALL';
    if (searchEl) searchEl.value = '';
    renderQueryStream();
  });

  // The shared quick-action menu. Its buttons are static, so they bind once, and
  // the three ways out — an outside click, Escape, or scrolling the log — all
  // call the same close.
  document.getElementById('stream-action-menu')?.addEventListener('click', onStreamActionMenuClick);
  document.addEventListener('click', (e) => {
    if (e.target.closest('.stream-action-btn') || e.target.closest('.proxied-route-btn') || e.target.closest('#stream-action-menu')) return;
    closeStreamActionMenu();
  });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') closeStreamActionMenu();
  });
  document.querySelector('.stream-viewport')?.addEventListener('scroll', closeStreamActionMenu);

  const pauseBtn = document.getElementById('stream-pause-btn');
  if (pauseBtn) {
    pauseBtn.onclick = () => setStreamPaused(!isStreamPaused);
  }

  const refreshBtn = document.getElementById('stream-refresh-btn');
  if (refreshBtn) {
    refreshBtn.onclick = () => refreshLiveStream();
  }

  const clearBtn = document.getElementById('stream-clear-btn');
  if (clearBtn) {
    // Clear empties the daemon's ring as well as this view; resetStreamView keeps the
    // placeholder contract (empty the data, never wipe the tbody) and invalidates any
    // insert frame already queued.
    clearBtn.onclick = () => {
      clearQueryStream();
    };
  }
}

// =======================================================
// RUN FULL DIAGNOSTICS
// =======================================================
// One run at a time. The endpoint dials eight TCP endpoints with a 2.5 s timeout each, so a
// run against a blocked network takes the full 2.5 s — a wide window in which the Run Test
// button sat live and enabled. Clicking it fired a second concurrent POST, and the results
// list was written by whichever response *finished* first rather than whichever started last:
// the older run could land second and overwrite the newer one's numbers, with nothing on
// screen to say which run the operator was looking at. Opening the modal starts a run too, so
// close-and-reopen did the same thing.
// =======================================================
// DIAGNOSTIC TARGETS (operator-owned server list)
// =======================================================
// The suite used to dial eight hardcoded game endpoints. v3.1.0 made the list
// wholly operator-owned, so this pair of functions owns the editable list and the
// server owns the persisted record: every write answers with the full list, and
// the renderer draws whatever the server returns rather than what was sent, so a
// rejected entry never appears as though it had saved.

let diagTargets = [];

// renderDiagTargets draws the operator's server list with a remove button per row.
// Every value goes in through textContent or a createElement attribute: the name
// and host are operator input and are echoed back by the GET, so innerHTML here
// would be a stored-XSS sink reached through the panel itself.
// The last run's results, keyed by the host that was measured. The row it belongs
// to is the target row itself - there used to be a second list below repeating
// every server, and reading "which one failed" meant matching the two by eye.
let diagResultsByHost = {};

function diagStatusFor(host) {
  const r = diagResultsByHost[host];
  if (!r) return { dot: 'bg-slate-600', text: 'not tested', cls: 'text-slate-500' };
  if (r.success) {
    const ms = Number.isFinite(r.latency_ms) ? r.latency_ms.toFixed(1) : '?';
    return { dot: 'bg-emerald-400', text: `${ms} ms`, cls: 'text-emerald-400' };
  }
  return { dot: 'bg-red-400', text: 'BLOCKED', cls: 'text-red-400' };
}

function renderDiagTargets() {
  const container = document.getElementById('diag-targets-list');
  if (!container) return;
  container.innerHTML = '';

  if (!Array.isArray(diagTargets) || diagTargets.length === 0) {
    const empty = document.createElement('div');
    empty.className = 'text-xs text-slate-500 text-center py-3';
    empty.textContent = 'No servers yet — add one below to measure its latency.';
    container.appendChild(empty);
    return;
  }

  for (const t of diagTargets) {
    const status = diagStatusFor(t.host || '');
    const row = document.createElement('div');
    row.className = 'flex items-center justify-between gap-2 py-1.5 px-2.5 rounded-lg bg-slate-900/60 border border-slate-800 text-xs';

    const label = document.createElement('div');
    label.className = 'min-w-0 flex-1 flex items-center gap-2';
    const dot = document.createElement('span');
    dot.className = `w-2 h-2 rounded-full shrink-0 ${status.dot}`;
    const text = document.createElement('div');
    text.className = 'min-w-0';
    const nameEl = document.createElement('div');
    nameEl.className = 'font-bold text-white truncate';
    nameEl.textContent = t.name || t.host || '';
    const hostEl = document.createElement('div');
    hostEl.className = 'text-[10px] text-slate-500 font-mono truncate';
    hostEl.textContent = t.host || '';
    text.appendChild(nameEl);
    text.appendChild(hostEl);
    label.appendChild(dot);
    label.appendChild(text);

    const remove = document.createElement('button');
    remove.type = 'button';
    remove.className = 'text-slate-500 hover:text-red-400 p-1 rounded transition shrink-0';
    remove.setAttribute('aria-label', `Remove ${t.host || 'target'}`);
    remove.title = 'Remove';
    remove.dataset.diagRemove = t.host || '';
    remove.innerHTML = '<i data-feather="trash-2" class="w-3.5 h-3.5"></i>';
    remove.onclick = () => removeDiagTarget(t.host || '');

    const result = document.createElement('span');
    result.className = `font-mono font-bold shrink-0 ${status.cls}`;
    result.textContent = status.text;

    row.appendChild(label);
    row.appendChild(result);
    row.appendChild(remove);
    container.appendChild(row);
  }
  safeFeatherReplace();
}

// loadDiagTargets fetches the server list and renders it. It returns a promise so
// openDiag can sequence "load the list, then run the probe".
async function loadDiagTargets() {
  if (!authToken) return;
  try {
    const res = await fetch(api('/api/diagnostics/targets'), {
      headers: { 'Authorization': `Bearer ${authToken}` }
    });
    if (res.status === 401) { showLoginModal(); return; }
    if (!res.ok) {
      showToast(await errorMessage(res, 'Could not load your diagnostic servers'), 'error');
      return;
    }
    const data = await res.json();
    diagTargets = Array.isArray(data.targets) ? data.targets : [];
    renderDiagTargets();
  } catch (e) {
    showToast('Could not reach the server to load your diagnostic servers.', 'error');
  }
}

// addDiagTarget posts one entry. A 400 carries the validation reason from the
// server (empty name, bad host), which is surfaced verbatim so the operator knows
// what to fix. The input is cleared only on success.
async function addDiagTarget() {
  const nameEl = document.getElementById('diag-target-name');
  const hostEl = document.getElementById('diag-target-host');
  const addBtn = document.getElementById('diag-target-add');
  const name = nameEl ? nameEl.value.trim() : '';
  const host = hostEl ? hostEl.value.trim() : '';

  if (!name || !host) {
    showToast('Enter both a name and a host for the server.', 'error');
    return;
  }

  if (addBtn) addBtn.disabled = true;
  try {
    const res = await fetch(api('/api/diagnostics/targets'), {
      method: 'POST',
      headers: { 'Authorization': `Bearer ${authToken}`, 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, host })
    });
    if (res.status === 401) { showLoginModal(); return; }
    if (!res.ok) {
      showToast(await errorMessage(res, 'The server refused the new target'), 'error');
      return;
    }
    const data = await res.json();
    diagTargets = Array.isArray(data.targets) ? data.targets : [];
    renderDiagTargets();
    if (nameEl) nameEl.value = '';
    if (hostEl) hostEl.value = '';
    // The list changed, so the previous run's score describes a different set of
    // servers. Clear it so the banner cannot outlive the list it was measured on.
    const scoreEl = document.getElementById('diag-score');
    const qualEl = document.getElementById('diag-quality');
    if (scoreEl) scoreEl.innerText = '--%';
    if (qualEl) qualEl.innerText = 'Ready to test';
  } catch (e) {
    showToast('Could not reach the server to add the target.', 'error');
  } finally {
    if (addBtn) addBtn.disabled = false;
  }
}

// removeDiagTarget deletes one entry by its stored host.
async function removeDiagTarget(host) {
  if (!host) return;
  try {
    const res = await fetch(api('/api/diagnostics/targets?host=' + encodeURIComponent(host)), {
      method: 'DELETE',
      headers: { 'Authorization': `Bearer ${authToken}` }
    });
    if (res.status === 401) { showLoginModal(); return; }
    if (!res.ok) {
      showToast(await errorMessage(res, 'The server refused to remove the target'), 'error');
      return;
    }
    const data = await res.json();
    diagTargets = Array.isArray(data.targets) ? data.targets : [];
    renderDiagTargets();
    // The deleted server's result row is stale the moment it is gone. Drop it from
    // the results list now instead of leaving it until the next run rebuilds the
    // whole list — a result for a target the operator just removed reads as if the
    // removal did not take.
    removeDiagResultRow(host);
    const scoreEl = document.getElementById('diag-score');
    const qualEl = document.getElementById('diag-quality');
    if (scoreEl) scoreEl.innerText = '--%';
    if (qualEl) qualEl.innerText = 'Ready to test';
  } catch (e) {
    showToast('Could not reach the server to remove the target.', 'error');
  }
}

// removeDiagResultRow drops the results row whose measured target matches host,
// then restores the empty-state hint when no rows remain. The row carries its
// target in data-diag-target (renderDiagnosticsReport sets it), so the match is an
// exact string compare against the same stored host:port the delete used.
function removeDiagResultRow(host) {
  delete diagResultsByHost[host];
  renderDiagTargets();
}

let diagRunning = false;

async function runFullDiagnostics() {
  if (diagRunning) return;
  diagRunning = true;

  const scoreEl = document.getElementById('diag-score');
  const qualEl = document.getElementById('diag-quality');
  const rerunBtn = document.getElementById('rerun-diagnostics-btn');

  // The banner used to keep the previous run's score for the whole of the next run and, if
  // that run then failed, for good — so the panel showed "88% / EXCELLENT (A+)" directly above
  // "The diagnostic run was refused", and nothing distinguished a fresh grade from a stale
  // one. A run in progress has no score; clear it before asking.
  if (scoreEl) scoreEl.innerText = '--%';
  if (qualEl) qualEl.innerText = 'Testing routes...';
  if (rerunBtn) {
    rerunBtn.disabled = true;
    rerunBtn.classList.add('opacity-50', 'cursor-not-allowed');
  }
  const fail = (msg) => {
    showToast(msg, 'error');
    if (qualEl) qualEl.innerText = 'Test failed';
  };

  try {
    const res = await fetch(api('/api/diagnostics/run'), {
      method: 'POST',
      headers: { 'Authorization': `Bearer ${authToken}` }
    });
    // A refusal used to `return` straight out, leaving the "Testing VPS
    // connectivity…" spinner on screen for good. An expired session answers 401
    // here, so the most likely reading of a permanent spinner was "the diagnostic
    // hangs", not "log in again".
    if (!res.ok) {
      fail(await errorMessage(res, 'The diagnostic run was refused'));
      return;
    }

    let report = null;
    try {
      report = await res.json();
    } catch (e) {
      // A 200 carrying something that is not JSON means a proxy answered, not the panel's
      // own server. Reporting that as "could not reach the server" named the one thing that
      // had demonstrably just worked.
      fail('The server answered the diagnostic run with a response that is not JSON.');
      return;
    }
    renderDiagnosticsReport(report);
  } catch (e) {
    fail('Could not reach the server to run diagnostics.');
  } finally {
    diagRunning = false;
    if (rerunBtn) {
      rerunBtn.disabled = false;
      rerunBtn.classList.remove('opacity-50', 'cursor-not-allowed');
    }
  }
}

// renderDiagnosticsReport draws one report and must not throw: it runs inside the try above,
// so an exception here would surface as the catch's "could not reach the server" — a message
// about the network, for a bug in the renderer.
//
// It also shows reachable/total and avg_latency_ms, which the server has always computed and
// sent and the panel has always discarded. The grade alone cannot tell "all eight endpoints
// answered, slowly" from "seven answered instantly and one is unreachable", and average
// handshake time is the number this panel exists to report.
function renderDiagnosticsReport(rep) {
  const report = rep || {};
  const results = Array.isArray(report.results) ? report.results : [];

  const scoreEl = document.getElementById('diag-score');
  if (scoreEl) {
    scoreEl.innerText = Number.isFinite(report.overall_score) ? `${report.overall_score}%` : '--%';
  }

  const qualEl = document.getElementById('diag-quality');
  if (qualEl) {
    const bits = [];
    if (report.overall_quality) bits.push(String(report.overall_quality));
    if (Number.isFinite(report.reachable) && Number.isFinite(report.total)) {
      bits.push(`${report.reachable}/${report.total} reachable`);
    }
    // Guarded on reachable, because the average is over successful dials only: with none, the
    // server sends 0 and "0 ms avg" would read as a perfect result on a fully blocked network.
    if (Number.isFinite(report.avg_latency_ms) && report.reachable > 0) {
      bits.push(`${report.avg_latency_ms} ms avg`);
    }
    // innerText, so the separator and the server's grade string are text either way.
    qualEl.innerText = bits.length ? bits.join(' · ') : 'No result';
  }

  // Fold the results into the rows that name the servers, and repaint that one
  // list. There used to be a second list here repeating every target; the result
  // belongs to the row it measured.
  diagResultsByHost = {};
  for (const r of results) {
    const host = r.target || r.name || '';
    if (host) diagResultsByHost[host] = r;
  }
  renderDiagTargets();
}

// =======================================================
// CLIENTS & IP WHITELIST MANAGEMENT (Shelter/Shecan Style)
// =======================================================
let clientsDataCache = null;

// clientsPanelMessage paints a full-width notice into the client grid, in the same shape as the
// two empty states renderClientsList already draws. `body` is markup the caller has escaped —
// there is one caller that interpolates a server string and it wraps it in escapeHTML.
//
// The wrapper carries .clients-placeholder, the same convention the query stream uses for its
// empty row: it is how loadClients tells "the grid is showing a notice" apart from "the grid is
// showing subscriber cards", without inspecting the copy.
function clientsPanelMessage(icon, title, body, titleClass = 'text-slate-300') {
  const listContainer = document.getElementById('clients-list');
  if (!listContainer) return;
  listContainer.innerHTML = `
    <div class="clients-placeholder col-span-1 md:col-span-2 glass-panel panel-card p-8 text-center text-slate-400">
      <i data-feather="${escapeHTML(icon)}" class="w-8 h-8 mx-auto text-slate-600 mb-2"></i>
      <div class="font-bold ${titleClass} font-heading">${title}</div>
      <p class="text-xs text-slate-500 mt-1">${body}</p>
    </div>
  `;
  safeFeatherReplace();
}

// loadClients fetches the subscriber list and the access-control mode.
//
// A failure used to be a bare `return` and a console line, and #clients-list used to ship as an
// empty grid holding nothing but an HTML comment — so a 500 or a dropped connection on the first
// load left the operator looking at blank space. Blank space in a list is read as "there is
// nothing here", which for this panel means "my subscribers are gone": the one reading an operator
// would act on, and the one that was never true. The markup now ships a loading notice and every
// failure path below replaces it with what actually happened.
//
// 401 is separated out because it is not a failure of this endpoint, it is an expired session,
// and the answer to it is the login gate rather than an error inside a panel behind it.
async function loadClients() {
  if (!authToken) return;

  // Only when the grid holds no subscriber cards — the first load, or a retry after one that
  // failed. loadClients also runs after every mutation, and flashing the whole grid away each
  // time a client is toggled would be worse than showing nothing.
  const listContainer = document.getElementById('clients-list');
  const showingCards = !!listContainer && listContainer.children.length > 0 &&
    !listContainer.querySelector('.clients-placeholder');
  if (!showingCards) {
    clientsPanelMessage('loader', 'Loading clients…', 'Reading the subscriber list from the server.');
  }

  try {
    const res = await fetch(api('/api/clients'), {
      headers: { 'Authorization': `Bearer ${authToken}` }
    });

    if (res.status === 401) {
      showLoginModal();
      return;
    }
    if (!res.ok) {
      const msg = await errorMessage(res, 'The server refused the request.');
      clientsPanelMessage('alert-triangle', 'Could not load clients',
        `${escapeHTML(msg)} Nothing has been changed — this is a failed read, not an empty list.`,
        'text-amber-300');
      showToast(msg, 'error');
      return;
    }

    clientsDataCache = await res.json();
    renderClientsView(clientsDataCache);
    // Client IP bindings feed the DoQ IP allowlist counts shown in Settings.
    refreshTransportView();
  } catch (e) {
    console.error('Failed to load clients:', e);
    clientsPanelMessage('wifi-off', 'Could not reach the server',
      'The subscriber list could not be read. Nothing has been changed — this is a failed read, not an empty list.',
      'text-amber-300');
    showToast('Could not reach the server to load the client list.', 'error');
  }
}

function renderClientsView(data) {
  if (!data) return;

  // Access Control Mode Switch & Badges
  const modeSwitch = document.getElementById('access-mode-switch');
  const modeBadge = document.getElementById('access-mode-badge');
  const modeText = document.getElementById('access-mode-status-text');

  const isWhitelistEnforced = !data.allow_all;
  if (modeSwitch) modeSwitch.checked = isWhitelistEnforced;

  // The two badges carry an icon rather than the 🔒/🔓 they used to, and innerHTML rather
  // than innerText because of it. Both names exist in the bundle, and both paths out of this
  // function redraw — the placeholder return below and the tail of the card render — so the
  // <i> never survives as an empty element.
  if (isWhitelistEnforced) {
    if (modeBadge) {
      modeBadge.className = 'badge badge-proxy badge-whitelist inline-flex items-center gap-1';
      modeBadge.innerHTML = '<i data-feather="lock" class="w-3 h-3"></i> WHITELIST ENFORCED';
    }
    if (modeText) {
      modeText.innerText = 'Whitelist Mode (Only Registered Clients)';
      modeText.className = 'text-xs font-mono text-cyan-400 font-bold';
    }
  } else {
    if (modeBadge) {
      modeBadge.className = 'badge badge-direct inline-flex items-center gap-1';
      modeBadge.innerHTML = '<i data-feather="unlock" class="w-3 h-3"></i> PUBLIC ACCESS';
    }
    if (modeText) {
      modeText.innerText = 'Public Mode (Anyone can connect)';
      modeText.className = 'text-xs font-mono text-slate-400 font-semibold';
    }
  }

  // Filter clients by search
  const searchInput = document.getElementById('client-search-input');
  const searchVal = searchInput ? searchInput.value.toLowerCase().trim() : '';

  // Kept apart from the filtered list on purpose: "you have no subscribers" and "your
  // search matched none of them" are opposite situations that used to print the same
  // message, and the message was the first one. An operator with fifty paying subscribers
  // who mistyped into the search box was told "No Clients Found — click Add New Client to
  // create client accounts", which reads exactly like a list that has just been wiped.
  const allClients = data.clients || [];
  let clients = allClients;
  if (searchVal) {
    clients = clients.filter(c =>
      c.name.toLowerCase().includes(searchVal) ||
      c.id.includes(searchVal) ||
      // Guarded because allowed_ips is not guaranteed to be an array on the wire: the
      // read path normalises a missing list to [], but POST /api/clients/add answers
      // with the record it just built, and an IP-less create left that field null. An
      // unguarded .some() there would throw on the first keystroke in the search box
      // and take the whole list render with it, so the panel would go blank with only
      // a console error to explain it. Every other allowed_ips reader here already
      // guards; this one was the exception.
      (Array.isArray(c.allowed_ips) && c.allowed_ips.some(ip => ip.includes(searchVal)))
    );
  }

  const listContainer = document.getElementById('clients-list');
  if (!listContainer) return;
  listContainer.innerHTML = '';

  if (clients.length === 0) {
    // Both carry .clients-placeholder for the same reason clientsPanelMessage does: they are
    // notices rather than subscriber cards, and loadClients uses that class to decide whether a
    // reload should show its loading state or leave a populated grid alone.
    listContainer.innerHTML = allClients.length === 0 ? `
      <div class="clients-placeholder col-span-1 md:col-span-2 glass-panel panel-card p-8 text-center text-slate-400">
        <i data-feather="users" class="w-8 h-8 mx-auto text-slate-600 mb-2"></i>
        <div class="font-bold text-slate-300 font-heading">No Clients Yet</div>
        <p class="text-xs text-slate-500 mt-1">Click "Add New Client" above to create client accounts &amp; registration links.</p>
      </div>
    ` : `
      <div class="clients-placeholder col-span-1 md:col-span-2 glass-panel panel-card p-8 text-center text-slate-400">
        <i data-feather="search" class="w-8 h-8 mx-auto text-slate-600 mb-2"></i>
        <div class="font-bold text-slate-300 font-heading">No Match</div>
        <p class="text-xs text-slate-500 mt-1">None of your ${allClients.length} client(s) match &ldquo;${escapeHTML(searchVal)}&rdquo;. The search looks at the name, the ID and the registered IPs.</p>
        <button class="clear-client-search-btn mt-3 text-[11px] font-bold text-cyan-400 border border-cyan-500/30 bg-cyan-500/10 px-3 py-1.5 rounded-lg hover:bg-cyan-500/20 transition">
          Clear search
        </button>
      </div>
    `;
    safeFeatherReplace();
    return;
  }

  // The subscription origin from /api/config wins (Phase 4): links must carry
  // the origin subscribers will actually open, which can differ from the
  // address the panel happens to be viewed at. The scheme arrives with it and
  // is never rebuilt here — a record carrying its own certificate pair is
  // HTTPS even when the panel is plain HTTP, a fact the panel's tls flag
  // cannot express. An empty origin means the daemon has nothing to advertise,
  // and the address the operator is viewing from is the least-bad fallback.
  const subOrigin = currentConfig?.subscription_origin || window.location.origin;
  const currentOrigin = subOrigin || window.location.origin;
  let dnsPrimaryIP = data.public_ip;
  if (!dnsPrimaryIP || dnsPrimaryIP === '127.0.0.1' || dnsPrimaryIP === '0.0.0.0' || dnsPrimaryIP === 'localhost') {
    if (window.location.hostname && window.location.hostname !== '127.0.0.1' && window.location.hostname !== 'localhost') {
      dnsPrimaryIP = window.location.hostname;
    } else {
      dnsPrimaryIP = data.public_ip || '127.0.0.1';
    }
  }

  clients.forEach(c => {
    const isExpired = c.expires_at && c.expires_at !== '0001-01-01T00:00:00Z' && new Date(c.expires_at) < new Date();
    
    let statusBadge = '<span class="badge badge-direct">ACTIVE</span>';
    if (!c.enabled) {
      statusBadge = '<span class="badge badge-block">DISABLED</span>';
    } else if (isExpired) {
      statusBadge = '<span class="badge badge-block">EXPIRED</span>';
    } else if (c.quota_exceeded === true) {
      // The third reason the resolver refuses an account, and the one the card used to
      // hide: a subscriber whose volume is spent was still badged ACTIVE, so the first
      // the reseller heard of it was the customer complaining that nothing resolves.
      // Red like EXPIRED because the effect is the same, worded differently because the
      // fix is not — this one needs volume, not days.
      statusBadge = '<span class="badge badge-block">NO QUOTA</span>';
    }

    // Two forms of the same instant, because the card shows it in a 103px column and the
    // Bot Card copies it into a message. expText stays precise — full date, hours, minutes,
    // seconds — and goes to the tooltip and to data-exp. expShort is what the cell renders.
    //
    // The cell used to render expText, and at 360px a `truncate` cut it to
    // "10/5/2026 7:39:…", which is worse than showing less: a half-printed clock reads as
    // corrupted data rather than as an elided one. Dropping the time entirely is the right
    // trade because the line directly underneath already carries the number a reseller
    // actually acts on — "(29d 23h remaining)" — so the clock was never the answer to a
    // question anyone was asking, only the date is, and the date fits in nine characters.
    let expText = 'Lifetime (No Expiry)';
    let expShort = expText;
    let remainingText = '';
    if (c.expires_at && c.expires_at !== '0001-01-01T00:00:00Z') {
      const expDate = new Date(c.expires_at);
      expText = expDate.toLocaleDateString() + ' ' + expDate.toLocaleTimeString();
      expShort = expDate.toLocaleDateString();
      const diffMs = expDate - new Date();
      if (diffMs > 0) {
        const days = Math.floor(diffMs / (1000 * 60 * 60 * 24));
        const hours = Math.floor((diffMs % (1000 * 60 * 60 * 24)) / (1000 * 60 * 60));
        remainingText = `(${days}d ${hours}h remaining)`;
      } else {
        remainingText = '(Expired)';
      }
    }

    // Two addresses, two jobs (the operator's own algorithm, Phase B):
    //   /sub/<token> is the subscriber's page — the link you hand out, and it is
    //     read-only by itself, so a leaked link moves nobody's binding.
    //   /ip/<token>  is the registration API — it demands the registration
    //     secret, which travels out-of-band (the Bot Card carries it).
    // Reg Link hands out the page; the Bot Card adds the API URL and the secret
    // so the subscriber can actually move their binding.
    const subUrl = `${currentOrigin}/sub/${c.token}`;
    const regApiUrl = `${currentOrigin}/ip/${c.token}`;
    // The first address on file, for pre-filling the "set IP" dialog. At most one is
    // ever stored — see SetClientIP and registerIP, which both replace rather than
    // append, because a subscriber is identified by the address they are on now.
    const currentIP = (c.allowed_ips && c.allowed_ips.length > 0) ? c.allowed_ips[0] : '';

    // Whitelisted IPs HTML tags.
    //
    // Every server-supplied value below goes through escapeHTML. This card is
    // assigned with innerHTML, and a client's name, note and UUID are free text that
    // the operator — or anything holding the API key, such as the Telegram
    // provisioner — can set to whatever it likes. Records written before the address
    // was validated may also hold arbitrary text where an IP belongs. Without
    // escaping, one such value renders as live markup inside the authenticated panel,
    // and a value in a double-quoted attribute does not even need a `<`: closing the
    // quote is enough to add an event handler.
    // The remove button is a `×` glyph, and a glyph is sized by its own font metrics:
    // this one measured 6.6 x 16.5 CSS px on a phone. WCAG 2.2 SC 2.5.8 puts the floor
    // for a touch target at 24 x 24, so it failed on the shorter axis by a factor of
    // three and a half — and it is the control that un-whitelists a paying subscriber.
    // A `w-6 h-6` inline-flex box is exactly 24 x 24 whatever the glyph inside does,
    // because the size no longer comes from the text; `leading-none` keeps the `×`
    // optically centred once the box stops hugging it. The chip grows to fit, which is
    // the trade being made on purpose: one address chip per line on a 360px screen is
    // better than a delete button nobody can hit without zooming.
    let ipsHtml = '';
    if (c.allowed_ips && c.allowed_ips.length > 0) {
      ipsHtml = c.allowed_ips.map(ip => `
        <span class="inline-flex items-center gap-1 ps-2 pe-0.5 py-0.5 rounded-md bg-slate-950 border border-cyan-500/30 text-[11px] font-mono text-cyan-300">
          <span>${escapeHTML(ip)}</span>
          <button class="remove-client-ip-btn inline-flex items-center justify-center w-6 h-6 rounded hover:text-red-400 hover:bg-red-500/10 transition leading-none" data-id="${escapeHTML(c.id)}" data-ip="${escapeHTML(ip)}" title="Remove IP" aria-label="Remove IP ${escapeHTML(ip)}">×</button>
        </span>
      `).join('');
    } else {
      ipsHtml = '<span class="text-slate-500 text-[11px] italic">No IPs registered yet (Share link below)</span>';
    }

    // Traffic. This card is what a reseller looks at to answer "how much of their plan
    // has this subscriber used", and until now it answered in megabytes only — a 40 GB
    // plan reported "41287.3 MB", which nobody can read against a limit in GB. It also
    // showed the used figure with no relation to the limit at all, so the one number
    // that decides whether to renew had to be worked out by hand every time.
    const usedBytes = (typeof c.traffic_used_bytes === 'number' && isFinite(c.traffic_used_bytes) && c.traffic_used_bytes > 0)
      ? c.traffic_used_bytes
      : 0;
    const limitGB = (typeof c.traffic_limit_gb === 'number' && isFinite(c.traffic_limit_gb) && c.traffic_limit_gb > 0)
      ? c.traffic_limit_gb
      : 0;
    const limitText = limitGB > 0 ? `${limitGB} GB` : 'Unlimited';

    let usedText = `Used: ${formatBytes(usedBytes)}`;
    let usedClass = 'text-slate-500';
    if (limitGB > 0) {
      const pct = Math.min(999, Math.round((usedBytes / (limitGB * 1024 * 1024 * 1024)) * 100));
      usedText = `Used: ${formatBytes(usedBytes)} · ${pct}%`;
      // Amber is the warning a reseller can act on before the subscriber calls. Red is
      // not derived from that percentage: c.quota_exceeded is the daemon's own verdict,
      // the same one the resolver enforces with, and 100% here is a rounded figure that
      // can read as full while the account is still being answered.
      if (pct >= 80) usedClass = 'text-amber-400 font-bold';
      if (c.quota_exceeded === true) usedClass = 'text-red-400 font-bold';
    }

    // When the volume comes back, straight from the server. The cycle name alone is
    // not the answer an operator needs — "monthly" on an account anchored to the 31st
    // means the 28th in February — and working it out here would be a second
    // implementation of the clamped-month arithmetic that only the daemon can settle.
    let cycleHtml = '';
    if (c.next_traffic_reset) {
      const next = new Date(c.next_traffic_reset);
      if (!isNaN(next.getTime())) {
        cycleHtml = `<div class="text-emerald-400/80 text-[9px] inline-flex items-center gap-1" title="Volume resets automatically (${escapeHTML(c.traffic_reset_cycle || '')})"><i data-feather="rotate-cw" class="w-2.5 h-2.5"></i>${escapeHTML(next.toLocaleDateString())}</div>`;
      }
    }

    const queriesText = (typeof c.total_queries === 'number' && isFinite(c.total_queries))
      ? c.total_queries.toLocaleString()
      : '—';

    // Two bugs lived on this one line. Go marshals a never-set time.Time as
    // "0001-01-01T00:00:00Z", which is a perfectly truthy string, so a subscriber who
    // has never sent a query was reported as last seen at midnight — the expiry field
    // right next to it already guarded for exactly this value. And the time was
    // rendered with toLocaleTimeString() alone, so "last seen 3:04 PM" was
    // indistinguishable between this afternoon and three weeks ago.
    let lastSeenText = 'Never';
    if (c.last_seen && c.last_seen !== '0001-01-01T00:00:00Z') {
      const seen = new Date(c.last_seen);
      if (!isNaN(seen.getTime())) {
        const ageMin = Math.floor((Date.now() - seen.getTime()) / 60000);
        if (ageMin < 1) lastSeenText = 'just now';
        else if (ageMin < 60) lastSeenText = `${ageMin}m ago`;
        else if (ageMin < 1440) lastSeenText = `${Math.floor(ageMin / 60)}h ago`;
        else lastSeenText = `${Math.floor(ageMin / 1440)}d ago`;
      }
    }

    const card = document.createElement('div');
    card.className = 'glass-panel panel-card p-4 sm:p-5 flex flex-col justify-between space-y-4 hover:border-cyan-500/40 transition';
    card.innerHTML = `
      <div>
        <!-- Card Header -->
        <!-- min-w-0/flex-1 on the text block and shrink-0 on the buttons are
             load-bearing on a phone. The Slug is one unbreakable 64-character
             token, so without min-w-0 it sets the block's minimum width at
             ~345px — wider than the whole header on a 375px screen — and the
             flex row pushed the edit/pause/delete group clean off the right
             edge of the viewport, unreachable. The slug line truncates like
             the UUID row below it does, with the full value kept on the title. -->
        <div class="flex items-start justify-between gap-2 mb-2 pb-2 border-b border-slate-800/80">
          <div class="min-w-0 flex-1">
            <div class="flex items-center gap-2 flex-wrap">
              <h4 class="text-sm font-bold text-white font-heading">${escapeHTML(c.name)}</h4>
              ${statusBadge}
            </div>
            <div class="text-[10px] text-slate-400 font-mono mt-0.5 truncate" title="Code: ${escapeHTML(c.id)} · Slug: ${escapeHTML(c.token)}">
              Code: <span class="text-amber-300 font-bold">${escapeHTML(c.id)}</span> · Slug: <span class="text-slate-500">${escapeHTML(c.token)}</span>
            </div>
          </div>

          <div class="flex items-center gap-1 shrink-0">
            <button class="edit-client-btn p-1.5 rounded-lg bg-slate-900 hover:bg-cyan-500/20 text-slate-400 hover:text-cyan-300 border border-slate-800 transition" data-id="${escapeHTML(c.id)}" title="Edit Client">
              <i data-feather="edit-2" class="w-3.5 h-3.5"></i>
            </button>
            <button class="toggle-client-btn p-1.5 rounded-lg bg-slate-900 hover:bg-slate-800 text-slate-400 hover:text-cyan-400 border border-slate-800 transition" data-id="${escapeHTML(c.id)}" data-enabled="${!c.enabled}" title="${c.enabled ? 'Disable Client' : 'Enable Client'}">
              <i data-feather="${c.enabled ? 'pause' : 'play'}" class="w-3.5 h-3.5"></i>
            </button>
            <button class="delete-client-btn p-1.5 rounded-lg bg-slate-900 hover:bg-red-500/20 text-slate-400 hover:text-red-400 border border-slate-800 transition" data-id="${escapeHTML(c.id)}" data-name="${escapeHTML(c.name)}" title="Delete Client">
              <i data-feather="trash-2" class="w-3.5 h-3.5"></i>
            </button>
          </div>
        </div>

        <!-- UUID Row -->
        <!-- shrink-0 on the copy button is load-bearing next to the truncating UUID: the
             row is justify-between with a flexible middle child, so without it the button
             is the first thing the layout takes width from when a long UUID and a narrow
             phone compete. A 6x6 box puts it at exactly the 24px WCAG 2.2 SC 2.5.8 floor,
             up from the 16x16 that p-0.5 around a 12px icon produced — and this is the
             control a reseller uses most, because the UUID is what goes into the
             subscriber's config. -->
        <div class="flex items-center justify-between text-[10px] font-mono bg-slate-950/80 px-2.5 py-1.5 rounded-lg border border-slate-800/80 mb-2.5">
          <span class="text-slate-500 font-semibold">UUID:</span>
          <span class="text-cyan-300 truncate max-w-[180px] sm:max-w-[210px] select-all" title="${escapeHTML(c.uuid)}">${escapeHTML(c.uuid) || 'N/A'}</span>
          <button class="copy-uuid-btn inline-flex items-center justify-center w-6 h-6 shrink-0 rounded text-slate-400 hover:text-cyan-300 hover:bg-cyan-500/10 transition ms-1" data-uuid="${escapeHTML(c.uuid)}" title="Copy UUID" aria-label="Copy UUID">
            <i data-feather="copy" class="w-3 h-3"></i>
          </button>
        </div>

        <!-- Whitelisted IPs Row -->
        <!-- The 10px label gave this button a 15px-tall hit area — wide enough to hit by
             accident, short enough to miss on purpose. A 24px min-height reaches the
             floor without moving the text, and the negative inline-end margin cancels the
             new padding so the label still sits flush with the row's edge; the hit area
             grows outward into the gutter rather than pushing the layout around. -->
        <div class="space-y-1.5 mb-3">
          <div class="flex items-center justify-between text-[11px]">
            <span class="text-slate-400 font-semibold">Registered IPs & CIDRs:</span>
            <button class="add-ip-prompt-btn text-cyan-400 hover:text-cyan-300 text-[10px] font-mono flex items-center justify-center gap-0.5 min-h-[24px] px-1.5 -me-1.5 rounded hover:bg-cyan-500/10 transition" data-id="${escapeHTML(c.id)}" data-ip="${escapeHTML(currentIP)}">
              + Set IP Manually
            </button>
          </div>
          <div class="client-ip-chips flex flex-wrap gap-1.5">
            ${ipsHtml}
          </div>
        </div>

        <!-- Expiration & Metrics -->
        <div class="grid grid-cols-3 gap-2 text-[10px] font-mono bg-slate-950/60 p-2.5 rounded-xl border border-slate-800/80">
          <div>
            <span class="text-slate-500 uppercase">Plan Expiry</span>
            <div class="text-slate-200 font-bold truncate" title="${escapeHTML(expText)}">${escapeHTML(expShort)}</div>
            <div class="text-emerald-400 text-[9px]">${escapeHTML(remainingText)}</div>
          </div>
          <div>
            <span class="text-slate-500 uppercase">Traffic Limit</span>
            <div class="text-amber-300 font-bold">${limitText}</div>
            <div class="${usedClass} text-[9px]">${usedText}</div>
            ${cycleHtml}
          </div>
          <div>
            <span class="text-slate-500 uppercase">Queries</span>
            <div class="text-cyan-300 font-bold">${queriesText}</div>
            <div class="text-slate-500 text-[9px]">Last seen: ${lastSeenText}</div>
          </div>
        </div>
      </div>

      <!-- Quick Action Buttons -->
      <div class="grid grid-cols-3 gap-2 pt-2 border-t border-slate-800/80 text-xs">
        <button class="copy-reg-link-btn py-1.5 px-2 rounded-lg bg-cyan-500/10 hover:bg-cyan-500/20 text-cyan-300 border border-cyan-500/30 text-[11px] font-bold font-heading flex items-center justify-center gap-1 transition truncate" data-url="${escapeHTML(subUrl)}">
          <i data-feather="link" class="w-3 h-3 flex-shrink-0"></i>
          <span>Reg Link</span>
        </button>

        <button class="copy-telegram-card-btn py-1.5 px-2 rounded-lg bg-purple-500/10 hover:bg-purple-500/20 text-purple-300 border border-purple-500/30 text-[11px] font-bold font-heading flex items-center justify-center gap-1 transition truncate"
          data-id="${escapeHTML(c.id)}" data-name="${escapeHTML(c.name)}" data-exp="${escapeHTML(expText)}" data-url="${escapeHTML(subUrl)}" data-reg-url="${escapeHTML(regApiUrl)}" data-secret="${escapeHTML(c.register_secret || '')}" data-ip="${escapeHTML(dnsPrimaryIP)}" data-uuid="${escapeHTML(c.uuid)}">
          <i data-feather="send" class="w-3 h-3 flex-shrink-0"></i>
          <span>Bot Card</span>
        </button>

        <button class="renew-client-btn py-1.5 px-2 rounded-lg bg-slate-900 hover:bg-emerald-500/20 text-slate-300 hover:text-emerald-400 border border-slate-800 text-[11px] font-bold font-heading flex items-center justify-center gap-1 transition truncate" data-id="${escapeHTML(c.id)}">
          <i data-feather="clock" class="w-3 h-3 flex-shrink-0"></i>
          <span>+30 Days</span>
        </button>
      </div>
    `;

    listContainer.appendChild(card);
  });

  safeFeatherReplace();
}

// Generate the Telegram provisioning card for a subscriber.
//
// The card is the out-of-band channel the Phase B design depends on: the /sub/
// page is read-only on its own, so the registration secret has to reach the
// subscriber somewhere other than the portal link. It used to be a Shelcan-
// style message that named the retired /ip/ link as if it were the subscriber
// page and printed a bare "Your dedicated DNS:" heading with nothing after
// it — an operator copying it sent an API link next to an empty field.
//
// Four pieces now, each with a job: where to manage the subscription, the
// secret, the address to point their console at, and the API endpoint for a
// scripted client. The secret is printed because this text is written by an
// authenticated operator into a private chat with their customer.
function generateClientTelegramMessage(clientId, clientName, expStr, subUrl, regApiUrl, secret, dnsIP) {
  const serverLine = dnsIP || '(server address not configured)';
  const secretLine = secret ? secret : '(no secret on file — regenerate it in the panel)';
  return `🎮 Your SmartDNS account is ready

🔹 Client ID : ${clientId}
🔹 Plan expires : ${expStr}

📄 Your subscription panel (status, usage and renewal):
${subUrl}

📶 Your dedicated DNS :
🔹 Primary : ${serverLine}
🔹 Secondary : 1.1.1.1

Steps to register your IP :
1️⃣ Connect your phone and other devices to the same internet connection .
2️⃣ Open the subscription panel link above (without a VPN/proxy) .
3️⃣ Enter the registration secret below and press "Register my IP" .
❌ Without registering your IP, DNS will not be enabled for you ❌

🔑 Registration Secret :
${secretLine}

🔗 IP registration API endpoint (for automation):
${regApiUrl}`;
}

// Attach Client View Event Listeners
function initClientEventListeners() {
  // Add Client Modal handlers. The form is reset on every open, so a half-filled
  // create attempt abandoned with the × never seeds the next one.
  const addModal = document.getElementById('add-client-modal');
  const openBtn = document.getElementById('open-add-client-btn');
  if (openBtn) {
    openBtn.onclick = () => {
      resetAddClientForm();
      addModal?.classList.remove('hidden');
    };
  }
  const closeBtn = document.getElementById('close-add-client-btn');
  if (closeBtn) {
    closeBtn.onclick = () => addModal?.classList.add('hidden');
  }

  // Add Client Form Submit (Direct onsubmit handler with button debounce)
  const addForm = document.getElementById('add-client-form');
  if (addForm) {
    addForm.onsubmit = async (e) => {
      e.preventDefault();
      const submitBtn = addForm.querySelector('button[type="submit"]');
      if (submitBtn && submitBtn.disabled) return;
      if (submitBtn) {
        submitBtn.disabled = true;
        submitBtn.classList.add('opacity-50');
      }

      const name = document.getElementById('client-name-input')?.value.trim();
      // A comma- or space-separated list, same split the edit modal uses for its
      // single field (app.js edit handler): bare IPs and operator CIDRs both
      // survive to the server, which normalises each entry. An empty field stays
      // an empty array, so "disregard the initial IP" is simply leaving it blank.
      const initIPs = (document.getElementById('client-initial-ip')?.value || '')
        .split(/[\s,]+/).map((s) => s.trim()).filter(Boolean);

      // The whole plan in one request. Until these fields were on this form, the
      // volume had to be added afterwards by reopening the account in the edit modal —
      // and between the two steps the subscriber was live with no limit at all, with
      // nothing on screen saying so. The policies are on the form for the same
      // reason: an account created inheriting everything and corrected later is live
      // with the wrong rules for the whole gap between the two writes.
      const trafficGB = parseFloat(document.getElementById('client-traffic-input')?.value) || 0;
      const cycle = document.getElementById('client-traffic-cycle')?.value || '';
      const maxDevices = parseInt(document.getElementById('client-max-devices')?.value, 10) || 0;

      // The exact moment the picker answered, sent as RFC 3339 like the edit form
      // sends its expiry. An empty field is a lifetime plan and omits the key, which
      // the server reads as "not chosen" rather than as the zero time.
      const expiryVal = document.getElementById('add-client-expiry')?.value || '';
      let expiresAtISO = '';
      if (expiryVal) {
        const parsed = new Date(expiryVal);
        if (!isNaN(parsed.getTime())) expiresAtISO = parsed.toISOString();
      }

      try {
        const res = await fetch(api('/api/clients/add'), {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${authToken}` },
          body: JSON.stringify({
            name: name,
            expires_at: expiresAtISO || undefined,
            initial_ip: initIPs[0] || '',
            ips: initIPs,
            traffic_limit_gb: trafficGB,
            traffic_reset_cycle: cycle,
            max_devices: maxDevices
          })
        });

        if (res.ok) {
          const client = await res.json().catch(() => ({}));
          addModal?.classList.add('hidden');
          resetAddClientForm();
          showToast('Client account created!', 'success');
          // The create response is the one moment the register link id and the
          // registration secret are both in hand (Phase B: the link displays,
          // the secret writes). Hand them to the created-modal instead of
          // letting the operator go hunting through the card's Bot Card — the
          // secret field there reads the LIST response, which is fine, but the
          // natural next action after creating a subscriber is sending them
          // their credentials, not navigating away to find them.
          showClientCreatedModal(client);
          await loadClients();
        } else {
          // The server's own reason, not a generic failure. A rejected cycle name and
          // a duplicate IP are both 400 here, and the difference is the whole of what
          // the operator has to fix.
          showToast(await errorMessage(res, 'Failed to create client'), 'error');
        }
      } catch (e) {
        showToast('Network error creating client', 'error');
      } finally {
        if (submitBtn) {
          submitBtn.disabled = false;
          submitBtn.classList.remove('opacity-50');
        }
      }
    };
  }

  // Access Mode Switch (Public vs Whitelist)
  document.getElementById('access-mode-switch')?.addEventListener('change', async (e) => {
    const isEnforced = e.target.checked;
    const allowAll = !isEnforced;

    try {
      const res = await fetch(api('/api/access/mode'), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${authToken}` },
        body: JSON.stringify({ allow_all: allowAll })
      });

      if (res.ok) {
        showToast(isEnforced ? 'Whitelist mode enforced (Only registered clients)' : 'Open public mode activated', 'info');
        loadClients();
      } else {
        // The server now persists BEFORE it applies and answers 500 when the
        // store write fails — an unchecked success here would show a green toast
        // for a mode change that did not survive to the next boot.
        const body = await res.json().catch(() => ({}));
        showToast(body.error || 'Could not save the access mode', 'error');
        // Snap the switch back to whatever the server actually holds, so the UI
        // never disagrees with a gate that silently stayed where it was.
        try {
          const st = await fetch(api('/api/config/access'), { headers: { 'Authorization': 'Bearer ' + authToken } });
          if (st.ok) {
            const cfg = await st.json();
            const enforced = !(cfg.allow_all ?? false);
            document.getElementById('access-mode-switch').checked = enforced;
          }
        } catch (ignore) { /* leave the switch as-is */ }
      }
    } catch (e) {
      showToast('Failed to update access mode', 'error');
    }
  });

  // Server-wide resolver allowlist (v3.1.0). Loaded with the access card and
  // saved through the same /api/config/access record the DoH tokens live in.
  async function loadAccessAllowlist() {
    const field = document.getElementById('access-allow-cidrs');
    if (!field) return;
    try {
      const res = await fetch(api('/api/config/access'), { headers: { 'Authorization': 'Bearer ' + authToken } });
      if (!res.ok) return;
      const cfg = await res.json();
      field.value = (cfg.allowed_ips || []).join(', ');
    } catch (ignore) { /* leave the field empty on a failed read */ }
  }
  loadAccessAllowlist();

  document.getElementById('access-allow-cidrs-save')?.addEventListener('click', async () => {
    const field = document.getElementById('access-allow-cidrs');
    if (!field) return;
    const cidrs = field.value.split(/[\s,]+/).map(s => s.trim()).filter(Boolean);
    try {
      const res = await fetch(api('/api/config/access'), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${authToken}` },
        body: JSON.stringify({ allowed_ips: cidrs })
      });
      if (res.ok) {
        showToast('Resolver allowlist saved', 'success');
        loadAccessAllowlist();
        refreshTransportView();
      } else {
        showToast(await errorMessage(res, 'Could not save the resolver allowlist'), 'error');
      }
    } catch (e) {
      showToast('Network error saving the resolver allowlist', 'error');
    }
  });

  // Search input live filter. Debounced: every keystroke
  // rebuilt the entire clients grid — one innerHTML card per subscriber plus
  // a document-wide icon sweep — which on a large roster made typing feel
  // like lag. 150 ms collapses a burst of keys into one rebuild.
  let clientSearchTimer = null;
  document.getElementById('client-search-input')?.addEventListener('input', () => {
    if (!clientsDataCache) return;
    clearTimeout(clientSearchTimer);
    clientSearchTimer = setTimeout(() => renderClientsView(clientsDataCache), 150);
  });

  // The "Clear search" button in the no-match state. Delegated on the container, not bound
  // after each render, because the container's innerHTML is replaced on every keystroke —
  // a listener attached to the button itself would be discarded by the next one.
  document.getElementById('clients-list')?.addEventListener('click', (e) => {
    if (!e.target.closest('.clear-client-search-btn')) return;
    const input = document.getElementById('client-search-input');
    if (!input) return;
    input.value = '';
    if (clientsDataCache) renderClientsView(clientsDataCache);
    input.focus();
  });

  // Event Delegation for Client Action Buttons
  document.addEventListener('click', async (e) => {
    // 1. Copy Registration Link
    const regBtn = e.target.closest('.copy-reg-link-btn');
    if (regBtn) {
      const url = regBtn.dataset.url;
      copyText(url, regBtn);
      return;
    }

    // 2. Copy Telegram Bot Card
    const cardBtn = e.target.closest('.copy-telegram-card-btn');
    if (cardBtn) {
      const id = cardBtn.dataset.id;
      const exp = cardBtn.dataset.exp;
      const subUrl = cardBtn.dataset.url;
      const regApiUrl = cardBtn.dataset.regUrl;
      const secret = cardBtn.dataset.secret;
      const ip = cardBtn.dataset.ip;
      const msg = generateClientTelegramMessage(id, cardBtn.dataset.name, exp, subUrl, regApiUrl, secret, ip);
      copyText(msg, cardBtn);
      showToast('Client card copied for Telegram!', 'success');
      return;
    }

    // 3. Renew Client (+30 Days)
    const renewBtn = e.target.closest('.renew-client-btn');
    if (renewBtn) {
      await clientAction(
        '/api/clients/renew',
        { id: renewBtn.dataset.id, extend_days: 30 },
        'Client plan extended by 30 days!',
        'success'
      );
      return;
    }

    // 4. Toggle Client
    const toggleBtn = e.target.closest('.toggle-client-btn');
    if (toggleBtn) {
      const enabled = toggleBtn.dataset.enabled === 'true';
      await clientAction(
        '/api/clients/toggle',
        { id: toggleBtn.dataset.id, enabled: enabled },
        `Client ${enabled ? 'enabled' : 'disabled'}`
      );
      return;
    }

    // 5. Delete Client
    const delBtn = e.target.closest('.delete-client-btn');
    if (delBtn) {
      const id = delBtn.dataset.id;
      const name = delBtn.dataset.name;
      const ok = await confirmAction({
        destructive: true,
        title: 'Delete this subscriber?',
        hint: 'PERMANENT — THE ACCOUNT AND ITS TRAFFIC HISTORY ARE GONE',
        // The name is operator-supplied free text and reaches the dialog through
        // textContent, so a name containing markup is shown, not run.
        message: `${name} (${id})\n\nTheir subscription link stops working immediately and their whitelisted address stops resolving. There is no undo — a new account gets a new ID, a new UUID and a new link.`,
        confirmText: 'DELETE SUBSCRIBER',
      });
      if (!ok) return;
      await clientAction('/api/clients/delete', { id: id }, 'Client deleted');
      return;
    }

    // 6. Set the client's whitelisted address
    const addIpBtn = e.target.closest('.add-ip-prompt-btn');
    if (addIpBtn) {
      const id = addIpBtn.dataset.id;
      const current = addIpBtn.dataset.ip || '';
      const ip = await promptForValue({
        title: 'Set the whitelisted address',
        label: 'IPv4 or IPv6 address',
        placeholder: '2.189.86.32',
        value: current,
        // "Set", not "add": the backend stores one address per subscriber and
        // replaces it, because a subscriber is identified by the address they are
        // on right now and their ISP moves it. The old copy said "add", so an
        // operator entering a second address believed they had two.
        message: current
          ? `This replaces the address on file (${current}). A subscriber has one address at a time — the resolver answers whichever one is stored here.`
          : 'A subscriber has one address at a time. The resolver answers the address stored here and refuses every other source.',
        confirmText: 'SAVE ADDRESS',
        validate: (value) => {
          if (!value) return 'Enter an address, or press Cancel to leave it unchanged.';
          if (!isProbablyIP(value)) return 'That is not an IP address. Expected something like 2.189.86.32 or 2001:db8::1.';
          return '';
        },
      });
      if (!ip) return;
      await clientAction('/api/clients/add_ip', { id: id, ip: ip }, `Whitelisted ${ip}`, 'success');
      return;
    }

    // 7. Remove Whitelisted IP
    const remIpBtn = e.target.closest('.remove-client-ip-btn');
    if (remIpBtn) {
      const id = remIpBtn.dataset.id;
      const ip = remIpBtn.dataset.ip;
      const ok = await confirmAction({
        destructive: true,
        title: 'Remove this address?',
        hint: 'THE SUBSCRIBER STOPS RESOLVING IMMEDIATELY',
        message: `${ip}\n\nThis is the only address on file for the account, so removing it leaves nothing whitelisted and every query from them is refused until an address is set again — theirs or one they register from the portal.`,
        confirmText: 'REMOVE ADDRESS',
      });
      if (!ok) return;
      await clientAction('/api/clients/remove_ip', { id: id, ip: ip }, 'IP removed from client');
      return;
    }
  });
}

// =======================================================
// BULLETPROOF COPY & TOAST NOTIFICATIONS
// =======================================================
function fallbackCopy(text) {
  const textArea = document.createElement('textarea');
  textArea.value = text;
  textArea.style.position = 'fixed';
  textArea.style.top = '0';
  textArea.style.left = '0';
  textArea.style.width = '2em';
  textArea.style.height = '2em';
  textArea.style.padding = '0';
  textArea.style.border = 'none';
  textArea.style.outline = 'none';
  textArea.style.boxShadow = 'none';
  textArea.style.background = 'transparent';
  document.body.appendChild(textArea);
  textArea.focus();
  textArea.select();
  try {
    document.execCommand('copy');
  } catch (err) {}
  document.body.removeChild(textArea);
}

function copyText(text, btnElement) {
  if (navigator.clipboard && window.isSecureContext) {
    navigator.clipboard.writeText(text)
      .then(() => {
        showToast(`Copied!`, 'success');
      })
      .catch(() => {
        fallbackCopy(text);
        showToast(`Copied!`, 'success');
      });
  } else {
    fallbackCopy(text);
    showToast(`Copied!`, 'success');
  }

  if (btnElement) {
    btnElement.classList.add('ring-2', 'ring-cyan-400');
    setTimeout(() => {
      btnElement.classList.remove('ring-2', 'ring-cyan-400');
    }, 800);
  }
}

// Copy-to-clipboard, by delegation.
//
// The four copyable boxes in the setup guide used to carry an inline click-handler
// attribute that called copyText with the referenced element's innerText. That works,
// and it is also the reason the Content-Security-Policy still has to allow
// 'unsafe-inline' in script-src — a policy that permits inline script permits any
// inline script an injection manages to place, which is most of what a CSP is there
// to stop. Every inline handler has to go before that can be tightened, so they are
// being converted rather than left alone.
//
// One listener on document is also simply more robust than an attribute per element:
// it keeps working when a view is re-rendered, and a new copyable box becomes markup
// only. The element declares what to copy with data-copy-target="<id>".
function copyFromTarget(host) {
  const id = host.getAttribute('data-copy-target');
  const source = id ? document.getElementById(id) : null;
  // Read the referenced element, not the host: the host also contains the copy icon,
  // and on a box whose text is an IP address a stray glyph is not obvious in the
  // clipboard but is fatal when pasted into a DNS field.
  const text = ((source || host).innerText || '').trim();
  if (text) copyText(text, host);
}

document.addEventListener('click', (e) => {
  const host = e.target.closest('[data-copy-target]');
  if (host) copyFromTarget(host);
});

// The boxes are exposed as buttons, so they have to answer what a button answers.
// Space needs preventDefault or the page scrolls out from under the copy; older
// WebKit reports it as 'Spacebar'.
document.addEventListener('keydown', (e) => {
  if (e.key !== 'Enter' && e.key !== ' ' && e.key !== 'Spacebar') return;
  const host = e.target.closest?.('[data-copy-target]');
  if (!host) return;
  e.preventDefault();
  copyFromTarget(host);
});

// =======================================================
// IN-APP DIALOGS (a promise-based confirm / prompt)
// =======================================================
// window.confirm and window.prompt block the event loop, so the live event
// stream and every chart on the page freeze until the operator answers. Worse,
// a browser is allowed to suppress them outright: once Chrome's "prevent this
// page from creating additional dialogs" box is ticked, confirm() returns false
// and prompt() returns null without asking anyone. A suppressed confirm() turned
// "delete this client" into a silent no-op and a suppressed prompt() into "no IP
// entered" — both indistinguishable from the operator pressing Cancel, on a
// panel whose whole job is destructive administrative actions.
//
// showDialog resolves to true/false for a confirm and to the trimmed string or
// null for a prompt. It never rejects, so a caller still reads as
// `if (!await confirmAction({...})) return;`.
let activeDialog = null;
let dialogSeq = 0;

function showDialog(opts) {
  const o = opts || {};
  const isPrompt = o.kind === 'prompt';
  const cancelValue = isPrompt ? null : false;

  return new Promise((resolve) => {
    // One at a time: a second call while one is open cancels the first rather
    // than stacking two backdrops, two focus traps and two Escape handlers.
    if (activeDialog) activeDialog.finish();

    const returnFocusTo = document.activeElement;
    const seq = ++dialogSeq;
    // Both branches spell every class out in full. A composed name like
    // `border-${accent}-500/40` is invisible to Tailwind's content scanner and
    // would simply be absent from the built stylesheet.
    const tone = o.destructive
      ? {
          icon: 'alert-triangle',
          panel: 'glass-panel p-6 w-full max-w-md shadow-2xl shadow-red-500/10',
          badge: 'w-10 h-10 rounded-xl bg-red-500/20 flex items-center justify-center text-red-400 border border-red-500/30 shrink-0',
          hint: 'text-xs text-red-400 font-mono',
          submit: 'flex-1 py-3 bg-gradient-to-r from-red-600 to-rose-600 hover:from-red-700 hover:to-rose-700 text-onfill font-bold rounded-lg transition duration-200 shadow-lg shadow-red-500/20 text-sm font-heading',
        }
      : {
          icon: 'help-circle',
          panel: 'glass-panel p-6 w-full max-w-md shadow-2xl shadow-cyan-500/10',
          badge: 'w-10 h-10 rounded-xl bg-cyan-500/20 flex items-center justify-center text-cyan-400 border border-cyan-500/30 shrink-0',
          hint: 'text-xs text-cyan-400 font-mono',
          submit: 'flex-1 py-3 bg-gradient-to-r from-cyan-500 to-blue-500 hover:from-cyan-400 text-slate-950 font-bold rounded-lg transition duration-200 shadow-lg shadow-cyan-500/20 text-sm font-heading',
        };
    const backdrop = document.createElement('div');
    backdrop.className = 'fixed inset-0 z-50 flex items-center justify-center bg-black/85 backdrop-blur-md px-4';

    const panel = document.createElement('div');
    panel.className = tone.panel;
    panel.setAttribute('role', 'dialog');
    panel.setAttribute('aria-modal', 'true');
    panel.setAttribute('aria-labelledby', `dialog-title-${seq}`);
    panel.setAttribute('aria-describedby', `dialog-msg-${seq}`);

    const header = document.createElement('div');
    header.className = 'flex items-center gap-3 mb-4';
    const badge = document.createElement('div');
    badge.className = tone.badge;
    badge.setAttribute('aria-hidden', 'true');
    badge.innerHTML = `<i data-feather="${tone.icon}"></i>`;
    const headings = document.createElement('div');
    const title = document.createElement('h2');
    title.id = `dialog-title-${seq}`;
    title.className = 'text-lg font-bold text-white tracking-wide font-heading';
    title.textContent = o.title || 'Confirm';
    headings.appendChild(title);
    if (o.hint) {
      const hint = document.createElement('p');
      hint.className = tone.hint;
      hint.textContent = o.hint;
      headings.appendChild(hint);
    }
    header.appendChild(badge);
    header.appendChild(headings);

    // textContent, not innerHTML: these messages name a client, and a client name
    // is free text that an operator or any API-key holder can set.
    const message = document.createElement('p');
    message.id = `dialog-msg-${seq}`;
    message.className = 'text-xs text-slate-300 leading-relaxed mb-5 whitespace-pre-line';
    message.textContent = o.message || '';

    const form = document.createElement('form');
    form.className = 'space-y-4';
    form.noValidate = true;
    let input = null;
    let errorLine = null;
    if (isPrompt) {
      const field = document.createElement('div');
      const label = document.createElement('label');
      label.className = 'block text-xs font-semibold text-slate-400 mb-1';
      label.setAttribute('for', `dialog-input-${seq}`);
      label.textContent = o.label || 'VALUE';
      input = document.createElement('input');
      input.id = `dialog-input-${seq}`;
      input.type = o.mask ? 'password' : 'text';
      input.className = 'w-full px-4 py-2.5 rounded-lg bg-slate-950/80 border border-slate-700 text-white focus:outline-none focus:border-cyan-400 text-sm font-mono';
      input.value = o.value || '';
      input.autocomplete = 'off';
      input.spellcheck = false;
      if (o.placeholder) input.placeholder = o.placeholder;
      if (o.maxlength) input.maxLength = o.maxlength;
      if (o.inputMode) input.inputMode = o.inputMode;
      errorLine = document.createElement('p');
      errorLine.id = `dialog-error-${seq}`;
      errorLine.className = 'mt-1.5 text-[11px] text-red-400 leading-relaxed hidden';
      // assertive, not polite: this text explains why the button the operator
      // just pressed did nothing, and a polite region waits for a quiet moment
      // that a modal with a single input never provides.
      errorLine.setAttribute('role', 'alert');
      errorLine.setAttribute('aria-live', 'assertive');
      input.setAttribute('aria-describedby', errorLine.id);
      field.appendChild(label);
      field.appendChild(input);
      field.appendChild(errorLine);
      form.appendChild(field);
    }

    const row = document.createElement('div');
    row.className = 'flex gap-3';
    const cancelBtn = document.createElement('button');
    cancelBtn.type = 'button';
    cancelBtn.className = 'px-4 py-3 bg-slate-800 hover:bg-slate-700 text-slate-300 font-bold rounded-lg transition duration-200 text-sm font-heading';
    cancelBtn.textContent = o.cancelText || 'CANCEL';
    const submitBtn = document.createElement('button');
    submitBtn.type = 'submit';
    submitBtn.className = tone.submit;
    submitBtn.textContent = o.confirmText || (isPrompt ? 'SAVE' : 'CONFIRM');
    row.appendChild(cancelBtn);
    row.appendChild(submitBtn);
    form.appendChild(row);
    panel.appendChild(header);
    panel.appendChild(message);
    panel.appendChild(form);
    backdrop.appendChild(panel);
    document.body.appendChild(backdrop);
    // The backdrop is already on screen at this point and none of the handlers below are wired
    // yet, so an exception here would leave a dialog with no working buttons, no Escape and no
    // resolve — a modal the operator cannot get out of. safeFeatherReplace is the same call with
    // that outcome removed.
    safeFeatherReplace();

    let settled = false;
    const finish = (result) => {
      if (settled) return;
      settled = true;
      if (activeDialog === handle) activeDialog = null;
      document.removeEventListener('keydown', onKeydown, true);
      backdrop.remove();
      // Hand focus back to whatever opened this, or a keyboard operator is
      // returned to the top of the document with no idea which row they were on.
      if (returnFocusTo && document.contains(returnFocusTo) && typeof returnFocusTo.focus === 'function') {
        try { returnFocusTo.focus(); } catch (e) { /* the element went away */ }
      }
      resolve(result === undefined ? cancelValue : result);
    };
    const handle = { finish };
    activeDialog = handle;

    const onKeydown = (e) => {
      if (e.key === 'Escape') {
        e.preventDefault();
        finish();
        return;
      }
      if (e.key !== 'Tab') return;
      // A real trap, not just a wrap: focus that has already escaped the panel
      // (a click on the backdrop, a browser-restored focus) is pulled back in.
      const focusables = panel.querySelectorAll('button:not([disabled]), input:not([disabled]), select, textarea, [href]');
      if (!focusables.length) return;
      const first = focusables[0];
      const last = focusables[focusables.length - 1];
      const inside = panel.contains(document.activeElement);
      if (e.shiftKey && (!inside || document.activeElement === first)) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && (!inside || document.activeElement === last)) {
        e.preventDefault();
        first.focus();
      }
    };
    document.addEventListener('keydown', onKeydown, true);
    cancelBtn.addEventListener('click', () => finish());
    // Only a press that lands on the backdrop itself, never one that bubbled up
    // from inside the panel.
    backdrop.addEventListener('mousedown', (e) => {
      if (e.target === backdrop) finish();
    });

    form.addEventListener('submit', (e) => {
      e.preventDefault();
      if (!isPrompt) {
        finish(true);
        return;
      }
      const value = (input.value || '').trim();
      const problem = typeof o.validate === 'function'
        ? o.validate(value)
        : (value ? '' : 'This field cannot be empty.');
      if (problem) {
        // The dialog stays open with the bad value still in it. A native prompt()
        // could only close and be reopened empty, which is why the old IP entry
        // had no validation at all: there was nowhere to put the complaint.
        errorLine.textContent = problem;
        errorLine.classList.remove('hidden');
        input.setAttribute('aria-invalid', 'true');
        input.focus();
        input.select();
        return;
      }
      finish(value);
    });

    if (isPrompt) {
      input.addEventListener('input', () => {
        errorLine.classList.add('hidden');
        input.removeAttribute('aria-invalid');
      });
    }

    // A destructive dialog opens with Cancel focused, so a stray Enter or Space
    // left over from activating the button cannot confirm it.
    if (input) input.focus();
    else if (o.destructive) cancelBtn.focus();
    else submitBtn.focus();
  });
}

function confirmAction(opts) {
  return showDialog(Object.assign({ kind: 'confirm' }, opts));
}

function promptForValue(opts) {
  return showDialog(Object.assign({ kind: 'prompt' }, opts));
}

function showToast(msg, type = 'info') {
  // A 401 body is the auth gate talking, not an error an operator can act on.
  // Rendering its raw "session expired, please login again" text produced a
  // toast that flashed over the login overlay until the reload cleared it, so
  // any such message only re-raises the gate.
  if (/session (?:has )?expired/i.test(String(msg || ''))) {
    showLoginModal();
    return;
  }

  const container = document.getElementById('toast-container');
  if (!container) return;

  const toast = document.createElement('div');
  toast.className = 'glass-panel px-4 py-2.5 rounded-xl border flex items-center gap-2.5 shadow-2xl text-xs font-semibold text-white pointer-events-auto transition-all duration-300 transform translate-y-2 opacity-0';

  // The dot and the label are built as nodes, and the label is set with
  // textContent. This used to be `innerHTML = \`...<span>${msg}</span>\``, and msg
  // is very often not a literal: errorMessage() returns whatever string the
  // response carried, so any handler that echoed part of a request back in its
  // error body could put live markup into the authenticated dashboard.
  const dot = document.createElement('span');
  dot.setAttribute('aria-hidden', 'true');
  const label = document.createElement('span');
  label.textContent = msg === null || msg === undefined ? '' : String(msg);

  if (type === 'success') {
    toast.classList.add('border-emerald-500/50', 'bg-slate-950/90');
    dot.className = 'w-2 h-2 rounded-full bg-emerald-400 animate-pulse shrink-0';
  } else if (type === 'error') {
    toast.classList.add('border-red-500/50', 'bg-slate-950/90');
    dot.className = 'w-2 h-2 rounded-full bg-red-400 shrink-0';
  } else if (type === 'warning') {
    // "It worked, but not the part you were hoping for." The TLS endpoint is the case
    // this exists for: the domain is saved and issuance did not start, which is neither
    // a success nor a failed request.
    toast.classList.add('border-amber-500/50', 'bg-slate-950/90');
    dot.className = 'w-2 h-2 rounded-full bg-amber-400 shrink-0';
  } else {
    toast.classList.add('border-cyan-500/50', 'bg-slate-950/90');
    dot.className = 'w-2 h-2 rounded-full bg-cyan-400 shrink-0';
  }

  toast.appendChild(dot);
  toast.appendChild(label);

  container.appendChild(toast);

  requestAnimationFrame(() => {
    toast.classList.remove('translate-y-2', 'opacity-0');
  });

  // 2.5 s is right for "Saved" and far too short for a reason. The TLS endpoint answers
  // with a whole sentence naming something only the operator can fix — certbot is not
  // installed, port 80 is held — and a message that disappears before it can be read is
  // the same as no message. So the linger follows the length of what is being said.
  const linger = label.textContent.length > 70 ? 9000 : 2500;

  setTimeout(() => {
    toast.classList.add('opacity-0', 'translate-y-2');
    setTimeout(() => {
      if (toast.parentElement) toast.parentElement.removeChild(toast);
    }, 300);
  }, linger);
}


  // =======================================================
  // EDIT CLIENT MODAL EVENT LISTENERS (Advanced Client Controls)
  // =======================================================

  // =======================================================
  // ANT-DESIGN STYLE GREGORIAN DATETIME PICKER
  // =======================================================
  // One picker per form. The create and the edit forms both carry an expiry now,
  // and a shared set of selection variables would have the calendar in one modal
  // showing the date the other modal last confirmed.
  const MONTH_NAMES = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];

  function createExpiryDatePicker(ids) {
    const popup = document.getElementById(ids.popup);
    const openBtn = document.getElementById(ids.openBtn);
    const expiryInput = document.getElementById(ids.expiryInput);
    const monthYearLabel = document.getElementById(ids.monthLabel);
    const daysGrid = document.getElementById(ids.daysGrid);
    const hourSelect = document.getElementById(ids.hourSelect);
    const minuteSelect = document.getElementById(ids.minuteSelect);
    const confirmBtn = document.getElementById(ids.confirmBtn);
    const setNowBtn = document.getElementById(ids.setNowBtn);
    const clearBtn = document.getElementById(ids.clearBtn);

    const noop = { applyDefaultDays: () => {}, clear: () => {} };
    if (!popup || !daysGrid || !hourSelect || !minuteSelect) return noop;

    let selYear = 0;
    let selMonth = 0; // 0-indexed
    let selDay = 0;
    let selHour = 0;
    let selMinute = 0;

    function renderCalendarGrid() {
      if (!monthYearLabel || !daysGrid) return;
      monthYearLabel.innerText = `${MONTH_NAMES[selMonth]} ${selYear}`;

      hourSelect.value = selHour;
      minuteSelect.value = selMinute;

      const firstDayIndex = new Date(selYear, selMonth, 1).getDay();
      const daysInMonth = new Date(selYear, selMonth + 1, 0).getDate();
      const daysInPrevMonth = new Date(selYear, selMonth, 0).getDate();

      let gridHTML = '';

      // Previous month padding days
      for (let i = firstDayIndex - 1; i >= 0; i--) {
        gridHTML += `<div class="p-1.5 text-slate-700 text-[11px] pointer-events-none">${daysInPrevMonth - i}</div>`;
      }

      const today = new Date();
      // Current month days
      for (let d = 1; d <= daysInMonth; d++) {
        const isSelected = (d === selDay);
        const isToday = (today.getFullYear() === selYear && today.getMonth() === selMonth && today.getDate() === d);

        if (isSelected) {
          gridHTML += `<button type="button" class="dp-day-btn p-1.5 rounded-lg bg-blue-600 text-onfill font-bold shadow-md shadow-blue-500/30 text-xs" data-day="${d}">${d}</button>`;
        } else if (isToday) {
          gridHTML += `<button type="button" class="dp-day-btn p-1.5 rounded-lg border border-cyan-400 text-cyan-300 hover:bg-slate-800 text-xs font-bold" data-day="${d}">${d}</button>`;
        } else {
          gridHTML += `<button type="button" class="dp-day-btn p-1.5 rounded-lg text-slate-300 hover:bg-slate-800 hover:text-white transition text-xs" data-day="${d}">${d}</button>`;
        }
      }

      daysGrid.innerHTML = gridHTML;
    }

    // Reads the field this picker owns, or defaults a month out — the same default
    // the create form seeds.
    function parseInputToState() {
      const val = expiryInput ? expiryInput.value.trim() : '';
      if (val) {
        const d = new Date(val.replace(' ', 'T'));
        if (!isNaN(d.getTime())) {
          selYear = d.getFullYear();
          selMonth = d.getMonth();
          selDay = d.getDate();
          selHour = d.getHours();
          selMinute = d.getMinutes();
          return;
        }
      }
      const d = new Date(Date.now() + 30 * 24 * 3600 * 1000);
      selYear = d.getFullYear();
      selMonth = d.getMonth();
      selDay = d.getDate();
      selHour = d.getHours();
      selMinute = d.getMinutes();
    }

    function applyToInput() {
      if (!expiryInput) return;
      const pad = (n) => String(n).padStart(2, '0');
      expiryInput.value = `${selYear}-${pad(selMonth + 1)}-${pad(selDay)} ${pad(selHour)}:${pad(selMinute)}:00`;
    }

    // Populate hours (00-23) and minutes (00-59)
    hourSelect.innerHTML = Array.from({length: 24}, (_, i) => {
      const h = String(i).padStart(2, '0');
      return `<option value="${i}">${h}</option>`;
    }).join('');
    minuteSelect.innerHTML = Array.from({length: 60}, (_, i) => {
      const m = String(i).padStart(2, '0');
      return `<option value="${i}">${m}</option>`;
    }).join('');
    parseInputToState();
    renderCalendarGrid();

    const toggleDatepicker = () => {
      popup.classList.toggle('hidden');
      if (!popup.classList.contains('hidden')) {
        parseInputToState();
        renderCalendarGrid();
      }
    };

    popup.addEventListener('click', (e) => {
      e.stopPropagation();
    });

    if (openBtn) {
      openBtn.addEventListener('click', (e) => {
        e.stopPropagation();
        toggleDatepicker();
      });
    }

    if (expiryInput) {
      expiryInput.addEventListener('click', (e) => {
        e.stopPropagation();
        toggleDatepicker();
      });
    }

    document.getElementById(ids.prevYear)?.addEventListener('click', () => {
      selYear--;
      renderCalendarGrid();
    });
    document.getElementById(ids.nextYear)?.addEventListener('click', () => {
      selYear++;
      renderCalendarGrid();
    });
    document.getElementById(ids.prevMonth)?.addEventListener('click', () => {
      selMonth--;
      if (selMonth < 0) { selMonth = 11; selYear--; }
      renderCalendarGrid();
    });
    document.getElementById(ids.nextMonth)?.addEventListener('click', () => {
      selMonth++;
      if (selMonth > 11) { selMonth = 0; selYear++; }
      renderCalendarGrid();
    });

    daysGrid.addEventListener('click', (e) => {
      const btn = e.target.closest('.dp-day-btn');
      if (btn) {
        selDay = parseInt(btn.getAttribute('data-day'), 10);
        renderCalendarGrid();
      }
    });

    hourSelect.addEventListener('change', (e) => { selHour = parseInt(e.target.value, 10); });
    minuteSelect.addEventListener('change', (e) => { selMinute = parseInt(e.target.value, 10); });

    setNowBtn?.addEventListener('click', () => {
      const future = new Date(Date.now() + 30 * 24 * 3600 * 1000);
      selYear = future.getFullYear();
      selMonth = future.getMonth();
      selDay = future.getDate();
      selHour = future.getHours();
      selMinute = future.getMinutes();
      renderCalendarGrid();
      applyToInput();
    });

    confirmBtn?.addEventListener('click', () => {
      applyToInput();
      popup.classList.add('hidden');
    });

    clearBtn?.addEventListener('click', () => {
      if (expiryInput) expiryInput.value = '';
      popup.classList.add('hidden');
    });

    document.addEventListener('click', (e) => {
      if (!popup.contains(e.target) && e.target !== openBtn && e.target !== expiryInput) {
        popup.classList.add('hidden');
      }
    });

    return {
      // The create form's opening default: a plan expiring a month out.
      applyDefaultDays(days) {
        const d = new Date(Date.now() + days * 24 * 3600 * 1000);
        selYear = d.getFullYear();
        selMonth = d.getMonth();
        selDay = d.getDate();
        selHour = d.getHours();
        selMinute = d.getMinutes();
        renderCalendarGrid();
        applyToInput();
      },
      clear: () => {
        if (expiryInput) expiryInput.value = '';
        popup.classList.add('hidden');
      }
    };
  }

  const editDatePicker = createExpiryDatePicker({
    popup: 'datepicker-popup',
    openBtn: 'open-datepicker-btn',
    expiryInput: 'edit-client-expiry',
    monthLabel: 'dp-month-year-label',
    daysGrid: 'dp-days-grid',
    hourSelect: 'dp-hour-select',
    minuteSelect: 'dp-minute-select',
    confirmBtn: 'dp-confirm-btn',
    setNowBtn: 'dp-set-now-btn',
    clearBtn: 'edit-client-clear-expiry-btn',
    prevYear: 'dp-prev-year',
    nextYear: 'dp-next-year',
    prevMonth: 'dp-prev-month',
    nextMonth: 'dp-next-month'
  });

  const addDatePicker = createExpiryDatePicker({
    popup: 'add-datepicker-popup',
    openBtn: 'add-open-datepicker-btn',
    expiryInput: 'add-client-expiry',
    monthLabel: 'add-dp-month-year-label',
    daysGrid: 'add-dp-days-grid',
    hourSelect: 'add-dp-hour-select',
    minuteSelect: 'add-dp-minute-select',
    confirmBtn: 'add-dp-confirm-btn',
    setNowBtn: 'add-dp-set-now-btn',
    clearBtn: 'add-client-clear-expiry-btn',
    prevYear: 'add-dp-prev-year',
    nextYear: 'add-dp-next-year',
    prevMonth: 'add-dp-prev-month',
    nextMonth: 'add-dp-next-month'
  });

  // The create form's blank slate: the fields the browser's reset() covers plus
  // the readonly expiry field that lives outside it.
  function resetAddClientForm() {
    const form = document.getElementById('add-client-form');
    if (form) form.reset();
    addDatePicker.applyDefaultDays(30);
  }

  // showClientCreatedModal puts the just-minted credentials on screen. The
  // secret lives only in this closure and the readonly input's value — no
  // localStorage, no URL, and the input is type=password until the operator
  // deliberately reveals it, so a screen-share or a passer-by sees a masked
  // field by default. The link is built from the cached subscription origin,
  // the same source the Reg Link button uses, never from window.location.
  function showClientCreatedModal(client) {
    const modal = document.getElementById('client-created-modal');
    const nameEl = document.getElementById('created-client-name');
    const linkEl = document.getElementById('created-register-link');
    const secretEl = document.getElementById('created-register-secret');
    const toggleBtn = document.getElementById('toggle-created-secret-btn');
    if (!modal || !client) return;

    if (nameEl) nameEl.textContent = client.name || client.id || 'client';
    const origin = currentConfig?.subscription_origin || '';
    // /sub/ is the page a subscriber opens (status, quota, IP registration);
    // /ip/ is the API endpoint the portal's register button posts to. The
    // Reg Link button on the client card already made this distinction —
    // this popup was still handing out the API URL.
    if (linkEl) linkEl.value = client.token && origin ? `${origin}/sub/${client.token}` : '(no origin configured — set the public address in Settings)';
    if (secretEl) {
      secretEl.value = client.register_secret || '(no secret in response)';
      secretEl.type = 'password';
    }
    if (toggleBtn) toggleBtn.textContent = 'Show';

    const copyLink = document.getElementById('copy-created-link-btn');
    const copySecret = document.getElementById('copy-created-secret-btn');
    if (copyLink) {
      copyLink.onclick = () => copyText(linkEl ? linkEl.value : '', copyLink);
    }
    if (copySecret) {
      copySecret.onclick = () => copyText(secretEl ? secretEl.value : '', copySecret);
    }
    if (toggleBtn) {
      toggleBtn.onclick = () => {
        if (!secretEl) return;
        const masked = secretEl.type === 'password';
        secretEl.type = masked ? 'text' : 'password';
        toggleBtn.textContent = masked ? 'Hide' : 'Show';
      };
    }
    const doneBtn = document.getElementById('close-client-created-done-btn');
    if (doneBtn) doneBtn.onclick = () => modal.classList.add('hidden');
    // The × in the corner: data-modal-close names it, so Escape and a backdrop
    // click reach it — it needs its own handler or those paths click a dead
    // button.
    const closeBtn = document.getElementById('close-client-created-btn');
    if (closeBtn) closeBtn.onclick = () => modal.classList.add('hidden');

    modal.classList.remove('hidden');
    // The modal a11y observer moves focus to the first focusable control when
    // the class flips; nothing to do here but let it.
  }

  function formatToDateTimeString(dateStr) {
    if (!dateStr || dateStr === '0001-01-01T00:00:00Z' || dateStr.startsWith('0001')) return '';
    try {
      const d = new Date(dateStr);
      if (isNaN(d.getTime())) return '';
      const pad = (n) => String(n).padStart(2, '0');
      return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:00`;
    } catch (e) {
      return '';
    }
  }

  const editClientModal = document.getElementById('edit-client-modal');
  const closeEditClientBtn = document.getElementById('close-edit-client-btn');
  const cancelEditClientBtn = document.getElementById('cancel-edit-client-btn');
  const editClientForm = document.getElementById('edit-client-form');
  const regenUUIDBtn = document.getElementById('edit-client-regen-uuid');
  const regenSecretBtn = document.getElementById('edit-client-regen-secret');
  const resetTrafficBtn = document.getElementById('edit-client-reset-traffic-btn');
  const enabledCheckbox = document.getElementById('edit-client-enabled');
  const statusLabel = document.getElementById('edit-client-status-label');

  if (closeEditClientBtn) {
    closeEditClientBtn.addEventListener('click', () => {
      if (editClientModal) editClientModal.classList.add('hidden');
    });
  }

  if (cancelEditClientBtn) {
    cancelEditClientBtn.addEventListener('click', () => {
      if (editClientModal) editClientModal.classList.add('hidden');
    });
  }

  if (enabledCheckbox && statusLabel) {
    enabledCheckbox.addEventListener('change', () => {
      if (enabledCheckbox.checked) {
        statusLabel.innerText = 'ACTIVE';
        statusLabel.className = 'text-[10px] px-2 py-0.5 rounded-full bg-emerald-500/20 text-emerald-400 font-bold';
      } else {
        statusLabel.innerText = 'DISABLED';
        statusLabel.className = 'text-[10px] px-2 py-0.5 rounded-full bg-red-500/20 text-red-400 font-bold';
      }
    });
  }

  // Open Edit Client Modal from Card Click
  document.addEventListener('click', (e) => {
    const editBtn = e.target.closest('.edit-client-btn');
    if (editBtn) {
      const clientId = editBtn.getAttribute('data-id');
      if (!clientsDataCache || !clientsDataCache.clients) return;
      const client = clientsDataCache.clients.find(c => c.id === clientId);
      if (!client) return;

      document.getElementById('edit-client-id').value = client.id;
      document.getElementById('edit-client-name').value = client.name || '';
      document.getElementById('edit-client-uuid').value = client.uuid || '';
      document.getElementById('edit-client-secret').value = client.register_secret || '';
      document.getElementById('edit-client-ip').value = (client.allowed_ips && client.allowed_ips.length > 0) ? client.allowed_ips.join(', ') : '';
      document.getElementById('edit-client-traffic').value = client.traffic_limit_gb || '';
      document.getElementById('edit-client-max-devices').value = client.max_devices || '';
      // The stored cycle, normalised to the empty option when the record predates
      // cycles or carries a name this build does not offer. A <select> handed an
      // unknown value silently shows its first option, so reopening the modal on such
      // a record and saving would rewrite the cycle to "never" without being asked.
      const cycleSelect = document.getElementById('edit-client-traffic-cycle');
      if (cycleSelect) {
        const stored = client.traffic_reset_cycle || '';
        const known = Array.prototype.some.call(cycleSelect.options, (o) => o.value === stored);
        cycleSelect.value = known ? stored : '';
      }
      document.getElementById('edit-client-expiry').value = formatToDateTimeString(client.expires_at);
      document.getElementById('edit-client-note').value = client.note || '';

      if (enabledCheckbox) {
        enabledCheckbox.checked = client.enabled !== false;
        enabledCheckbox.dispatchEvent(new Event('change'));
      }

      if (editClientModal) editClientModal.classList.remove('hidden');
      safeFeatherReplace();
    }

    // Copy UUID to clipboard
    const copyUuidBtn = e.target.closest('.copy-uuid-btn');
    if (copyUuidBtn) {
      const uuid = copyUuidBtn.getAttribute('data-uuid');
      if (uuid) {
        navigator.clipboard.writeText(uuid);
        showToast('UUID copied to clipboard!', 'success');
      }
    }
  });

  // Regenerate UUID Button inside Edit Modal
  if (regenUUIDBtn) {
    regenUUIDBtn.addEventListener('click', async () => {
      const clientId = document.getElementById('edit-client-id').value;
      if (!clientId) return;
      try {
        const res = await fetch(api(`/api/clients/${clientId}/regenerate-uuid`), {
          method: 'POST',
          headers: { 'Authorization': `Bearer ${authToken}` }
        });
        if (res.ok) {
          const data = await res.json();
          document.getElementById('edit-client-uuid').value = data.uuid;
          showToast('New UUID generated!', 'success');
        }
      } catch (e) {
        showToast('Failed to regenerate UUID', 'error');
      }
    });
  }

  // Regenerate Register Secret inside Edit Modal (Phase B): the out-of-band
  // credential dies with this click, so the subscriber needs the new value
  // through the same channel the original came from.
  if (regenSecretBtn) {
    regenSecretBtn.addEventListener('click', async () => {
      const clientId = document.getElementById('edit-client-id').value;
      if (!clientId) return;
      try {
        const res = await fetch(api(`/api/clients/${clientId}/regenerate-register-secret`), {
          method: 'POST',
          headers: { 'Authorization': `Bearer ${authToken}` }
        });
        if (res.ok) {
          const data = await res.json();
          document.getElementById('edit-client-secret').value = data.register_secret;
          showToast('New registration secret generated!', 'success');
        } else {
          showToast(await errorMessage(res, 'Failed to regenerate the secret'), 'error');
        }
      } catch (e) {
        showToast('Failed to regenerate the secret', 'error');
      }
    });
  }

  // Reset Traffic Button inside Edit Modal
  if (resetTrafficBtn) {
    resetTrafficBtn.addEventListener('click', async () => {
      const clientId = document.getElementById('edit-client-id').value;
      if (!clientId) return;
      try {
        const res = await fetch(api(`/api/clients/${clientId}/reset-traffic`), {
          method: 'POST',
          headers: { 'Authorization': `Bearer ${authToken}` }
        });
        if (res.ok) {
          showToast('Client traffic counter reset to 0!', 'success');
          loadClients();
        } else {
          showToast('Failed to reset traffic counter', 'error');
        }
      } catch (e) {
        showToast('Network error resetting traffic', 'error');
      }
    });
  }

  // Edit Client Save.
  //
  // Bound twice on purpose: the form's submit (Enter in a field) and the Save
  // button's own click. A single submit binding on the form used to be the only
  // way in, so if anything upstream left the handler unattached the button fell
  // back to the browser's native submit — a full page reload with the edit
  // silently dropped — and an exception while reading a field produced no
  // toast at all. The shared, guarded handler below means either path saves
  // once, every outcome is visible, and success always closes the card.
  if (editClientForm) {
    let editSaveInFlight = false;

    const saveEditClient = async (e) => {
      if (e && typeof e.preventDefault === 'function') e.preventDefault();
      if (editSaveInFlight) return;
      editSaveInFlight = true;

      try {
        const clientId = document.getElementById('edit-client-id')?.value?.trim() || '';
        if (!clientId) return;

        const val = (id) => document.getElementById(id)?.value ?? '';
        const name = val('edit-client-name').trim();
        const uuid = val('edit-client-uuid').trim();
        const ip = val('edit-client-ip').trim();
        const trafficGB = parseFloat(val('edit-client-traffic')) || 0;
        const cycle = val('edit-client-traffic-cycle');
        const maxDevices = parseInt(val('edit-client-max-devices'), 10) || 0;
        const expiryVal = val('edit-client-expiry');
        const note = val('edit-client-note').trim();
        const isEnabled = enabledCheckbox ? enabledCheckbox.checked : true;

        let expiresAtISO = '0001-01-01T00:00:00Z';
        if (expiryVal) {
          // The picker writes "YYYY-MM-DD HH:mm:ss"; Safari does not parse that
          // space-separated shape, so the T form is tried first and the plain
          // form is kept as the fallback for everything else.
          const parsed = new Date(expiryVal.replace(' ', 'T'));
          if (!isNaN(parsed.getTime())) {
            expiresAtISO = parsed.toISOString();
          }
        }

        const payload = {
          name: name,
          uuid: uuid,
          // Split the comma/newline-separated field into the list the server
          // stores, so an operator can attach a host address and a CIDR together.
          allowed_ips: ip.split(/[\s,]+/).map(s => s.trim()).filter(Boolean),
          traffic_limit_gb: trafficGB,
          max_devices: maxDevices,
          // Sent on every save, including as "" — the field is a pointer on the
          // server, so omitting it means "leave the cycle alone" and there would
          // then be no way to turn a cycle back off from this form.
          traffic_reset_cycle: cycle,
          expires_at: expiresAtISO,
          enabled: isEnabled,
          note: note
        };

        const res = await fetch(api(`/api/clients/${clientId}`), {
          method: 'PUT',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${authToken}`
          },
          body: JSON.stringify(payload)
        });

        if (res.ok) {
          // The server answers with the stored record. Compare its IP list to the
          // one submitted: host addresses above Max Devices are trimmed on write
          // (CIDRs are exempt), and a silent trim reads as "my address was not
          // saved". Say which addresses actually landed instead.
          const saved = await res.json().catch(() => null);
          const sentIPs = payload.allowed_ips;
          const savedIPs = (saved && Array.isArray(saved.allowed_ips)) ? saved.allowed_ips : null;
          const trimmed = savedIPs && (savedIPs.length !== sentIPs.length ||
            savedIPs.some(ip => !sentIPs.includes(ip)));
          if (trimmed) {
            showToast(
              'Saved, but the device limit keeps ' + savedIPs.length +
              ' host address(es): ' + (savedIPs.join(', ') || 'none') + '.',
              'warning'
            );
          } else {
            showToast('Client configuration updated successfully!', 'success');
          }
          if (editClientModal) editClientModal.classList.add('hidden');
          await loadClients();
        } else {
          // "Invalid traffic reset cycle", "IP already assigned to another
          // client", "UUID already in use" — each is a 400 with a different fix,
          // and the generic message used to send the operator back to guess.
          showToast(await errorMessage(res, 'Failed to update client details'), 'error');
        }
      } catch (err) {
        console.error('Saving the client failed', err);
        showToast('Could not save the client: ' + (err && err.message ? err.message : 'unexpected error'), 'error');
      } finally {
        editSaveInFlight = false;
      }
    };

    editClientForm.addEventListener('submit', saveEditClient);
    document.getElementById('edit-client-save-btn')?.addEventListener('click', saveEditClient);

  }

// =======================================================
// SELF-UPDATE: check the main-branch version, then apply a
// SHA256-verified binary swap + restart. Data is preserved and backed up.
// =======================================================
async function checkForUpdate() {
  try {
    const res = await fetch(api('/api/update/check'), { headers: { 'Authorization': 'Bearer ' + authToken } });
    if (!res.ok) return;
    const st = await res.json();
    renderServerUpdate(st);
    if (st && st.update_available && st.supported) showUpdateAvailable(st);
  } catch (e) { /* an offline check is non-fatal — no badge, no error */ }
}

// ---- Server Update card (Settings & SSL) -------------------------------------
//
// A second entry point to the self-updater the sidebar badge already uses. The
// card only checks; "Update now" hands the run to openUpdateModal(), so the
// verification, backup, restart and progress reporting stay in that one place.

let serverUpdateState = null;

function serverUpdateStatusText(st) {
  if (!st) return 'Not checked yet.';
  if (st.error) {
    return st.container
      ? 'Could not check right now — Docker controls remain available.'
      : 'Could not check right now.';
  }
  if (st.update_available) return `Update available: v${st.current} → v${st.latest}`;
  return `Up to date (v${st.current}).`;
}

function serverUpdateMetaText(st) {
  if (!st) return 'Updates come from the GitHub release channel.';
  if (st.error) {
    return st.container
      ? 'The update method below is unchanged; retry when GitHub is reachable.'
      : 'Retry when the server can reach GitHub.';
  }
  if (st.update_available && !st.supported) {
    return st.note || 'This install cannot apply it from the app.';
  }
  if (st.update_available && st.supported) {
    return st.note || 'Applying an update restarts the service.';
  }
  return 'Checked just now · data is preserved and backed up.';
}

function renderServerUpdate(st) {
  const prev = serverUpdateState || {};
  if (st && st.error) {
    // A failed check must not erase what the previous answer established about
    // this install. In particular container:true keeps the Docker update-method
    // block open instead of collapsing the card on a transient GitHub failure.
    st = {
      ...prev,
      ...st,
      container: st.container !== undefined ? st.container : prev.container,
      update_mode: st.update_mode || prev.update_mode,
      container_swap_active: st.container_swap_active !== undefined
        ? st.container_swap_active
        : prev.container_swap_active,
      container_image_version: st.container_image_version || prev.container_image_version,
    };
  }
  serverUpdateState = st;
  const statusEl = document.getElementById('server-update-status');
  if (statusEl) {
    statusEl.textContent = serverUpdateStatusText(st);
    const tone = st && st.error ? 'text-amber-300'
      : st && st.update_available ? 'text-cyan-300'
      : 'text-slate-300';
    statusEl.className = 'text-xs mt-1 ' + tone;
  }
  const metaEl = document.getElementById('server-update-meta');
  if (metaEl) metaEl.textContent = serverUpdateMetaText(st);
  const applyBtn = document.getElementById('server-update-apply-btn');
  if (applyBtn) applyBtn.classList.toggle('hidden', !(st && st.update_available && st.supported));
  const errEl = document.getElementById('server-update-error');
  if (errEl) {
    const message = st && st.error ? String(st.error) : '';
    errEl.textContent = message;
    errEl.classList.toggle('hidden', !message);
  }

  // Container-only controls. The block is absent on a systemd install, where
  // there is nothing to choose.
  const modeBox = document.getElementById('server-update-mode');
  if (modeBox) modeBox.classList.toggle('hidden', !(st && st.container));
  const mode = (st && st.update_mode) || 'image';
  document.querySelectorAll('input[name="server-update-mode"]').forEach((el) => {
    el.checked = el.value === mode;
    el.disabled = !(st && st.container);
  });
  const hint = document.getElementById('server-update-mode-hint');
  if (hint) {
    hint.textContent = mode === 'binary'
      ? 'Temporary: a recreate restores the image binary; the image tag and digest stay unchanged.'
      : 'The panel refuses the in-container swap and shows the pull/recreate command.';
  }
  const notes = document.getElementById('server-update-notes');
  if (notes) {
    if (st && st.latest) {
      // setAttribute keeps this out of the "off-origin resource" scan: it is a
      // navigation link, not something the page fetches.
      notes.setAttribute('href', 'https://github.com/digitrob/DigitDNS/releases/tag/v' + encodeURIComponent(st.latest));
      notes.classList.remove('hidden');
    } else {
      notes.classList.add('hidden');
    }
  }
  const temp = document.getElementById('server-update-temp');
  if (temp) {
    const active = !!(st && st.container && st.container_swap_active);
    temp.classList.toggle('hidden', !active);
    if (active) {
      const image = st.container_image_version || 'the image';
      temp.textContent = `Temporary binary update active — a container recreate restores v${image}.`;
    }
  }
}

// setContainerUpdateMode persists the container-only choice and reflects it in
// the card without another GitHub round trip. The server is the source of truth
// on the next "Check for updates".
async function setContainerUpdateMode(mode) {
  if (!mode) return;
  try {
    const res = await fetch(api('/api/update/mode'), {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${authToken}`,
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({ mode }),
    });
    if (!res.ok) throw new Error(await errorMessage(res, 'Could not save the update method.'));
    const note = mode === 'binary'
      ? 'Temporary binary update: a recreate restores the image\'s binary.'
      : 'Update the image: docker compose pull && docker compose up -d';
    renderServerUpdate({
      ...(serverUpdateState || {}),
      update_mode: mode,
      supported: mode === 'binary',
      note,
      error: '',
    });
  } catch (e) {
    renderServerUpdate({
      ...(serverUpdateState || {}),
      error: e.message || 'Could not save the update method.',
    });
  }
}

async function checkServerUpdate() {
  const btn = document.getElementById('server-update-check-btn');
  const label = btn ? btn.querySelector('span') : null;
  if (btn) btn.disabled = true;
  if (label) label.textContent = 'Checking…';
  try {
    // force=1 bypasses the server's one-minute cache: this is the manual
    // retry, and it must not repeat a stale failure after the operator fixed
    // the network.
    const res = await fetch(api('/api/update/check?force=1'), {
      headers: { 'Authorization': `Bearer ${authToken}` },
      cache: 'no-store',
    });
    if (!res.ok) throw new Error(await errorMessage(res, 'The update check failed.'));
    const st = await res.json();
    renderServerUpdate(st);
    if (st && st.update_available && st.supported) showUpdateAvailable(st);
  } catch (e) {
    renderServerUpdate({
      ...(serverUpdateState || {}),
      current: (serverUpdateState && serverUpdateState.current) || '',
      latest: '',
      update_available: false,
      supported: false,
      error: e.message || 'The update check failed.',
    });
  } finally {
    if (btn) btn.disabled = false;
    if (label) label.textContent = 'Check for updates';
  }
}

function applyServerUpdate() {
  if (!serverUpdateState) return;
  openUpdateModal(serverUpdateState);
}

let hdnsUpdateBtn = null;
function showUpdateAvailable(st) {
  if (!hdnsUpdateBtn) {
    const btn = document.createElement('button');
    btn.id = 'update-available-btn';
    btn.type = 'button';
    btn.className = 'mt-1 inline-flex items-center gap-1.5 text-[10px] font-bold px-2 py-1 rounded-lg bg-amber-500/20 text-amber-300 border border-amber-500/40 hover:bg-amber-500/30 transition';
    btn.addEventListener('click', () => openUpdateModal(st));
    // The version badge this used to sit next to is gone (the sidebar shows
    // only the wordmark now), so the anchor is the brand row itself.
    const anchor = document.getElementById('sidebar-brand');
    if (anchor) anchor.appendChild(btn);
    else document.body.appendChild(btn);
    hdnsUpdateBtn = btn;
  }
  hdnsUpdateBtn.innerHTML = `<i data-feather="download-cloud" class="w-3 h-3"></i> Update &rarr; v${escapeHTML(st.latest)}`;
  hdnsUpdateBtn.classList.remove('hidden');
  safeFeatherReplace();
}
let hdnsUpdateModal = null;
function openUpdateModal(st) {  if (hdnsUpdateModal) hdnsUpdateModal.remove();
  const backdrop = document.createElement('div');
  backdrop.className = 'fixed inset-0 z-50 flex items-center justify-center bg-black/85 backdrop-blur-md px-4';
  backdrop.innerHTML = `
    <div class="glass-panel p-6 w-full max-w-md shadow-2xl shadow-cyan-500/10" role="dialog" aria-modal="true" aria-labelledby="hdns-update-title">
      <div class="flex items-center gap-3 mb-4">
        <div class="w-10 h-10 rounded-xl bg-cyan-500/20 flex items-center justify-center text-cyan-400 border border-cyan-500/30 shrink-0"><i data-feather="download-cloud"></i></div>
        <div>
          <h2 id="hdns-update-title" class="text-lg font-bold text-white font-heading">Update available</h2>
          <p class="text-xs text-cyan-400 font-mono">v${escapeHTML(st.current)} &rarr; v${escapeHTML(st.latest)}${st.codename ? ' · ' + escapeHTML(st.codename) : ''}</p>
        </div>
      </div>
      <p class="text-sm text-slate-300 mb-4">The new binary is downloaded from GitHub, its SHA-256 is verified against the release checksums, your data is backed up, and the service restarts on the new version. Subscribers, clients and settings are preserved.</p>
      <div id="hdns-update-progress-wrap" class="hidden mb-4">
        <div class="h-2 w-full rounded-full bg-slate-800 overflow-hidden"><div id="hdns-update-bar" class="h-full bg-gradient-to-r from-cyan-500 to-blue-500 transition-all duration-300" style="width:0%"></div></div>
        <p id="hdns-update-msg" class="text-xs text-slate-400 mt-1 font-mono">Starting…</p>
      </div>
      <div class="flex gap-3">
        <button id="hdns-update-cancel" type="button" class="flex-1 py-3 bg-slate-800 hover:bg-slate-700 text-slate-200 font-bold rounded-lg text-sm">Cancel</button>
        <button id="hdns-update-go" type="button" class="flex-1 py-3 bg-gradient-to-r from-cyan-500 to-blue-500 hover:from-cyan-400 text-slate-950 font-bold rounded-lg text-sm">Update now</button>
      </div>
    </div>`;
  document.body.appendChild(backdrop);
  hdnsUpdateModal = backdrop;
  safeFeatherReplace();
  const close = () => { backdrop.remove(); hdnsUpdateModal = null; };
  backdrop.querySelector('#hdns-update-cancel').addEventListener('click', close);
  backdrop.addEventListener('click', (e) => { if (e.target === backdrop) close(); });
  backdrop.querySelector('#hdns-update-go').addEventListener('click', () => applyUpdate(backdrop));
}
async function applyUpdate(modal) {
  const go = modal.querySelector('#hdns-update-go');
  const cancel = modal.querySelector('#hdns-update-cancel');
  const wrap = modal.querySelector('#hdns-update-progress-wrap');
  const bar = modal.querySelector('#hdns-update-bar');
  const msg = modal.querySelector('#hdns-update-msg');
  const lock = (on) => { go.disabled = on; go.classList.toggle('opacity-50', on); };
  lock(true);
  wrap.classList.remove('hidden');

  try {
    const res = await fetch(api('/api/update/apply'), { method: 'POST', headers: { 'Authorization': 'Bearer ' + authToken } });
    if (!res.ok) {
      msg.textContent = await errorMessage(res, 'Could not start the update');
      lock(false);
      return;
    }
  } catch (e) {
    msg.textContent = 'Network error starting the update';
    lock(false);
    return;
  }

  const poll = setInterval(async () => {
    let p;
    try {
      const r = await fetch(api('/api/update/status'), { headers: { 'Authorization': 'Bearer ' + authToken }, cache: 'no-store' });
      p = await r.json();
    } catch (e) {
      // The restart drops the connection — that is the success path, not a failure.
      clearInterval(poll);
      bar.style.width = '100%';
      msg.textContent = 'Restarting on the new version… this page will reload automatically.';
      waitForServerBack();
      return;
    }
    bar.style.width = (p.percent || 0) + '%';
    msg.textContent = p.message || p.phase || '';
    if (p.phase === 'error') {
      clearInterval(poll);
      msg.textContent = 'Update failed: ' + (p.error || 'unknown error');
      cancel.textContent = 'Close';
    } else if (p.restarting) {
      clearInterval(poll);
      bar.style.width = '100%';
      msg.textContent = 'Restarting on the new version… this page will reload automatically.';
      waitForServerBack();
    } else if (!p.running) {
      // The daemon may come back from the restart before a poll observes the
      // "restarting" flag — the in-memory progress state is then idle. Without
      // this branch the modal would poll a finished update every 1.5 s forever
      // with an empty message.
      clearInterval(poll);
      bar.style.width = '100%';
      msg.textContent = 'Update finished. Reloading…';
      waitForServerBack();
    }
  }, 1500);
}

function waitForServerBack() {
  let tries = 0;
  const probe = setInterval(async () => {
    tries++;
    try {
      const r = await fetch(api('/api/update/check'), { headers: { 'Authorization': 'Bearer ' + authToken }, cache: 'no-store' });
      if (r.ok || r.status === 401) { clearInterval(probe); location.reload(); return; }
    } catch (e) { /* still restarting */ }
    if (tries > 40) { clearInterval(probe); location.reload(); }
  }, 3000);
}

// =======================================================
// FIRST-RUN CHECKLIST
// =======================================================
// The state is derived server-side from the live configuration, so a restore or
// a manual change moves the checklist without a stored "done" flag. Dismissal is
// browser-local: it is a preference about the card, not a claim about the install.
const ONBOARDING_DISMISSED_KEY = 'digitdns_onboarding_dismissed';

function onboardingDismissed() {
  try { return localStorage.getItem(ONBOARDING_DISMISSED_KEY) === '1'; } catch (e) { return false; }
}

async function loadOnboarding() {
  const card = document.getElementById('onboarding-card');
  if (!card) return;
  if (onboardingDismissed()) {
    card.classList.add('hidden');
    return;
  }
  try {
    const res = await fetch(api('/api/onboarding'), {
      headers: { 'Authorization': 'Bearer ' + authToken },
      cache: 'no-store',
    });
    if (!res.ok) return;
    renderOnboarding(await res.json());
  } catch (e) { /* the checklist is advisory, never a boot blocker */ }
}

function renderOnboarding(data) {
  const card = document.getElementById('onboarding-card');
  const stepsEl = document.getElementById('onboarding-steps');
  const progress = document.getElementById('onboarding-progress');
  if (!card || !stepsEl) return;
  if (!data || data.all_done || onboardingDismissed()) {
    card.classList.add('hidden');
    return;
  }
  const steps = Array.isArray(data.steps) ? data.steps : [];
  card.classList.remove('hidden');
  if (progress) {
    progress.textContent = `${data.completed || 0} of ${data.total || steps.length} steps complete`;
  }
  stepsEl.innerHTML = '';
  for (const step of steps) {
    const btn = document.createElement('button');
    btn.type = 'button';
    btn.className = 'onboarding-step' + (step.done ? ' done' : '');
    btn.dataset.onboardingTab = step.tab || '';
    btn.dataset.onboardingRoute = step.route || '';
    btn.innerHTML = `<span class="onboarding-mark">${step.done ? '✓' : ''}</span>` +
      `<span><span class="onboarding-step-title">${escapeHTML(step.title || '')}</span>` +
      `<span class="onboarding-step-desc">${escapeHTML(step.description || '')}</span></span>`;
    if (!step.done && step.tab) {
      btn.addEventListener('click', () => switchTab(step.tab));
    }
    stepsEl.appendChild(btn);
  }
  safeFeatherReplace();
}

function initOnboarding() {
  document.getElementById('onboarding-dismiss-btn')?.addEventListener('click', () => {
    try { localStorage.setItem(ONBOARDING_DISMISSED_KEY, '1'); } catch (e) { /* private mode */ }
    document.getElementById('onboarding-card')?.classList.add('hidden');
  });
}

// =======================================================
// CHANGE PREVIEW
// =======================================================
// The explicit Save buttons show a redacted line diff before the write leaves
// the browser. The renderer is shared by every preview so a rules save and an
// ad-blocking save read the same way.
let diffResolve = null;

function prettyJSON(value) {
  if (value === undefined || value === null || value === '') return '';
  if (typeof value === 'string') {
    try { return JSON.stringify(JSON.parse(value), null, 2); } catch (e) { return value; }
  }
  try { return JSON.stringify(value, null, 2); } catch (e) { return String(value); }
}

function diffLines(before, after) {
  const A = before ? before.split('\n') : [];
  const B = after ? after.split('\n') : [];
  if (A.length > 400 || B.length > 400) {
    const cap = 200;
    const out = [];
    A.slice(0, cap).forEach((l) => out.push({ t: 'del', l }));
    if (A.length > cap) out.push({ t: 'ctx', l: `… ${A.length - cap} more line(s) in the previous value` });
    B.slice(0, cap).forEach((l) => out.push({ t: 'add', l }));
    if (B.length > cap) out.push({ t: 'ctx', l: `… ${B.length - cap} more line(s) in the new value` });
    return out;
  }
  const dp = Array.from({ length: A.length + 1 }, () => new Array(B.length + 1).fill(0));
  for (let i = A.length - 1; i >= 0; i--) {
    for (let j = B.length - 1; j >= 0; j--) {
      dp[i][j] = A[i] === B[j] ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1]);
    }
  }
  const out = [];
  let i = 0;
  let j = 0;
  while (i < A.length && j < B.length) {
    if (A[i] === B[j]) { out.push({ t: 'ctx', l: A[i] }); i++; j++; }
    else if (dp[i + 1][j] >= dp[i][j + 1]) { out.push({ t: 'del', l: A[i] }); i++; }
    else { out.push({ t: 'add', l: B[j] }); j++; }
  }
  while (i < A.length) out.push({ t: 'del', l: A[i++] });
  while (j < B.length) out.push({ t: 'add', l: B[j++] });
  return out;
}

function diffHtml(before, after) {
  return diffLines(before, after).map(({ t, l }) => {
    const prefix = t === 'add' ? '+ ' : t === 'del' ? '- ' : '  ';
    return `<span class="audit-diff-line ${escapeHTML(t)}">${escapeHTML(prefix + l)}</span>`;
  }).join('');
}


function showDiffConfirm(title, before, after, confirmLabel = 'Save changes') {
  const modal = document.getElementById('diff-modal');
  const body = document.getElementById('diff-body');
  if (!modal || !body) return Promise.resolve(true);
  const beforeText = prettyJSON(before);
  const afterText = prettyJSON(after);
  if (beforeText === afterText) return Promise.resolve(true);

  const lines = diffLines(beforeText, afterText);
  const added = lines.filter((l) => l.t === 'add').length;
  const removed = lines.filter((l) => l.t === 'del').length;
  const titleEl = document.getElementById('diff-title');
  const sub = document.getElementById('diff-subtitle');
  const confirm = document.getElementById('diff-confirm-btn');
  if (titleEl) titleEl.textContent = title || 'Review changes';
  if (sub) {
    sub.textContent = `${added} line${added === 1 ? '' : 's'} added · ${removed} line${removed === 1 ? '' : 's'} removed`;
  }
  body.innerHTML = '<div class="audit-diff"><div class="audit-diff-title">Proposed change</div>' +
    `<pre class="audit-diff-body">${diffHtml(beforeText, afterText)}</pre></div>`;
  if (confirm) confirm.textContent = confirmLabel || 'Save changes';
  modal.classList.remove('hidden');
  safeFeatherReplace();
  return new Promise((resolve) => { diffResolve = resolve; });
}

function settleDiff(ok) {
  const modal = document.getElementById('diff-modal');
  if (modal) modal.classList.add('hidden');
  const resolve = diffResolve;
  diffResolve = null;
  if (resolve) resolve(ok);
}

function initDiffModal() {
  document.getElementById('diff-confirm-btn')?.addEventListener('click', () => settleDiff(true));
  document.getElementById('diff-cancel-btn')?.addEventListener('click', () => settleDiff(false));
  document.getElementById('diff-cancel-x')?.addEventListener('click', () => settleDiff(false));
}
