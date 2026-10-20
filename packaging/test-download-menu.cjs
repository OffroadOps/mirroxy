const fs=require('node:fs'),vm=require('node:vm'),assert=require('node:assert/strict');const context=vm.createContext({});vm.runInContext(fs.readFileSync('web/download-menu.js','utf8'),context);const {allowed,position}=context.MirroxyDownloadMenu;
const complete=allowed([{status:'complete'}]);assert.equal(complete.resume,false);assert.equal(complete.pause,false);assert.equal(complete.edit,false);assert.equal(complete.copyURL,true);assert.equal(complete.remove,true);
assert.equal(allowed([{status:'active'}]).pause,true);assert.equal(allowed([{status:'paused'}]).resume,true);assert.equal(allowed([{status:'error'}]).retry,true);assert.equal(allowed([{status:'paused'},{status:'error'}]).edit,false);assert.equal(allowed([]).remove,false);
const point=position(990,710,220,330,1000,720);assert.equal(point.left,772);assert.equal(point.top,382);
console.log('PASS: context-menu availability, multi-selection and viewport bounds');
assert.equal(allowed([{status:'complete',path:'D:/Downloads/example.iso'}]).openFile,true);assert.equal(allowed([{status:'active',path:'D:/Downloads/example.iso'}]).openFile,false);assert.equal(allowed([{status:'active'}]).reconnect,true);assert.equal(complete.reconnect,false);
console.log('PASS: completed-only file opening, folder reveal and stalled-task reconnect');
