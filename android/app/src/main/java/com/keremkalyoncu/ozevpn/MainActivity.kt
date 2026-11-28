package com.keremkalyoncu.ozevpn

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import android.net.VpnService
import android.os.Build
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.SystemBarStyle
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.runtime.getValue
import androidx.core.content.ContextCompat
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.lifecycleScope
import com.keremkalyoncu.ozevpn.core.ConnectivityTester
import com.keremkalyoncu.ozevpn.data.PreferencesStore
import com.keremkalyoncu.ozevpn.data.RoutingMode
import com.keremkalyoncu.ozevpn.data.TunnelPhase
import com.keremkalyoncu.ozevpn.data.TunnelStateHolder
import com.keremkalyoncu.ozevpn.ui.OzeVpnScreen
import com.keremkalyoncu.ozevpn.ui.theme.OzeVpnTheme
import com.keremkalyoncu.ozevpn.vpn.OzeVpnService
import kotlinx.coroutines.launch

class MainActivity : ComponentActivity() {
    private lateinit var store: PreferencesStore

    private val vpnConsentLauncher =
        registerForActivityResult(ActivityResultContracts.StartActivityForResult()) { result ->
            if (result.resultCode == RESULT_OK) {
                OzeVpnService.start(this)
            } else {
                TunnelStateHolder.update {
                    it.copy(
                        phase = TunnelPhase.DISCONNECTED,
                        statusDetail = "VPN bağlantısı için tek seferlik Android izni gereklidir",
                    )
                }
            }
        }

    private val notifPermissionLauncher =
        registerForActivityResult(ActivityResultContracts.RequestPermission()) { /* optional */ }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge(
            statusBarStyle = SystemBarStyle.dark(android.graphics.Color.TRANSPARENT),
            navigationBarStyle = SystemBarStyle.dark(android.graphics.Color.TRANSPARENT),
        )
        store = PreferencesStore(this)
        TunnelStateHolder.update {
            it.copy(
                mode = store.loadMode(),
                adBlockEnabled = store.loadAdBlock(),
            )
        }
        requestNotifPermissionIfNeeded()

        setContent {
            OzeVpnTheme {
                val snapshot by TunnelStateHolder.state.collectAsStateWithLifecycle()
                OzeVpnScreen(
                    snapshot = snapshot,
                    onTogglePower = { toggleVpn() },
                    onSelectMode = { mode -> updateMode(mode) },
                    onToggleAdBlock = { enabled -> updateAdBlock(enabled) },
                    onRunProbe = { runLiveProbe() },
                )
            }
        }

        if (intent?.getBooleanExtra(EXTRA_AUTO_CONNECT, false) == true) {
            toggleVpn()
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        if (intent.getBooleanExtra(EXTRA_AUTO_CONNECT, false)) {
            if (TunnelStateHolder.current().phase == TunnelPhase.DISCONNECTED) {
                toggleVpn()
            }
        }
    }

    private fun toggleVpn() {
        val phase = TunnelStateHolder.current().phase
        if (phase == TunnelPhase.CONNECTED || phase == TunnelPhase.CONNECTING) {
            OzeVpnService.stop(this)
            return
        }
        val consent = VpnService.prepare(this)
        if (consent != null) {
            vpnConsentLauncher.launch(consent)
        } else {
            OzeVpnService.start(this)
        }
    }

    private fun updateMode(mode: RoutingMode) {
        if (store.loadMode() == mode) return
        store.saveMode(mode)
        TunnelStateHolder.update { it.copy(mode = mode) }
        OzeVpnService.reloadIfConnected(this)
    }

    private fun updateAdBlock(enabled: Boolean) {
        if (store.loadAdBlock() == enabled) return
        store.saveAdBlock(enabled)
        TunnelStateHolder.update { it.copy(adBlockEnabled = enabled) }
        OzeVpnService.reloadIfConnected(this)
    }

    private fun runLiveProbe() {
        if (TunnelStateHolder.current().probing) return
        TunnelStateHolder.update { it.copy(probing = true) }
        lifecycleScope.launch {
            try {
                val viaTunnel = TunnelStateHolder.current().phase == TunnelPhase.CONNECTED
                val (items, trace) = ConnectivityTester.runAll(viaTunnel)
                TunnelStateHolder.update { snap ->
                    snap.copy(
                        probes = items,
                        probing = false,
                        exitColo = trace?.colo?.ifBlank { snap.exitColo } ?: snap.exitColo,
                        exitCountry = trace?.country?.ifBlank { snap.exitCountry } ?: snap.exitCountry,
                    )
                }
            } catch (_: Throwable) {
                TunnelStateHolder.update { it.copy(probing = false) }
            }
        }
    }

    private fun requestNotifPermissionIfNeeded() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            val granted = ContextCompat.checkSelfPermission(
                this,
                Manifest.permission.POST_NOTIFICATIONS,
            ) == PackageManager.PERMISSION_GRANTED
            if (!granted) {
                notifPermissionLauncher.launch(Manifest.permission.POST_NOTIFICATIONS)
            }
        }
    }

    companion object {
        const val EXTRA_AUTO_CONNECT = "extra_auto_connect"
    }
}
