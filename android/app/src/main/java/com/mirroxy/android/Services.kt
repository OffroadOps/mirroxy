package com.mirroxy.android

import android.app.*
import android.content.Context
import android.content.Intent
import android.net.VpnService
import android.os.IBinder
import android.os.ParcelFileDescriptor
import kotlinx.coroutines.*
import kotlinx.coroutines.flow.MutableStateFlow
import libXray.DialerController
import libXray.LibXray

data class RuntimeState(val nodeId: String? = null, val mode: String = "已停止", val busy: Boolean = false, val error: String? = null)
object Runtime { val state = MutableStateFlow(RuntimeState()); val lock = Any() }
private object SocketProtection {
    @Volatile var vpn: VpnService? = null
    val controller = object : DialerController { override fun protectFd(fd: Long) = vpn?.protect(fd.toInt()) ?: true }
    fun install(service: VpnService?) { vpn = service; LibXray.registerDialerController(controller) }
}

private fun Service.foreground(label: String) {
    val manager = getSystemService(NotificationManager::class.java)
    manager.createNotificationChannel(NotificationChannel("proxy", "代理服务", NotificationManager.IMPORTANCE_LOW))
    val open = PendingIntent.getActivity(this, 0, Intent(this, MainActivity::class.java), PendingIntent.FLAG_IMMUTABLE)
    val stop = PendingIntent.getService(this, 1, Intent(this, javaClass).setAction("stop"), PendingIntent.FLAG_IMMUTABLE)
    startForeground(1, Notification.Builder(this, "proxy").setSmallIcon(android.R.drawable.stat_sys_upload_done)
        .setContentTitle("Mirroxy · $label").setContentText("点按返回；停止按钮结束代理").setContentIntent(open)
        .addAction(Notification.Action.Builder(null, "停止", stop).build()).setOngoing(true).build())
}

class ProxyService : Service() {
    private var owned = false
    private var destroyed = false
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    override fun onBind(intent: Intent?): IBinder? = null
    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent?.action == "stop") { stopSelf(); return START_NOT_STICKY }
        foreground("本地代理")
        scope.launch {
            synchronized(Runtime.lock) {
            if (destroyed || owned) return@synchronized
            if (Runtime.state.value.nodeId != null || Runtime.state.value.busy) {
                Runtime.state.value = Runtime.state.value.copy(error = "请先停止当前服务")
                stopSelf(); return@synchronized
            }
            try {
                Runtime.state.value = RuntimeState(busy = true)
                val node = Repository.store(this@ProxyService).nodes.value.first { it.id == intent?.getStringExtra("node") }
                SocketProtection.install(null)
                Core.start(node)
                owned = true
                Runtime.state.value = RuntimeState(node.id, "本地 HTTP / SOCKS")
            } catch (_: Exception) {
                Runtime.state.value = RuntimeState(error = "启动失败：检查节点配置、端口或现有服务")
                stopSelf()
            }
            }
        }
        return START_NOT_STICKY
    }
    override fun onDestroy() {
        scope.cancel()
        synchronized(Runtime.lock) {
            destroyed = true
            if (owned) { runCatching { Core.stop() }; owned = false; Runtime.state.value = RuntimeState() }
        }
        super.onDestroy()
    }
}

class TunnelService : VpnService() {
    private var owned = false
    private var destroyed = false
    private var tunnel: ParcelFileDescriptor? = null
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent?.action == "stop") { stopSelf(); return START_NOT_STICKY }
        foreground("VPN")
        scope.launch {
            synchronized(Runtime.lock) {
            if (destroyed || owned) return@synchronized
            if (Runtime.state.value.nodeId != null || Runtime.state.value.busy) {
                Runtime.state.value = Runtime.state.value.copy(error = "请先停止当前服务")
                stopSelf(); return@synchronized
            }
            try {
                check(prepare(this@TunnelService) == null)
                Runtime.state.value = RuntimeState(busy = true)
                val node = Repository.store(this@TunnelService).nodes.value.first { it.id == intent?.getStringExtra("node") }
                val builder = Builder().setSession("Mirroxy").setMtu(1500)
                    .addAddress("172.19.0.1", 30).addRoute("0.0.0.0", 0)
                    .addAddress("fdfe:dcba:9876::1", 126).addRoute("::", 0)
                    .addDnsServer("1.1.1.1").setBlocking(false)
                val apps = getSharedPreferences("ui", Context.MODE_PRIVATE).getStringSet("apps", emptySet()).orEmpty()
                apps.forEach { builder.addAllowedApplication(it) }
                tunnel = builder.establish() ?: error("VPN 未授权")
                SocketProtection.install(this@TunnelService)
                LibXray.setDNS(SocketProtection.controller, "1.1.1.1:53")
                Core.start(node, tunnel!!.fd)
                owned = true
                Runtime.state.value = RuntimeState(node.id, if (apps.isEmpty()) "全设备 VPN" else "分应用 VPN (${apps.size})")
            } catch (_: Exception) {
                runCatching { LibXray.resetDNS() }
                SocketProtection.install(null)
                tunnel?.close(); tunnel = null
                Runtime.state.value = RuntimeState(error = "VPN 启动失败：请检查授权、应用列表与节点配置")
                stopSelf()
            }
            }
        }
        return START_NOT_STICKY
    }
    override fun onRevoke() { stopSelf() }
    override fun onDestroy() {
        scope.cancel()
        synchronized(Runtime.lock) {
            destroyed = true
            if (owned) { runCatching { Core.stop() }; owned = false; Runtime.state.value = RuntimeState() }
            tunnel?.close(); tunnel = null
            if (SocketProtection.vpn === this) SocketProtection.install(null)
        }
        super.onDestroy()
    }
}
