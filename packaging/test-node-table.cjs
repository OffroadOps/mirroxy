const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const context = {};
vm.runInNewContext(fs.readFileSync(path.join(__dirname, '../web/node-table.js'), 'utf8'), context);
const { groups, filterNodes, sortNodes, nextSort, range, menuState, classifyPaste, position, latencyClass } = context.MirroxyNodeTable;
const ids = list => Array.from(list, n => n.id);

const subs = [{ id: 's1', name: 'Airport A' }, { id: 's2', name: 'Airport B' }];
const nodes = [
  { id: 'a', name: 'Tokyo 01', protocol: 'vless', server: 'jp.example', port: 10811, source: 's1', status: 'running', enabled: true },
  { id: 'b', name: 'hong kong', protocol: 'trojan', server: 'hk.example', port: 0, source: 's1', status: 'stopped', enabled: false },
  { id: 'c', name: 'Local SOCKS', protocol: 'socks', server: '10.0.0.2', port: 10812, source: '', status: 'running', enabled: true, listBottom: true },
  { id: 'd', name: 'US West', protocol: 'hysteria2', server: 'us.example', port: 0, source: 's2', status: 'stopped', enabled: false },
];

// Tabs: all, running, one per subscription, then manual nodes.
const tabs = groups(nodes, subs);
assert.deepEqual(Array.from(tabs, g => g.id + ':' + g.count), ['all:4', 'running:2', 'sub:s1:2', 'sub:s2:1', 'manual:1']);
assert.equal(groups(nodes, []).some(g => g.id === 'manual'), false, 'no manual tab without subscriptions');
console.log('PASS: group tabs follow subscriptions and keep a manual tab');

assert.deepEqual(ids(filterNodes(nodes, { group: 'sub:s1', query: '', subscriptions: subs })), ['a', 'b']);
assert.deepEqual(ids(filterNodes(nodes, { group: 'running', query: '', subscriptions: subs })), ['a', 'c']);
assert.deepEqual(ids(filterNodes(nodes, { group: 'manual', query: '', subscriptions: subs })), ['c']);
assert.deepEqual(ids(filterNodes(nodes, { group: 'all', query: 'HK.EXAMPLE', subscriptions: subs })), ['b']);
assert.deepEqual(ids(filterNodes(nodes, { group: 'all', query: '10812', subscriptions: subs })), ['c']);
console.log('PASS: group and search filtering');

const health = { a: { status: 'online', latencyMs: 320 }, b: { status: 'online', latencyMs: 45 }, c: { status: 'offline' }, d: { status: 'unknown' } };
const traffic = { a: { uploadedBytes: 10, downloadedBytes: 90 }, c: { uploadedBytes: 5, downloadedBytes: 0 } };
const original = JSON.stringify(nodes);
assert.deepEqual(ids(sortNodes(nodes, { key: 'latency', dir: 'asc' }, health, traffic)), ['b', 'a', 'c', 'd']);
assert.deepEqual(ids(sortNodes(nodes, { key: 'latency', dir: 'desc' }, health, traffic)), ['a', 'b', 'c', 'd'], 'untested nodes stay last either way');
assert.deepEqual(ids(sortNodes(nodes, { key: 'name', dir: 'asc' }, health, traffic)), ['b', 'c', 'a', 'd']);
assert.deepEqual(ids(sortNodes(nodes, { key: 'total', dir: 'desc' }, health, traffic)), ['a', 'c', 'b', 'd']);
assert.deepEqual(ids(sortNodes(nodes, { key: '', dir: 'asc' }, health, traffic, list => list.slice().reverse())), ['d', 'c', 'b', 'a'], 'automatic order defers to the list order');
assert.equal(JSON.stringify(nodes), original, 'sorting must not mutate the server order');
assert.deepEqual({ ...nextSort({ key: '', dir: 'asc' }, 'latency') }, { key: 'latency', dir: 'asc' });
assert.deepEqual({ ...nextSort({ key: 'latency', dir: 'asc' }, 'latency') }, { key: 'latency', dir: 'desc' });
assert.deepEqual({ ...nextSort({ key: 'latency', dir: 'desc' }, 'latency') }, { key: '', dir: 'asc' });
console.log('PASS: column sorting with unknown latency last and a three-step header cycle');

assert.deepEqual(Array.from(range(['a', 'b', 'c', 'd'], 'c', 'a')), ['a', 'b', 'c']);
assert.deepEqual(Array.from(range(['a', 'b', 'c', 'd'], 'x', 'b')), ['b']);
console.log('PASS: shift-click range selection');

const one = menuState([nodes[0]]);
assert.equal(one.setProxy, true); assert.equal(one.stop, true); assert.equal(one.start, false);
assert.equal(one.copyAddress, true); assert.equal(one.reassign, false, 'a started node keeps its port');
const many = menuState([nodes[0], nodes[1]]);
assert.equal(many.setProxy, false); assert.equal(many.edit, false); assert.equal(many.start, true); assert.equal(many.stop, true);
assert.equal(many.copyLinks, true); assert.equal(many.remove, true);
assert.equal(menuState([nodes[1]], { systemProxy: false }).setProxy, false, 'no system proxy action off Windows');
assert.equal(menuState([nodes[1]]).reassign, true);
assert.equal(menuState([nodes[2]]).autoOrder, true);
console.log('PASS: context menu availability for single and multiple nodes');

assert.equal(classifyPaste('vless://abc@host:443#a\n\ntrojan://p@h:443#b').kind, 'links');
assert.equal(classifyPaste('vless://abc@host:443#a\n\ntrojan://p@h:443#b').count, 2);
assert.equal(classifyPaste('https://sub.example/api?token=1').kind, 'subscription');
assert.equal(classifyPaste('hello world').kind, 'none');
assert.equal(classifyPaste('').kind, 'none');
assert.equal(classifyPaste('dmxlc3M6Ly9hYmNAaG9zdDo0NDMjYQp0cm9qYW46Ly9wQGg6NDQzI2I=').kind, 'links', 'base64 subscription bodies are imported');
console.log('PASS: Ctrl+V paste classification');

const point = position(990, 710, 220, 330, 1000, 720);
assert.equal(point.left, 772); assert.equal(point.top, 382);
assert.equal(latencyClass(80), 'fast'); assert.equal(latencyClass(300), 'medium'); assert.equal(latencyClass(900), 'slow');
console.log('PASS: menu stays inside the viewport; latency colour bands');
