// Render the approved transparent Mirroxy PNG to browser/Windows icon sizes.
const fs=require('node:fs'),path=require('node:path');
const{chromium}=require(process.env.PLAYWRIGHT_MODULE||'playwright');
(async()=>{
 const root=path.resolve(__dirname,'..'),source=fs.readFileSync(path.join(root,'docs/brand/Mirroxy-icon-source.png')).toString('base64');
 const browser=await chromium.launch(process.env.CHROME_EXE?{executablePath:process.env.CHROME_EXE,headless:true}:{headless:true});
 try{
  const page=await browser.newPage();
  const sizes=[16,20,24,32,40,48,64,128,256];const images=[];
  for(const size of sizes){
   const data=await page.evaluate(async({source,size})=>{const img=new Image();img.src='data:image/png;base64,'+source;await img.decode();const canvas=document.createElement('canvas');canvas.width=canvas.height=size;const c=canvas.getContext('2d');c.imageSmoothingEnabled=true;c.imageSmoothingQuality='high';c.drawImage(img,0,0,size,size);return canvas.toDataURL('image/png').split(',')[1]},{source,size});
   const png=Buffer.from(data,'base64');images.push({size,png});if(size===256)fs.writeFileSync(path.join(root,'web/icon.png'),png);
  }
  const header=Buffer.alloc(6+16*images.length);header.writeUInt16LE(1,2);header.writeUInt16LE(images.length,4);let offset=header.length;
  images.forEach(({size,png},i)=>{const pos=6+16*i;header[pos]=header[pos+1]=size===256?0:size;header.writeUInt16LE(1,pos+4);header.writeUInt16LE(32,pos+6);header.writeUInt32LE(png.length,pos+8);header.writeUInt32LE(offset,pos+12);offset+=png.length});
  fs.writeFileSync(path.join(root,'web/app.ico'),Buffer.concat([header,...images.map(i=>i.png)]));
  console.log('Mirroxy PNG rendered to 9 Windows icon sizes and 256px PNG');
 }finally{await browser.close()}
})().catch(e=>{console.error(e);process.exit(1)});
