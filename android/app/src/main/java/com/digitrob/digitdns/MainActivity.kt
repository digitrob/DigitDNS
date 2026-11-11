package com.digitrob.digitdns

import android.os.Bundle
import androidx.activity.compose.setContent
import androidx.activity.viewModels
import androidx.fragment.app.FragmentActivity
import androidx.annotation.StringRes
import androidx.activity.enableEdgeToEdge
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ListAlt
import androidx.compose.material.icons.filled.Dashboard
import androidx.compose.material.icons.filled.People
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material.icons.filled.Shield
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.NavigationRail
import androidx.compose.material3.NavigationRailItem
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.VerticalDivider
import androidx.compose.ui.res.stringResource
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.navigation.NavDestination.Companion.hierarchy
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController
import com.digitrob.digitdns.R
import com.digitrob.digitdns.data.AppMode
import com.digitrob.digitdns.data.AppPreferences
import com.digitrob.digitdns.data.AuthRepository
import com.digitrob.digitdns.data.SubscriberSession
import com.digitrob.digitdns.data.ThemeMode
import com.digitrob.digitdns.core.network.AuthState
import com.digitrob.digitdns.ui.auth.LoginScreen
import com.digitrob.digitdns.ui.clients.ClientsScreen
import com.digitrob.digitdns.ui.dashboard.DashboardScreen
import com.digitrob.digitdns.ui.rules.ProxiedDomainsScreen
import com.digitrob.digitdns.ui.security.AppLockScreen
import com.digitrob.digitdns.ui.security.AppLockViewModel
import com.digitrob.digitdns.ui.stream.LiveQueryLogScreen
import com.digitrob.digitdns.ui.subscriber.SubscriberScreen
import com.digitrob.digitdns.ui.system.SettingsHubScreen
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import dagger.hilt.android.AndroidEntryPoint
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import javax.inject.Inject

/**
 * The single activity.
 *
 * The signed-in graph is a bottom-bar shell over **five** destinations, chosen to fit
 * the panel's seven tabs into a phone's bar by promoting the ones a phone acts on and
 * folding the rest into Settings & SSL:
 *
 *   Dashboard · Proxied Domains · Clients & Whitelist · Live Query Log · Settings & SSL
 *
 * The panel's `REST API Gateway` and `Connect Guide` become sections inside
 * Settings & SSL rather than peers of the bar. Client detail is a pushed destination
 * *outside* the bar (reached from the list), because it is a drill-in, not a peer of
 * the top-level tabs.
 */
@AndroidEntryPoint
class MainActivity : FragmentActivity() {
    @Inject lateinit var authRepository: AuthRepository
    @Inject lateinit var appPreferences: AppPreferences
    @Inject lateinit var subscriberSession: SubscriberSession
    @Inject lateinit var authState: AuthState

    private val lockViewModel: AppLockViewModel by viewModels()

    override fun onStart() {
        super.onStart()
        lockViewModel.onForeground()
    }

    override fun onStop() {
        lockViewModel.onBackground()
        super.onStop()
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        enableEdgeToEdge()
        super.onCreate(savedInstanceState)
        setContent {
            // The theme follows the stored preference; SYSTEM resolves to the OS
            // setting. `isSystemInDarkTheme()` is read here (in composition) so a live
            // OS theme change re-renders too.
            val themeMode by appPreferences.themeMode.collectAsStateWithLifecycle(
                initialValue = ThemeMode.SYSTEM,
            )
            val darkTheme = when (themeMode) {
                ThemeMode.SYSTEM -> isSystemInDarkTheme()
                ThemeMode.DARK, ThemeMode.GREY -> true
                ThemeMode.LIGHT -> false
            }
            DigitDnsTheme(
                darkTheme = darkTheme,
                greyTheme = themeMode == ThemeMode.GREY,
            ) {
                Surface(
                    modifier = Modifier.fillMaxSize(),
                    color = MaterialTheme.colorScheme.background,
                ) {
                    // Bootstrap must finish before routing: it adopts a pre-multi-server
                    // install and pulls the active install's credential into memory, and
                    // [AuthRepository.hasCredential] is only truthful after that. Routing
                    // early would flash the login screen for an already-signed-in user.
                    var ready by remember { mutableStateOf(false) }
                    var startSubscriber by remember { mutableStateOf(false) }
                    LaunchedEffect(Unit) {
                        // DataStore and the Keystore-backed secret store do disk and
                        // crypto work; none of it belongs on the frame that draws the
                        // first screen.
                        startSubscriber = withContext(Dispatchers.IO) {
                            authRepository.migrateLegacyIfNeeded()
                            authRepository.loadActiveCredential()
                            appPreferences.appMode.first() == AppMode.SUBSCRIBER &&
                                appPreferences.subscriberActive.first() &&
                                subscriberSession.currentLink() != null
                        }
                        ready = true
                        // Re-mint a lapsed session after the shell is on screen. The
                        // retry interceptor already turns a 401 into the same refresh,
                        // so blocking navigation on this round trip only delayed the
                        // first usable frame (especially with saved credentials).
                        launch(Dispatchers.IO) {
                            runCatching { authRepository.ensureSession() }
                        }
                    }
                    if (ready) {
                        lockViewModel.onReady()
                        val locked by lockViewModel.locked.collectAsStateWithLifecycle()
                        val lockMessage by lockViewModel.message.collectAsStateWithLifecycle()
                        val authenticating by lockViewModel.authenticating.collectAsStateWithLifecycle()
                        if (locked) {
                            AppLockScreen(
                                message = lockMessage,
                                authenticating = authenticating,
                                onUnlock = { lockViewModel.unlock(this@MainActivity) },
                                onSignOut = { lockViewModel.signOut() },
                            )
                        } else {
                            // A subscriber session wins over the admin shortcut: it was
                            // the last surface the user opened. Otherwise a surviving
                            // admin credential goes straight to the dashboard, and
                            // neither means the login screen.
                            val start = when {
                                startSubscriber && subscriberSession.currentLink() != null -> Routes.SUBSCRIBER
                                authRepository.hasCredential() -> Routes.DASHBOARD
                                else -> Routes.LOGIN
                            }
                            DigitDnsNavHost(
                                startDestination = start,
                                sessionExpired = authState.sessionExpired,
                                onReauthenticated = authState::clearSessionExpired,
                            )
                        }
                    }
                }
            }
        }
    }
}

private object Routes {
    const val LOGIN = "login"

    /**
     * The add-server entry point. It is a distinct path, not `login?fresh=true`:
     * Navigation 2.8 matches a deep-link path regex against the whole URI including
     * the query, and the `login` destination is registered first, so a query-string
     * route resolved there — the form then prefilled the active install and saved
     * over it instead of adding a second profile.
     */
    const val LOGIN_ADD = "login/add"
    const val DASHBOARD = "dashboard"
    const val CLIENTS = "clients"
    const val PROXIED = "proxied"
    const val STREAM = "stream"
    const val SETTINGS = "settings"
    const val SUBSCRIBER = "subscriber"
}

/** The top-level tabs the bottom bar carries, labelled as the panel labels them. */
private data class Tab(
    val route: String,
    @StringRes val labelRes: Int,
    val icon: ImageVector,
)

private val tabs = listOf(
    Tab(Routes.DASHBOARD, R.string.tab_dashboard, Icons.Filled.Dashboard),
    Tab(Routes.PROXIED, R.string.tab_domains, Icons.Filled.Shield),
    Tab(Routes.STREAM, R.string.tab_live_log, Icons.AutoMirrored.Filled.ListAlt),
    Tab(Routes.CLIENTS, R.string.tab_clients, Icons.Filled.People),
    Tab(Routes.SETTINGS, R.string.tab_settings, Icons.Filled.Settings),
)

@Composable
private fun DigitDnsNavHost(
    startDestination: String,
    sessionExpired: kotlinx.coroutines.flow.StateFlow<Boolean>,
    onReauthenticated: () -> Unit,
) {
    val navController = rememberNavController()

    // One definition of "back to login", used by both the fresh-login path and the
    // sign-out path, so the two cannot drift.
    val goToLogin: () -> Unit = {
        navController.navigate(Routes.LOGIN) {
            popUpTo(navController.graph.id) { inclusive = true }
        }
    }
    val goToDashboard: () -> Unit = {
        onReauthenticated()
        // Clearing the whole graph, not just LOGIN: after a fresh sign-in the login
        // entry is already gone, and after an add-server flow the stack also holds a
        // second LOGIN. Leaving it behind made Back walk through dead routes.
        navController.navigate(Routes.DASHBOARD) {
            popUpTo(navController.graph.id) { inclusive = true }
        }
    }
    val goToSubscriber: () -> Unit = {
        navController.navigate(Routes.SUBSCRIBER) {
            popUpTo(navController.graph.id) { inclusive = true }
        }
    }

    // When a session lapses with no silent recovery available, any screen's 401 flips
    // this and the app returns to the login form rather than leaving dead panels up.
    val expired by sessionExpired.collectAsStateWithLifecycle()
    LaunchedEffect(expired) {
        if (expired) {
            val current = navController.currentDestination?.route
            // Only the admin graph has sessions; a subscriber screen never sets this.
            if (current != Routes.LOGIN && current != Routes.LOGIN_ADD && current != Routes.SUBSCRIBER) goToLogin()
        }
    }

    DigitDnsGraph(
        navController = navController,
        startDestination = startDestination,
        sessionExpired = expired,
        goToLogin = goToLogin,
        goToDashboard = goToDashboard,
        goToSubscriber = goToSubscriber,
    )
}

/**
 * The destination table, kept apart from the navigation lambdas it closes over: the
 * graph reads as a list of routes, while the host owns "back to login/dashboard".
 */
@Composable
private fun DigitDnsGraph(
    navController: androidx.navigation.NavHostController,
    startDestination: String,
    sessionExpired: Boolean,
    goToLogin: () -> Unit,
    goToDashboard: () -> Unit,
    goToSubscriber: () -> Unit,
) {
    NavHost(navController = navController, startDestination = startDestination) {
        composable(Routes.LOGIN) {
            LoginScreen(
                onAuthenticated = goToDashboard,
                onSubscriberAuthenticated = goToSubscriber,
                sessionExpired = sessionExpired,
            )
        }
        // The "Add server" entry point: the same form with an empty prefill, so the
        // operator does not have to clear the active install's fields by hand.
        composable(Routes.LOGIN_ADD) {
            LoginScreen(
                onAuthenticated = goToDashboard,
                onSubscriberAuthenticated = goToSubscriber,
                addNewServer = true,
                onCancel = { navController.popBackStack() },
            )
        }

        composable(Routes.SUBSCRIBER) {
            SubscriberScreen(
                onSignedOut = goToLogin,
                onForgotten = goToLogin,
                onSwitchToAdmin = goToDashboard,
            )
        }
        composable(Routes.DASHBOARD) {
            TabShell(navController, Routes.DASHBOARD) {
                DashboardScreen(
                    onAddServer = {
                        navController.navigate(Routes.LOGIN_ADD) { launchSingleTop = true }
                    },
                    onLoginRequired = goToLogin,
                    onOpenSubscriber = goToSubscriber,
                )
            }
        }
        composable(Routes.PROXIED) {
            TabShell(navController, Routes.PROXIED) { ProxiedDomainsScreen() }
        }
        composable(Routes.CLIENTS) {
            TabShell(navController, Routes.CLIENTS) {
                ClientsScreen()
            }
        }
        composable(Routes.STREAM) {
            TabShell(navController, Routes.STREAM) { LiveQueryLogScreen() }
        }
        composable(Routes.SETTINGS) {
            TabShell(navController, Routes.SETTINGS) {
                SettingsHubScreen(
                    onLoggedOut = goToLogin,
                    onAddServer = { navController.navigate(Routes.LOGIN_ADD) { launchSingleTop = true } },
                    onOpenSubscriber = goToSubscriber,
                )
            }
        }
    }
}

/**
 * One top-level tab. A phone keeps the bottom bar; a tablet or foldable gets a
 * navigation rail on the leading edge, where the thumb is not travelling the
 * height of a 12-inch screen to switch tabs. The breakpoint is the standard
 * 600 dp compact/medium boundary.
 */
@Composable
private fun TabShell(
    navController: androidx.navigation.NavHostController,
    current: String,
    content: @Composable () -> Unit,
) {
    BoxWithConstraints {
        val wide = maxWidth >= 600.dp
        val backStackEntry by navController.currentBackStackEntryAsState()
        val destination = backStackEntry?.destination
        val selectedTab = { tab: Tab ->
            destination?.hierarchy?.any { it.route == tab.route } == true || current == tab.route
        }
        val open = { tab: Tab ->
            navController.navigate(tab.route) {
                // One entry per tab, resuming where you left it. The anchor is
                // DASHBOARD, not findStartDestination(): when the app started
                // logged out the graph's start is LOGIN, which goToDashboard has
                // already popped, so the pop target was missing and tab taps did
                // nothing.
                popUpTo(Routes.DASHBOARD) { saveState = true }
                launchSingleTop = true
                restoreState = true
            }
            Unit
        }

        Scaffold(
            bottomBar = {
                if (!wide) {
                    NavigationBar {
                        tabs.forEach { tab ->
                            NavigationBarItem(
                                selected = selectedTab(tab),
                                onClick = { open(tab) },
                                icon = { Icon(tab.icon, contentDescription = stringResource(tab.labelRes)) },
                                label = { Text(stringResource(tab.labelRes)) },
                            )
                        }
                    }
                }
            },
        ) { padding ->
            Row(modifier = Modifier.fillMaxSize().padding(padding)) {
                if (wide) {
                    NavigationRail {
                        Spacer(Modifier.height(8.dp))
                        tabs.forEach { tab ->
                            NavigationRailItem(
                                selected = selectedTab(tab),
                                onClick = { open(tab) },
                                icon = { Icon(tab.icon, contentDescription = stringResource(tab.labelRes)) },
                                label = { Text(stringResource(tab.labelRes)) },
                            )
                        }
                    }
                    VerticalDivider()
                }
                Box(modifier = Modifier.weight(1f).fillMaxSize()) {
                    content()
                }
            }
        }
    }
}
