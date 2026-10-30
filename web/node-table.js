'use strict';
(() => {
  // Pure helpers for the node table: grouping by subscription, filtering,
  // column sorting, selection ranges and which menu actions apply. Kept free
  // of DOM access so packaging/test-node-table.cjs can exercise them.
  const latencyOf = (health, id) => {
    const h = health?.[id];
    return h?.status === 'online' && h.latencyMs > 0 ? h.latencyMs : Infinity;
  };

  // Tabs in v2rayN order: everything, what is running, then one tab per
  // subscription and a final tab for nodes that came from no subscription.
  function groups(nodes, subscriptions) {
    const subs = subscriptions || [];
    const known = new Set(subs.map(s => s.id));
    const list = [
      { id: 'all', count: nodes.length },
      { id: 'running', count: nodes.filter(n => n.status === 'running').length },
      ...subs.map(s => ({ id: 'sub:' + s.id, name: s.name, count: nodes.filter(n => n.source === s.id).length })),
    ];
    const manual = nodes.filter(n => !known.has(n.source)).length;
    if (manual && subs.length) list.push({ id: 'manual', count: manual });
    return list;
  }

  function inGroup(node, group, subscriptions) {
    if (!group || group === 'all') return true;
    if (group === 'running') return node.status === 'running';
    if (group === 'manual') return !(subscriptions || []).some(s => s.id === node.source);
    return group.startsWith('sub:') && node.source === group.slice(4);
  }

  function filterNodes(nodes, { group, query, subscriptions }) {
    const q = String(query || '').trim().toLowerCase();
    return nodes.filter(n => inGroup(n, group, subscriptions) &&
      (!q || (n.name + ' ' + n.protocol + ' ' + n.server + ' ' + (n.port || '')).toLowerCase().includes(q)));
  }

  // sort.key is '' for the automatic order (started nodes first, as the
  // backend list), otherwise a column; unknown latencies always sort last.
  function sortNodes(nodes, sort, health, traffic, autoSort) {
    const base = autoSort ? autoSort(nodes) : nodes.slice();
    const key = sort?.key, dir = sort?.dir === 'desc' ? -1 : 1;
    if (!key) return base;
    const total = n => { const s = traffic?.[n.id] || n.stats || {}; return (Number(s.uploadedBytes) || 0) + (Number(s.downloadedBytes) || 0); };
    const value = {
      name: n => n.name.toLowerCase(),
      protocol: n => String(n.protocol).toLowerCase(),
      latency: n => latencyOf(health, n.id),
      port: n => n.port || Infinity,
      total,
    }[key];
    if (!value) return base;
    return base.map((node, index) => ({ node, index })).sort((a, b) => {
      const x = value(a.node), y = value(b.node);
      if (x === Infinity || y === Infinity) return x === y ? a.index - b.index : x === Infinity ? 1 : -1;
      return (x < y ? -1 : x > y ? 1 : 0) * dir || a.index - b.index;
    }).map(entry => entry.node);
  }

  // Header clicks cycle ascending → descending → automatic order.
  function nextSort(sort, key) {
    if (sort?.key !== key) return { key, dir: 'asc' };
    return sort.dir === 'asc' ? { key, dir: 'desc' } : { key: '', dir: 'asc' };
  }

  // Shift-click selects the visible rows between the anchor and the target.
  function range(ids, anchor, target) {
    const a = ids.indexOf(anchor), b = ids.indexOf(target);
    if (a < 0 || b < 0) return [target];
    return ids.slice(Math.min(a, b), Math.max(a, b) + 1);
  }

  function menuState(nodes, { systemProxy = true, appRouting = false } = {}) {
    const one = nodes.length === 1, node = nodes[0];
    const any = nodes.length > 0;
    return {
      setProxy: one && systemProxy,
      start: nodes.some(n => !n.enabled),
      stop: nodes.some(n => n.enabled),
      test: any,
      edit: one,
      rename: one,
      share: one,
      copyLinks: any,
      copyAddress: one && !!node.port,
      route: any,
      apps: one && appRouting,
      moveBottom: nodes.some(n => !n.listBottom),
      autoOrder: nodes.some(n => n.listBottom),
      reassign: one && !node.enabled,
      log: one,
      remove: any,
    };
  }

  // Pasted text is imported directly when it is made of share links; a lone
  // http(s) address is treated as a subscription address instead.
  function classifyPaste(text) {
    const lines = String(text || '').split(/\r?\n/).map(line => line.trim()).filter(Boolean);
    if (!lines.length) return { kind: 'none' };
    if (lines.length === 1 && /^https?:\/\//i.test(lines[0])) return { kind: 'subscription', url: lines[0] };
    const links = lines.filter(line => /^[a-z][a-z0-9+.-]*:\/\//i.test(line) && !/^https?:\/\//i.test(line));
    if (links.length) return { kind: 'links', text: links.join('\n'), count: links.length };
    if (/^[A-Za-z0-9+/=_-]{40,}$/.test(lines.join(''))) return { kind: 'links', text: lines.join('\n'), count: 0 };
    return { kind: 'none' };
  }

  function position(x, y, width, height, viewportWidth, viewportHeight) {
    return {
      left: Math.max(8, Math.min(x, viewportWidth - width - 8)),
      top: Math.max(8, Math.min(y, viewportHeight - height - 8)),
    };
  }

  function latencyClass(ms) {
    return ms < 200 ? 'fast' : ms < 500 ? 'medium' : 'slow';
  }

  globalThis.MirroxyNodeTable = { groups, filterNodes, sortNodes, nextSort, range, menuState, classifyPaste, position, latencyClass };
})();
