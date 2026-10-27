'use strict';
(() => {
  // Every supported protocol carries a password, so the password box is always
  // shown. HTTP and SOCKS authenticate with a username and a password, so they
  // keep both boxes; the labels follow the protocol so a SOCKS password is not
  // presented as a UUID.
  function fields(protocol) {
    const proto = String(protocol || '').toLowerCase();
    const userPass = proto === 'http' || proto === 'socks';
    const tuic = proto === 'tuic';
    return {
      password: true,
      username: userPass || tuic,
      method: proto === 'vmess' || proto === 'shadowsocks',
      flow: proto === 'vless',
      alter: proto === 'vmess',
      passwordLabel: userPass || tuic ? '密码 (Password)' : '用户 ID / 密码 (UUID / Password)',
      usernameLabel: tuic ? 'UUID' : '用户名 (Username)',
      passwordPlaceholder: userPass ? '与用户名配套的密码' : '节点认证信息'
    };
  }
  globalThis.MirroxyNodeEdit = {fields};
})();
