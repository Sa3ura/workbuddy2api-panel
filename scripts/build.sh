#!/usr/bin/env bash
# WorkBuddy2API Panel — 本地构建 / 校验入口（POSIX）
#
# 与 Makefile / scripts/build.ps1 对齐的最小实现，供没装 make 的 Linux/mac 环境使用。
#
# 用法：
#   ./scripts/build.sh            # 构建 4 个二进制到 bin/
#   ./scripts/build.sh check      # gofmt + vet + test
#   ./scripts/build.sh test|vet|race|version|clean
#   GO=../tools/go/bin/go ./scripts/build.sh   # 指定 Go

set -euo pipefail

cd "$(dirname "$0")/.."
repo=$(pwd)

GO=${GO:-go}
BIN_DIR=${BIN_DIR:-bin}
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo unknown)
git diff --quiet 2>/dev/null || COMMIT="$COMMIT-dirty"
STRIP=${STRIP:-1}
if [ "$STRIP" = "1" ]; then STRIPFLAGS="-s -w"; else STRIPFLAGS=""; fi
LDFLAGS="$STRIPFLAGS -X main.buildCommit=$COMMIT"

if [ -z "${GOPROXY:-}" ]; then
  export GOPROXY="https://goproxy.cn,direct"
fi
export GOTOOLCHAIN=local

app_version() {
  grep -oP 'appVersion = "\K[^"]+' cmd/server/main.go 2>/dev/null || echo unknown
}

# 格式门禁。CRLF 容错：Windows checkout（core.autocrlf=true）下工作区是 CRLF，
# gofmt 会把每个文件都判为"需格式化"。这里对存疑文件先做一次 LF 归一后再判，
# 只有真正的格式问题才算失败（行尾差异不算）。
fmt_check() {
  local out f tmp real=""
  out=$(gofmt -l cmd internal || true)
  if [ -z "$out" ]; then
    echo "gofmt: clean"
    return 0
  fi
  tmp="${TMPDIR:-/tmp}/wb2api-fmt-$$.go"
  while IFS= read -r f; do
    [ -z "$f" ] && continue
    if ! grep -q $'\r' "$f"; then
      real+="$f"$'\n'
      continue
    fi
    tr -d '\r' <"$f" >"$tmp"
    if [ -n "$(gofmt -l "$tmp" || true)" ]; then
      real+="$f"$'\n'
    fi
  done <<<"$out"
  rm -f "$tmp"
  if [ -n "$real" ]; then
    echo "以下文件未 gofmt：" >&2
    printf '%s' "$real" >&2
    return 1
  fi
  echo "gofmt: clean（已忽略 CRLF 行尾差异）"
}

build_all() {
  mkdir -p "$BIN_DIR"
  local pairs="wb2api:server signin_bin:signin login:login credit:credit"
  for pair in $pairs; do
    local name=${pair%%:*} pkg=${pair##*:}
    echo "==> $name  (./cmd/$pkg)"
    CGO_ENABLED=0 "$GO" build -trimpath -ldflags="$LDFLAGS" -o "$BIN_DIR/$name" "./cmd/$pkg"
  done
  echo "built $(app_version) -> $BIN_DIR/"
}

case "${1:-all}" in
  all|build) build_all ;;
  check)
    # 与 CI（go-binaries.yml 的 test job）同口径：fmt → vet → test → 两个文档守卫
    fmt_check
    "$GO" vet ./...
    "$GO" test ./...
    if command -v python3 >/dev/null 2>&1; then
      python3 scripts/check_docs_secrets.py
      python3 scripts/check_protocol_refs.py
    else
      echo "python3 未安装：跳过文档守卫（CI 上会跑）" >&2
    fi
    echo "check: all green"
    ;;
  fmt-check) fmt_check ;;
  vet)       "$GO" vet ./... ;;
  test)      "$GO" test ./... ;;
  race)      "$GO" test -race ./internal/pool/... ./internal/scheduler/... ;;
  version)   app_version ;;
  clean)     rm -rf "$BIN_DIR"; echo "cleaned $BIN_DIR" ;;
  *)
    echo "未知任务：$1（可选：all|build|check|fmt-check|vet|test|race|version|clean）" >&2
    exit 2
    ;;
esac
