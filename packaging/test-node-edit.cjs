const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const context = {};
vm.runInNewContext(fs.readFileSync(path.join(__dirname, '../web/node-edit.js'), 'utf8'), context);
const {fields} = context.MirroxyNodeEdit;

// HTTP and SOCKS authenticate with a username and a password, so the editor
// must keep both boxes visible and label the password as a password.
for (const proto of ['http', 'socks']) {
  const f = fields(proto);
  assert.equal(f.password, true, `${proto}: the password box must stay visible`);
  assert.equal(f.username, true, `${proto}: the username box must stay visible`);
  assert.equal(f.passwordLabel, '密码 (Password)', `${proto}: wrong password label`);
  assert.equal(f.usernameLabel, '用户名 (Username)', `${proto}: wrong username label`);
}

// Other protocols keep their own labels and hide the fields that do not apply.
const vless = fields('vless');
assert.equal(vless.password, true);
assert.equal(vless.username, false, 'vless has no username field');
assert.equal(vless.passwordLabel, '用户 ID / 密码 (UUID / Password)');
assert.equal(vless.method, false);
assert.equal(vless.flow, true);

const tuic = fields('tuic');
assert.equal(tuic.username, true, 'tuic needs the UUID field');
assert.equal(tuic.usernameLabel, 'UUID');
assert.equal(tuic.passwordLabel, '密码 (Password)');

const vmess = fields('vmess');
assert.equal(vmess.method, true);
assert.equal(vmess.alter, true);
assert.equal(vmess.username, false);

console.log('PASS: HTTP/SOCKS keep the password field, labels follow the protocol');
