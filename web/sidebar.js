'use strict';
// Apply the persisted sidebar state before the stylesheets paint, so a
// collapsed sidebar never flashes open on reload.
(() => {
  try {
    if (localStorage.getItem('Mirroxy.sidebar') === 'collapsed') document.documentElement.classList.add('sidebar-collapsed');
  } catch {}
})();
