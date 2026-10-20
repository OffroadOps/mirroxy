package com.mirroxy.android

import libXray.LibXray
import org.json.JSONArray
import org.json.JSONObject

object Core {
    @Synchronized fun call(method: String, payload: JSONObject = JSONObject()): Any {
        val response = JSONObject(LibXray.invoke(JSONObject().put("apiVersion", 3).put("method", method).put("payload", payload).toString()))
        // Native errors may contain node credentials; never log or display the raw text.
        check(response.optBoolean("success")) { "核心拒绝操作，请检查协议、节点配置或端口占用" }
        return response.opt("data") ?: JSONObject()
    }
    fun config(node: Node, tunFd: Int? = null): JSONObject {
        val out = JSONObject(node.outbound).put("tag", "selected")
        val inbound = JSONArray()
            .put(JSONObject().put("tag", "http").put("listen", "127.0.0.1").put("port", node.port).put("protocol", "http").put("settings", JSONObject()))
            .put(JSONObject().put("tag", "socks").put("listen", "127.0.0.1").put("port", node.port + 1).put("protocol", "socks").put("settings", JSONObject().put("auth", "noauth").put("udp", true)))
        if (tunFd != null) inbound.put(JSONObject().put("tag", "tun").put("protocol", "tun").put("port", 0)
            .put("settings", JSONObject().put("name", "mirroxy").put("mtu", 1500)))
        return JSONObject().put("log", JSONObject().put("loglevel", "none"))
            .put("inbounds", inbound).put("outbounds", JSONArray().put(out))
            .apply { if (tunFd != null) put("env", JSONObject().put("xray.tun.fd", tunFd.toString())) }
    }
    fun start(node: Node, tunFd: Int? = null) = call("runXray", JSONObject().put("xrayJson", config(node, tunFd).toString()))
    fun stop() { call("stopXray"); LibXray.resetDNS() }
}
