#!/bin/bash
set -euo pipefail
umask 077
[[ $(uname -s) == Darwin && $(uname -m) == arm64 ]] || { echo '此包仅用于 macOS ARM64' >&2; exit 1; }
mac_version=$(sw_vers -productVersion)
[[ ${mac_version%%.*} -ge 26 ]] || { echo '预置的 sing-box 1.14.0 官方核心要求 macOS 26 或更高版本，此安装尚未修改系统。' >&2; exit 1; }
[[ $(id -u) != 0 ]] || { echo '请使用已登录的普通用户运行，不要加 sudo' >&2; exit 1; }
src=$(cd -- "$(dirname -- "$0")" && pwd -P)
target="$HOME/Library/Application Support/Mirroxy"
legacy_target="$HOME/Library/Application Support/scrcpyproxy"
agent_dir="$HOME/Library/LaunchAgents"
plist="$agent_dir/com.offroadops.mirroxy.plist"
label="gui/$(id -u)/com.offroadops.mirroxy"
legacy_plist="$agent_dir/com.offroadops.scrcpyproxy.plist"
legacy_label="gui/$(id -u)/com.offroadops.scrcpyproxy"
cd "$src"
shasum -a 256 -c SHA256SUMS
chmod +x Mirroxy bin/runtime/Mirroxy_core_A bin/runtime/Mirroxy_core_B
./Mirroxy -version
./bin/runtime/Mirroxy_core_A version >/dev/null
./bin/runtime/Mirroxy_core_B version >/dev/null
mkdir -p "$target/data" "$agent_dir"
# Stop the previous managed process before replacing executables; bootout also
# removes its process group, including proxy cores.
if launchctl print "$label" >/dev/null 2>&1; then
  launchctl bootout "$label"
fi
if launchctl print "$legacy_label" >/dev/null 2>&1; then
  launchctl bootout "$legacy_label"
fi
# One-time 0.4.x migration. The first Mirroxy start imports these JSON files
# into its encrypted local database and removes the plaintext copies.
if [[ -d "$legacy_target/data" && ! -f "$target/data/Mirroxy.db" && ! -f "$target/data/settings.json" ]]; then
  for name in settings.json traffic.json; do
    [[ ! -f "$legacy_target/data/$name" ]] || cp -p "$legacy_target/data/$name" "$target/data/$name"
  done
fi
for name in Mirroxy bin licenses PLATFORM manifest.json SHA256SUMS README.txt; do
  [[ "$src" == "$target" ]] || cp -R "$src/$name" "$target/"
done
"$target/Mirroxy" -dir "$target" -init
"$target/Mirroxy" -dir "$target" -check
xml_escape() { printf '%s' "$1" | sed -e 's/\&/\&amp;/g' -e 's/</\&lt;/g' -e 's/>/\&gt;/g' -e 's/"/\&quot;/g'; }
escaped=$(xml_escape "$target")
cat > "$plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>com.offroadops.mirroxy</string>
<key>ProgramArguments</key><array><string>$escaped/Mirroxy</string><string>-dir</string><string>$escaped</string><string>-no-open</string></array>
<key>RunAtLoad</key><true/>
<key>KeepAlive</key><dict><key>SuccessfulExit</key><false/></dict>
<key>WorkingDirectory</key><string>$escaped</string>
<key>StandardOutPath</key><string>$escaped/data/launchd.log</string>
<key>StandardErrorPath</key><string>$escaped/data/launchd.log</string>
<key>ExitTimeOut</key><integer>20</integer>
</dict></plist>
EOF
plutil -lint "$plist"
launchctl bootstrap "gui/$(id -u)" "$plist"
rm -f "$legacy_plist"
ready=false
for attempt in {1..40}; do
  if "$target/Mirroxy" -ready >/dev/null 2>&1; then ready=true; break; fi
  sleep 1
done
[[ "$ready" == true ]] || { echo "后台启动失败，请检查 $target/data/launchd.log" >&2; exit 1; }
echo '安装完成，本机面板：http://127.0.0.1:15060/'
"$target/Mirroxy" -dir "$target" -access-info
open 'http://127.0.0.1:15060/'
