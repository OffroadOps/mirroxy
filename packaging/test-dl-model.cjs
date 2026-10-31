const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const context = { URL };
vm.runInNewContext(fs.readFileSync(path.join(__dirname, '../web/dl-model.js'), 'utf8'), context);
const M = context.MirroxyDownloadModel;
const ids = list => Array.from(list, task => task.id);

assert.equal(M.category('setup.EXE'), 'programs');
assert.equal(M.category('movie.mkv'), 'video');
assert.equal(M.category('archive.tar.gz'), 'compressed');
assert.equal(M.category('song.flac'), 'music');
assert.equal(M.category('paper.pdf'), 'documents');
assert.equal(M.category('noext'), 'other');
assert.equal(M.filename({ path: 'D:\\Downloads\\a.zip', name: 'x' }), 'a.zip');
assert.equal(M.filename({ url: 'https://h.example/dir/%E4%B8%AD.iso' }), '中.iso');
assert.equal(M.filename({ name: 'given.bin', url: 'https://h.example/' }), 'given.bin');
console.log('PASS: file names and categories by extension');

const tasks = [
  { id: 'a', name: 'a.zip', url: 'https://x/a.zip', status: 'active', totalLength: '1000', completedLength: '250', downloadSpeed: '50', created: '2026-10-30T10:00:00' },
  { id: 'b', name: 'b.mp4', url: 'https://x/b.mp4', status: 'complete', totalLength: '5000', completedLength: '5000', downloadSpeed: '0', created: '2026-10-29T10:00:00' },
  { id: 'c', name: 'c.exe', url: 'https://x/c.exe', status: 'paused', totalLength: '2000', completedLength: '1000', downloadSpeed: '0', created: '2026-10-31T10:00:00' },
  { id: 'd', name: 'd.pdf', url: 'https://x/d.pdf', status: 'error', totalLength: '0', completedLength: '0', downloadSpeed: '0', created: '2026-10-28T10:00:00', error: 'boom' },
];
const counts = M.counts(tasks);
assert.equal(counts.all, 4); assert.equal(counts.unfinished, 3); assert.equal(counts.finished, 1);
assert.equal(counts.compressed, 1); assert.equal(counts.video, 1); assert.equal(counts.programs, 1); assert.equal(counts.documents, 1);
assert.deepEqual(ids(M.filterTasks(tasks, 'unfinished', '')), ['a', 'c', 'd']);
assert.deepEqual(ids(M.filterTasks(tasks, 'finished', '')), ['b']);
assert.deepEqual(ids(M.filterTasks(tasks, 'video', '')), ['b']);
assert.deepEqual(ids(M.filterTasks(tasks, 'all', 'C.EXE')), ['c']);
console.log('PASS: category tree counts and filtering');

const original = JSON.stringify(tasks);
assert.deepEqual(ids(M.sortTasks(tasks)), ['c', 'a', 'b', 'd'], 'newest first by default');
assert.deepEqual(ids(M.sortTasks(tasks, { key: 'size', dir: 'desc' })), ['b', 'c', 'a', 'd']);
assert.deepEqual(ids(M.sortTasks(tasks, { key: 'name', dir: 'asc' })), ['a', 'b', 'c', 'd']);
assert.deepEqual(ids(M.sortTasks(tasks, { key: 'status', dir: 'desc' })), ['b', 'c', 'a', 'd']);
assert.equal(JSON.stringify(tasks), original, 'sorting must not mutate the list');
assert.deepEqual({ ...M.nextSort({ key: 'created', dir: 'desc' }, 'name') }, { key: 'name', dir: 'asc' });
assert.deepEqual({ ...M.nextSort({ key: 'size', dir: 'desc' }, 'size') }, { key: 'size', dir: 'asc' });
console.log('PASS: column sorting');

assert.equal(M.statusText(tasks[0]), '25.0%');
assert.equal(M.statusText(tasks[1]), '完成');
assert.equal(M.statusText(tasks[2]), '已暂停 50.0%');
assert.equal(M.statusText(tasks[3]), '错误');
assert.equal(M.statusText({ ...tasks[0], stalled: true }), '无速度 25.0%');
assert.equal(M.etaText(tasks[0]), '15 s');
assert.equal(M.etaText({ ...tasks[0], totalLength: '100000', downloadSpeed: '10' }), '2 h 46 min');
assert.equal(M.etaText(tasks[2]), '', 'no ETA unless downloading');
assert.equal(M.percent({ totalLength: '0', completedLength: '5' }), 0);
assert.equal(M.percent({ totalLength: '100', completedLength: '200' }), 100);
console.log('PASS: status, percentage and remaining time');

assert.equal(M.pastedURL(' https://h.example/f.zip '), 'https://h.example/f.zip');
assert.equal(M.pastedURL('ftp://h.example/f.iso'), 'ftp://h.example/f.iso');
assert.equal(M.pastedURL('vless://abc@h:443'), '');
assert.equal(M.pastedURL('two\nhttps://a https://b'), '');
console.log('PASS: Ctrl+V link detection');

// The panel CSP forbids inline styles; the window sets progress widths through
// the CSSOM instead of style attributes.
for (const file of ['web/downloads.html', 'web/dl-app.js']) assert.doesNotMatch(fs.readFileSync(path.join(__dirname, '..', file), 'utf8'), /style="/, file + ' must not use inline style attributes');
assert.doesNotMatch(fs.readFileSync(path.join(__dirname, '../web/downloads.html'), 'utf8'), /<script>(?!<\/script>)/, 'no inline scripts');
assert.match(fs.readFileSync(path.join(__dirname, '../remote.go'), 'utf8'), /style-src 'self'/);
console.log('PASS: no inline styles or scripts; CSP preserved');
