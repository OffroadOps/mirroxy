#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
package_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)
target=/opt/mirroxy
host=''
port=15061
while (($#)); do
  case "$1" in
    --host) host=${2:?--host 缺少 IP 或域名}; shift 2 ;;
    --port) port=${2:?--port 缺少端口}; shift 2 ;;
    *) echo "不支持的选项：$1" >&2; exit 2 ;;
  esac
done
[[ $(id -u) == 0 ]] || { echo '请使用 sudo bash 安装脚本' >&2; exit 1; }
[[ $(uname -s) == Linux ]] || { echo '此安装包仅适用于 Linux' >&2; exit 1; }
case "$(uname -m)" in aarch64|arm64) arch=arm64 ;; x86_64|amd64) arch=amd64 ;; *) echo '仅支持 ARM64 / AMD64' >&2; exit 1 ;; esac
[[ $(cat "$package_dir/PLATFORM") == "linux-$arch" ]] || { echo '安装包架构与本机不符' >&2; exit 1; }
[[ $port =~ ^[0-9]+$ ]] && ((port >= 1024 && port <= 65000 && port != 15060)) || { echo '管理端口应为 1024–65000，且不能是 15060' >&2; exit 1; }
[[ $host != *[[:space:]/@]* ]] || { echo '--host 只接受 IP 或域名' >&2; exit 1; }
command -v systemctl >/dev/null && [[ -d /run/systemd/system ]] || { echo '需要正在运行的 systemd（Armbian / Debian / Ubuntu）。' >&2; exit 1; }
for command in install cp mv sha256sum getent useradd groupadd; do command -v "$command" >/dev/null || { echo "系统缺少 $command" >&2; exit 1; }; done
# Every payload file is listed; verify before stopping an existing service.
(cd "$package_dir" && sha256sum --quiet -c SHA256SUMS)
"$package_dir/Mirroxy" -version
check_dir=$(mktemp -d /tmp/Mirroxy-check.XXXXXX)
backup=''
changed=0
was_active=0
legacy_active=0
systemctl is-active --quiet mirroxy && was_active=1
systemctl is-active --quiet scrcpyproxy && legacy_active=1
cleanup() {
  result=$?
  if ((result != 0 && changed)); then
    echo '安装失败，正在恢复之前的版本。' >&2
    systemctl stop mirroxy || true
    if [[ -n $backup && -d $backup ]]; then
      [[ ! -d "$backup/bin" ]] || { [[ ! -d "$target/bin" ]] || mv "$target/bin" "$backup/failed-bin"; mv "$backup/bin" "$target/bin"; }
      [[ ! -f "$backup/Mirroxy" ]] || cp -a "$backup/Mirroxy" "$target/Mirroxy"
      [[ ! -d "$backup/data" ]] || cp -a "$backup/data/." "$target/data/"
      if [[ -f "$backup/mirroxy.service" ]]; then cp -a "$backup/mirroxy.service" /etc/systemd/system/mirroxy.service; fi
      systemctl daemon-reload || true
      ((was_active == 0)) || systemctl start mirroxy || true
    fi
    ((legacy_active == 0)) || systemctl start scrcpyproxy || true
    echo "诊断：journalctl -u mirroxy -n 50；备份：$backup" >&2
  fi
  case "$check_dir" in /tmp/Mirroxy-check.*) rm -rf -- "$check_dir" ;; esac
  exit "$result"
}
trap cleanup EXIT
mkdir -p "$check_dir/data"
ln -s "$package_dir/bin" "$check_dir/bin"
state_dir="$target/data"
if [[ ! -f "$state_dir/Mirroxy.db" && ! -f "$state_dir/settings.json" && -d /opt/scrcpyproxy/data ]]; then state_dir=/opt/scrcpyproxy/data; fi
for name in Mirroxy.db Mirroxy.key settings.json traffic.json; do
  [[ ! -f "$state_dir/$name" ]] || cp "$state_dir/$name" "$check_dir/data/$name"
done
SSL_CERT_FILE="$package_dir/bin/resources/Mirroxy_trust.pem" "$package_dir/Mirroxy" -dir "$check_dir" -check
getent group mirroxy >/dev/null || groupadd --system mirroxy
id mirroxy >/dev/null 2>&1 || useradd --system --gid mirroxy --home-dir "$target" --no-create-home --shell /usr/sbin/nologin mirroxy
install -d -m 0755 -o mirroxy -g mirroxy "$target"
[[ ! -L "$target/bin" && ! -L "$target/data" ]] || { echo '安装目录不能使用 bin/data 符号链接' >&2; exit 1; }
if [[ -f /etc/systemd/system/mirroxy.service ]]; then systemctl stop mirroxy; fi
if ((legacy_active)); then systemctl stop scrcpyproxy; fi
backup="$target/.backups/$(date +%Y%m%d-%H%M%S)-$$"
install -d -m 0700 "$backup"
[[ ! -f "$target/Mirroxy" ]] || cp -a "$target/Mirroxy" "$backup/Mirroxy"
[[ ! -d "$target/data" ]] || cp -a "$target/data" "$backup/data"
[[ ! -f /etc/systemd/system/mirroxy.service ]] || cp -a /etc/systemd/system/mirroxy.service "$backup/mirroxy.service"
changed=1
[[ ! -d "$target/bin" ]] || mv "$target/bin" "$backup/bin"
cp -a "$package_dir/bin" "$target/bin"
install -m 0755 "$package_dir/Mirroxy" "$target/Mirroxy"
install -m 0644 "$package_dir/manifest.json" "$target/manifest.json"
cp -a "$package_dir/licenses" "$target/"
install -d -m 0700 "$target/data"
if [[ -d /opt/scrcpyproxy/data && ! -f "$target/data/Mirroxy.db" && ! -f "$target/data/settings.json" ]]; then
  for name in settings.json traffic.json; do
    [[ ! -f "/opt/scrcpyproxy/data/$name" ]] || cp -p "/opt/scrcpyproxy/data/$name" "$target/data/$name"
  done
fi
"$target/Mirroxy" -dir "$target" -init -remote-port "$port" -public-host "$host" > "$check_dir/access.txt"
chown -R mirroxy:mirroxy "$target/bin" "$target/data"
install -m 0644 "$package_dir/mirroxy.service" /etc/systemd/system/mirroxy.service
install -m 0755 "$package_dir/mirroxyctl" /usr/local/bin/mirroxyctl
systemctl daemon-reload
systemctl enable mirroxy
systemctl start mirroxy
ready=0
for ((i=0; i<30; i++)); do
  if "$target/Mirroxy" -ready >/dev/null 2>&1; then ready=1; break; fi
  sleep 1
done
((ready)) || { echo '面板未能启动，请检查管理端口是否被占用' >&2; exit 1; }
if ((legacy_active)); then systemctl disable scrcpyproxy >/dev/null 2>&1 || true; fi
changed=0
echo
echo '安装完成，开机自动运行：'
cat "$check_dir/access.txt"
echo '查看地址和密码：sudo mirroxyctl info'
echo '查看运行状态：sudo mirroxyctl status'
echo '查看日志：sudo mirroxyctl logs'
