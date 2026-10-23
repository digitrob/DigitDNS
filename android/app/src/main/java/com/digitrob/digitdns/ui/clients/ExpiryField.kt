package com.digitrob.digitdns.ui.clients

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.CalendarMonth
import androidx.compose.material.icons.filled.Clear
import androidx.compose.material3.DatePicker
import androidx.compose.material3.DatePickerDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TimePicker
import androidx.compose.material3.rememberDatePickerState
import androidx.compose.material3.rememberTimePickerState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.digitrob.digitdns.ui.components.PanelDialog
import java.time.Instant
import java.time.LocalDate
import java.time.LocalDateTime
import java.time.LocalTime
import java.time.ZoneOffset
import java.time.format.DateTimeFormatter

private val FIELD_FORMAT = DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm")

/**
 * The absolute-expiry editor, matching the panel's date-and-time picker rather
 * than a "valid for N days" field.
 *
 * A blank value is lifetime and is the default for an untouched edit; the create
 * form pre-fills +30 days because that is what the web panel's form does. The
 * value round-trips through [com.digitrob.digitdns.domain.model.ClientLinks],
 * which is what turns it into the wire timestamp.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun ExpiryField(
    value: String,
    onChange: (String) -> Unit,
    onClear: () -> Unit,
    modifier: Modifier = Modifier,
) {
    var showDate by remember { mutableStateOf(false) }
    var showTime by remember { mutableStateOf(false) }
    var pickedDate by remember { mutableStateOf<LocalDate?>(null) }

    val parsed = remember(value) { runCatching { LocalDateTime.parse(value, FIELD_FORMAT) }.getOrNull() }

    Row(
        modifier = modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        OutlinedButton(
            onClick = { showDate = true },
            modifier = Modifier.weight(1f),
        ) {
            Icon(Icons.Filled.CalendarMonth, contentDescription = null)
            Text(
                text = if (value.isBlank()) "Expiry: lifetime" else "Expiry: $value",
                modifier = Modifier.padding(start = 8.dp),
                maxLines = 1,
            )
        }
        if (value.isNotBlank()) {
            IconButton(onClick = onClear) {
                Icon(Icons.Filled.Clear, contentDescription = "Clear expiry")
            }
        }
    }

    if (showDate) {
        DatePickerStep(
            initial = parsed,
            onCancel = { showDate = false },
            onPicked = { date ->
                pickedDate = date
                showDate = false
                showTime = true
            },
        )
    }

    if (showTime) {
        TimePickerStep(
            initial = parsed,
            pickedDate = pickedDate,
            onCancel = { showTime = false },
            onPicked = { text ->
                onChange(text)
                showTime = false
            },
        )
    }
}

/** The date half of the picker: one hop, then the time half opens. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun DatePickerStep(
    initial: LocalDateTime?,
    onCancel: () -> Unit,
    onPicked: (java.time.LocalDate?) -> Unit,
) {
    val initialMillis = initial
        ?.atZone(java.time.ZoneId.systemDefault())
        ?.toInstant()
        ?.toEpochMilli()
    val state = rememberDatePickerState(initialSelectedDateMillis = initialMillis)
    DatePickerDialog(
        onDismissRequest = onCancel,
        confirmButton = {
            TextButton(onClick = {
                val millis = state.selectedDateMillis
                onPicked(
                    millis?.let {
                        Instant.ofEpochMilli(it).atZone(ZoneOffset.UTC).toLocalDate()
                    },
                )
            }) { Text("Next") }
        },
        dismissButton = { TextButton(onClick = onCancel) { Text("Cancel") } },
    ) {
        DatePicker(state = state)
    }
}

/** The time half of the picker; confirms the whole date-time. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun TimePickerStep(
    initial: LocalDateTime?,
    pickedDate: LocalDate?,
    onCancel: () -> Unit,
    onPicked: (String) -> Unit,
) {
    val base = initial ?: LocalDateTime.now().withSecond(0).withNano(0)
    val timeState = rememberTimePickerState(
        initialHour = base.hour,
        initialMinute = base.minute,
        is24Hour = true,
    )
    PanelDialog(
        onDismissRequest = onCancel,
        title = { Text("Expiry time") },
        text = {
            Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.Center) {
                TimePicker(state = timeState)
            }
        },
        confirmButton = {
            TextButton(onClick = {
                val date = pickedDate ?: base.toLocalDate()
                val time = LocalTime.of(timeState.hour, timeState.minute)
                onPicked(LocalDateTime.of(date, time).format(FIELD_FORMAT))
            }) { Text("Set") }
        },
        dismissButton = { TextButton(onClick = onCancel) { Text("Cancel") } },
    )
}

/** The `yyyy-MM-dd HH:mm` parse the view model saves; null when blank or malformed. */
internal fun parseExpiryField(value: String): Instant? =
    com.digitrob.digitdns.domain.model.ClientLinks.parseExpiryField(value)

/** The editable text for an existing expiry. */
internal fun expiryFieldValue(instant: Instant?): String =
    com.digitrob.digitdns.domain.model.ClientLinks.expiryFieldValue(instant)
