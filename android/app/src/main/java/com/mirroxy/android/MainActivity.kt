package com.mirroxy.android

import android.Manifest
import android.content.Context
import android.content.Intent
import android.net.VpnService
import android.os.Build
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import androidx.lifecycle.lifecycleScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.net.InetSocketAddress
import java.net.Proxy
import java.net.URL

class MainActivity : ComponentActivity() {
    private var pendingNode: String? = null
    private val vpnConsent = registerForActivityResult(ActivityResultContracts.StartActivityForResult()) { result ->
        if (result.resultCode == RESULT_OK) pendingNode?.let { start(it, true) }
        pendingNode = null
    }
    private val notifications = registerForActivityResult(ActivityResultContracts.RequestPermission()) { }
    private val importFile = registerForActivityResult(ActivityResultContracts.OpenDocument()) { uri ->
        if (uri != null) lifecycleScope.launch {
            runCatching {
                val text = withContext(Dispatchers.IO) { contentResolver.openInputStream(uri)!!.use { stream ->
                    val output = java.io.ByteArrayOutputStream()
                    val buffer = ByteArray(8192)
                    while (true) {
                        val count = stream.read(buffer)
                        if (count < 0) break
                        output.write(buffer, 0, count)
                        require(output.size() <= 1024 * 1024)
                    }
                    val bytes = output.toByteArray()
                    require(bytes.size <= 1024 * 1024)
                    String(bytes, Charsets.UTF_8)
                } }
                withContext(Dispatchers.IO) { Repository.store(this@MainActivity).import(text) }
            }.onFailure { Runtime.state.value = Runtime.state.value.copy(error = "文件导入失败，请检查大小或格式") }
        }
    }
    private fun start(id: String, vpn: Boolean) {
        startForegroundService(Intent(this, if (vpn) TunnelService::class.java else ProxyService::class.java).putExtra("node", id))
    }
    private fun requestVpn(id: String) {
        val intent = VpnService.prepare(this)
        if (intent == null) start(id, true) else { pendingNode = id; vpnConsent.launch(intent) }
    }
    private fun stop() { stopService(Intent(this, ProxyService::class.java)); stopService(Intent(this, TunnelService::class.java)) }
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        if (Build.VERSION.SDK_INT >= 33) notifications.launch(Manifest.permission.POST_NOTIFICATIONS)
        val store = runCatching { Repository.store(this) }.getOrElse {
            setContent { MaterialTheme { Text("加密存储无法读取，请保留应用数据并检查 Keystore。") } }; return
        }
        setContent {
            MaterialTheme(colorScheme = darkColorScheme(primary = Color(0xFF8FDCCB), background = Color(0xFF101A24), surface = Color(0xFF1B2836))) {
                val nodes by store.nodes.collectAsState()
                val runtime by Runtime.state.collectAsState()
                var tab by remember { mutableIntStateOf(0) }
                var import by remember { mutableStateOf(false) }
                var editing by remember { mutableStateOf<Node?>(null) }
                var replacing by remember { mutableStateOf<Node?>(null) }
                var deleting by remember { mutableStateOf<Node?>(null) }
                var message by remember { mutableStateOf("") }
                val scope = rememberCoroutineScope()
                fun work(action: suspend () -> String) { scope.launch { message = try { withContext(Dispatchers.IO) { action() } } catch (_: Exception) { "操作失败，请检查输入或网络" } } }
                Scaffold(bottomBar = {
                    NavigationBar { listOf("节点", "应用路由", "关于").forEachIndexed { index, title ->
                        NavigationBarItem(selected = tab == index, onClick = { tab = index }, icon = { Text(listOf("◈", "⇄", "ⓘ")[index]) }, label = { Text(title) })
                    } }
                }) { padding ->
                    Column(Modifier.fillMaxSize().padding(padding).padding(horizontal = 20.dp)) {
                        Spacer(Modifier.height(22.dp))
                        Text("Mirroxy", style = MaterialTheme.typography.headlineLarge)
                        Text("独立 Android · 0.1.0-dev", color = MaterialTheme.colorScheme.primary)
                        Spacer(Modifier.height(16.dp))
                        Card(Modifier.fillMaxWidth()) { Column(Modifier.padding(16.dp)) {
                            Text(if (runtime.busy) "正在启动…" else runtime.mode, style = MaterialTheme.typography.titleMedium)
                            runtime.nodeId?.let { Text(nodes.firstOrNull { n -> n.id == it }?.name.orEmpty()); TextButton(onClick = { stop() }) { Text("停止代理") } }
                            if (runtime.nodeId == null) Text("本地代理与 VPN 分开启动", style = MaterialTheme.typography.bodySmall)
                            runtime.error?.let { Text(it, color = MaterialTheme.colorScheme.error) }
                        } }
                        if (message.isNotEmpty()) Text(message, Modifier.padding(vertical = 8.dp))
                        when (tab) {
                            0 -> {
                                Row(Modifier.padding(vertical = 12.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                    Button(onClick = { import = true }) { Text("导入链接") }
                                    OutlinedButton(onClick = { importFile.launch(arrayOf("text/*", "application/json", "application/octet-stream")) }) { Text("导入文件") }
                                }
                                if (nodes.isEmpty()) { Text("添加你的第一个节点", style = MaterialTheme.typography.titleLarge); Text("支持 VLESS / REALITY、VMess、SS、Trojan、HTTP、SOCKS。使用系统文件选择器或粘贴分享链接。", Modifier.padding(top = 12.dp)) }
                                LazyColumn(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                                    items(nodes, key = { it.id }) { node ->
                                        Card(Modifier.fillMaxWidth()) { Column(Modifier.padding(16.dp)) {
                                            Text(node.name, style = MaterialTheme.typography.titleMedium)
                                            Text("${node.protocol.uppercase()} · HTTP ${node.port} · SOCKS ${node.port + 1}", style = MaterialTheme.typography.bodySmall)
                                            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                                TextButton(enabled = runtime.nodeId == null && !runtime.busy, onClick = { start(node.id, false) }) { Text("本地代理") }
                                                TextButton(enabled = runtime.nodeId == null && !runtime.busy, onClick = { requestVpn(node.id) }) { Text("启动 VPN") }
                                                TextButton(onClick = { editing = node }) { Text("改名") }
                                            }
                                            Row {
                                                TextButton(enabled = runtime.nodeId != node.id && !runtime.busy, onClick = { replacing = node }) { Text("编辑") }
                                                TextButton(enabled = runtime.nodeId == node.id, onClick = { work {
                                                    val begin = System.nanoTime()
                                                    val proxy = Proxy(Proxy.Type.HTTP, InetSocketAddress("127.0.0.1", node.port))
                                                    val connection = URL("https://www.gstatic.com/generate_204").openConnection(proxy) as java.net.HttpURLConnection
                                                    connection.connectTimeout = 10000; connection.readTimeout = 10000
                                                    try { "代理检测：HTTP ${connection.responseCode} · ${(System.nanoTime() - begin) / 1_000_000} ms" } finally { connection.disconnect() }
                                                } }) { Text("真实请求检测") }
                                                TextButton(enabled = runtime.nodeId != node.id && !runtime.busy, onClick = { deleting = node }) { Text("删除") }
                                            }
                                        } }
                                    }
                                }
                            }
                            1 -> AppRouting(this@MainActivity, runtime.nodeId == null && !runtime.busy)
                            else -> Column(Modifier.padding(top = 20.dp)) {
                                Text("官方 XTLS libXray 26.10.10", style = MaterialTheme.typography.titleLarge)
                                Text("Kotlin + Jetpack Compose 原生界面；Android Keystore AES-GCM 加密节点；关闭界面后前台服务继续运行。", Modifier.padding(vertical = 12.dp))
                                Text("此开发版每次启动一个节点，HTTP 与 SOCKS 使用相邻固定回环端口。不会在失败时自动换节点或直连。进程终止后需手动重新启动。")
                                Text("VPN：支持 IPv4 / IPv6 与分应用白名单，空白名单为全设备。启动须系统授权。流量统计、订阅定时刷新、下载管理尚未实现。", Modifier.padding(vertical = 12.dp))
                                Text("第三方许可：libXray MIT；Xray-core MPL-2.0。应用使用未修改的官方 AAR。")
                            }
                        }
                    }
                }
                if (import) TextDialog("导入节点", "粘贴分享链接、Base64 订阅内容或 Xray outbounds JSON", "", { import = false }) { text ->
                    import = false; work { "已导入 ${store.import(text)} 个节点（重复项跳过）" }
                }
                editing?.let { node -> TextDialog("改名", "名称", node.name, { editing = null }) { name -> editing = null; work { store.rename(node.id, name); "名称已保存" } } }
                replacing?.let { node -> TextDialog("编辑节点", "新的分享链接或 outbounds JSON", "", { replacing = null }) { text -> replacing = null; work { store.replace(node.id, text); "配置已更新，名称和端口保留" } } }
                deleting?.let { node -> AlertDialog(onDismissRequest = { deleting = null }, title = { Text("删除节点？") }, text = { Text("将删除 ${node.name} 的本机记录。") }, confirmButton = { TextButton(onClick = { deleting = null; work { store.delete(node.id); "节点已删除" } }) { Text("删除") } }, dismissButton = { TextButton(onClick = { deleting = null }) { Text("取消") } }) }
            }
        }
    }
}

@Composable private fun TextDialog(title: String, hint: String, initial: String, dismiss: () -> Unit, save: (String) -> Unit) {
    var text by remember { mutableStateOf(initial) }
    AlertDialog(onDismissRequest = dismiss, title = { Text(title) }, text = { OutlinedTextField(value = text, onValueChange = { text = it }, label = { Text(hint) }, modifier = Modifier.fillMaxWidth().heightIn(max = 260.dp)) },
        confirmButton = { TextButton(enabled = text.isNotBlank(), onClick = { save(text) }) { Text("保存") } }, dismissButton = { TextButton(onClick = dismiss) { Text("取消") } })
}

@Composable private fun AppRouting(context: Context, enabled: Boolean) {
    val prefs = remember { context.getSharedPreferences("ui", Context.MODE_PRIVATE) }
    var selected by remember { mutableStateOf(prefs.getStringSet("apps", emptySet()).orEmpty().toSet()) }
    var packages by remember { mutableStateOf<List<Pair<String, String>>>(emptyList()) }
    LaunchedEffect(Unit) { packages = withContext(Dispatchers.IO) {
        context.packageManager.queryIntentActivities(Intent(Intent.ACTION_MAIN).addCategory(Intent.CATEGORY_LAUNCHER), 0)
            .map { it.activityInfo.packageName to it.loadLabel(context.packageManager).toString() }.distinctBy { it.first }.sortedBy { it.second }
    } }
    Text("选择使用 VPN 的应用", Modifier.padding(top = 16.dp), style = MaterialTheme.typography.titleMedium)
    Text("${selected.size} 个应用 · 不选则接管全部应用。修改后下次启动 VPN 生效。", Modifier.padding(vertical = 8.dp))
    LazyColumn { items(packages, key = { it.first }) { (pkg, label) ->
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
            Column(Modifier.weight(1f).padding(vertical = 10.dp)) { Text(label); Text(pkg, style = MaterialTheme.typography.bodySmall) }
            Checkbox(checked = pkg in selected, enabled = enabled, onCheckedChange = { checked ->
                selected = if (checked) selected + pkg else selected - pkg
                prefs.edit().putStringSet("apps", selected).apply()
            })
        }
    } }
}
