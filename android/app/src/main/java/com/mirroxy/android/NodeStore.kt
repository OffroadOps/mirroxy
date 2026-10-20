package com.mirroxy.android

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.AtomicFile
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import org.json.JSONArray
import org.json.JSONObject
import java.io.File
import java.security.KeyStore
import java.util.UUID
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

data class Node(val id: String, val name: String, val outbound: String, val port: Int) {
    val protocol get() = JSONObject(outbound).optString("protocol")
    fun json() = JSONObject().put("id", id).put("name", name).put("outbound", outbound).put("port", port)
}

class NodeStore(context: Context, filename: String = "nodes.enc") {
    private val file = AtomicFile(File(context.noBackupFilesDir, filename))
    private val key: SecretKey
        get() {
            val store = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
            (store.getKey("mirroxy.nodes.v1", null) as? SecretKey)?.let { return it }
            return KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore").apply {
                init(KeyGenParameterSpec.Builder("mirroxy.nodes.v1", KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
                    .setBlockModes(KeyProperties.BLOCK_MODE_GCM).setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE).build())
            }.generateKey()
        }
    private val mutable = MutableStateFlow<List<Node>>(emptyList())
    val nodes = mutable.asStateFlow()
    init {
        if (file.baseFile.exists()) {
            val bytes = file.readFully()
            require(bytes.size > 28) { "加密节点文件损坏，请保留文件" }
            val cipher = Cipher.getInstance("AES/GCM/NoPadding")
            cipher.init(Cipher.DECRYPT_MODE, key, GCMParameterSpec(128, bytes.copyOfRange(0, 12)))
            val array = JSONArray(String(cipher.doFinal(bytes.copyOfRange(12, bytes.size)), Charsets.UTF_8))
            mutable.value = (0 until array.length()).map { i -> array.getJSONObject(i).let {
                Node(it.getString("id"), it.getString("name"), it.getString("outbound"), it.getInt("port"))
            } }
        }
    }
    @Synchronized fun import(text: String): Int {
        require(text.toByteArray().size <= 1024 * 1024) { "导入内容超过 1 MiB" }
        val outs = Importer.parse(text)
        val current = mutable.value.toMutableList()
        var added = 0
        for (i in 0 until outs.length()) {
            val out = outs.getJSONObject(i)
            require(out.optString("protocol") in setOf("vless", "vmess", "shadowsocks", "trojan", "socks", "http")) { "此版本不支持该协议" }
            val label = out.optString("tag").ifBlank { "节点 ${current.size + 1}" }
            out.remove("tag")
            if (current.any { it.outbound == out.toString() }) continue
            val port = (15060..65000 step 2).firstOrNull { p -> current.none { it.port == p } } ?: error("端口已用完")
            current += Node(UUID.randomUUID().toString(), label, out.toString(), port)
            added++
        }
        save(current)
        return added
    }
    @Synchronized fun rename(id: String, name: String) {
        require(name.trim().isNotEmpty() && name.length <= 100) { "名称长度需为 1–100" }
        save(mutable.value.map { if (it.id == id) it.copy(name = name.trim()) else it })
    }
    @Synchronized fun replace(id: String, text: String) {
        val outbounds = Importer.parse(text)
        require(outbounds.length() == 1) { "编辑时只能输入一个节点" }
        val outbound = outbounds.getJSONObject(0).apply { remove("tag") }
        require(outbound.optString("protocol") in setOf("vless", "vmess", "shadowsocks", "trojan", "socks", "http"))
        save(mutable.value.map { if (it.id == id) it.copy(outbound = outbound.toString()) else it })
    }
    @Synchronized fun delete(id: String) = save(mutable.value.filterNot { it.id == id })
    private fun save(nodes: List<Node>) {
        val raw = JSONArray().apply { nodes.forEach { put(it.json()) } }.toString().toByteArray()
        val cipher = Cipher.getInstance("AES/GCM/NoPadding").apply { init(Cipher.ENCRYPT_MODE, key) }
        val stream = file.startWrite()
        try { stream.write(cipher.iv + cipher.doFinal(raw)); file.finishWrite(stream) }
        catch (e: Exception) { file.failWrite(stream); throw e }
        mutable.value = nodes
    }
}

object Repository {
    @Volatile private var value: NodeStore? = null
    @Synchronized fun store(context: Context): NodeStore = value ?: NodeStore(context.applicationContext).also { value = it }
}
