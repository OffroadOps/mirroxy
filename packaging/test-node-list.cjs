const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const context = {};
vm.runInNewContext(fs.readFileSync(path.join(__dirname, '../web/node-list.js'), 'utf8'), context);
const sorted = context.MirroxyNodeList.sortedNodes;
const ids = nodes => Array.from(sorted(nodes), node => node.id);
const nodes = [
  { id: 'stopped-1', enabled: false, status: 'stopped' },
  { id: 'started-1', enabled: true, status: 'running' },
  { id: 'stopped-2', enabled: false, status: 'stopped' },
  { id: 'started-2', enabled: true, status: 'running' },
];
const original = JSON.stringify(nodes);
assert.deepEqual(ids(nodes), ['started-1', 'started-2', 'stopped-1', 'stopped-2']);
assert.equal(JSON.stringify(nodes), original, 'display sorting must not mutate server order');
nodes[1].listBottom = true;
assert.deepEqual(ids(nodes), ['started-2', 'stopped-1', 'stopped-2', 'started-1']);
nodes[1].enabled = false; nodes[1].status = 'stopped';
assert.equal(ids(nodes).at(-1), 'started-1', 'explicit bottom must survive stop');
nodes[1].enabled = true; nodes[1].status = 'running';
assert.equal(ids(nodes).at(-1), 'started-1', 'explicit bottom must survive restart');
nodes[1].listBottom = false;
assert.deepEqual(ids(nodes), ['started-1', 'started-2', 'stopped-1', 'stopped-2']);
nodes[0].enabled = true; nodes[0].status = 'error';
assert.equal(ids(nodes)[0], 'stopped-1', 'started errors should remain visible, not muted as stopped');
console.log('PASS: started-first stable ordering, explicit bottom, restart, and automatic-order reset');
