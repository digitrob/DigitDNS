package com.digitrob.digitdns.core.network

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The problem document is the one error shape every v3 route uses, and two fields on
 * it drive real branching: `detail` is what the operator reads, and
 * `twofactor_required` is what turns a failed login into a code prompt. Both are
 * pinned here, along with the tolerant decoding a server that adds members must not
 * break.
 */
class ProblemTest {

    @Test
    fun parsesARealProblemBody() {
        val body = """
            {"type":"https://digitdns.dev/problems/unauthorized","title":"Unauthorized",
             "status":401,"detail":"invalid credentials","instance":"/api/v3/auth/login"}
        """.trimIndent()

        val problem = Problem.parseOrNull(body)!!

        assertEquals(401, problem.status)
        assertEquals("invalid credentials", problem.detail)
        assertEquals("invalid credentials", problem.message)
        assertFalse(problem.twoFactorRequired)
    }

    @Test
    fun readsTheTwoFactorFlagTheServerAddsOnALoginThatNeedsACode() {
        val body = """
            {"type":"https://digitdns.dev/problems/unauthorized","title":"Unauthorized",
             "status":401,"detail":"invalid credentials","twofactor_required":true}
        """.trimIndent()

        val problem = Problem.parseOrNull(body)!!
        assertTrue(problem.twoFactorRequired)

        val exception = ApiException.from(status = 401, problem = problem)
        assertTrue(exception.twoFactorRequired)
        assertTrue(exception.isUnauthorized)
    }

    @Test
    fun messageFallsBackThroughTitleThenStatus() {
        assertEquals("Title only", Problem(status = 500, title = "Title only").message)
        assertEquals("Request failed (HTTP 500)", Problem(status = 500).message)
    }

    @Test
    fun ignoresUnknownMembers() {
        val body = """{"status":403,"detail":"not exposed","api_bind_list":"100.64.0.0/10"}"""
        val problem = Problem.parseOrNull(body)!!
        assertEquals(403, problem.status)
        assertEquals("not exposed", problem.detail)
    }

    @Test
    fun nonJsonBodiesAreNotProblems() {
        assertNull(Problem.parseOrNull("<html>403</html>"))
        assertNull(Problem.parseOrNull(""))
        assertNull(Problem.parseOrNull(null))
    }
}
