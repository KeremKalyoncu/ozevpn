package com.keremkalyoncu.ozevpn

import com.keremkalyoncu.ozevpn.core.UpdateChecker
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class UpdateCheckerTest {
    @Test
    fun comparesSemverTagsCorrectly() {
        assertTrue(UpdateChecker.isNewerVersion("v6.2.1", "6.2.0"))
        assertTrue(UpdateChecker.isNewerVersion("v6.3.0", "6.2.9"))
        assertTrue(UpdateChecker.isNewerVersion("v7.0.0", "6.9.9"))
        assertFalse(UpdateChecker.isNewerVersion("v6.2.0", "6.2.0"))
        assertFalse(UpdateChecker.isNewerVersion("v6.1.9", "6.2.0"))
        assertFalse(UpdateChecker.isNewerVersion("invalid", "6.2.0"))
    }
}
