'use strict';
(() => {
  function sortedNodes(nodes) {
    const rank = node => node.listBottom ? 2 : (node.enabled || node.status === 'running' ? 0 : 1);
    return nodes.map((node, index) => ({ node, index }))
      .sort((a, b) => rank(a.node) - rank(b.node) || a.index - b.index)
      .map(entry => entry.node);
  }
  globalThis.MirroxyNodeList = { sortedNodes };
})();
