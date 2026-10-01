#!/usr/bin/env bash
# ci-llvm-pkgconfig.sh — 输出 llvm.pc 所在目录，供 PKG_CONFIG_PATH 使用。
# Ubuntu 的 llvm-*-dev 包不带 llvm.pc（Debian 系惯例），find 落空时生成
# shim 指向标准安装前缀；brew/自装 LLVM 通常自带，直接用真实文件。
set -euo pipefail

PC_FILE=$(find /usr/lib/llvm-* -name llvm.pc 2>/dev/null | head -1 || true)
if [ -n "$PC_FILE" ]; then
  dirname "$PC_FILE"
  exit 0
fi

LLVMDIR=$(ls -d /usr/lib/llvm-* 2>/dev/null | head -1 || true)
if [ -z "$LLVMDIR" ]; then
  echo "no LLVM installation found under /usr/lib/llvm-*" >&2
  exit 1
fi
LLVM_MAJOR=$(basename "$LLVMDIR" | sed 's/llvm-//')

mkdir -p /tmp/gp-pkgconfig
cat > /tmp/gp-pkgconfig/llvm.pc <<EOF
prefix=$LLVMDIR
Name: llvm
Description: LLVM (Ubuntu shim)
Version: ${LLVM_MAJOR}.0
Cflags: -I\${prefix}/include
Libs: -L\${prefix}/lib -lLLVM-${LLVM_MAJOR}
EOF
echo /tmp/gp-pkgconfig
