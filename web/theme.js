'use strict';
// Read the explicit preference before styles load. First visit stays light,
// regardless of the operating system's appearance setting.
(()=>{
 const key='Mirroxy.theme';
 const translate=value=>window.MirroxyI18n?.t(value)||value;
 const normalize=value=>value==='dark'?'dark':'light';
 const apply=value=>{
  const theme=normalize(value);document.documentElement.dataset.theme=theme;
  const button=document.getElementById('themeToggle');
  if(button){const dark=theme==='dark';button.setAttribute('aria-pressed',String(dark));button.title=translate(dark?'切换到亮色模式':'切换到暗色模式');button.setAttribute('aria-label',button.title);}
 };
 let initial='light';try{initial=localStorage.getItem(key)}catch{}
 apply(initial);
 document.addEventListener('DOMContentLoaded',()=>{
  apply(document.documentElement.dataset.theme);
  document.getElementById('themeToggle').addEventListener('click',()=>{
   const next=document.documentElement.dataset.theme==='dark'?'light':'dark';apply(next);try{localStorage.setItem(key,next)}catch{}
  });
 });
 window.addEventListener('storage',event=>{if(event.key===key||event.key===null)apply(event.newValue)});
})();
