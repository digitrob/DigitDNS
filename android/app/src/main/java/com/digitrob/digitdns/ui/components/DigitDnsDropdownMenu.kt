package com.digitrob.digitdns.ui.components

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.DropdownMenu
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.compositeOver
import androidx.compose.ui.unit.dp
import com.digitrob.digitdns.ui.theme.DigitDnsTheme

/**
 * A floating menu in the panel's own language: the same 12 dp card radius, the
 * glass surface and the cyan hairline border the cards use, instead of Material's
 * default menu surface. The trigger must be wrapped in a Box together with this
 * menu so the popup anchors to the button, not to the surrounding layout.
 */
@Composable
fun DigitDnsDropdownMenu(
    expanded: Boolean,
    onDismissRequest: () -> Unit,
    modifier: Modifier = Modifier,
    content: @Composable ColumnScope.() -> Unit,
) {
    val colors = DigitDnsTheme.colors
    DropdownMenu(
        expanded = expanded,
        onDismissRequest = onDismissRequest,
        modifier = modifier,
        shape = RoundedCornerShape(12.dp),
        // bgCardHover carries alpha in dark mode (the cards are glass), and a menu
        // that lets the list show through its own popup is unreadable. Compositing
        // it over the page background gives the same colour, fully opaque.
        containerColor = colors.bgCardHover.compositeOver(colors.bgMain),
        tonalElevation = 0.dp,
        shadowElevation = 8.dp,
        border = BorderStroke(1.dp, colors.borderColor),
        content = content,
    )
}
