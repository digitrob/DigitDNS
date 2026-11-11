// Swagger UI bootstrap for /api/v2/docs and /api/v3/docs.
//
// This lives in its own file because the dashboard's Content-Security-Policy
// allows only 'self' scripts plus the hashes of the dashboard document's own
// inline blocks. An inline bootstrap here was silently blocked, leaving the
// Swagger shell empty. The spec itself is in a <script type="application/json">
// data block, which is never executed and therefore not subject to script-src.
(function () {
  'use strict';

  var data = document.getElementById('openapi-spec');
  if (!data || typeof SwaggerUIBundle !== 'function') return;

  var spec;
  try {
    spec = JSON.parse(data.textContent || '{}');
  } catch (err) {
    console.error('API docs: could not parse the embedded OpenAPI spec', err);
    return;
  }

  // The docs page is served at /<admin>/api/v3/docs (or /api/v3/docs), while
  // the spec's paths start at /api/v3. Point Swagger's Try it out at the same
  // admin namespace the page itself was reached through.
  var prefix = window.location.pathname.replace(/\/api\/v[23]\/docs\/?$/, '');
  spec.servers = [{ url: prefix || '/', description: 'This installation' }];

  // The dashboard keeps its session token in localStorage. Same origin, so the
  // docs page can reuse it for Try it out on v3. v2 accepts the master REST key
  // only; adding a session token there would mask a key the operator pasted
  // into Authorize, so it is v3-only.
  var token = '';
  try { token = window.localStorage.getItem('digitdns_token') || ''; } catch (err) { /* private mode */ }
  var isV3 = String(spec.openapi || '').indexOf('3.') === 0;

  SwaggerUIBundle({
    dom_id: '#swagger-ui',
    spec: spec,
    deepLinking: true,
    persistAuthorization: true,
    // validator.swagger.io is an external origin and connect-src is 'self'.
    validatorUrl: null,
    presets: [SwaggerUIBundle.presets.apis, SwaggerUIBundle.presets.operations],
    requestInterceptor: function (req) {
      if (isV3 && token) req.headers.Authorization = 'Bearer ' + token;
      return req;
    }
  });
})();
