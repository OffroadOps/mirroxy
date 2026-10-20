package com.mirroxy.android

import android.util.Base64
import org.json.JSONArray
import org.json.JSONObject
import java.net.URI
import java.net.URLDecoder

object Importer {
    private fun decode(value: String) = URLDecoder.decode(value.replace("+", "%2B"), "UTF-8")
    fun parse(input: String): JSONArray {
        var text = input.trim().removePrefix("\uFEFF")
        if (!text.startsWith("{") && !text.contains("://")) {
            text = runCatching { String(Base64.decode(text, Base64.DEFAULT), Charsets.UTF_8) }
                .getOrElse { String(Base64.decode(text, Base64.URL_SAFE), Charsets.UTF_8) }.trim()
        }
        val direct = JSONArray()
        val nativeInput: String
        if (text.startsWith("{")) {
            val array = JSONObject(text).getJSONArray("outbounds")
            val normalized = JSONArray()
            for (i in 0 until array.length()) {
                val outbound = array.getJSONObject(i)
                val settings = outbound.getJSONObject("settings")
                val legacy = settings.optJSONArray("servers") ?: settings.optJSONArray("vnext")
                if (legacy != null) {
                    require(legacy.length() == 1) { "一个节点只能有一个服务器" }
                    val server = legacy.getJSONObject(0)
                    val flat = JSONObject(server.toString())
                    val users = flat.optJSONArray("users")
                    if (users != null) {
                        require(users.length() <= 1)
                        if (users.length() == 1) users.getJSONObject(0).let { user -> user.keys().forEach { key -> flat.put(key, user.get(key)) } }
                        flat.remove("users")
                    }
                    outbound.put("settings", flat)
                }
                if (outbound.optString("protocol") == "http") direct.put(validateHttp(outbound)) else normalized.put(outbound)
            }
            nativeInput = if (normalized.length() == 0) "" else JSONObject().put("outbounds", normalized).toString()
        } else {
            val other = mutableListOf<String>()
            text.lines().map { it.trim() }.filter { it.isNotEmpty() && !it.startsWith("#") }.forEach { line ->
                if (line.startsWith("http://") || line.startsWith("https://")) {
                    val uri = URI(line)
                    val settings = JSONObject().put("address", uri.host ?: error("服务器为空"))
                        .put("port", if (uri.port > 0) uri.port else if (uri.scheme == "https") 443 else 80)
                    uri.rawUserInfo?.let { auth -> val pieces = auth.split(":", limit = 2); settings.put("user", decode(pieces[0])); settings.put("pass", decode(pieces.getOrElse(1) { "" })) }
                    val out = JSONObject().put("protocol", "http").put("tag", uri.rawFragment?.let(::decode) ?: "HTTP 节点").put("settings", settings)
                    if (uri.scheme == "https") out.put("streamSettings", JSONObject().put("security", "tls").put("tlsSettings", JSONObject().put("serverName", uri.host)))
                    direct.put(validateHttp(out))
                } else other += line
            }
            nativeInput = other.joinToString("\n")
        }
        if (nativeInput.isNotBlank()) {
            val data = Core.call("convertShareLinksToXrayJson", JSONObject().put("text", nativeInput))
            val root = if (data is JSONObject) data else JSONObject(data.toString())
            val nodes = root.getJSONArray("outbounds")
            for (i in 0 until nodes.length()) direct.put(nodes.getJSONObject(i))
        }
        require(direct.length() > 0) { "没有支持的节点" }
        return direct
    }
    private fun validateHttp(outbound: JSONObject): JSONObject {
        val settings = outbound.getJSONObject("settings")
        require(settings.optString("address").isNotBlank() && settings.optInt("port") in 1..65535)
        val safe = JSONObject().put("protocol", "http").put("tag", outbound.optString("tag"))
            .put("settings", JSONObject().put("address", settings.getString("address")).put("port", settings.getInt("port")))
        for (field in listOf("user", "pass")) if (settings.has(field)) safe.getJSONObject("settings").put(field, settings.getString(field))
        outbound.optJSONObject("streamSettings")?.let { stream ->
            require(stream.optString("security") in setOf("", "none", "tls"))
            safe.put("streamSettings", stream)
        }
        return safe
    }
}
