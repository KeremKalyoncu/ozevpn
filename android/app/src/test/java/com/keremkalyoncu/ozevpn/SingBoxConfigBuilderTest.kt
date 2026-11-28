package com.keremkalyoncu.ozevpn

import com.keremkalyoncu.ozevpn.core.SingBoxConfigBuilder
import com.keremkalyoncu.ozevpn.core.WarpRegistrar
import com.keremkalyoncu.ozevpn.data.RoutingMode
import com.keremkalyoncu.ozevpn.data.WarpProfile
import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class SingBoxConfigBuilderTest {
    private val sampleProfile = WarpProfile(
        privateKey = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
        addressV4 = "172.16.0.2/32",
        addressV6 = "2606:4700:110:8a36:dfab:1234:5678:9abc/128",
        peerPublicKey = "bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo=",
        endpoint = "162.159.192.1",
        port = 2408,
        reserved = listOf(12, 34, 56),
    )

    @Test
    fun buildsValidFullModeConfigWithWarpReservedAndAdBlock() {
        val raw = SingBoxConfigBuilder.buildTunnelConfig(sampleProfile, RoutingMode.FULL, adBlockEnabled = true)
        val obj = JSONObject(raw)

        val ep = obj.getJSONArray("endpoints").getJSONObject(0)
        assertEquals("wireguard", ep.getString("type"))
        assertEquals(1280, ep.getInt("mtu"))
        val peer = ep.getJSONArray("peers").getJSONObject(0)
        assertEquals(3, peer.getJSONArray("reserved").length())
        assertEquals(12, peer.getJSONArray("reserved").getInt(0))

        val route = obj.getJSONObject("route")
        assertFalse(route.getBoolean("auto_detect_interface"))
        assertEquals("warp-fast", route.getString("final"))
        assertEquals("dns-warp", route.getString("default_domain_resolver"))
        assertEquals("dns-warp", obj.getJSONObject("dns").getString("final"))
        assertEquals("ipv4_only", obj.getJSONObject("dns").getString("strategy"))
        assertTrue(raw.contains("doubleclick.net"))
        assertFalse(raw.contains("gov.tr"))
    }

    @Test
    fun buildsHybridTrModeWithGovAndBankBypass() {
        val raw = SingBoxConfigBuilder.buildTunnelConfig(sampleProfile, RoutingMode.HYBRID_TR, adBlockEnabled = false)
        assertTrue(raw.contains("gov.tr"))
        assertTrue(raw.contains("ziraatbank.com.tr"))
        assertFalse(raw.contains("doubleclick.net"))
    }

    @Test
    fun buildsMultiProbeConfigForAll8WarpCandidates() {
        val candidates = WarpRegistrar.defaultCandidates
        val ports = (19000 until 19000 + candidates.size).toList()
        val obj = JSONObject(SingBoxConfigBuilder.buildMultiProbeConfig(sampleProfile, candidates, ports))
        assertEquals(8, obj.getJSONArray("inbounds").length())
        assertEquals(8, obj.getJSONArray("endpoints").length())
    }

    @Test
    fun hevTunnelYamlEnablesDualStackIpv4AndIpv6() {
        val yaml = SingBoxConfigBuilder.buildHevTunnelYaml()
        assertTrue(yaml.contains("ipv4: 10.233.233.1"))
        assertTrue(yaml.contains("ipv6: 'fdfe:dcba:9876::1'"))
        assertTrue(yaml.contains("udp: 'udp'"))
    }
}
