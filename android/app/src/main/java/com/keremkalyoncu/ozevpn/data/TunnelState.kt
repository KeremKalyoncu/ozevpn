package com.keremkalyoncu.ozevpn.data

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update

enum class TunnelPhase {
    DISCONNECTED,
    CONNECTING,
    CONNECTED,
    ERROR,
}

enum class RoutingMode(val key: String) {
    FULL("full"),
    HYBRID_TR("hybrid_tr");

    companion object {
        fun fromKey(k: String?): RoutingMode = entries.firstOrNull { it.key == k } ?: FULL
    }
}

data class ProbeItem(
    val name: String,
    val reachable: Boolean,
    val latencyMs: Long,
    val detail: String,
)

data class TunnelSnapshot(
    val phase: TunnelPhase = TunnelPhase.DISCONNECTED,
    val statusDetail: String = "Hazır — Vaşağa dokun, Discord, Roblox ve tüm internete bağlan",
    val mode: RoutingMode = RoutingMode.FULL,
    val adBlockEnabled: Boolean = true,
    val exitColo: String = "",
    val exitCountry: String = "",
    val endpointLabel: String = "",
    val rttMs: Int = 0,
    val uptimeSec: Long = 0,
    val rxBytes: Long = 0,
    val txBytes: Long = 0,
    val rxBytesPerSec: Long = 0,
    val txBytesPerSec: Long = 0,
    val lastError: String = "",
    val probes: List<ProbeItem> = emptyList(),
    val probing: Boolean = false,
)

object TunnelStateHolder {
    private val _state = MutableStateFlow(TunnelSnapshot())
    val state: StateFlow<TunnelSnapshot> = _state.asStateFlow()

    fun update(transform: (TunnelSnapshot) -> TunnelSnapshot) {
        _state.update(transform)
    }

    fun current(): TunnelSnapshot = _state.value
}
