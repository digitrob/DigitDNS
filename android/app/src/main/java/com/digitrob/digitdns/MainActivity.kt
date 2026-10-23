package com.digitrob.digitdns

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
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
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.navigation.NavDestination.Companion.hierarchy
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController
import com.digitrob.digitdns.data.AppPreferences
import com.digitrob.digitdns.data.AuthRepository
import com.digitrob.digitdns.data.ThemeMode
import com.digitrob.digitdns.core.network.AuthState
import com.digitrob.digitdns.ui.auth.LoginScreen
import com.digitrob.digitdns.ui.clients.ClientsScreen
import com.digitrob.digitdns.ui.dashboard.DashboardScreen
import com.digitrob.digitdns.ui.rules.ProxiedDomainsScreen
import com.digitrob.digitdns.ui.stream.LiveQueryLogScreen
import com.digitrob.digitdns.ui.system.SettingsHubScreen
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import dagger.hilt.android.AndroidEntryPoint
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
class MainActivity : ComponentActivity() {
    @Inject lateinit var authRepository: AuthRepository
    @Inject lateinit var appPreferences: AppPreferences
    @Inject lateinit var authState: AuthState

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
                ThemeMode.DARK -> true
                ThemeMode.LIGHT -> false
            }
            DigitDnsTheme(darkTheme = darkTheme) {
                Surface(
                    modifier = Modifier.fillMaxSize(),
                    color = MaterialTheme.colorScheme.background,
                ) {
                    // Bootstrap must finish before routing: it adopts a pre-multi-server
                    // install and pulls the active install's credential into memory, and
                    // [AuthRepository.hasCredential] is only truthful after that. Routing
                    // early would flash the login screen for an already-signed-in user.
                    var ready by remember { mutableStateOf(false) }
                    LaunchedEffect(Unit) {
                        authRepository.migrateLegacyIfNeeded()
                        authRepository.loadActiveCredential()
                        // Re-mint a lapsed session from the saved pair before the first
                        // screen fires its calls, so a backgrounded-then-reopened app
                        // does not come back to a wall of "Unauthorized" banners.
                        runCatching { authRepository.ensureSession() }
                        ready = true
                    }
                    if (ready) {
                        // Skip the login screen when a credential survived the last run:
                        // either a session token [AuthState] revived from [SecretStore],
                        // or an API key. The screens above handle a lapsed token by
                        // re-prompting, so this is a shortcut, not a bypass.
                        val start = if (authRepository.hasCredential()) Routes.DASHBOARD else Routes.LOGIN
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
}

/** The top-level tabs the bottom bar carries, labelled as the panel labels them. */
private data class Tab(val route: String, val label: String, val icon: ImageVector)

private val tabs = listOf(
    Tab(Routes.DASHBOARD, "Dashboard", Icons.Filled.Dashboard),
    Tab(Routes.PROXIED, "Domains", Icons.Filled.Shield),
    Tab(Routes.STREAM, "Live Log", Icons.AutoMirrored.Filled.ListAlt),
    Tab(Routes.CLIENTS, "Clients", Icons.Filled.People),
    Tab(Routes.SETTINGS, "Settings", Icons.Filled.Settings),
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

    // When a session lapses with no silent recovery available, any screen's 401 flips
    // this and the app returns to the login form rather than leaving dead panels up.
    val expired by sessionExpired.collectAsStateWithLifecycle()
    LaunchedEffect(expired) {
        if (expired) {
            val current = navController.currentDestination?.route
            if (current != Routes.LOGIN && current != Routes.LOGIN_ADD) goToLogin()
        }
    }

    NavHost(navController = navController, startDestination = startDestination) {
        composable(Routes.LOGIN) {
            LoginScreen(onAuthenticated = goToDashboard, sessionExpired = expired)
        }
        // The "Add server" entry point: the same form with an empty prefill, so the
        // operator does not have to clear the active install's fields by hand.
        composable(Routes.LOGIN_ADD) {
            LoginScreen(onAuthenticated = goToDashboard, addNewServer = true)
        }

        composable(Routes.DASHBOARD) {
            TabShell(navController, Routes.DASHBOARD) {
                DashboardScreen(
                    onAddServer = {
                        navController.navigate(Routes.LOGIN_ADD) { launchSingleTop = true }
                    },
                    onLoginRequired = goToLogin,
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
                )
            }
        }
    }
}

/** One top-level tab: its content under a bottom navigation bar. */
@Composable
private fun TabShell(
    navController: androidx.navigation.NavHostController,
    current: String,
    content: @Composable () -> Unit,
) {
    Scaffold(
        bottomBar = {
            NavigationBar {
                val backStackEntry by navController.currentBackStackEntryAsState()
                val destination = backStackEntry?.destination
                tabs.forEach { tab ->
                    val selected = destination?.hierarchy?.any { it.route == tab.route } == true
                    NavigationBarItem(
                        selected = selected || current == tab.route,
                        onClick = {
                            navController.navigate(tab.route) {
                                // Standard bottom-nav behaviour: one entry per tab, and
                                // resuming where you left the tab rather than resetting it.
                                // The anchor is DASHBOARD, not findStartDestination(): when
                                // the app started logged out the graph's start is LOGIN, which
                                // goToDashboard has already popped, so the pop target was
                                // missing and tab taps did nothing.
                                popUpTo(Routes.DASHBOARD) { saveState = true }
                                launchSingleTop = true
                                restoreState = true
                            }
                        },
                        icon = { Icon(tab.icon, contentDescription = tab.label) },
                        label = { Text(tab.label) },
                    )
                }
            }
        },
    ) { padding ->
        Box(modifier = Modifier.fillMaxSize().padding(padding)) {
            content()
        }
    }
}
