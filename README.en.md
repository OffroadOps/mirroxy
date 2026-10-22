<div align="center">

# Mirroxy

**Multi-node local proxy manager — open source**

[English](README.en.md) · [简体中文](README.md)

[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26%2B-blue.svg)](go.mod)
[![Platforms](https://img.shields.io/badge/platform-Windows%20%7C%20Linux%20%7C%20macOS-lightgrey.svg)](#download)

</div>

Mirroxy is a multi-node local proxy manager written in Go. A single background process
provides a local web dashboard, a Windows tray icon, an HTTP API for automation, node
lifecycle management, traffic accounting, split routing and online updates.

Every running node owns a **fixed local mixed port** that accepts both HTTP and SOCKS5, and
several nodes can **run at the same time** without fighting over ports or routes. Each node
runs in **its own core process**, so starting or stopping one node never interrupts the
connections of the others.

> Switching to a node takes one step: **double-click a node row**. The node is started when
> needed and the system proxy is handed over to it.

## Features

- **One-step node switching** — double-clicking a node row starts it and takes over the system
  proxy. The "Take over system proxy" switch, the node picker and the routing mode all apply
  immediately; there is no extra "Apply" button.
- **One core process per node** — starting or stopping a node only restarts its own core, so
  the other nodes keep their connections.
- **Parallel nodes** — nodes get fixed ports from 5060 upwards, run side by side and report
  their own connection counts and traffic.
- **Many protocols** — VMess, VLESS/REALITY, Shadowsocks, Trojan, SOCKS, HTTP/HTTPS,
  Hysteria2, TUIC and AnyTLS. VMess/VLESS/Trojan and friends run on network core A,
  Hysteria2/TUIC/AnyTLS on network core B.
- **Split routing** — global and per-node routing: bypass mainland China, blacklist or global.
  In global mode LAN and loopback addresses stay direct (can be switched off).
- **Windows game / application proxy** — pick programs by `.exe` name or full path; their
  TCP/UDP traffic follows the bound node while everything else keeps the current network path.
  TUN only, no system route changes.
- **Built-in download manager** — an aria2 page inside the dashboard: create, pause, resume,
  limit bandwidth, batch operations and resume broken transfers. Every task picks direct
  access, a running Mirroxy node or a custom HTTP proxy.
- **Privacy first** — nodes, subscriptions, remote credentials and traffic counters live in an
  AES-GCM encrypted SQLite database; the Windows master key is protected with DPAPI. Regular
  status endpoints never return node passwords, outbound configuration, subscription URLs or
  the UI token.
- **Remote management** — optional remote dashboard and API, always behind an account and
  password. Sensitive settings can only be changed from the local machine.
- **Chinese and English UI** with light and dark themes; the language follows the browser.

## Download

Binaries are published as [GitHub Releases](https://github.com/OffroadOps/mirroxy/releases)
for four platforms: Windows x86_64, Linux ARM64, Linux AMD64 and macOS ARM64. Every release
ships a `SHA256SUMS` manifest.

| Platform | File | Notes |
|---|---|---|
| Windows x86_64 | `Mirroxy-windows-amd64.zip` | Full program package; an `.exe` installer is provided as well |
| Linux ARM64 | `Mirroxy-linux-arm64.run` | Offline installer (`sudo bash ./Mirroxy-linux-arm64.run`) |
| Linux AMD64 | `Mirroxy-linux-amd64.run` | Offline installer |
| macOS ARM64 | `Mirroxy-macos-arm64.zip` | Unpack, then `bash ./install-macos.sh` |

Linux one-line install (systemd, auto-detects ARM64 / AMD64):

```bash
curl -fsSL --retry 3 https://raw.githubusercontent.com/OffroadOps/mirroxy/master/install.sh | sudo bash
```

> Older releases and the legacy entry point stay on the
> [S3 release page](https://ny-1s.enzonix.com/bucket-1776-2431/scrcpyproxy/index.html).
> GitHub Releases is the channel for new versions.

## Usage

On Windows, unpack or install Mirroxy, start it and open <https://127.0.0.1:9527/> (the port
is selectable during installation). Import share links, a subscription or a v2rayN SQLite
node library, then start the nodes you need. Ports are allocated from 5060 upwards and stay
bound. Downloaders should use `127.0.0.1` and the port shown for the node.

Switching to a node takes one step: **double-click the node row**. The node is started when
needed and the system proxy is handed over to it. The "Take over system proxy" switch hands
the proxy back or takes it over again, and the node picker and routing mode apply instantly.

The Windows installer detects a busy dashboard port. Close the program that holds it or pick
another port in the wizard; upgrades keep the existing port. Silent installs accept
`/PANELPORT=9528`.

By default Mirroxy leaves the system proxy alone, so v2rayN can keep using
`127.0.0.1:10808`. You can switch to taking the proxy over or clearing it from the top bar;
the previous settings are restored on exit.

Double-click the tray icon to open the dashboard, right-click to quit. Closing the page does
not stop the proxy. The node table shows connection counts, live and cumulative traffic and
availability; nodes can be renamed, shared, tested, set as the system proxy or given their
own routing policy.

### System proxy and routing

The "Take over system proxy" switch is the daily entry point: on, the system proxy points at
the selected node's local port; off, the settings from before the takeover are handed back
unchanged. The node picker lists every node, marks the ones that are not running yet and
starts them before taking over. "Clear" disables the system proxy entirely. Mirroxy does not
touch the system proxy by default, so other clients keep working.

| Action | Behaviour |
|---|---|
| Leave unchanged | Default; Windows proxy settings are not written. |
| Take over | Uses the selected node's HTTP port; a stopped node is started first. |
| Clear | Disables the current static proxy and PAC. |

The global routing mode defaults to bypassing mainland China and can be switched to a
blacklist or global routing. Each node can follow the global mode or override it. A failing
node never silently falls back to another line: its fixed port keeps failing so downloads and
applications never change routes behind your back.

Every node runs in its own core process. Starting or stopping one node only restarts its own
core and never interrupts the connections of the others; only switching the global routing
mode rebuilds all node cores and reconnects briefly.

### Downloads (aria2)

The dashboard has an IDM-style download page: a top toolbar and a task table, without a
category sidebar. It supports creating, searching, pausing, resuming, editing the
link/proxy/connections/speed limit, retrying and single or multi selection. Removing a task
only drops the record and keeps the file and its `.aria2` resume data; ticking "also delete
the downloaded file and resume data" moves them into a `.Mirroxy-trash` recovery folder next
to the download directory.

Every task explicitly picks direct access, a running Mirroxy node or a custom HTTP proxy
(optionally with credentials). Direct access ignores environment and Windows proxy settings;
node mode follows that node's exit policy; a node that is not running is an error and never
falls back to direct access. aria2 does not speak SOCKS5 — use the same Mirroxy port as an
HTTP proxy.

The Windows package bundles the official aria2 1.37.0 binary and its license; update packages
keep the existing engine. Linux and macOS use the system `aria2c` (for example
`sudo apt install aria2` or `brew install aria2`); no other platform is claimed to be
verified on real hardware.

### Windows game / application proxy

Pick running processes by full path, or add `.exe` files manually. Each profile can follow the
default application node or override it. TCP and UDP of the selected programs follow the bound
node while everything else keeps the current network path; local and LAN addresses stay
direct.

The application proxy uses a separate sing-box TUN. Each node exposes a random, authenticated
loopback-only entry point so traffic is forced into the bound node and never inherits the
regular 506x bypass or blacklist rules. The feature is off by default; enabling it asks for
Windows administrator rights once and revokes the TUN when Mirroxy stops or exits. There is no
claim of a persistent WFP kill-switch: force-killing the elevated helper can let Windows
return to plain direct access.

The page can also probe TCP, UDP, IPv6 and UDP mapping characteristics through the default
application node. A server host can run `Mirroxy -probe-server` to print a read-only JSON
report.

## Files and local data

```text
Mirroxy.exe
upgrade.exe
bin/
  runtime/
    Mirroxy_core_A.exe
    Mirroxy_core_B.exe
  resources/
    Mirroxy_route_ip.bin
    Mirroxy_route_domain.bin
    Mirroxy_set_01.bin ... Mirroxy_set_12.bin
    Mirroxy_trust.pem
data/
  Mirroxy.db
  Mirroxy.key
  Mirroxy.log
  Mirroxy_upgrade.log
  runtime/
    Mirroxy_worker_*.log
```

Settings, node outbounds, subscriptions, remote credentials, traffic counters, update state
and the system proxy restore information are serialised individually and stored in the SQLite
database `data/Mirroxy.db` encrypted with AES-GCM. The 256-bit Windows master key is
additionally protected with the current user's DPAPI and stored as `data/Mirroxy.key`. On
Linux and macOS the key file is mode 0600. Back up the database and the key together.

Core configurations are written to `data/runtime/Mirroxy_session_*.cfg.pending`, validated,
used to start the core and deleted once the port is ready; failure paths clean up as well.
Runtime logs use generic names that do not reveal which core is running.

The first start of 0.5.0 reads the legacy `settings.json` and `traffic.json`, writes them to
the encrypted database and then deletes the plain files and the legacy generated
configuration. A failed migration keeps the originals and stops the start, so nothing is
silently lost.

## API and remote access

The local API lives at `http://127.0.0.1:15060/api/v1/` and needs no credentials from
loopback. Remote listening is off by default; when enabled both the dashboard and the API
require an account and password, including requests forwarded from the local machine to the
remote port.

- `GET /api/v1/guide`
- `GET /api/v1/openapi.json`
- `GET /api/v1/nodes`
- `GET /api/v1/traffic`
- `GET /api/v1/status`
- `POST /api/v1/nodes/{id}/start|stop|test|share|rename`
- `POST /api/v1/nodes/import`
- `POST /api/v1/routing`
- `POST /api/v1/updates/check`
- `POST /api/v1/updates/apply`

Regular status endpoints never return node passwords, full outbound configuration,
subscription URLs or the UI token. The share endpoint returns the full share link; remote
calls stay protected by authentication.

## Build from source

Go 1.26 or newer is required.

```bash
go mod download
go test ./...
go build -trimpath -ldflags '-s -w -H=windowsgui' -o Mirroxy.exe .
go build -trimpath -ldflags '-s -w' -o upgrade.exe ./cmd/upgrade
```

The Windows GUI needs `-H=windowsgui`; `cmd/upgrade` is the standalone update helper.

### Cross-platform builds and packaging

```bash
python packaging/build-release.py --go <go>
```

This produces full packages and `Mirroxy-update-*.zip` archives for Windows x86_64, Linux
ARM64, Linux AMD64 and macOS ARM64. The Windows payload is `Mirroxy-windows-amd64.zip`. By
default no Windows installer is compiled, only the four program packages; add
`--offline-windows --iscc <path>` for the offline wizard.

The fixed `Mirroxy-online-setup.exe` reads the latest version from the HTTPS `releases.json`
and downloads the full ZIP, verifying the source, size, SHA256, the internal file list and the
AMD64 architecture before installing. It never disables TLS verification and never packages
source code. A download proxy can be supplied, empty means direct, and the system proxy is
never changed. `/DOWNLOADPROXY=http://127.0.0.1:10808` works for silent installs.

The same version never modifies files and downgrades are refused; a higher version upgrades
in place with the same AppId. Installing, upgrading and uninstalling all keep the `data`
directory inside the installation folder. The release whitelist never contains local data,
nodes, keys, logs or storage credentials. Online updates keep the existing cores and ports
and only add missing ones; the old backend is replaced only after it exits normally, and
connections reconnect briefly. A failed download or verification never replaces the installed
program.

## Tests

```bash
go test ./...
go vet ./...
```

The native core integration tests need the real cores and the rule files:

```powershell
$env:MIRROXY_TEST_XRAY = 'D:\Mirroxy\bin\runtime\Mirroxy_core_A.exe'
$env:MIRROXY_TEST_SINGBOX = 'D:\Mirroxy\bin\runtime\Mirroxy_core_B.exe'
$env:MIRROXY_TEST_RULES = 'D:\Mirroxy\bin\resources'
go test -v .
```

`MIRROXY_TEST_RULES` provides the split-routing rule files; the tests copy them into the
temporary run directory, and they are skipped when the variable is unset. Tests use temporary
directories and a local mock upstream, and never touch the real system proxy, nodes or ports.
Windows proxy tests use a disposable registry path. macOS is only cross-compiled and
structurally verified; there is no claim of running on real hardware.

Frontend JavaScript can be syntax checked with `node --check web/app.js`; `packaging/` holds
several UI regression scripts.

## Release process

```bash
python packaging/build-release.py --go <go>
python packaging/publish-release.py --version 0.8.0
```

`publish-release.py` verifies the SHA-256 sums in `dist/<tag>`, creates a draft release,
uploads the four platform packages and the update archives, publishes the release and syncs
`README.md` and `install.sh` to the default branch. Historical versions are kept and the same
version must never be replaced with different bytes.

`packaging/publish-s3.py` and `packaging/release_site.py` remain for the archived S3 release
page; new versions are no longer uploaded to S3.

The real upstream core names, versions, source links and licenses only live in the build
metadata and in the NOTICE inside the packages. The project follows the protocol
configuration behaviour of v2rayN; no part of the v2rayN client is copied.

## Project layout

| Path | Contents |
|---|---|
| Root `*.go` | Go backend: dashboard, API, nodes, cores, routing, tray, updates |
| `web/` | Dashboard frontend (plain HTML/CSS/JS with i18n and themes) |
| `cmd/upgrade/` | Standalone Windows update helper |
| `cmd/bootstrap/` | Bootstrap installer backend |
| `packaging/` | Packaging, release and page generation scripts |
| `android/` | Independently developed Android app (Kotlin + Jetpack Compose) |
| `licenses/` | Third-party licenses and the NOTICE |
| `docs/` | Design and verification notes |

## Contributing

Issues and pull requests are welcome. Please read [CONTRIBUTING.md](CONTRIBUTING.md) first.
Report security problems privately as described in [SECURITY.md](SECURITY.md) instead of
opening a public issue.

## License

Mirroxy itself is released under the [MIT License](LICENSE). The third-party components it
ships are not modified and keep their own terms (Xray is MPL-2.0, sing-box is MIT, aria2 is
GPL-2.0-or-later); see [licenses/NOTICE.txt](licenses/NOTICE.txt) for the full list.

## Credits

- [Xray-core](https://github.com/XTLS/Xray-core) — network core A
- [sing-box](https://github.com/SagerNet/sing-box) — network core B
- [aria2](https://github.com/aria2/aria2) — download engine
- [v2ray-rules-dat](https://github.com/Loyalsoldier/v2ray-rules-dat) and
  [meta-rules-dat](https://github.com/MetaCubeX/meta-rules-dat) — split-routing data
- [v2rayN](https://github.com/2dust/v2rayN) — reference for protocol configuration behaviour
