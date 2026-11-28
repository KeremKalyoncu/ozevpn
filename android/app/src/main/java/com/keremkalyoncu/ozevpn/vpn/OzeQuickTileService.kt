package com.keremkalyoncu.ozevpn.vpn

import android.app.PendingIntent
import android.content.Intent
import android.net.VpnService
import android.os.Build
import android.service.quicksettings.Tile
import android.service.quicksettings.TileService
import com.keremkalyoncu.ozevpn.MainActivity
import com.keremkalyoncu.ozevpn.data.TunnelPhase
import com.keremkalyoncu.ozevpn.data.TunnelStateHolder

class OzeQuickTileService : TileService() {
    override fun onStartListening() {
        super.onStartListening()
        syncTile()
    }

    override fun onClick() {
        super.onClick()
        val phase = TunnelStateHolder.current().phase
        if (phase == TunnelPhase.CONNECTED || phase == TunnelPhase.CONNECTING) {
            OzeVpnService.stop(this)
            syncTile()
            return
        }
        val prep = VpnService.prepare(this)
        if (prep == null) {
            OzeVpnService.start(this)
        } else {
            // User has not granted VPN consent yet; open MainActivity cleanly
            val launch = Intent(this, MainActivity::class.java).apply {
                flags = Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_SINGLE_TOP
                putExtra(MainActivity.EXTRA_AUTO_CONNECT, true)
            }
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
                val pi = PendingIntent.getActivity(
                    this,
                    10,
                    launch,
                    PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
                )
                startActivityAndCollapse(pi)
            } else {
                @Suppress("DEPRECATION")
                startActivityAndCollapse(launch)
            }
        }
        syncTile()
    }

    private fun syncTile() {
        val tile = qsTile ?: return
        val snap = TunnelStateHolder.current()
        when (snap.phase) {
            TunnelPhase.CONNECTED -> {
                tile.state = Tile.STATE_ACTIVE
                tile.label = "OzeVPN"
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
                    tile.subtitle = snap.exitColo.ifBlank { "Bağlı" }
                }
            }
            TunnelPhase.CONNECTING -> {
                tile.state = Tile.STATE_ACTIVE
                tile.label = "OzeVPN"
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
                    tile.subtitle = "Bağlanıyor..."
                }
            }
            else -> {
                tile.state = Tile.STATE_INACTIVE
                tile.label = "OzeVPN"
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
                    tile.subtitle = "Kapalı"
                }
            }
        }
        tile.updateTile()
    }
}
