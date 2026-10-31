'use strict';
(() => {
  // Pure helpers for the download window: file categories by extension, the
  // category tree, sorting and the status column. No DOM access, so
  // packaging/test-dl-model.cjs can exercise them directly.
  const EXT = {
    compressed: ['zip', 'rar', '7z', 'gz', 'tgz', 'bz2', 'xz', 'tar', 'zst', 'iso', 'cab', 'arj', 'lz', 'lzh'],
    documents: ['pdf', 'doc', 'docx', 'xls', 'xlsx', 'ppt', 'pptx', 'txt', 'md', 'rtf', 'odt', 'ods', 'odp', 'epub', 'csv', 'json', 'xml'],
    music: ['mp3', 'flac', 'wav', 'aac', 'm4a', 'ogg', 'opus', 'wma', 'ape', 'alac'],
    programs: ['exe', 'msi', 'msix', 'appx', 'apk', 'dmg', 'pkg', 'deb', 'rpm', 'appimage', 'bat', 'cmd', 'ps1', 'sh', 'jar'],
    video: ['mp4', 'mkv', 'avi', 'mov', 'wmv', 'flv', 'webm', 'm4v', 'ts', 'mpg', 'mpeg', '3gp', 'rmvb'],
  };
  const CATEGORIES = Object.keys(EXT);
  const UNFINISHED = new Set(['active', 'waiting', 'paused', 'error']);

  function filename(task) {
    if (task.path) return task.path.split(/[\\/]/).pop();
    if (task.name) return task.name;
    try { return decodeURIComponent(new URL(task.url).pathname.split('/').pop()) || task.url; } catch { return task.url || ''; }
  }

  function category(name) {
    const ext = String(name || '').toLowerCase().split('.').pop();
    return CATEGORIES.find(key => EXT[key].includes(ext)) || 'other';
  }

  // Tree nodes: everything (with one child per file type), unfinished, finished.
  function inNode(task, node) {
    if (node === 'all') return true;
    if (node === 'unfinished') return UNFINISHED.has(task.status);
    if (node === 'finished') return task.status === 'complete';
    return category(filename(task)) === node;
  }

  function counts(tasks) {
    const result = { all: tasks.length, unfinished: 0, finished: 0, other: 0 };
    for (const key of CATEGORIES) result[key] = 0;
    for (const task of tasks) {
      result[category(filename(task))]++;
      if (task.status === 'complete') result.finished++;
      else if (UNFINISHED.has(task.status)) result.unfinished++;
    }
    return result;
  }

  function filterTasks(tasks, node, query) {
    const q = String(query || '').trim().toLowerCase();
    return tasks.filter(task => inNode(task, node) && (!q || `${filename(task)} ${task.url} ${task.path || task.dir || ''}`.toLowerCase().includes(q)));
  }

  const num = value => Number(value) || 0;
  function percent(task) {
    const total = num(task.totalLength), done = num(task.completedLength);
    return total > 0 ? Math.min(100, Math.max(0, done / total * 100)) : 0;
  }
  function remaining(task) {
    const speed = num(task.downloadSpeed), left = num(task.totalLength) - num(task.completedLength);
    return task.status === 'active' && speed > 0 && left > 0 ? Math.ceil(left / speed) : Infinity;
  }

  // Newest first by default, like a download list; columns sort both ways.
  function sortTasks(tasks, sort) {
    const key = sort?.key || 'created', dir = sort?.dir === 'asc' ? 1 : -1;
    const value = {
      name: t => filename(t).toLowerCase(),
      size: t => num(t.totalLength),
      status: t => t.status === 'complete' ? 101 : percent(t),
      eta: t => remaining(t),
      speed: t => num(t.downloadSpeed),
      created: t => String(t.created || ''),
    }[key] || (t => String(t.created || ''));
    return tasks.map((task, index) => ({ task, index })).sort((a, b) => {
      const x = value(a.task), y = value(b.task);
      return (x < y ? -1 : x > y ? 1 : 0) * dir || a.index - b.index;
    }).map(entry => entry.task);
  }

  function nextSort(sort, key) {
    if (sort?.key !== key) return { key, dir: key === 'name' ? 'asc' : 'desc' };
    return { key, dir: sort.dir === 'asc' ? 'desc' : 'asc' };
  }

  // The status column reads like a download manager: a percentage while
  // running, the state otherwise.
  function statusText(task, t = s => s) {
    switch (task.status) {
      case 'complete': return t('完成');
      case 'active': return task.stalled ? t('无速度') + ' ' + percent(task).toFixed(1) + '%' : percent(task).toFixed(1) + '%';
      case 'waiting': return t('排队中');
      case 'paused': return t('已暂停') + (num(task.totalLength) ? ' ' + percent(task).toFixed(1) + '%' : '');
      case 'error': return t('错误');
      case 'removed': return t('已移除');
      default: return task.status;
    }
  }

  function etaText(task) {
    const sec = remaining(task);
    if (sec === Infinity) return '';
    if (sec < 60) return sec + ' s';
    if (sec < 3600) return Math.floor(sec / 60) + ' min ' + String(sec % 60).padStart(2, '0') + ' s';
    return Math.floor(sec / 3600) + ' h ' + String(Math.floor(sec % 3600 / 60)).padStart(2, '0') + ' min';
  }

  // A single http(s)/ftp address on the clipboard starts a new download.
  function pastedURL(text) {
    const value = String(text || '').trim();
    return /^(https?|ftp):\/\/\S+$/i.test(value) && !/\s/.test(value) ? value : '';
  }

  globalThis.MirroxyDownloadModel = { CATEGORIES, filename, category, inNode, counts, filterTasks, percent, remaining, sortTasks, nextSort, statusText, etaText, pastedURL };
})();
