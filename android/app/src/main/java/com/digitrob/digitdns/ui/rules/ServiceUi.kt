package com.digitrob.digitdns.ui.rules

import com.digitrob.digitdns.R
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.ArrowDropDown
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material.icons.filled.Public
import androidx.compose.material.icons.filled.PublicOff
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.digitrob.digitdns.ui.components.DigitDnsDropdownMenu
import coil.compose.SubcomposeAsyncImage
import timber.log.Timber
import com.digitrob.digitdns.domain.model.serviceDisplayIcon
import com.digitrob.digitdns.ui.components.PanelDialog
import com.digitrob.digitdns.ui.theme.PanelMono
import java.util.Locale

/**
 * The service filter, in the header beside Reload. On a phone this replaces the
 * panel's Services column: one dropdown, with the same three kinds of entry
 * (all, unlabelled, each named service) and a count per entry.
 */
@Composable
internal fun ServiceFilterDropdown(
    state: RulesUiState,
    onSelect: (String?) -> Unit,
    onManage: () -> Unit,
    modifier: Modifier = Modifier.widthIn(min = 168.dp),
) {
    var expanded by remember { mutableStateOf(false) }
    val selected = state.serviceFilter
    val selectedIcon = selected
        ?.takeIf { it != NO_SERVICE }
        ?.let { state.lists.serviceIcons[it] }
    val label = when (selected) {
        null -> stringResource(R.string.services_all)
        NO_SERVICE -> stringResource(R.string.services_none)
        else -> state.serviceName(selected)
    }
    Box {
        // Outlined like the sort selector: the fill-in-the-blank text button read
        // as a label rather than a control. The minimum width also gives the menu
        // enough room for a service name without truncating it to "BBC i…".
        OutlinedButton(
            onClick = { expanded = true },
            modifier = modifier,
        ) {
            // All services is a plain globe; No service is the crossed globe; a
            // named service shows its own mark, falling back to the globe.
            ServiceIcon(
                url = selectedIcon,
                modifier = Modifier.size(20.dp),
                noService = selected == NO_SERVICE,
            )
            Spacer(Modifier.width(8.dp))
            Text(
                label,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.weight(1f, fill = false),
            )
            Icon(Icons.Filled.ArrowDropDown, contentDescription = stringResource(R.string.services_filter))
        }
        DigitDnsDropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
            DropdownMenuItem(
                text = { Text("${stringResource(R.string.services_all)} (${state.totalCount})") },
                leadingIcon = { ServiceIcon(null, Modifier.size(20.dp)) },
                trailingIcon = {
                    if (selected == null) Icon(Icons.Filled.Check, contentDescription = null, modifier = Modifier.size(18.dp))
                },
                onClick = { expanded = false; onSelect(null) },
            )
            DropdownMenuItem(
                text = { Text("${stringResource(R.string.services_none)} (${state.noServiceCount})") },
                leadingIcon = { ServiceIcon(null, Modifier.size(20.dp), noService = true) },
                trailingIcon = {
                    if (selected == NO_SERVICE) Icon(Icons.Filled.Check, contentDescription = null, modifier = Modifier.size(18.dp))
                },
                onClick = { expanded = false; onSelect(NO_SERVICE) },
            )
            state.lists.services.entries
                .sortedBy { it.value.lowercase(Locale.US) }
                .forEach { (id, name) ->
                    DropdownMenuItem(
                        text = { Text("$name (${state.serviceCounts[id] ?: 0})", maxLines = 1, overflow = TextOverflow.Ellipsis) },
                        leadingIcon = { ServiceIcon(state.lists.serviceIcons[id], Modifier.size(20.dp)) },
                        trailingIcon = {
                            if (selected == id) Icon(Icons.Filled.Check, contentDescription = null, modifier = Modifier.size(18.dp))
                        },
                        onClick = { expanded = false; onSelect(id) },
                    )
                }
            // Reachable with no domains at all: the only other entry point is a
            // row's label, so a fresh install had no way to create a service.
            HorizontalDivider(Modifier.padding(vertical = 4.dp))
            DropdownMenuItem(
                text = { Text(stringResource(R.string.services_manage)) },
                leadingIcon = {
                    Icon(Icons.Filled.Settings, contentDescription = null, modifier = Modifier.size(20.dp))
                },
                onClick = { expanded = false; onManage() },
            )
        }
    }
}

/**
 * Pick the service for one domain, and manage the services themselves. Assigning is
 * the common act, so a single tap on a name both labels the domain and closes this.
 * Rename and delete sit beside each name because a service has no other home on a
 * phone screen.
 */
@Composable
@Suppress("LongParameterList")
internal fun ServicePickerDialog(
    domain: String?,
    state: RulesUiState,
    onAssign: (String) -> Unit,
    onSaveService: (String?, String, String) -> Unit,
    onDeleteService: (String) -> Unit,
    onDismiss: () -> Unit,
) {
    var form by remember { mutableStateOf<ServiceFormState?>(null) }
    var confirmDelete by remember { mutableStateOf<Pair<String, String>?>(null) }
    // No domain means "manage services": the same list, but a row tap edits rather
    // than assigns, and the "No service" choice would mean nothing.
    val managing = domain == null
    val current = domain?.let { state.lists.serviceOf(it) }.orEmpty()

    PanelDialog(
        onDismissRequest = onDismiss,
        solid = true,
        title = {
            Text(
                if (domain == null) {
                    stringResource(R.string.services_title)
                } else {
                    stringResource(R.string.services_picker_title, domain)
                },
            )
        },
        text = {
            Column(
                modifier = Modifier
                    .heightIn(max = 340.dp)
                    .verticalScroll(rememberScrollState()),
            ) {
                Text(
                    text = stringResource(
                        if (managing) R.string.services_manage_body else R.string.services_picker_body,
                    ),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Spacer(Modifier.height(8.dp))

                ServiceChoices(
                    state = state,
                    current = current,
                    managing = managing,
                    onAssign = onAssign,
                    onEdit = { form = it },
                    onDelete = { id, name -> confirmDelete = id to name },
                )

                HorizontalDivider(Modifier.padding(vertical = 6.dp))
                TextButton(onClick = { form = ServiceFormState(null, "", "") }) {
                    Icon(Icons.Filled.Add, contentDescription = null, modifier = Modifier.size(18.dp))
                    Spacer(Modifier.width(6.dp))
                    Text(stringResource(R.string.services_new))
                }
            }
        },
        confirmButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) } },
    )

    form?.let { editing ->
        ServiceFormDialog(
            state = editing,
            onDismiss = { form = null },
            onSave = { name, icon ->
                onSaveService(editing.id, name, icon)
                form = null
            },
        )
    }

    confirmDelete?.let { (id, name) ->
        PanelDialog(
            onDismissRequest = { confirmDelete = null },
            solid = true,
            title = { Text(stringResource(R.string.services_delete_title, name)) },
            text = { Text(stringResource(R.string.services_delete_body)) },
            confirmButton = {
                TextButton(onClick = {
                    confirmDelete = null
                    onDeleteService(id)
                }) { Text(stringResource(R.string.action_delete)) }
            },
            dismissButton = { TextButton(onClick = { confirmDelete = null }) { Text(stringResource(R.string.action_cancel)) } },
        )
    }
}

/**
 * A service's mark: its favicon or custom icon when one is set, a globe when it has
 * none (or the URL fails to load). Icons are decorative here — the name is always
 * beside them — so they carry no content description.
 */
@Composable
private fun ServiceIcon(
    url: String?,
    modifier: Modifier = Modifier,
    noService: Boolean = false,
) {
    if (noService) {
        // "No service" is a deliberate absence, not a service whose icon is
        // missing; the slash distinguishes it from the plain globe fallback.
        NoServiceIcon(modifier)
        return
    }
    val display = url?.takeIf { it.isNotBlank() }?.let(::serviceDisplayIcon)
    if (display == null) {
        GlobeIcon(modifier)
        return
    }
    SubcomposeAsyncImage(
        model = display,
        contentDescription = null,
        contentScale = ContentScale.Fit,
        modifier = modifier.clip(RoundedCornerShape(4.dp)),
        loading = { GlobeIcon(Modifier.matchParentSize()) },
        error = { GlobeIcon(Modifier.matchParentSize()) },
        // The fallback is silent by design, but a favicon that never loads
        // should still leave a trace for diagnosis.
        onError = { Timber.w(it.result.throwable, "service icon failed: %s", display) },
    )
}

@Composable
private fun NoServiceIcon(modifier: Modifier = Modifier) {
    Icon(
        Icons.Filled.PublicOff,
        contentDescription = null,
        tint = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = modifier,
    )
}

@Composable
private fun GlobeIcon(modifier: Modifier = Modifier) {
    Icon(
        Icons.Filled.Public,
        contentDescription = null,
        tint = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = modifier,
    )
}

/**
 * The services inside the picker. Selecting assigns in the per-domain case; in the
 * manage case there is nothing to assign to, so a tap opens the rename form instead.
 */
@Composable
@Suppress("LongParameterList")
private fun ServiceChoices(
    state: RulesUiState,
    current: String,
    managing: Boolean,
    onAssign: (String) -> Unit,
    onEdit: (ServiceFormState) -> Unit,
    onDelete: (String, String) -> Unit,
) {
    if (!managing) {
        ServiceChoiceRow(
            name = stringResource(R.string.services_none),
            count = state.noServiceCount,
            selected = current.isEmpty(),
            onSelect = { onAssign("") },
            iconUrl = null,
            noService = true,
        )
    }
    state.lists.services.entries.sortedBy { it.value.lowercase(Locale.US) }.forEach { (id, name) ->
        ServiceChoiceRow(
            name = name,
            count = state.serviceCounts[id] ?: 0,
            selected = !managing && id == current,
            iconUrl = state.lists.serviceIcons[id],
            onSelect = {
                if (managing) {
                    onEdit(ServiceFormState(id, name, state.lists.serviceIcons[id].orEmpty()))
                } else {
                    onAssign(id)
                }
            },
            onEdit = { onEdit(ServiceFormState(id, name, state.lists.serviceIcons[id].orEmpty())) },
            onDelete = { onDelete(id, name) },
            editLabel = stringResource(R.string.services_edit, name),
            deleteLabel = stringResource(R.string.services_delete, name),
        )
    }
}

/** One selectable service line inside the picker, with its rename/delete controls. */
@Composable
@Suppress("LongParameterList")
private fun ServiceChoiceRow(
    name: String,
    count: Int,
    selected: Boolean,
    onSelect: () -> Unit,
    iconUrl: String?,
    noService: Boolean = false,
    onEdit: (() -> Unit)? = null,
    onDelete: (() -> Unit)? = null,
    editLabel: String = "",
    deleteLabel: String = "",
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(6.dp))
            .clickable(role = Role.Button, onClick = onSelect)
            .padding(vertical = 2.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        if (selected) {
            Icon(Icons.Filled.Check, contentDescription = null, modifier = Modifier.size(16.dp))
        } else {
            Spacer(Modifier.width(16.dp))
        }
        Spacer(Modifier.width(6.dp))
        ServiceIcon(iconUrl, Modifier.size(20.dp), noService)
        Spacer(Modifier.width(8.dp))
        Text(
            text = name,
            modifier = Modifier.weight(1f),
            style = MaterialTheme.typography.bodyMedium,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
        if (onEdit != null && onDelete != null) {
            IconButton(onClick = onEdit) {
                Icon(Icons.Filled.Edit, contentDescription = editLabel, modifier = Modifier.size(18.dp))
            }
            IconButton(onClick = onDelete) {
                Icon(Icons.Filled.Delete, contentDescription = deleteLabel, modifier = Modifier.size(18.dp))
            }
        }
        Text(
            text = count.toString(),
            // Rightmost and fixed: with the actions before it, a row that has no
            // edit/delete pair still ends its count at the same edge as the others.
            modifier = Modifier.width(28.dp),
            textAlign = TextAlign.End,
            style = MaterialTheme.typography.labelSmall.copy(fontFamily = PanelMono),
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

/**
 * Bulk assignment: the same service list as the per-domain picker, but a tap
 * applies to every selected domain rather than one. Management controls are
 * deliberately absent here; this dialog only answers "which label?".
 */
@Composable
internal fun BulkServiceDialog(
    count: Int,
    state: RulesUiState,
    onApply: (String) -> Unit,
    onDismiss: () -> Unit,
) {
    val services = state.lists.services.entries.sortedBy { it.value.lowercase(Locale.US) }
    PanelDialog(
        onDismissRequest = onDismiss,
        solid = true,
        title = { Text(stringResource(R.string.domains_assign_service_title, count)) },
        text = {
            Column(
                modifier = Modifier
                    .heightIn(max = 340.dp)
                    .verticalScroll(rememberScrollState()),
            ) {
                Text(
                    text = stringResource(R.string.domains_assign_service_body),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Spacer(Modifier.height(8.dp))
                ServiceChoiceRow(
                    name = stringResource(R.string.services_none),
                    count = state.noServiceCount,
                    selected = false,
                    onSelect = { onApply("") },
                    iconUrl = null,
                    noService = true,
                )
                services.forEach { (id, name) ->
                    ServiceChoiceRow(
                        name = name,
                        count = state.serviceCounts[id] ?: 0,
                        selected = false,
                        onSelect = { onApply(id) },
                        iconUrl = state.lists.serviceIcons[id],
                    )
                }
            }
        },
        confirmButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) }
        },
    )
}

/** The sort selector as a dropdown button, placed to the right of the search field. */
@Composable
internal fun SortDropdown(selected: DomainSort, onSelect: (DomainSort) -> Unit) {
    var expanded by remember { mutableStateOf(false) }
    Box {
        // Same width as Bulk import so the two row-ends line up; the custom padding
        // is what lets "Added (newest)" fit at that width without clipping.
        OutlinedButton(
            onClick = { expanded = true },
            modifier = Modifier.width(CompactActionWidth),
            contentPadding = PaddingValues(horizontal = 10.dp, vertical = 4.dp),
        ) {
            Text(stringResource(selected.labelRes), maxLines = 1, overflow = TextOverflow.Ellipsis)
            Icon(Icons.Filled.ArrowDropDown, contentDescription = stringResource(R.string.domains_sort))
        }
        DigitDnsDropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
            DomainSort.entries.forEach { sort ->
                DropdownMenuItem(
                    text = { Text(stringResource(sort.labelRes)) },
                    trailingIcon = {
                        if (sort == selected) {
                            Icon(Icons.Filled.Check, contentDescription = null, modifier = Modifier.size(18.dp))
                        }
                    },
                    onClick = {
                        expanded = false
                        onSelect(sort)
                    },
                )
            }
        }
    }
}

/** Opens the bulk service dialog while a selection exists. */
@Composable
internal fun BulkAssignHost(
    open: Boolean,
    domains: Set<String>,
    state: RulesUiState,
    onApply: (String) -> Unit,
    onDismiss: () -> Unit,
) {
    if (!open || domains.isEmpty()) return
    BulkServiceDialog(
        count = domains.size,
        state = state,
        onApply = onApply,
        onDismiss = onDismiss,
    )
}

/** The name/icon pair the service form edits; a null [id] means "create". */
private data class ServiceFormState(val id: String?, val name: String, val icon: String)

@Composable
private fun ServiceFormDialog(
    state: ServiceFormState,
    onDismiss: () -> Unit,
    onSave: (String, String) -> Unit,
) {
    var name by remember(state) { mutableStateOf(state.name) }
    var icon by remember(state) { mutableStateOf(state.icon) }
    PanelDialog(
        onDismissRequest = onDismiss,
        solid = true,
        title = {
            Text(
                if (state.id == null) {
                    stringResource(R.string.services_new)
                } else {
                    stringResource(R.string.services_rename, state.name)
                },
            )
        },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                OutlinedTextField(
                    value = name,
                    onValueChange = { name = it },
                    label = { Text(stringResource(R.string.services_name)) },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
                OutlinedTextField(
                    value = icon,
                    onValueChange = { icon = it },
                    label = { Text(stringResource(R.string.services_icon)) },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
                Text(
                    text = stringResource(R.string.services_icon_hint),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        },
        confirmButton = {
            TextButton(
                onClick = { onSave(name, icon) },
                enabled = name.isNotBlank(),
            ) {
                Text(stringResource(if (state.id == null) R.string.services_add else R.string.services_save))
            }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) } },
    )
}
