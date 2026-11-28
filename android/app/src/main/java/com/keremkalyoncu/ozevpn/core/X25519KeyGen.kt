package com.keremkalyoncu.ozevpn.core

import java.math.BigInteger
import java.security.SecureRandom
import java.util.Base64

/**
 * Pure RFC 7748 X25519 keypair generator with zero external crypto dependencies.
 * Works identically on every Android API level (26–36) without BouncyCastle bloat.
 */
object X25519KeyGen {
    private val P: BigInteger = BigInteger.ONE.shiftLeft(255).subtract(BigInteger.valueOf(19))
    private val A24: BigInteger = BigInteger.valueOf(121665)
    private val BASE_U = ByteArray(32).apply { this[0] = 9 }

    data class KeyPairB64(val privateKeyBase64: String, val publicKeyBase64: String)

    fun generateKeyPair(): KeyPairB64 {
        val priv = ByteArray(32)
        SecureRandom().nextBytes(priv)
        clampScalar(priv)
        val pub = scalarMult(priv, BASE_U)
        val enc = Base64.getEncoder()
        return KeyPairB64(
            privateKeyBase64 = enc.encodeToString(priv),
            publicKeyBase64 = enc.encodeToString(pub),
        )
    }

    internal fun clampScalar(k: ByteArray) {
        k[0] = (k[0].toInt() and 248).toByte()
        k[31] = (k[31].toInt() and 127).toByte()
        k[31] = (k[31].toInt() or 64).toByte()
    }

    internal fun scalarMult(scalarBytes: ByteArray, uBytes: ByteArray): ByteArray {
        val k = scalarBytes.copyOf(32)
        clampScalar(k)
        val uCopy = uBytes.copyOf(32)
        uCopy[31] = (uCopy[31].toInt() and 127).toByte()

        val x1 = decodeLittleEndian(uCopy).mod(P)
        var x2 = BigInteger.ONE
        var z2 = BigInteger.ZERO
        var x3 = x1
        var z3 = BigInteger.ONE
        var swap = 0

        for (t in 254 downTo 0) {
            val kt = (k[t ushr 3].toInt() ushr (t and 7)) and 1
            swap = swap xor kt
            if (swap != 0) {
                val tx = x2; x2 = x3; x3 = tx
                val tz = z2; z2 = z3; z3 = tz
            }
            swap = kt

            val a = x2.add(z2).mod(P)
            val aa = a.multiply(a).mod(P)
            val b = x2.subtract(z2).mod(P)
            val bb = b.multiply(b).mod(P)
            val e = aa.subtract(bb).mod(P)
            val c = x3.add(z3).mod(P)
            val d = x3.subtract(z3).mod(P)
            val da = d.multiply(a).mod(P)
            val cb = c.multiply(b).mod(P)

            val x3Sum = da.add(cb).mod(P)
            x3 = x3Sum.multiply(x3Sum).mod(P)
            val z3Diff = da.subtract(cb).mod(P)
            z3 = x1.multiply(z3Diff.multiply(z3Diff).mod(P)).mod(P)
            x2 = aa.multiply(bb).mod(P)
            z2 = e.multiply(aa.add(A24.multiply(e).mod(P)).mod(P)).mod(P)
        }

        if (swap != 0) {
            val tx = x2; x2 = x3; x3 = tx
            val tz = z2; z2 = z3; z3 = tz
        }

        val res = x2.multiply(z2.modInverse(P)).mod(P)
        return encodeLittleEndian(res)
    }

    private fun decodeLittleEndian(bytes: ByteArray): BigInteger {
        val be = ByteArray(bytes.size)
        for (i in bytes.indices) be[i] = bytes[bytes.size - 1 - i]
        return BigInteger(1, be)
    }

    private fun encodeLittleEndian(value: BigInteger): ByteArray {
        val out = ByteArray(32)
        val be = value.toByteArray()
        var outIdx = 0
        for (i in be.size - 1 downTo 0) {
            if (outIdx >= 32) break
            out[outIdx++] = be[i]
        }
        return out
    }
}
