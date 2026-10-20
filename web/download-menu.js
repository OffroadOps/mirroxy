'use strict';
(() => {
  function allowed(tasks) {
    const one=tasks.length===1;
    return {resume:tasks.some(t=>t.status==='paused'),pause:tasks.some(t=>['active','waiting'].includes(t.status)),retry:tasks.some(t=>['error','removed'].includes(t.status)),reconnect:tasks.some(t=>t.status!=='complete'),edit:one&&['paused','error'].includes(tasks[0].status),openFile:one&&tasks[0].status==='complete'&&!!tasks[0].path,openFolder:one,copyURL:one,copyPath:one,remove:tasks.length>0};
  }
  function position(x,y,width,height,viewportWidth,viewportHeight) {
    return {left:Math.max(8,Math.min(x,viewportWidth-width-8)),top:Math.max(8,Math.min(y,viewportHeight-height-8))};
  }
  globalThis.MirroxyDownloadMenu={allowed,position};
})();
