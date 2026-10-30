// =======================================================
// DigitDNS Front-End Application Logic (ES6)
// Bulletproof, Offline-Safe, Self-Healing
// =======================================================


// authToken is empty until a real login. Earlier builds seeded the literal
// 'hdns_session_admin' here and wrote it to localStorage, which the server has
// never accepted — it only produced a guaranteed 401 on first load and made a
// logged-out browser look logged in.
let authToken = localStorage.getItem('digitdns_token') || localStorage.getItem('hyperdns_token') || '';
// Carry a pre-rename session forward once, then keep only the current key. The
// token itself is unchanged; this is a storage-key migration, not a new session.
if (authToken && !localStorage.getItem('digitdns_token')) {
  try {
    localStorage.setItem('digitdns_token', authToken);
    localStorage.removeItem('hyperdns_token');
  } catch (e) {}
}

// The admin namespace. Since v2.1 the panel is not at the root of the host: the
// SPA lives at /<admin-path>/dash/... and every dashboard API at
// /<admin-path>/api/..., where <admin-path> is a 16-hex-character value drawn
// once per install. The bundle is built without knowing it — the same bytes ship
// to every install, and baking one in would publish the hidden path — so it is
// read from the address bar instead. The shape is exactly what the server
// generates (16 lowercase hex characters, security.go's IsValidAdminPath), so a
// match is reliable, and anything else means the page was reached some other way,
// in which case the empty prefix degrades to the pre-v2.1 root paths.
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
let qpsChart = null;
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
      dbEl.textContent = 'GeoIP database: not downloaded yet — press Update database';
    } else {
      const date = status.database_date || 'date unknown';
      dbEl.textContent = `GeoIP database: ${date} · updates daily`;
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

document.addEventListener('DOMContentLoaded', () => {
  safeFeatherReplace();
  try { initChart(); } catch (e) {}
  // Before checkAuthAndBoot, which unhides the login overlay synchronously: the observers
  // inside have to be watching already or they miss the modal that is up first.
  initModalA11y();
  checkAuthAndBoot();

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
      // Not trimmed: the password is compared byte for byte, and a pre-v1.5.0
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
        try { localStorage.removeItem('hyperdns_token'); } catch (e) {}
        hideLoginModal();
        bootDashboard();

        if (data.password_weak || data.is_default_password) {
          showChangePwdModal();
        }
      } catch (err) {
        if (errDiv) {
          errDiv.innerText = 'Server connection failed';
          errDiv.classList.remove('hidden');
        }
      }
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
      // v2.1.0 (B-04 remediation): the second factor rides the same body. The
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
        try { localStorage.removeItem('hyperdns_token'); } catch (e) {}
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
    try { localStorage.removeItem('hyperdns_token'); } catch (e) {}
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

function hideLoginModal() {
  document.getElementById('login-modal')?.classList.add('hidden');
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
const MODAL_IDS = ['login-modal', 'change-pwd-modal', 'diagnostics-modal', 'edit-client-modal', 'add-client-modal', 'client-created-modal', 'bulk-import-modal'];

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
  const authed = await loadConfig();
  if (!authed) return; // 401 -> login modal is already shown
  await loadClients();
  startStatsPolling();
  startLiveStream();
  handleRouteFromURL();
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
    const res = await fetch(api('/api/auth/me'), {
      headers: { 'Authorization': `Bearer ${authToken}` }
    });

    if (res.status === 401) {
      showLoginModal();
      return false;
    }
    if (!res.ok) {
      showToast(await errorMessage(res, 'Could not read the account state.'), 'error');
      return false;
    }

    const meData = await res.json();
    const weak = !!(meData.password_weak || meData.is_default_password);

    const cfgRes = await fetch(api('/api/config'), {
      headers: { 'Authorization': `Bearer ${authToken}` }
    });
    if (cfgRes.status === 401) {
      showLoginModal();
      return false;
    }
    if (!cfgRes.ok) {
      showToast(await errorMessage(cfgRes, 'Could not load the configuration.'), 'error');
      return false;
    }
    currentConfig = await cfgRes.json();
    renderConfig(currentConfig);
    loadResolutionSettings();
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

  render('dot-cert-details', cfg.dot_cert, "Let's Encrypt", 'Renew', 'issue-dot-ssl-btn');
  render('panel-cert-details', cfg.panel_cert, 'Issue SSL Certificate', 'Renew SSL Certificate', 'issue-ssl-btn');

  // The DoH/DoT auto-renew toggle's live state. The snapshot carries the
  // positive form; a config predating the field reads as undefined, and
  // defaulting that to checked matches the daemon's "renewal keeps running"
  // reading of the same absence.
  const dotAutoRenew = document.getElementById('dot-auto-renew-input');
  if (dotAutoRenew) {
    dotAutoRenew.checked = !cfg.tls || cfg.tls.dot_auto_renew_acme !== false;
  }
}

function renderConfig(cfg) {
  if (!cfg) return;
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

  // The subscription record may be absent on a pre-v2.1 config response; an
  // empty object keeps the renderers below total.
  if (!cfg.subscription || typeof cfg.subscription !== 'object') cfg.subscription = {};

  // The v2.1 panels read straight from the config they were handed.
  if (typeof window.renderSubscriptionSettings === 'function') window.renderSubscriptionSettings();
  if (typeof window.renderAdminPath === 'function') window.renderAdminPath();
  if (typeof window.renderTwoFactor === 'function') window.renderTwoFactor();

  // Version badge (single source of truth: version.json embedded in the binary).
  //
  // The mobile header deliberately gets the version *without* the commit hash. The two strings
  // differ by eleven characters — "v1.5.0-beta" against "v1.5.0-beta [3585f9bf]" — and at 9px
  // mono in a 56px sticky header that is the difference between a badge sitting beside the
  // wordmark and one that wraps, taking the brand line out through the top border. The hash
  // still reaches both places it is actually used from: the desktop sidebar badge, which has
  // Version badge displays: header and mobile show only version (e.g. "v2.0.0-beta"),
  // footer shows version + hash for bug reports (e.g. "v2.0.0-beta [3585f9bf]").
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

  // v2.6: once the running version is known, ask the server whether a newer one
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
  renderProxiedList();
  proxiedFillViewport();
  renderList('custom-blocked-list', cfg.rules?.custom_blocked || [], 'remove-blocked');
  renderList('tokens-list', cfg.access?.doh_tokens || [], 'remove-token');
  renderCustomRecords(cfg.rules?.custom_records || {});
  loadCustomGroups();
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
  proxy: { label: 'PROXIED', badge: 'badge-proxy', domainClass: 'text-cyan-300' },
  paused: { label: 'PAUSED', badge: 'badge-unknown', domainClass: 'text-slate-400' },
  blocked: { label: 'BLOCKED', badge: 'badge-block', domainClass: 'text-red-300' },
  direct: { label: 'DIRECT', badge: 'badge-direct', domainClass: 'text-emerald-300' },
};

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
  const all = document.getElementById('proxied-select-all');
  if (all) {
    const selectedShown = shown.filter((r) => proxiedSelected.has(r.domain)).length;
    all.checked = shown.length > 0 && selectedShown === shown.length;
    all.indeterminate = selectedShown > 0 && selectedShown < shown.length;
  }
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
  loaded: false,
  encrypted_available: false,
  encrypted_upstreams: [],
  encrypted_only: false,
  default_upstream: '',
  default_strategy: '',
  profiles: {},
  upstreams: []
};

let resolutionLoading = false;

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
    renderProxiedList();
  } catch (e) { /* keep the defaults; the dialog still works */ }
  finally { resolutionLoading = false; }
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
  out.default_strategy = out.default_strategy || '';
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
  if (!pinned || transportExcludes(pinned, transport)) return '';
  return pinned;
}

function upstreamDimmed(address, transport) {
  if (transportExcludes(address, transport)) return true;
  const pin = activePin(transport);
  return !!pin && pin !== address;
}

// syncResolutionControls fills the three global controls in the resolver card.
// The encrypted choice is disabled when the resolver list holds no DoT/DoH/DoQ
// upstream, because selecting it would leave the pool with no candidates.
function syncResolutionControls() {
  const encAvailable = hasEncryptedUpstream();
  const regAvailable = hasRegularUpstream();
  const transport = document.getElementById('resolution-transport');
  if (transport) {
    const encrypted = transport.querySelector('option[value="encrypted"]');
    const regular = transport.querySelector('option[value="regular"]');
    if (encrypted) encrypted.disabled = !encAvailable;
    if (regular) regular.disabled = !regAvailable;
    transport.value = resolutionSettings.transport || 'any';
  }

  const strategy = document.getElementById('resolution-default-strategy');
  if (strategy) strategy.value = resolutionSettings.default_strategy || '';

  const currentTransport = resolutionSettings.transport || 'any';
  const upstreamSel = document.getElementById('resolution-default-upstream');
  if (upstreamSel) {
    // A pin is a preference, not a policy override: the pool ignores one the
    // transport excludes, so the operator must still be able to choose it.
    const options = ['<option value="">Pool Default</option>'].concat(
      availableUpstreams().map(u => `<option value="${escapeHTML(u)}">${escapeHTML(u)}</option>`)
    );
    upstreamSel.innerHTML = options.join('');
    upstreamSel.value = resolutionSettings.default_upstream || '';
  }

  // A pin the active transport excludes is ignored by the pool; say so instead
  // of letting the disabled entry look like a working selection.
  const pinNote = document.getElementById('resolution-pin-note');
  if (pinNote) {
    const pinned = resolutionSettings.default_upstream || '';
    const ignored = !!pinned && transportExcludes(pinned, currentTransport);
    pinNote.classList.toggle('hidden', !ignored);
    if (ignored) {
      const kind = upstreamIsEncrypted(pinned) ? 'encrypted' : 'regular DNS';
      pinNote.textContent = `Default upstream ${pinned} is ${kind} and is ignored while ${transportLabel(currentTransport)} is active.`;
    }
  }

  // Repaint the resolver list at once: selecting a transport or a pin changes
  // which resolvers are dimmed, without waiting for the next stats tick.
  if (lastUpstreamsPayload) {
    lastUpstreamsSig = '';
    renderUpstreams(lastUpstreamsPayload);
  }
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
    default_upstream: resolutionSettings.default_upstream || '',
    default_strategy: resolutionSettings.default_strategy || '',
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
    renderProxiedList();
    if (message) showToast(message, 'success');
    return true;
  } catch (e) {
    showToast('Could not save the resolution settings', 'error');
    return false;
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
            <option value="" ${saved.strategy ? '' : 'selected'}>Inherit Global</option>
            <option value="fastest" ${saved.strategy === 'fastest' ? 'selected' : ''}>Fastest Wins</option>
            <option value="sequential" ${saved.strategy === 'sequential' ? 'selected' : ''}>In Order</option>
            <option value="hedged" ${saved.strategy === 'hedged' ? 'selected' : ''}>Hedged</option>
            <option value="best" ${saved.strategy === 'best' ? 'selected' : ''}>Best Known</option>
          </select>
        </label>
        <label class="block">
          <span class="block text-[11px] font-semibold text-slate-400 mb-1">Transports</span>
          <select id="resolution-dialog-transport" class="w-full bg-slate-950 border border-slate-700 text-xs text-slate-200 rounded-lg px-2.5 py-1.5 focus:outline-none focus:border-cyan-400 font-mono">${transportOptions}</select>
        </label>
        <p class="text-[10px] text-slate-500">Fallback: Pool Default</p>
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
  proxiedPruneSelection();
  updateDomainFilterChips();
  const needle = proxiedFilterNeedle();
  const ordered = proxiedFilteredRows();
  const total = ordered.length;
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
      : proxiedStatusFilter
        ? `No ${statusLabel} domains yet.`
        : 'No domains yet.';
    container.innerHTML = `<tr><td colspan="4" class="py-8 text-center text-slate-500 text-xs">${empty}</td></tr>`;
    proxiedHasMore = false;
    if (countEl) countEl.textContent = needle ? '0 matches' : (proxiedStatusFilter ? `0 ${statusLabel} domains` : '');
    if (moreBtn) moreBtn.classList.add('hidden');
    updateProxiedBulkBar();
    return;
  }

  const rows = ordered.slice(0, proxiedVisible);
  proxiedHasMore = rows.length < total;
  if (countEl) {
    if (needle) {
      countEl.textContent = proxiedHasMore
        ? `Showing ${rows.length} of ${total} matches`
        : `${total} match${total === 1 ? '' : 'es'}`;
    } else if (proxiedStatusFilter) {
      countEl.textContent = proxiedHasMore
        ? `Showing ${rows.length} of ${total} ${statusLabel} domains`
        : `${total} ${statusLabel} domain${total === 1 ? '' : 's'}`;
    } else {
      countEl.textContent = proxiedHasMore
        ? `Showing ${rows.length} of ${total}`
        : `${total} domain${total === 1 ? '' : 's'}`;
    }
  }
  if (moreBtn) moreBtn.classList.toggle('hidden', !proxiedHasMore);

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
      <td class="py-2.5 px-3 text-center">
        <div class="flex items-center justify-center gap-3">
          ${resolutionButton}
          ${pauseControl}
          <button class="remove-proxied inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-slate-900 hover:bg-red-500/10 text-red-400 border border-red-500/30 text-[11px] font-semibold transition" data-val="${escapeHTML(domain)}" aria-label="Remove ${escapeHTML(domain)}">Remove</button>
        </div>
      </td>
      <td class="py-2.5 px-3 text-center">${statusBadge}</td>
    `;
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

// ---- Named custom policy groups (v2.3) --------------------------------------

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

// ---- Group icon upload (v2.7) ----------------------------------------------
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
    container.innerHTML = '<div class="text-slate-500 text-xs py-1">No static records yet</div>';
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
async function saveRules() {
  if (!currentConfig) return;

  const payload = {
    custom_proxied: currentConfig.rules.custom_proxied,
    custom_proxied_disabled: currentConfig.rules.custom_proxied_disabled || [],
    custom_blocked: currentConfig.rules.custom_blocked,
    custom_direct: currentConfig.rules.custom_direct,
    custom_records: currentConfig.rules.custom_records,
    custom_domain_added: currentConfig.rules.custom_domain_added || {}
  };

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
      showToast('Routing rules saved & active!', 'success');
    }
  } catch (e) {
    showToast('Failed to save routing rules', 'error');
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
  const delay = statsFailures > 0
    ? Math.min(STATS_POLL_MS * 2 ** statsFailures, STATS_POLL_MAX_BACKOFF_MS)
    : STATS_POLL_MS;

  statsPollTimer = setTimeout(pollStatsOnce, delay);
}

function startStatsPolling() {
  // Called once, from the post-login boot path. The listener is registered here rather
  // than at module scope so that a page which never logs in never installs it.
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'hidden') {
      clearTimeout(statsPollTimer);
      statsPollTimer = null;
      return;
    }
    if (statsPollTimer === null) pollStatsOnce();
    // Returning to a visible page while the stream tab is showing: the SSE
    // buffer kept growing hidden (per-query DOM work is skipped then — see
    // pushStreamQuery), so this is where the backlog becomes rows. Without
    // it the table stays stale until the next query happens to arrive.
    if (activeDashTab === 'stream' && !isStreamPaused) renderQueryStream();
  });

  pollStatsOnce();
}

async function updateStats() {
  if (!authToken) return;
  try {
    const res = await fetch(api('/api/stats'), {
      headers: { 'Authorization': `Bearer ${authToken}` }
    });
    if (!res.ok) {
      markStatsHealth(false, `The server answered ${res.status}`);
      return;
    }

    const data = await res.json();

    const setTxt = (id, txt) => {
      const el = document.getElementById(id);
      // Only touch the DOM when the text actually changes. A steady value (an
      // idle QPS, a flat cache ratio) would otherwise rewrite innerText every
      // poll, and each write invalidates the blurred backdrop of the glass
      // panel it sits in — which Chromium re-rasterizes on the whole viewport.
      if (el && el.innerText !== txt) el.innerText = txt;
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

// markStatsHealth is the only thing that makes the numbers on the overview page
// trustworthy: without it a daemon that died two minutes ago is indistinguishable
// from a healthy one, because every tile simply keeps its last value.
function markStatsHealth(ok, reason) {
  const badge = document.getElementById('live-engine-badge');
  const dot = document.getElementById('live-engine-dot');
  const label = document.getElementById('live-engine-label');

  if (ok) {
    statsFailures = 0;
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
    if (el) el.innerText = txt;
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
  const unreadable = data.relays_unreadable || 0;

  if (refusedEl) {
    refusedEl.textContent = refused.toLocaleString();
    refusedEl.className = refused > 0 ? 'text-slate-300 font-bold' : 'text-slate-400';
    refusedEl.title = 'Connections dropped for a blocked target, a self-dial, or a full relay table';
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

// Signature of the last rendered upstream set: the poll repaints the list
// unconditionally and most ticks change nothing (latency only moves when the
// racer re-measures). Skipping identical payloads removes the innerHTML
// rebuild + the SVG re-parse for every one of those ticks. Built by
// concatenation because the row template below interpolates the escaped
// address into innerHTML; a bare template here would trip the raw-
// interpolation scan for exactly the value it exists to protect.
let lastUpstreamsSig = '';
// The last stats payload, kept so a policy change can repaint the list without
// waiting for the next 1 Hz poll.
let lastUpstreamsPayload = null;
// The last address set rendered, so the settings fallback is rebuilt once per
// pool change rather than on every 1 Hz repaint.
let lastUpstreamsAddresses = '';
// Throttles the settings re-fetch when a stats poll and the settings list agree
// only after a moment (adding an upstream writes both, but not atomically).
let lastResolutionSyncAt = 0;

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

  if (!upstreams || upstreams.length === 0) {
    if (lastUpstreamsSig !== 'empty') {
      container.innerHTML = '<div class="text-slate-500 text-xs py-2">No upstreams configured</div>';
      lastUpstreamsSig = 'empty';
    }
    return;
  }
  // The policy is part of the signature: a changed transport or pin dims a
  // different set of rows even when the upstreams themselves did not move.
  const policySig = [
    resolutionSettings.transport || 'any',
    resolutionSettings.default_upstream || '',
    (resolutionSettings.encrypted_upstreams || []).join(','),
  ].join('~');
  const sig = upstreams.map(u => u.address + ':' + (u.latency || 0)).join('|') + '|' + policySig;
  if (sig === lastUpstreamsSig) return;
  lastUpstreamsSig = sig;

  // A selected transport promotes the resolvers it allows to the top, keeping the
  // server's latency order inside each group. Any Transport leaves the list alone.
  const renderTransport = resolutionSettings.transport || 'any';
  const ordered = (renderTransport === 'encrypted' || renderTransport === 'regular')
    ? upstreams.filter(u => !transportExcludes(u.address, renderTransport))
        .concat(upstreams.filter(u => transportExcludes(u.address, renderTransport)))
    : upstreams;

  container.innerHTML = '';

  ordered.forEach(u => {
    // u.latency is nanoseconds, and it is absent until the racer has actually measured
    // this upstream — a freshly added one, or one that has never answered. The old code
    // did `(u.latency / 1000000).toFixed(1)` unconditionally and then compared the
    // resulting *string* to 40 and 80: for a missing latency that yields "NaN", both
    // comparisons are false, and the row rendered "NaN ms" wearing the green badge that
    // means "fastest". Zero was worse, because "0.0 ms" with a green badge reads as a
    // perfect upstream when it means nobody has timed it yet.
    const latency = Number(u.latency) / 1e6;
    const measured = Number.isFinite(latency) && latency > 0;

    let badgeClass = 'text-slate-400 bg-slate-500/10 border-slate-600/40';
    let dotClass = 'w-2 h-2 rounded-full bg-slate-600';
    if (measured) {
      badgeClass = 'text-emerald-400 bg-emerald-500/10 border-emerald-500/30';
      dotClass = 'w-2 h-2 rounded-full bg-emerald-400';
      if (latency > 40) {
        badgeClass = 'text-yellow-400 bg-yellow-500/10 border-yellow-500/30';
        dotClass = 'w-2 h-2 rounded-full bg-yellow-400';
      }
      if (latency > 80) {
        badgeClass = 'text-red-400 bg-red-500/10 border-red-500/30';
        dotClass = 'w-2 h-2 rounded-full bg-red-400';
      }
    }
    // "not timed yet" and "timed at 0.4 ms" have to look different, and the title is
    // where the distinction is spelled out for whoever hovers it.
    const latText = measured ? `${latency.toFixed(1)} ms` : '—';
    const latTitle = measured ? 'Last measured round-trip' : 'Not measured yet — no answer has been timed from this upstream';

    const row = document.createElement('div');
    // A resolver the active transport excludes, or one a valid pin bypasses, is
    // dimmed but stays removable: the list is also the place removals happen.
    const dimmed = upstreamDimmed(u.address, resolutionSettings.transport || 'any');
    row.className = 'upstream-row flex items-center justify-between py-1.5 px-3 rounded-lg bg-slate-950/70 border border-slate-800 text-xs'
      + (dimmed ? ' is-dimmed' : '');
    // The left half is `min-w-0 truncate`: a flex item's default min-width:auto
    // resolves to its content width, so a long DoH URL (https://dns.google/dns-query)
    // would refuse to shrink and crush the latency badge on the right into a
    // square. min-w-0 lets the name shrink, truncate ellipsizes it, and the
    // badge keeps shrink-0 so it never wraps.
    row.innerHTML = `
      <div class="flex items-center gap-2 min-w-0">
        <span class="${dotClass} shrink-0" aria-hidden="true"></span>
        <!-- The address is its own horizontal scroll area, not a clipped ellipsis:
             a DoH URL is routinely wider than this column and the operator needs
             to read the whole host to know which resolver a latency belongs to.
             Drag it, scroll it, or focus and use the arrow keys; the title is the
             hover fallback. The dot and the latency/delete controls never move. -->
        <span class="upstream-addr min-w-0 font-mono text-slate-200" tabindex="0" title="${escapeHTML(u.address)}">${escapeHTML(u.address)}</span>
      </div>
      <div class="flex items-center gap-2 shrink-0">
        <span class="shrink-0 px-2 py-0.5 rounded border ${badgeClass} text-[10px] font-bold font-mono whitespace-nowrap" title="${latTitle}">${latText}</span>
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
    container.appendChild(row);
  });
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
  if (sseBackoffTimer) {
    clearTimeout(sseBackoffTimer);
    sseBackoffTimer = null;
  }
  if (sseSource) {
    try { sseSource.close(); } catch(e) {}
    sseSource = null;
  }

  // v2.1.0: the long-lived token no longer rides the query string. The
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
        showLoginModal();
        return;
      }
      if (tr.ok) {
        const tj = await tr.json();
        ticketParam = `?ticket=${encodeURIComponent(tj.ticket)}`;
      }
    } catch (e) { /* network blip: fall through and retry on backoff */ }
  }

  // The stream URL carries only a fresh ticket; the fallback to the long-lived
  // token is gone, so a failed mint means the connect will 401. Treat that as a
  // retryable drop (the mint itself may have hit a network blip) rather than
  // opening a stream we know is unauthenticated.
  if (!ticketParam) {
    scheduleStreamReconnect();
    return;
  }

  const source = new EventSource(`${api('/api/stream/queries')}${ticketParam}`);
  sseSource = source;

  source.onopen = () => {
    // A live connection is the only proof the ticket was accepted: reset the
    // backoff so the next drop reconnects promptly.
    sseBackoffMs = 0;
  };

  // Handle single query event
  source.addEventListener('query', (event) => {
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
    try {
      const list = JSON.parse(event.data);
      if (!Array.isArray(list)) return;
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
    resetStreamView();
  });

  // Fallback for default messages
  source.onmessage = (event) => {
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

// stopLiveStream tears the stream down and cancels any pending reconnect, so a
// sign-out does not race a backoff timer into re-opening an authenticated route.
function stopLiveStream() {
  if (sseBackoffTimer) {
    clearTimeout(sseBackoffTimer);
    sseBackoffTimer = null;
  }
  sseBackoffMs = 0;
  if (sseSource) {
    try { sseSource.close(); } catch(e) {}
    sseSource = null;
  }
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

// queryMatchesStream decides whether one query belongs in the current view. CACHED is handled
// apart from the others because it is not an action — it is a flag that can accompany any of
// them, so comparing it against q.action would match nothing.
function queryMatchesStream(q, f) {
  if (f.action !== 'ALL') {
    if (f.action === 'CACHED') {
      if (!q.cached) return false;
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
  BLOCK: '<span class="badge badge-block">BLOCK</span>',
  RATELIMIT: '<span class="badge badge-block">RATE LIMITED</span>',
  QUOTA: '<span class="badge badge-block">QUOTA</span>',
  CUSTOM: '<span class="badge badge-custom">CUSTOM</span>',
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

// queryRowHTML renders one query into the seven cells of the stream table. Everything that
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
  const protoStr = resolutionProto || clientProto;
  const protoTitle = resolutionProto
    ? `Resolution: ${resolutionProto} · Client: ${clientProto}`
    : `Client: ${clientProto}`;
  const protoClass = protocolClass(protoStr);

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

  return `
    <td class="stream-col-time py-2.5 px-2 sm:px-3 text-slate-400 whitespace-nowrap">${timeStr}</td>
    <td class="stream-col-client py-2.5 px-2 sm:px-3">${clientDisplay}</td>
    <td class="stream-col-proto py-2.5 px-2 sm:px-3 ${protoClass} font-bold hidden sm:table-cell" title="${escapeHTML(protoTitle)}">${escapeHTML(protoStr)}</td>
    <td class="stream-col-domain py-2.5 px-2 sm:px-3">
      <div class="text-cyan-300 font-semibold truncate" title="${escapeHTML(domain)}">${escapeHTML(domain)}</div>
      ${upstreamLine}
    </td>
    <td class="stream-col-rule py-2.5 px-2 sm:px-3 text-slate-400 hidden sm:table-cell">${escapeHTML(ruleStr)}</td>
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
const STREAM_LISTENING_ROW = `
  <tr class="stream-placeholder">
    <td colspan="7" class="py-12 text-center text-slate-500">
      <div class="flex flex-col items-center gap-2">
        <div class="pulse-dot"></div>
        <span>Listening for live DNS queries...</span>
      </div>
    </td>
  </tr>`;

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
      <td colspan="7" class="py-12 text-center text-slate-500">
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
    tbody.innerHTML = STREAM_LISTENING_ROW;
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
  if (targetKey) lists[targetKey] = replacePatterns(lists[targetKey], remove, targets);
  else if (action !== 'remove') return;

  rules.custom_proxied = lists.custom_proxied;
  rules.custom_proxied_disabled = lists.custom_proxied_disabled;
  rules.custom_blocked = lists.custom_blocked;
  rules.custom_direct = lists.custom_direct;
  streamActionApplied = [];
  const added = rules.custom_domain_added || {};
  for (const pattern of rowPatterns) delete added[pattern];
  if (stamp && action !== 'remove') for (const target of targets) added[target] = stamp;
  rules.custom_domain_added = added;

  // Rewrite the buffered rows for this exact host so the badge the operator just
  // acted on (and any sibling row for the same host) shows the new action at once.
  const logBadge = STREAM_ACTION_LOG_BADGE[action];
  if (logBadge) {
    streamBuffer.forEach((q) => {
      if ((q.domain || '') === streamActionDomain) q.action = logBadge;
    });
  }

  const label = { proxy: 'Proxied', block: 'Blocked', direct: 'Set direct', remove: 'Removed' }[action] || 'Updated';
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
// Perf (v2.2.0): the buffer write is cheap and always runs; the DOM work is
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

document.addEventListener('hyperdns:theme', applyChartTheme);

function initChart() {
  try {
    const canvas = document.getElementById('qpsChart');
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (typeof Chart === 'undefined') {
      console.warn('Chart.js not yet available');
      return;
    }

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
    qpsChart.update('none');
  } catch (e) {}
}

// =======================================================
// EVENT LISTENERS & SPA ROUTING
// =======================================================
const tabRoutes = {
  'dashboard': '/home',
  'proxied': '/proxied',
  'clients': '/clients',
  'stream': '/logs',
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
  '/clients': 'clients',
  '/logs': 'stream',
  '/stream': 'stream',
  '/api': 'api',
  '/settings': 'rules',
  '/guide': 'connect',
  '/connect': 'connect'
};

function switchTab(target, updateUrl = true) {
  activeDashTab = target;
  document.querySelectorAll('.sidebar-nav-item, .nav-tab').forEach(t => {
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
    if (target === 'clients') {
      loadClients();
    }
    // Returning to the Logs view repaints the stream from the buffer: the
    // per-query DOM work was skipped while another tab was showing (see
    // pushStreamQuery), so this is where the queued history becomes rows.
    if (target === 'stream') {
      renderQueryStream();
    }
    // The Proxied list fills as the region scrolls, but a panel that was hidden
    // while it rendered has no height to measure against, so the first batch is
    // topped up once the tab is actually on screen.
    if (target === 'proxied') {
      proxiedFillViewport();
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
}

function handleRouteFromURL() {
  // The admin prefix and the /dash mount point are stripped before the table is
  // consulted, so the same keys work whatever path the install was given. A path
  // that matches nothing falls back to the dashboard, which is the pre-v2.1
  // behaviour for an unknown route and still the least surprising one.
  const stripped = window.location.pathname.toLowerCase().replace(/\/$/, '');
  const withoutBase = ADMIN_BASE && stripped.toLowerCase().startsWith(ADMIN_BASE + '/dash')
    ? stripped.slice((ADMIN_BASE + '/dash').length) || '/'
    : stripped;
  const path = withoutBase || '/home';
  const targetTab = routeTabs[path] || 'dashboard';
  switchTab(targetTab, false);
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
  // v2.1.0 (B-02/B-18 remediation): the snippets are generated from the live
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
  -d '{"display_name":"Gamer-VIP","validity_days":30}'`;
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
      document.querySelectorAll('.api-snippet-tab').forEach(t => {
        t.className = 'api-snippet-tab px-3 py-1.5 rounded-lg text-xs font-bold font-heading bg-slate-900 text-slate-400 hover:text-white';
      });
      tab.className = 'api-snippet-tab px-3 py-1.5 rounded-lg text-xs font-bold font-heading bg-cyan-500/20 text-cyan-300 border border-cyan-500/30';

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
// v2.1 HIDDEN ADMIN PATH DISPLAY + REGENERATION
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
        try { localStorage.removeItem('hyperdns_token'); } catch (e) {}
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

function initEventListeners() {
  initUpstreamAddressScroll();
  // Tabs Navigation (Sidebar & Mobile Bottom Bar)
  document.querySelectorAll('.sidebar-nav-item, .nav-tab, .mobile-nav-item').forEach(tab => {
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
      const addr = input ? input.value.trim() : '';
      if (!addr) return;

      try {
        const res = await fetch(api('/api/upstreams/add'), {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${authToken}` },
          body: JSON.stringify({ address: addr })
        });
        if (res.ok) {
          if (input) input.value = '';
          showToast('Upstream resolver added & tested!', 'success');
          loadResolutionSettings();
          updateStats();
        } else {
          // The handler rejects a value it cannot canonicalise and says why
          // (unknown scheme, bad port, not a host). Showing its message beats
          // the old silence, where an add that never happened looked like a
          // no-op and the operator retyped the same bad address.
          let reason = 'Invalid upstream address';
          try {
            const body = await res.json();
            if (body && body.error) reason = body.error;
          } catch (_) { /* non-JSON error body */ }
          showToast(reason, 'error');
        }
      } catch (e) {
        showToast('Failed to add upstream', 'error');
      }
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

  // The panel + DoH/DoT buttons live in app.js's own settings scope; the
  // subscription button binds itself from its module (twofa.js).
  window.bindLEButton(document.getElementById('issue-ssl-btn'), document.getElementById('ssl-domain-input'), 'panel', 'panel-le-progress');
  window.bindLEButton(document.getElementById('issue-dot-ssl-btn'), document.getElementById('dot-domain-input'), 'dot', 'dot-le-progress');
  const dotDomainInput = document.getElementById('dot-domain-input');
  if (dotDomainInput && currentConfig && currentConfig.tls) dotDomainInput.value = currentConfig.tls.dot_domain || '';

  // The DoH/DoT auto-renew toggle saves the moment it is clicked — the card
  // has no Save button, and the pattern matches the session-idle toggle: a
  // preference, not a form field. A failed save flips the checkbox back so
  // the UI never claims a state the daemon refused.
  const dotAutoRenewInput = document.getElementById('dot-auto-renew-input');
  if (dotAutoRenewInput) {
    dotAutoRenewInput.onchange = async () => {
      const enabled = dotAutoRenewInput.checked;
      dotAutoRenewInput.disabled = true;
      try {
        const res = await fetch(api('/api/tls/auto-renew'), {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${authToken}` },
          body: JSON.stringify({ enabled })
        });
        const body = await res.json().catch(() => ({}));
        if (!res.ok) {
          dotAutoRenewInput.checked = !enabled;
          showToast((body && body.error) || 'The auto-renew preference could not be saved.', 'error');
          return;
        }
        showToast(enabled ? 'DoH/DoT auto-renew enabled.' : 'DoH/DoT auto-renew disabled.', 'success');
        await loadConfig();
        if (typeof window.renderSubscriptionSettings === 'function') window.renderSubscriptionSettings();
      } catch (e) {
        dotAutoRenewInput.checked = !enabled;
        showToast('Could not reach the server.', 'error');
      } finally {
        dotAutoRenewInput.disabled = false;
      }
    };
  }

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

  // Custom policy groups (v2.3)
  document.getElementById('add-custom-group-btn')?.addEventListener('click', () => openCustomGroupEditor(null));
  document.getElementById('cancel-custom-group-btn')?.addEventListener('click', () => {
    document.getElementById('custom-group-editor')?.classList.add('hidden');
  });
  document.getElementById('save-custom-group-btn')?.addEventListener('click', saveCustomGroup);
  // Group icon (v2.7): choose a file → hold it as pending; Save flushes it to
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
    if (titleEl) titleEl.textContent = records ? 'Bulk Import Static Records' : proxied ? 'Bulk Import Proxied Domains' : 'Bulk Import Blocked Domains';
    const subtitleEl = document.getElementById('bulk-import-subtitle');
    if (subtitleEl) subtitleEl.textContent = records
      ? 'Each line maps one hostname to one IPv4 or IPv6 address.'
      : proxied
        ? 'Each name listed will resolve to this server through the SNI proxy.'
        : 'Each name listed will be sinkholed to 0.0.0.0.';
    const applyBtn = document.getElementById('bulk-import-apply-btn');
    if (applyBtn) applyBtn.textContent = records ? 'Import Records' : proxied ? 'Import Proxied' : 'Import Blocked';
    const labelEl = document.getElementById('bulk-import-textarea-label');
    if (labelEl) labelEl.textContent = records ? 'Records' : 'Domains';
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
  document.getElementById('resolution-transport')?.addEventListener('change', (e) => {
    saveResolutionSettings({ transport: e.target.value }, 'Resolution defaults saved');
  });
  document.getElementById('resolution-default-strategy')?.addEventListener('change', (e) => {
    saveResolutionSettings({ default_strategy: e.target.value }, 'Resolution defaults saved');
  });
  document.getElementById('resolution-default-upstream')?.addEventListener('change', (e) => {
    saveResolutionSettings({ default_upstream: e.target.value }, 'Resolution defaults saved');
  });
  document.getElementById('server-update-check-btn')?.addEventListener('click', checkServerUpdate);
  document.getElementById('server-update-apply-btn')?.addEventListener('click', applyServerUpdate);

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
      const summary = `Static records: ${parts.join(', ')}.`;
      if (invalid.length) {
        // Keep the modal open so the operator can see the offending lines, but the
        // good ones are already saved.
        setBulkImportStatus(summary, 'error');
        return;
      }
      closeBulkImport();
      showToast(`Imported ${accepted.length + updated.length} static record(s)`, 'success');
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
      const val = btn.dataset.val;
      if (currentConfig) {
        // Delete from both lists: the × on a switched-off row must remove it for
        // good, not just from the active set the row no longer lives in.
        currentConfig.rules.custom_proxied = currentConfig.rules.custom_proxied.filter(x => x !== val);
        if (Array.isArray(currentConfig.rules.custom_proxied_disabled)) {
          currentConfig.rules.custom_proxied_disabled = currentConfig.rules.custom_proxied_disabled.filter(x => x !== val);
        }
        saveRules();
        renderConfig(currentConfig);
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
    pauseBtn.onclick = (e) => {
      isStreamPaused = !isStreamPaused;
      const btn = e.currentTarget;
      btn.innerHTML = isStreamPaused
        ? '<i data-feather="play" class="w-3.5 h-3.5"></i> <span>Resume</span>'
        : '<i data-feather="pause" class="w-3.5 h-3.5"></i> <span>Pause</span>';
      safeFeatherReplace();
      // While paused the buffer keeps filling but nothing is inserted, so on resume the
      // table is behind it by however many queries arrived in between. A redraw from the
      // buffer is what brings the frozen view level again; the incremental insert that
      // follows only ever places the newest arrival.
      if (!isStreamPaused) renderQueryStream();
    };
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
    const row = document.createElement('div');
    row.className = 'flex items-center justify-between gap-2 py-1.5 px-2.5 rounded-lg bg-slate-900/60 border border-slate-800 text-xs';

    const label = document.createElement('div');
    label.className = 'min-w-0 flex-1';
    const nameEl = document.createElement('div');
    nameEl.className = 'font-bold text-white truncate';
    nameEl.textContent = t.name || t.host || '';
    const hostEl = document.createElement('div');
    hostEl.className = 'text-[10px] text-slate-500 font-mono truncate';
    hostEl.textContent = t.host || '';
    label.appendChild(nameEl);
    label.appendChild(hostEl);

    const remove = document.createElement('button');
    remove.type = 'button';
    remove.className = 'text-slate-500 hover:text-red-400 p-1 rounded transition shrink-0';
    remove.setAttribute('aria-label', `Remove ${t.host || 'target'}`);
    remove.title = 'Remove';
    remove.dataset.diagRemove = t.host || '';
    remove.innerHTML = '<i data-feather="trash-2" class="w-3.5 h-3.5"></i>';
    remove.onclick = () => removeDiagTarget(t.host || '');

    row.appendChild(label);
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
  const container = document.getElementById('diag-items-list');
  if (!container) return;
  container.querySelectorAll('[data-diag-target]').forEach((row) => {
    if (row.dataset.diagTarget === host) row.remove();
  });
  if (!container.querySelector('[data-diag-target]')) {
    container.innerHTML = '<div class="text-center py-6 text-slate-500 text-xs">No servers configured. Add a name and host above, then run the test again.</div>';
  }
}

let diagRunning = false;

async function runFullDiagnostics() {
  if (diagRunning) return;
  diagRunning = true;

  const container = document.getElementById('diag-items-list');
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
  if (container) {
    container.innerHTML = '<div class="text-center py-8 text-cyan-400 font-mono text-xs"><span class="pulse-dot inline-block me-2"></span> Testing your servers\' reachability and latency...</div>';
  }

  const fail = (msg) => {
    showToast(msg, 'error');
    if (qualEl) qualEl.innerText = 'Test failed';
    if (container) {
      container.innerHTML = `<div class="text-center py-6 text-red-400 text-xs">${escapeHTML(msg)}</div>`;
    }
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

  const container = document.getElementById('diag-items-list');
  if (!container) return;

  if (results.length === 0) {
    container.innerHTML = '<div class="text-center py-6 text-slate-500 text-xs">No servers configured. Add a name and host above, then run the test again.</div>';
    return;
  }

  const frag = document.createDocumentFragment();
  for (const r of results) {
    const row = document.createElement('div');
    row.className = 'flex items-center justify-between py-2 px-3 rounded-lg bg-slate-950/70 border border-slate-800 text-xs';
    // The stored host:port this row measured, so removeDiagTarget can drop the row
    // the moment its server is deleted rather than leaving a result on screen for
    // a target that no longer exists.
    row.dataset.diagTarget = r.target || '';

    // latency_ms is 0 on a failed dial and absent from nothing the server sends — but
    // toFixed on a missing field throws, and that throw used to be reported as a network
    // error.
    const ms = Number.isFinite(r.latency_ms) ? r.latency_ms.toFixed(1) : '?';
    const badge = r.success
      ? `<span class="text-emerald-400 font-bold font-mono">${ms} ms</span>`
      : '<span class="text-red-400 font-bold font-mono">BLOCKED</span>';

    row.innerHTML = `
      <div class="flex items-center gap-2">
        <span class="w-2 h-2 rounded-full ${r.success ? 'bg-emerald-400' : 'bg-red-400'}"></span>
        <div>
          <span class="font-bold text-white">${escapeHTML(r.name)}</span>
          <span class="text-[10px] text-slate-500 font-mono ms-1.5">(${escapeHTML(r.target)})</span>
        </div>
      </div>
      <div>${badge}</div>
    `;
    frag.appendChild(row);
  }
  container.innerHTML = '';
  container.appendChild(frag);
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
    <div class="clients-placeholder col-span-1 md:col-span-2 glass-panel p-8 text-center text-slate-400 border border-slate-800">
      <i data-feather="${icon}" class="w-8 h-8 mx-auto text-slate-600 mb-2"></i>
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
      modeBadge.className = 'badge badge-proxy inline-flex items-center gap-1';
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
      <div class="clients-placeholder col-span-1 md:col-span-2 glass-panel p-8 text-center text-slate-400 border border-slate-800">
        <i data-feather="users" class="w-8 h-8 mx-auto text-slate-600 mb-2"></i>
        <div class="font-bold text-slate-300 font-heading">No Clients Yet</div>
        <p class="text-xs text-slate-500 mt-1">Click "Add New Client" above to create client accounts &amp; registration links.</p>
      </div>
    ` : `
      <div class="clients-placeholder col-span-1 md:col-span-2 glass-panel p-8 text-center text-slate-400 border border-slate-800">
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
    card.className = 'glass-panel p-4 sm:p-5 flex flex-col justify-between space-y-4 border border-slate-800 hover:border-cyan-500/40 transition';
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
// it — an operator copying it sent a link that has been an API since
// v2.1, next to an empty field.
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
1️⃣ Connect your phone and game console to the same internet connection .
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
        // for a mode change that did not survive to the next boot (v2.8).
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
      } else {
        showToast(await errorMessage(res, 'Could not save the resolver allowlist'), 'error');
      }
    } catch (e) {
      showToast('Network error saving the resolver allowlist', 'error');
    }
  });

  // Search input live filter. Debounced (v2.2.0 perf pass): every keystroke
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
          panel: 'glass-panel p-6 w-full max-w-md border border-red-500/40 shadow-2xl shadow-red-500/10',
          badge: 'w-10 h-10 rounded-xl bg-red-500/20 flex items-center justify-center text-red-400 border border-red-500/30 shrink-0',
          hint: 'text-xs text-red-400 font-mono',
          submit: 'flex-1 py-3 bg-gradient-to-r from-red-600 to-rose-600 hover:from-red-700 hover:to-rose-700 text-onfill font-bold rounded-lg transition duration-200 shadow-lg shadow-red-500/20 text-sm font-heading',
        }
      : {
          icon: 'help-circle',
          panel: 'glass-panel p-6 w-full max-w-md border border-cyan-500/40 shadow-2xl shadow-cyan-500/10',
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
    // Reg Link button on the client card made this distinction in v2.1 —
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
// SELF-UPDATE (v2.6): check the main-branch version, then apply a
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
  if (st.error) return 'Update check failed.';
  if (st.update_available) return `Update available: v${st.current} → v${st.latest}`;
  return `Up to date (v${st.current}).`;
}

function serverUpdateMetaText(st) {
  if (!st) return 'Updates come from the GitHub release channel.';
  if (st.update_available && !st.supported) return 'This install cannot apply it from the app.';
  if (st.update_available) return 'Applying an update restarts the service.';
  return 'Checked just now · data is preserved and backed up.';
}

function renderServerUpdate(st) {
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
}

async function checkServerUpdate() {
  const btn = document.getElementById('server-update-check-btn');
  const label = btn ? btn.querySelector('span') : null;
  if (btn) btn.disabled = true;
  if (label) label.textContent = 'Checking…';
  try {
    const res = await fetch(api('/api/update/check'), {
      headers: { 'Authorization': `Bearer ${authToken}` },
      cache: 'no-store',
    });
    if (!res.ok) throw new Error(await errorMessage(res, 'The update check failed.'));
    const st = await res.json();
    renderServerUpdate(st);
    if (st && st.update_available && st.supported) showUpdateAvailable(st);
  } catch (e) {
    renderServerUpdate({ current: '', latest: '', update_available: false, supported: false, error: e.message || 'The update check failed.' });
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
    <div class="glass-panel p-6 w-full max-w-md border border-cyan-500/40 shadow-2xl shadow-cyan-500/10" role="dialog" aria-modal="true" aria-labelledby="hdns-update-title">
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
