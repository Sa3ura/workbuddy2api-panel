# WorkBuddy2API Panel — 本地构建 / 校验入口（POSIX：Linux / macOS / CI）
#
# 用法：
#   make all        # 构建全部 4 个二进制到 bin/
#   make check      # gofmt + vet + test（提交前跑这个）
#   make test       # 全量测试
#   make race       # 并发关键包竞态检测（pool / scheduler）
#   make version    # 打印源码 appVersion（发版时须与 tag 对齐）
#   make clean
#
# 说明：
#   - 二进制统一输出到 bin/（.gitignore 已忽略），不污染仓库根目录。
#   - Windows 无 make，请用 scripts/build.ps1（参数与上面一一对应）。
#   - 没有全局 go 时可用 GO= 覆盖，例如：
#       make GO=../tools/go/bin/go test

GO       ?= go
BIN_DIR  ?= bin
GOFMT    ?= gofmt
COMMIT   := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)$(shell git diff --quiet 2>/dev/null || echo -dirty)
STRIP    ?= 1
LDFLAGS  := $(if $(filter 1,$(STRIP)),-s -w )-X main.buildCommit=$(COMMIT)
VERSION  := $(shell grep -oP 'appVersion = "\K[^"]+' cmd/server/main.go 2>/dev/null || echo unknown)

.PHONY: all build test vet fmt-check check race vuln version docs-refs docs-secrets ci clean help

all: build

# 4 个入口包 → 二进制名（与 CI 发布口径一致：wb2api / signin_bin / login / credit）
define build_bin
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/$(1) ./cmd/$(2)
endef

build:
	$(call build_bin,wb2api,server)
	$(call build_bin,signin_bin,signin)
	$(call build_bin,login,login)
	$(call build_bin,credit,credit)
	@echo "built $(VERSION) -> $(BIN_DIR)/"

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

# 格式门禁：有任何未 gofmt 的文件即失败（不自动改写，避免误改他人代码）。
# 委托 scripts/build.sh，因为需要 CRLF 容错（Windows checkout 下工作区是 CRLF，
# 直接 gofmt -l 会把每个文件都判为未格式化）。
fmt-check:
	@bash scripts/build.sh fmt-check

race:
	$(GO) test -race ./internal/pool/... ./internal/scheduler/...

# 依赖漏洞扫描（首次运行会下载 govulncheck，需要网络）
vuln:
	$(GO) run golang.org/x/vuln/cmd/govulncheck@latest ./...

check: fmt-check vet test

version:
	@echo $(VERSION)

# 协议引用守卫：代码里指向外部材料的注释必须带 docs/protocol/ 锚点
docs-refs:
	@python scripts/check_protocol_refs.py

# 维护文档的密钥 / 个人信息扫描（CI 同款门禁）
docs-secrets:
	@python scripts/check_docs_secrets.py

# 与 CI 对齐的完整门禁：gofmt + vet + test + 两个文档守卫
ci: fmt-check vet test docs-secrets docs-refs
	@echo "ci: all green"

clean:
	rm -rf $(BIN_DIR)

help:
	@grep -E '^#  ' Makefile | sed 's/^#  //'
