package com.digitrob.digitdns.ui.security

import android.content.Context
import android.content.ContextWrapper
import androidx.fragment.app.FragmentActivity

/**
 * Walk the context wrapper chain to the hosting activity. Compose's LocalContext is
 * often a ContextThemeWrapper, so a direct `as FragmentActivity` cast fails on some
 * devices. The biometric prompt needs the real FragmentActivity.
 */
tailrec fun Context.findFragmentActivity(): FragmentActivity? = when (this) {
    is FragmentActivity -> this
    is ContextWrapper -> baseContext.findFragmentActivity()
    else -> null
}
