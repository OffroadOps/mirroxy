#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
[[ $(id -u) == 0 ]] || { echo '请使用 sudo bash 运行安装脚本' >&2; exit 1; }
[[ $(uname -s) == Linux ]] || { echo '仅支持 Linux' >&2; exit 1; }
case "$(uname -m)" in
  aarch64|arm64) arch=arm64; expected=47e71760f5f81dd994f460b025ff2a53f08bb03f026eff33bb38e622526cf946 ;;
  x86_64|amd64) arch=amd64; expected=eb4c0a1dc0d8bccc4cc3bdc7c546ccf8aacef9ccdb2e26f08d1dd8db8014d2ce ;;
  *) echo '仅支持 ARM64 / AMD64' >&2; exit 1 ;;
esac
base=${MIRROXY_DOWNLOAD_BASE:-https://github.com/OffroadOps/mirroxy/releases/download/v0.8.0}
temp_dir=$(mktemp -d /tmp/Mirroxy-download.XXXXXX)
cleanup() { case "$temp_dir" in /tmp/Mirroxy-download.*) rm -rf -- "$temp_dir" ;; esac; }
trap cleanup EXIT
echo "正在下载 Mirroxy linux-$arch（包含两个内核和路由数据）…"
if ! curl --fail --location --retry 5 --retry-delay 3 --connect-timeout 15 --max-time 900 "$base/Mirroxy-linux-$arch.tar.gz" -o "$temp_dir/package.tar.gz"; then
  echo '下载失败。可下载对应架构的 .run 文件，复制到设备离线安装；也可设置 MIRROXY_DOWNLOAD_BASE 使用镜像。' >&2
  exit 1
fi
echo "$expected  $temp_dir/package.tar.gz" | sha256sum -c -
mkdir "$temp_dir/package"
tar -xzf "$temp_dir/package.tar.gz" -C "$temp_dir/package" --no-same-owner
bash "$temp_dir/package/install-local.sh" "$@"
