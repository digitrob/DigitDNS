package com.digitrob.digitdns.core.network

import java.io.IOException

/**
 * Raised when a request is attempted before any install has been configured.
 *
 * The Retrofit base URL is a placeholder (`https://127.0.0.1/`); without a stored
 * connection there is nothing to rebase it onto, and letting the request proceed
 * would contact that literal placeholder. The operator would then see
 * "Failed to connect to 127.0.0.1:443" — an address they never typed — instead of
 * "no server configured", which is the actual state and the actual remedy. Thrown
 * from [AuthInterceptor] so every call site gets one typed, readable failure.
 */
class NoConnectionException :
    IOException("No server configured. Add a server to continue.")
