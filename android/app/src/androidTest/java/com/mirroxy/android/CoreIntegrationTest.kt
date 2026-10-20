package com.mirroxy.android

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import org.junit.Test
import org.junit.Assert.*
import org.junit.runner.RunWith
import org.json.JSONArray
import org.json.JSONObject
import java.net.*
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit
import java.io.File
import android.content.Intent
import android.os.SystemClock

@RunWith(AndroidJUnit4::class)
class CoreIntegrationTest {
    private fun freePort() = ServerSocket(0, 1, InetAddress.getByName("127.0.0.1")).use { it.localPort }
    @Test fun encryptedShadowsocksThroughHttpAndSocks() {
        check(Runtime.state.value.nodeId == null) { "Stop the user service before running isolated QA" }
        val origin = ServerSocket(0, 2, InetAddress.getByName("127.0.0.1"))
        origin.soTimeout = 15000
        val executor = Executors.newSingleThreadExecutor()
        val server = executor.submit {
            repeat(2) {
                origin.accept().use { client ->
                    client.soTimeout = 10000
                    val reader = client.getInputStream().bufferedReader()
                    while (!reader.readLine().isNullOrEmpty()) { }
                    val payload = "mirroxy-isolated-core-ok"
                    client.getOutputStream().write("HTTP/1.1 200 OK\r\nContent-Length: ${payload.length}\r\nConnection: close\r\n\r\n$payload".toByteArray())
                }
            }
        }
        val httpPort = freePort(); val socksPort = freePort(); val encryptedPort = freePort()
        val secret = "isolated-test-only-password"
        val ssOutbound = JSONObject().put("protocol", "shadowsocks").put("tag", "encrypted")
            .put("settings", JSONObject().put("servers", JSONArray().put(JSONObject().put("address", "127.0.0.1").put("port", encryptedPort).put("method", "aes-128-gcm").put("password", secret))))
        val inbounds = JSONArray()
            .put(JSONObject().put("protocol", "http").put("tag", "test-http").put("listen", "127.0.0.1").put("port", httpPort).put("settings", JSONObject()))
            .put(JSONObject().put("protocol", "socks").put("tag", "test-socks").put("listen", "127.0.0.1").put("port", socksPort).put("settings", JSONObject()))
            .put(JSONObject().put("protocol", "shadowsocks").put("tag", "ss-server").put("listen", "127.0.0.1").put("port", encryptedPort)
                .put("settings", JSONObject().put("method", "aes-128-gcm").put("password", secret).put("network", "tcp")))
        val config = JSONObject().put("log", JSONObject().put("loglevel", "debug")).put("inbounds", inbounds)
            .put("outbounds", JSONArray().put(ssOutbound).put(JSONObject().put("protocol", "freedom").put("tag", "origin")
                .put("settings", JSONObject().put("finalRules", JSONArray().put(JSONObject().put("action", "allow").put("ip", JSONArray().put("127.0.0.1/32")).put("port", origin.localPort.toString()))))))
            .put("routing", JSONObject().put("rules", JSONArray().put(JSONObject().put("type", "field").put("inboundTag", JSONArray().put("ss-server")).put("outboundTag", "origin"))))
        try {
            Core.call("runXray", JSONObject().put("xrayJson", config.toString()))
            val connection = URL("http://127.0.0.1:${origin.localPort}/probe").openConnection(Proxy(Proxy.Type.HTTP, InetSocketAddress("127.0.0.1", httpPort)))
            connection.connectTimeout = 10000; connection.readTimeout = 10000
            assertEquals("mirroxy-isolated-core-ok", connection.getInputStream().bufferedReader().use { it.readText() })
            // A SOCKS socket explicitly targets an IP, preventing platform proxy exclusions.
            Socket(Proxy(Proxy.Type.SOCKS, InetSocketAddress("127.0.0.1", socksPort))).use { socket ->
                socket.soTimeout = 10000
                socket.connect(InetSocketAddress("127.0.0.1", origin.localPort), 10000)
                socket.getOutputStream().write("GET /probe HTTP/1.1\r\nHost: isolated\r\nConnection: close\r\n\r\n".toByteArray())
                assertTrue(socket.getInputStream().bufferedReader().readText().endsWith("mirroxy-isolated-core-ok"))
            }
            server.get(15, TimeUnit.SECONDS)
        } finally { Core.stop(); origin.close(); executor.shutdownNow() }
    }
    @Test fun encryptedPersistenceRenameAndFixedPorts() {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        val name = "qa-nodes.enc"
        val file = File(context.noBackupFilesDir, name)
        file.delete()
        try {
            val store = NodeStore(context, name)
            val raw = "{\"outbounds\":[{\"protocol\":\"socks\",\"tag\":\"isolated-node\",\"settings\":{\"servers\":[{\"address\":\"127.0.0.1\",\"port\":39001}]}}]}"
            assertEquals(1, store.import(raw))
            val first = store.nodes.value.single()
            store.rename(first.id, "private-test-name")
            val reload = NodeStore(context, name).nodes.value.single()
            assertEquals(first.port, reload.port); assertEquals("private-test-name", reload.name)
            assertFalse(String(file.readBytes()).contains("private-test-name"))
            assertEquals(0, store.import(raw))
            store.replace(first.id, "http://127.0.0.1:39002#replacement")
            val updated = NodeStore(context, name).nodes.value.single()
            assertEquals(first.port, updated.port); assertEquals("private-test-name", updated.name)
            assertEquals("http", updated.protocol)
            store.delete(first.id); assertTrue(NodeStore(context, name).nodes.value.isEmpty())
        } finally { file.delete() }
    }
    @Test fun supportedShareLinksUseOfficialParser() {
        val links = listOf(
            "vless://00000000-0000-4000-8000-000000000001@127.0.0.1:39001?encryption=none#isolated-vless",
            "trojan://isolated-password@127.0.0.1:39001?security=tls&sni=example.com#isolated-trojan",
            "ss://YWVzLTEyOC1nY206aXNvbGF0ZWQtcGFzc3dvcmQ@127.0.0.1:39001#isolated-ss",
            "socks://127.0.0.1:39001#isolated-socks",
            "http://test-user:test-pass@127.0.0.1:39001#isolated-http"
        )
        links.forEach { assertEquals(1, Importer.parse(it).length()) }
        val encoded = android.util.Base64.encodeToString(links.joinToString("\n").toByteArray(), android.util.Base64.NO_WRAP)
        assertEquals(5, Importer.parse(encoded).length())
    }
    @Test fun foregroundServiceCarriesRealHttpRequestAndStops() {
        check(Runtime.state.value.nodeId == null)
        val instrument = InstrumentationRegistry.getInstrumentation()
        val context = instrument.targetContext
        val upstream = ServerSocket(0, 1, InetAddress.getByName("127.0.0.1"))
        upstream.soTimeout = 15000
        val executor = Executors.newSingleThreadExecutor()
        val server = executor.submit {
            upstream.accept().use { socket ->
                socket.soTimeout = 10000
                val reader = socket.getInputStream().bufferedReader()
                val first = reader.readLine()
                check(first.startsWith("CONNECT "))
                while (!reader.readLine().isNullOrEmpty()) { }
                socket.getOutputStream().write("HTTP/1.1 200 Connection Established\r\n\r\n".toByteArray())
                check(reader.readLine().startsWith("GET /probe"))
                while (!reader.readLine().isNullOrEmpty()) { }
                socket.getOutputStream().write("HTTP/1.1 200 OK\r\nContent-Length: 10\r\nConnection: close\r\n\r\nservice-ok".toByteArray())
            }
        }
        val store = Repository.store(context)
        val before = store.nodes.value.map { it.id }.toSet()
        var node: Node? = null
        try {
            store.import("http://127.0.0.1:${upstream.localPort}#isolated-service-QA")
            node = store.nodes.value.first { it.id !in before }
            val activity = instrument.startActivitySync(Intent(context, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
            context.startForegroundService(Intent(context, ProxyService::class.java).putExtra("node", node.id))
            val limit = SystemClock.elapsedRealtime() + 10000
            while (Runtime.state.value.nodeId != node.id && SystemClock.elapsedRealtime() < limit) SystemClock.sleep(100)
            assertEquals(node.id, Runtime.state.value.nodeId)
            instrument.runOnMainSync { activity.finish() }
            val connection = URL("http://198.18.0.2/probe").openConnection(Proxy(Proxy.Type.HTTP, InetSocketAddress("127.0.0.1", node.port)))
            connection.connectTimeout = 10000; connection.readTimeout = 10000
            assertEquals("service-ok", connection.getInputStream().bufferedReader().use { it.readText() })
            server.get(15, TimeUnit.SECONDS)
            assertEquals(node.id, Runtime.state.value.nodeId)
        } finally {
            context.stopService(Intent(context, ProxyService::class.java))
            val limit = SystemClock.elapsedRealtime() + 10000
            while (Runtime.state.value.nodeId != null && SystemClock.elapsedRealtime() < limit) SystemClock.sleep(100)
            node?.let { store.delete(it.id) }
            upstream.close(); executor.shutdownNow()
        }
        assertNull(Runtime.state.value.nodeId)
    }
}
