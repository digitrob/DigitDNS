package com.digitrob.digitdns.core.network

import javax.inject.Qualifier

/**
 * The HTTP client for long-lived server-sent event streams.
 *
 * It is the singleton client with one change: the read timeout is disabled,
 * because an idle queue between events is normal and must not be mistaken for a
 * dead connection. Every ordinary request keeps a finite read timeout, so a
 * stalled API call fails and the screen can retry instead of spinning forever.
 */
@Qualifier
@Retention(AnnotationRetention.BINARY)
annotation class StreamClient
