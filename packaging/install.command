#!/bin/bash
cd -- "$(dirname -- "$0")"
if ! bash ./install-macos.sh; then
  echo '安装未完成，请查看上面的错误。按回车关闭。'
  read -r _
  exit 1
fi
