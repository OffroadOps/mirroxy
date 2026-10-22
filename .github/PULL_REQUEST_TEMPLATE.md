## 变更内容 / What changed

<!-- 一句话说明这个 PR 做了什么。/ One sentence describing this change. -->

## 关联 Issue / Related issue

<!-- Closes #123 -->

## 验证方式 / How this was verified

- [ ] `go test ./...`
- [ ] `go vet ./...`
- [ ] 前端语法检查 / Frontend syntax check (`node --check web/app.js`)
- [ ] 真实核心集成测试 / Native core integration tests
- [ ] Windows 实机 / Real hardware on Windows
- [ ] Linux 实机 / Real hardware on Linux
- [ ] macOS 实机 / Real hardware on macOS

<!-- 没有实机验证的平台请明确写出来，并说明只做了交叉编译。 -->
<!-- State explicitly which platforms were only cross-compiled. -->

## 检查清单 / Checklist

- [ ] 已更新 `web/changelog.json` 与 `web/changelog.en.json`
- [ ] 新增文案在 `web/i18n.js` 中有对应英文翻译
- [ ] 未提交节点、订阅、凭据、数据库、密钥、日志或本机 `data/`
- [ ] 涉及本机状态的操作有回滚路径，且测试未修改真实节点、端口与系统代理
- [ ] 未关闭 TLS 校验，未在节点失败时静默回退直连
