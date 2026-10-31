'use strict';
// Browser relay card on the settings page and the sidebar link to the download
// window. The download window itself lives on /downloads and opens from the
// tray; platforms without a tray reach it through the sidebar link.
(() => {
  let info = null, timer = 0, checked = false;
  async function refreshRelay() {
    clearTimeout(timer);
    try {
      info = await apiGet('downloads/connector');
      $('relayStatus').textContent = t(info.connected ? '扩展已连接' : '等待扩展连接');
      $('relayStatus').classList.toggle('connected', !!info.connected);
    } catch {}
    timer = setTimeout(refreshRelay, 10000);
  }
  $('relayConfig').onclick = () => act($('relayConfig'), async () => {
    if (!info) info = await apiGet('downloads/connector');
    await copyText(JSON.stringify({ url: info.url, connector: info.connector, token: info.token }));
    toast(t('扩展配置已复制；通常不需要手动粘贴，扩展会自动连接'));
  });
  $('relayHelp').onclick = () => $('relayHelpBox').classList.toggle('hidden');
  // The download window's Help menu links here.
  function showRelayFromHash() {
    if (location.hash !== '#browser-relay' || !state) return;
    pendingView = null;
    view('settings');
    setTimeout(() => $('browser-relay').scrollIntoView({ block: 'start' }), 300);
    history.replaceState(null, '', location.pathname);
  }
  window.MirroxyRelay = {
    onState(next) {
      $('browser-relay').classList.toggle('hidden', !next.localAccess);
      $('downloadsLink').classList.toggle('hidden', !next.localAccess || !!next.tray);
      if (next.localAccess && !checked) { checked = true; refreshRelay(); }
      showRelayFromHash();
    },
  };
  addEventListener('hashchange', showRelayFromHash);
})();
