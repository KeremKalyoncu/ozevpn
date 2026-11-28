package com.keremkalyoncu.ozevpn.vpn

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.net.VpnService
import android.os.Build
import android.os.ParcelFileDescriptor
import android.service.quicksettings.TileService
import androidx.core.app.NotificationCompat
import com.keremkalyoncu.ozevpn.MainActivity
import com.keremkalyoncu.ozevpn.R
import com.keremkalyoncu.ozevpn.core.ConnectivityTester
import com.keremkalyoncu.ozevpn.core.EndpointRacer
import com.keremkalyoncu.ozevpn.core.SingBoxConfigBuilder
import com.keremkalyoncu.ozevpn.core.WarpRegistrar
import com.keremkalyoncu.ozevpn.data.PreferencesStore
import com.keremkalyoncu.ozevpn.data.TunnelPhase
import com.keremkalyoncu.ozevpn.data.TunnelStateHolder
import hev.htproxy.TProxyService
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import java.io.File
import java.net.InetSocketAddress
import java.net.Socket

class OzeVpnService : VpnService() {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val opMutex = Mutex()

    private lateinit var store: PreferencesStore
    private var tunPfd: ParcelFileDescriptor? = null
    private var singBoxProcess: Process? = null
    private var statsJob: Job? = null
    private var connectedAtMs: Long = 0L

    override fun onCreate() {
        super.onCreate()
        store = PreferencesStore(this)
        ensureNotificationChannel()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_STOP -> {
                scope.launch { opMutex.withLock { teardownLocked(stopServiceAfter = true) } }
            }
            ACTION_RELOAD -> {
                scope.launch {
                    opMutex.withLock {
                        if (TunnelStateHolder.current().phase == TunnelPhase.CONNECTED) {
                            teardownLocked(stopServiceAfter = false)
                            startTunnelLocked()
                        }
                    }
                }
            }
            else -> {
                promoteForeground("Bağlanıyor...", "Anadolu Vaşağı şifreli tüneli hazırlıyor")
                scope.launch {
                    opMutex.withLock {
                        teardownLocked(stopServiceAfter = false)
                        startTunnelLocked()
                    }
                }
            }
        }
        return START_STICKY
    }

    override fun onRevoke() {
        scope.launch { opMutex.withLock { teardownLocked(stopServiceAfter = true) } }
        super.onRevoke()
    }

    override fun onDestroy() {
        runCatching { TProxyService.TProxyStopService() }
        runCatching { tunPfd?.close() }
        tunPfd = null
        runCatching { singBoxProcess?.destroyForcibly() }
        singBoxProcess = null
        scope.cancel()
        super.onDestroy()
    }

    private suspend fun startTunnelLocked() {
        val mode = store.loadMode()
        val adBlock = store.loadAdBlock()
        TunnelStateHolder.update {
            it.copy(
                phase = TunnelPhase.CONNECTING,
                mode = mode,
                adBlockEnabled = adBlock,
                statusDetail = "Cloudflare WARP anahtarı hazırlanıyor...",
                lastError = "",
                uptimeSec = 0,
                rxBytes = 0,
                txBytes = 0,
                rxBytesPerSec = 0,
                txBytesPerSec = 0,
            )
        }
        requestTileSync()

        try {
            val singBoxBin = File(applicationInfo.nativeLibraryDir, "libsingbox.so")
            if (!singBoxBin.exists()) {
                throw IllegalStateException("sing-box motoru (libsingbox.so) pakette bulunamadı")
            }

            var profile = WarpRegistrar.ensureProfile(store)

            TunnelStateHolder.update {
                it.copy(statusDetail = "8 WARP sunucusu yarıştırılıyor (en düşük ping)...")
            }
            val race = EndpointRacer.selectBestEndpoint(singBoxBin, cacheDir, profile)
            if (race != null) {
                profile = profile.copy(
                    endpoint = race.endpoint,
                    port = race.port,
                    lastRttMs = race.rttMs,
                    lastColo = race.colo.ifBlank { profile.lastColo },
                    lastProbedEpochSec = System.currentTimeMillis() / 1000L,
                )
                store.saveWarpProfile(profile)
            } else if (profile.endpoint.startsWith("162.159.")) {
                // Never stay pinned to an Istanbul/TR 162.159.x.x endpoint where Roblox is geo-blocked
                val euFallback = WarpRegistrar.defaultCandidates.first()
                profile = profile.copy(
                    endpoint = euFallback.host,
                    port = euFallback.port,
                    lastColo = "",
                )
                store.saveWarpProfile(profile)
            }

            TunnelStateHolder.update {
                it.copy(statusDetail = "Şifreli WireGuard (TCP+UDP) tüneli başlatılıyor...")
            }

            val sbCfgFile = File(filesDir, "sing-box-active.json")
            sbCfgFile.writeText(
                SingBoxConfigBuilder.buildTunnelConfig(
                    profile = profile,
                    mode = mode,
                    adBlockEnabled = adBlock,
                ),
                Charsets.UTF_8,
            )

            val sbProc = ProcessBuilder(singBoxBin.absolutePath, "run", "-c", sbCfgFile.absolutePath)
                .directory(filesDir)
                .redirectErrorStream(true)
                .start()
            singBoxProcess = sbProc

            // Drain process stdout/stderr in background so pipe buffers never block
            scope.launch {
                runCatching {
                    sbProc.inputStream.bufferedReader().use { r ->
                        while (r.readLine() != null) { /* discard */ }
                    }
                }
            }

            if (!waitForLocalPort(SingBoxConfigBuilder.LOCAL_MIXED_PORT, 3500L)) {
                val exited = !sbProc.isAlive
                throw IllegalStateException(
                    if (exited) "sing-box motoru başlatılamadı (çıkış=${sbProc.exitValue()})"
                    else "Yerel tünel portu açılamadı"
                )
            }

            val builder = Builder()
                .setSession("OzeVPN")
                .setMtu(1280)
                .addAddress("10.233.233.1", 30)
                .addRoute("0.0.0.0", 0)
                .addDnsServer("1.1.1.1")
                .addDnsServer("1.0.0.1")

            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
                builder.setMetered(false)
            }
            // Exclude our own app UID so libsingbox.so traffic leaves directly over Wi-Fi/5G without looping
            runCatching { builder.addDisallowedApplication(packageName) }

            val pfd = builder.establish()
                ?: throw IllegalStateException("Android VPN izni alınamadı veya başka bir VPN aktif")
            tunPfd = pfd

            val hevYamlFile = File(filesDir, "hev-tunnel.yml")
            hevYamlFile.writeText(SingBoxConfigBuilder.buildHevTunnelYaml(), Charsets.UTF_8)

            val started = TProxyService.TProxyStartService(hevYamlFile.absolutePath, pfd.fd)
            if (!started) {
                throw IllegalStateException("TUN köprüsü (hev-socks5-tunnel) başlatılamadı")
            }

            connectedAtMs = System.currentTimeMillis()
            val colo = race?.colo?.ifBlank { profile.lastColo } ?: profile.lastColo
            val country = race?.country.orEmpty()
            val rtt = race?.rttMs ?: profile.lastRttMs

            TunnelStateHolder.update {
                it.copy(
                    phase = TunnelPhase.CONNECTED,
                    statusDetail = "Discord (UDP) · Roblox · Tüm İnternet Aktif",
                    exitColo = colo,
                    exitCountry = country,
                    endpointLabel = "${profile.endpoint}:${profile.port}",
                    rttMs = rtt,
                    lastError = "",
                )
            }
            val notifTitle = buildNotifTitle(colo, rtt)
            promoteForeground(notifTitle, "Discord, Roblox ve tüm internet korunuyor")
            requestTileSync()

            // Enrich exit country/POP and true warm 1-RTT ping asynchronously via encrypted tunnel
            scope.launch {
                val trace = ConnectivityTester.fetchTrace(viaTunnel = true)
                if (trace != null && TunnelStateHolder.current().phase == TunnelPhase.CONNECTED) {
                    TunnelStateHolder.update {
                        it.copy(
                            exitColo = trace.colo.ifBlank { it.exitColo },
                            exitCountry = trace.country.ifBlank { it.exitCountry },
                            rttMs = if (trace.rttMs > 0) trace.rttMs else it.rttMs,
                        )
                    }
                }
            }

            startStatsLoop()
        } catch (t: Throwable) {
            teardownLocked(stopServiceAfter = false)
            val errMsg = t.message ?: "Bilinmeyen bağlantı hatası"
            TunnelStateHolder.update {
                it.copy(
                    phase = TunnelPhase.ERROR,
                    statusDetail = "Bağlantı kurulamadı",
                    lastError = errMsg,
                )
            }
            requestTileSync()
            stopForeground(STOP_FOREGROUND_REMOVE)
            stopSelf()
        }
    }

    private fun startStatsLoop() {
        statsJob?.cancel()
        statsJob = scope.launch {
            var prevRx = 0L
            var prevTx = 0L
            var tick = 0
            while (isActive) {
                delay(1000L)
                val proc = singBoxProcess
                if (proc == null || !proc.isAlive) {
                    opMutex.withLock { teardownLocked(stopServiceAfter = true) }
                    break
                }
                val raw = runCatching { TProxyService.TProxyGetStats() }.getOrNull()
                val txBytes = if (raw != null && raw.size >= 4) raw[1] else prevTx
                val rxBytes = if (raw != null && raw.size >= 4) raw[3] else prevRx
                val rxRate = (rxBytes - prevRx).coerceAtLeast(0L)
                val txRate = (txBytes - prevTx).coerceAtLeast(0L)
                prevRx = rxBytes
                prevTx = txBytes

                val upSec = ((System.currentTimeMillis() - connectedAtMs) / 1000L).coerceAtLeast(0L)
                TunnelStateHolder.update {
                    it.copy(
                        uptimeSec = upSec,
                        rxBytes = rxBytes,
                        txBytes = txBytes,
                        rxBytesPerSec = rxRate,
                        txBytesPerSec = txRate,
                    )
                }

                tick++
                if (tick % 3 == 0) {
                    val snap = TunnelStateHolder.current()
                    val nm = getSystemService(NotificationManager::class.java)
                    nm?.notify(
                        NOTIF_ID,
                        buildNotification(
                            buildNotifTitle(snap.exitColo, snap.rttMs),
                            "↓ ${formatRate(rxRate)} · ↑ ${formatRate(txRate)}",
                        ),
                    )
                }
            }
        }
    }

    private fun teardownLocked(stopServiceAfter: Boolean) {
        statsJob?.cancel()
        statsJob = null
        runCatching { TProxyService.TProxyStopService() }
        runCatching { tunPfd?.close() }
        tunPfd = null
        runCatching { singBoxProcess?.destroyForcibly() }
        singBoxProcess = null
        runCatching { File(filesDir, "sing-box-active.json").delete() }

        if (stopServiceAfter) {
            TunnelStateHolder.update {
                it.copy(
                    phase = TunnelPhase.DISCONNECTED,
                    statusDetail = "Hazır — Vaşağa dokun, Discord, Roblox ve tüm internete bağlan",
                    uptimeSec = 0,
                    rxBytesPerSec = 0,
                    txBytesPerSec = 0,
                    lastError = "",
                )
            }
            requestTileSync()
            stopForeground(STOP_FOREGROUND_REMOVE)
            stopSelf()
        }
    }

    private fun promoteForeground(title: String, body: String) {
        val notif = buildNotification(title, body)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
            startForeground(NOTIF_ID, notif, ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE)
        } else {
            startForeground(NOTIF_ID, notif)
        }
    }

    private fun buildNotification(title: String, body: String): Notification {
        val openIntent = PendingIntent.getActivity(
            this,
            1,
            Intent(this, MainActivity::class.java).apply {
                flags = Intent.FLAG_ACTIVITY_SINGLE_TOP or Intent.FLAG_ACTIVITY_CLEAR_TOP
            },
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
        )
        val stopIntent = PendingIntent.getService(
            this,
            2,
            Intent(this, OzeVpnService::class.java).apply { action = ACTION_STOP },
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
        )
        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setSmallIcon(R.drawable.ic_lynx_tile)
            .setContentTitle(title)
            .setContentText(body)
            .setOngoing(true)
            .setOnlyAlertOnce(true)
            .setSilent(true)
            .setContentIntent(openIntent)
            .addAction(0, "Kes", stopIntent)
            .build()
    }

    private fun ensureNotificationChannel() {
        val nm = getSystemService(NotificationManager::class.java) ?: return
        val ch = NotificationChannel(
            CHANNEL_ID,
            getString(R.string.notif_channel_name),
            NotificationManager.IMPORTANCE_LOW,
        ).apply {
            description = getString(R.string.notif_channel_desc)
            setShowBadge(false)
        }
        nm.createNotificationChannel(ch)
    }

    private fun requestTileSync() {
        runCatching {
            TileService.requestListeningState(this, ComponentName(this, OzeQuickTileService::class.java))
        }
    }

    private fun waitForLocalPort(port: Int, timeoutMs: Long): Boolean {
        val deadline = System.currentTimeMillis() + timeoutMs
        while (System.currentTimeMillis() < deadline) {
            val ok = runCatching {
                Socket().use { s ->
                    s.connect(InetSocketAddress("127.0.0.1", port), 150)
                    true
                }
            }.getOrDefault(false)
            if (ok) return true
            Thread.sleep(75)
        }
        return false
    }

    private fun buildNotifTitle(colo: String, rttMs: Int): String = when {
        colo.isNotBlank() && rttMs > 0 -> "OzeVPN — Bağlı ($colo · ${rttMs} ms)"
        colo.isNotBlank() -> "OzeVPN — Bağlı ($colo)"
        else -> "OzeVPN — Bağlı (Anadolu Vaşağı)"
    }

    private fun formatRate(bps: Long): String = when {
        bps < 1024L -> "$bps B/s"
        bps < 1024L * 1024L -> String.format("%.1f KB/s", bps / 1024.0)
        else -> String.format("%.1f MB/s", bps / (1024.0 * 1024.0))
    }

    companion object {
        const val ACTION_START = "com.keremkalyoncu.ozevpn.ACTION_START"
        const val ACTION_STOP = "com.keremkalyoncu.ozevpn.ACTION_STOP"
        const val ACTION_RELOAD = "com.keremkalyoncu.ozevpn.ACTION_RELOAD"
        private const val CHANNEL_ID = "ozevpn_tunnel"
        private const val NOTIF_ID = 6201

        fun start(context: Context) {
            val i = Intent(context, OzeVpnService::class.java).apply { action = ACTION_START }
            context.startForegroundService(i)
        }

        fun stop(context: Context) {
            val i = Intent(context, OzeVpnService::class.java).apply { action = ACTION_STOP }
            context.startService(i)
        }

        fun reloadIfConnected(context: Context) {
            if (TunnelStateHolder.current().phase == TunnelPhase.CONNECTED) {
                val i = Intent(context, OzeVpnService::class.java).apply { action = ACTION_RELOAD }
                context.startService(i)
            }
        }
    }
}
