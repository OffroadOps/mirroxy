'use strict';
(() => {
  function percent(completed, total) {
    completed = Number(completed); total = Number(total);
    if (!Number.isFinite(completed) || !Number.isFinite(total) || total <= 0) return 0;
    return Math.max(0, Math.min(100, completed * 100 / total));
  }
  globalThis.MirroxyDownloadProgress = {percent};
})();
