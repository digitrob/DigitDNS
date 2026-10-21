package com.digitrob.digitdns.ui.components

import androidx.compose.foundation.clickable
import androidx.compose.ui.Modifier

/**
 * `Modifier.clickable` with an indication that matches the panel's restrained
 * interaction feel. Kept as a named helper so the screens read as intent
 * ("this is tappable") rather than as a Compose incantation, and so a future change
 * to the ripple/indication lands in one place.
 */
fun Modifier.clickableText(onClick: () -> Unit): Modifier = this.clickable(onClick = onClick)
