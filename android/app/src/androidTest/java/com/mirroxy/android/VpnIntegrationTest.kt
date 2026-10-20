package com.mirroxy.android

import android.content.Intent
import android.net.VpnService
import android.os.SystemClock
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import org.junit.Assert.*
import org.junit.Assume.assumeTrue
import org.junit.Test
import org.junit.runner.RunWith
import java.net.InetAddress
import java.net.ServerSocket
import java.net.URL
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit

/** Explicit opt-in only: human must approve Android's VPN dialog. */
@RunWith(AndroidJUnit4::class)
class VpnIntegrationTest {
    @Test fun humanAuthorizedVpnCarriesIpv4AndIpv6() {
        assumeTrue(InstrumentationRegistry.getArguments().getString("vpn") == "true")
        val instrument = InstrumentationRegistry.getInstrumentation()
        val context = instrument.targetContext
        check(Runtime.state.value.nodeId == null)
        val prefs = context.getSharedPreferences("ui", android.content.Context.MODE_PRIVATE)
        val previous = prefs.getStringSet("apps", emptySet()).orEmpty().toSet()
        val store = Repository.store(context)
        val before = store.nodes.value.map { it.id }.toSet()
        val upstream = ServerSocket(0, 2, InetAddress.getByName("127.0.0.1"))
        upstream.soTimeout = 15000
        val executor = Executors.newSingleThreadExecutor()
        var node: Node? = null
        try {
            store.import("http://127.0.0.1:${upstream.localPort}#isolated-VPN-QA")
            node = store.nodes.value.first { it.id !in before }
            prefs.edit().putStringSet("apps", setOf(context.packageName)).commit()
            instrument.startActivitySync(Intent(context, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
            val consent = VpnService.prepare(context)
            if (consent != null) context.startActivity(consent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
            val deadline = SystemClock.elapsedRealtime() + 120000
            while (VpnService.prepare(context) != null && SystemClock.elapsedRealtime() < deadline) SystemClock.sleep(250)
            assertNull("Human VPN consent was not granted within 120 seconds", VpnService.prepare(context))
            val server = executor.submit {
                repeat(2) {
                    upstream.accept().use { socket ->
                        socket.soTimeout = 10000
                        val reader = socket.getInputStream().bufferedReader()
                        check(reader.readLine().startsWith("CONNECT "))
                        while (!reader.readLine().isNullOrEmpty()) { }
                        socket.getOutputStream().write("HTTP/1.1 200 Connection Established\r\n\r\n".toByteArray())
                        check(reader.readLine().startsWith("GET /probe"))
                        while (!reader.readLine().isNullOrEmpty()) { }
                        socket.getOutputStream().write("HTTP/1.1 200 OK\r\nContent-Length: 11\r\nConnection: close\r\n\r\nvpn-test-ok".toByteArray())
                    }
                }
            }
            context.startForegroundService(Intent(context, TunnelService::class.java).putExtra("node", node.id))
            val started = SystemClock.elapsedRealtime() + 10000
            while (Runtime.state.value.nodeId != node.id && SystemClock.elapsedRealtime() < started) SystemClock.sleep(100)
            assertEquals("VPN runtime: ${Runtime.state.value}", node.id, Runtime.state.value.nodeId)
            listOf("http://198.18.0.2/probe", "http://[2001:db8::2]/probe").forEach { address ->
                // No Proxy object: this request must enter Android's VPN and native TUN.
                val connection = URL(address).openConnection()
                connection.connectTimeout = 10000; connection.readTimeout = 10000
                assertEquals("vpn-test-ok", connection.getInputStream().bufferedReader().use { it.readText() })
            }
            server.get(15, TimeUnit.SECONDS)
        } finally {
            context.stopService(Intent(context, TunnelService::class.java))
            val deadline = SystemClock.elapsedRealtime() + 10000
            while (Runtime.state.value.nodeId != null && SystemClock.elapsedRealtime() < deadline) SystemClock.sleep(100)
            prefs.edit().putStringSet("apps", previous).commit()
            node?.let { store.delete(it.id) }
            upstream.close(); executor.shutdownNow()
        }
    }
}
