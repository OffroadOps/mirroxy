# 贡献指南 / Contributing

感谢你愿意参与 Mirroxy。中文说明在下方，英文版本见文末。

## 开始之前

- 搜索现有 Issue，避免重复劳动。
- 较大改动请先开 Issue 讨论方案，避免白做。
- 任何功能都要同步更新 `web/changelog.json` 与 `web/changelog.en.json`（面板内「更新日志」
  页面直接读这两个文件）。
- 不要提交节点、订阅、凭据、数据库、密钥、日志或本机 `data/` 目录。

## 开发环境

- Go 1.26+
- Node.js（仅用于前端语法检查与 `packaging/` 下的界面回归脚本）
- Python 3（打包与发布脚本）
- 可选：真实核心 `bin/runtime/Mirroxy_core_A.exe`、`Mirroxy_core_B.exe` 与
  `bin/resources/` 下的分流规则文件

```bash
go mod download
go test ./...
go vet ./...
go build -trimpath -ldflags '-s -w -H=windowsgui' -o Mirroxy.exe .
```

## 代码约定

- Go 代码遵循标准 `gofmt`；提交前跑一次 `gofmt -l .`。
- 前端是原生 HTML/CSS/JS，没有构建步骤。新增文案要以中文原文写进界面，并在
  `web/i18n.js` 的英文表里补上对应翻译；界面文字由 `translateTree` 自动处理，动态生成的
  元素用 `t()` 包裹。
- 后端错误信息面向用户时用中文，面向开发者时说明清楚原因。
- 任何涉及本机状态的操作（系统代理、注册表、端口、文件）都必须落在临时目录或一次性键值里，
  并有对应的回滚路径；测试不得修改真实节点、端口与系统代理。
- 涉及网络的改动不得关闭 TLS 校验、不得在节点失败时静默回退直连、不得把凭据写进日志。

## 提交信息

用中文或英文均可，格式参考：

```text
feat: 节点行双击切换到系统代理
fix: 切换路由模式后端口未重新分配
docs: 补充 Android 构建说明
```

## 拉取请求

1. 从 `master` 开分支。
2. 保持一次 PR 只做一件事。
3. 附上验证方式：跑了哪些命令、哪些平台实机验证过、哪些没有。
4. 未在真实硬件验证的平台要明确写出来（例如「macOS 仅交叉编译，未实机运行」）。

## 安全问题

不要用公开 Issue 报告安全问题，按 [SECURITY.md](SECURITY.md) 的方式私下报告。

---

# Contributing (English)

Thanks for helping out. Please read this before opening a pull request.

- Search existing issues first.
- Open an issue before large changes so the design can be discussed.
- Every user-visible change must update `web/changelog.json` and `web/changelog.en.json`,
  which the dashboard renders on its "Changelog" page.
- Never commit nodes, subscriptions, credentials, databases, keys, logs or a local `data/`
  directory.

## Development

- Go 1.26+
- Node.js (syntax checks and the UI regression scripts under `packaging/`)
- Python 3 (packaging and release scripts)
- Optional: the real cores in `bin/runtime/` and the rule files in `bin/resources/`

```bash
go mod download
go test ./...
go vet ./...
go build -trimpath -ldflags '-s -w -H=windowsgui' -o Mirroxy.exe .
```

## Conventions

- Go code is `gofmt` clean; run `gofmt -l .` before committing.
- The frontend is plain HTML/CSS/JS without a build step. New user-facing strings are written
  in Chinese in the UI and translated in the English table in `web/i18n.js`. Static text is
  translated automatically by `translateTree`; dynamically created elements must use `t()`.
- User-facing errors are written in Chinese and should explain what actually went wrong.
- Anything that touches machine state (system proxy, registry, ports, files) must work inside
  a temporary directory or a disposable registry key and must have a rollback path. Tests must
  never modify real nodes, ports or the system proxy.
- Network related changes must not disable TLS verification, must not silently fall back to
  direct access when a node fails, and must never write credentials into logs.

## Pull requests

1. Branch from `master`.
2. One concern per pull request.
3. State how you verified it: which commands you ran, which platforms you tested on real
   hardware, and which ones you did not.
4. Platforms that were only cross-compiled must be stated explicitly.

## Security

Do not report security problems in a public issue. Follow [SECURITY.md](SECURITY.md).
