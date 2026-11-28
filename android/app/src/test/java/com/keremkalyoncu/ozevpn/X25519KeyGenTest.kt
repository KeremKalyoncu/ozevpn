package com.keremkalyoncu.ozevpn

import com.keremkalyoncu.ozevpn.core.X25519KeyGen
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Test
import java.util.Base64

class X25519KeyGenTest {
    @Test
    fun matchesRfc7748Section52TestVector() {
        // Official RFC 7748 Section 5.2 test vector
        val scalar = hexToBytes("a546e36bf0527c9d3b16154b82465edd62144c0ac1fc5a18506a2244ba449ac4")
        val uCoord = hexToBytes("e6db6867583030db3594c1a424b15f7c726624ec26b3353b10a903a6d0ab1c4c")
        val expected = "c3da55379de9c6908e94ea4df28d084f32eccf03491c71f754b4075577a28552"

        val actual = X25519KeyGen.scalarMult(scalar, uCoord).joinToString("") { "%02x".format(it) }
        assertEquals(expected, actual)
    }

    @Test
    fun generatesValidBase64KeyPair() {
        val kp1 = X25519KeyGen.generateKeyPair()
        val kp2 = X25519KeyGen.generateKeyPair()
        assertEquals(32, Base64.getDecoder().decode(kp1.privateKeyBase64).size)
        assertEquals(32, Base64.getDecoder().decode(kp1.publicKeyBase64).size)
        assertNotEquals(kp1.privateKeyBase64, kp2.privateKeyBase64)
        assertNotEquals(kp1.publicKeyBase64, kp2.publicKeyBase64)
    }

    private fun hexToBytes(hex: String): ByteArray =
        ByteArray(hex.length / 2) { i -> hex.substring(i * 2, i * 2 + 2).toInt(16).toByte() }
}
