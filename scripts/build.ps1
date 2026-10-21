<#
.SYNOPSIS
    WorkBuddy2API Panel 本地构建 / 校验入口（Windows）。

.DESCRIPTION
    Windows 没有 make，本脚本提供与 Makefile 对齐的任务入口。
    会自动寻找 Go：先看 -GoBin / GBIN 环境变量，其次 PATH 里的 go，
    最后尝试仓库同级的便携版（..\tools\go）与仓库内 .tools\go。

.PARAMETER Task
    all / build  构建 4 个二进制到 bin/
    check        gofmt + vet + test（提交前跑这个）
    test         全量测试
    vet          go vet
    fmt-check    格式门禁（有未 gofmt 文件即失败，不改写）
    race         pool / scheduler 竞态检测
    version      打印源码 appVersion
    clean        删除 bin/

.PARAMETER GoBin
    指定 go.exe 路径；等价于设置环境变量 GBIN。

.PARAMETER BinDir
    二进制输出目录，默认 bin。

.EXAMPLE
    .\scripts\build.ps1
    .\scripts\build.ps1 check
    .\scripts\build.ps1 race
    .\scripts\build.ps1 -GoBin 'D:\go\bin\go.exe' build

.NOTES
    本文件必须保存为 UTF-8 with BOM。Windows PowerShell 5.1 会把无 BOM 的 UTF-8
    按 GBK 解析，中文串直接引发语法错误（"字符串缺少终止符"）。编辑后若报此类错误，
    请确认 BOM 仍在（前三个字节 EF BB BF）。
#>
[CmdletBinding()]
param(
    [ValidateSet('all', 'build', 'check', 'test', 'vet', 'fmt-check', 'race', 'version', 'clean')]
    [string]$Task = 'all',

    [string]$GoBin = $env:GBIN,

    [string]$BinDir = 'bin'
)

$ErrorActionPreference = 'Stop'

$repo = Split-Path $PSScriptRoot -Parent
Set-Location $repo

function Resolve-Go {
    if ($GoBin) {
        if (-not (Test-Path $GoBin)) { throw "GBIN/-GoBin 指向的文件不存在：$GoBin" }
        return (Resolve-Path $GoBin).Path
    }
    $cmd = Get-Command go -ErrorAction SilentlyContinue
    if ($cmd) { return $cmd.Source }

    # 便携版约定：把 go 解压到仓库同级 tools\go 或仓库内 .tools\go
    $candidates = @(
        (Join-Path (Split-Path $repo) 'tools\go\bin\go.exe'),
        (Join-Path $repo '.tools\go\bin\go.exe')
    )
    foreach ($c in $candidates) {
        if (Test-Path $c) { return (Resolve-Path $c).Path }
    }
    throw "未找到 Go。请安装 Go 1.22+，或下载便携版解压到 <仓库上级>\tools\go，再重试（也可用 -GoBin 指定）。"
}

$script:go = Resolve-Go
$script:gofmt = Join-Path (Split-Path $script:go) 'gofmt.exe'
if (-not (Test-Path $script:gofmt)) { $script:gofmt = 'gofmt' }

# 国内网络默认走 goproxy.cn（与 Dockerfile 口径一致）；已有设置则不覆盖
if (-not $env:GOPROXY) { $env:GOPROXY = 'https://goproxy.cn,direct' }
# 固定本地工具链：避免 go 自动下载更高版本 toolchain（受限网络下会卡住）
$env:GOTOOLCHAIN = 'local'

# 注意：必须以数组整体传入参数。直接写 Invoke-Go build -o x 会让 PowerShell
# 把 -o 当成函数参数名解析（与 -OutVariable/-OutBuffer 二义），报参数绑定错误。
function Invoke-Go {
    param([Parameter(Mandatory = $true)][string[]]$GoArgs)
    & $script:go @GoArgs
    if ($LASTEXITCODE -ne 0) { throw ("go {0} 失败（exit={1}）" -f ($GoArgs -join ' '), $LASTEXITCODE) }
}

function Get-AppVersion {
    $main = Join-Path $repo 'cmd\server\main.go'
    $m = Select-String -Path $main -Pattern 'appVersion\s*=\s*"([^"]+)"' | Select-Object -First 1
    if ($m) { return $m.Matches[0].Groups[1].Value }
    return 'unknown'
}

function Test-GoFmt {
    $flagged = @(& $script:gofmt -l cmd internal)
    if ($LASTEXITCODE -ne 0) { throw "gofmt -l 执行失败（exit=$LASTEXITCODE）" }
    if (-not $flagged) { Write-Host 'gofmt: clean'; return }

    # CRLF 容错：Windows checkout（core.autocrlf=true）下工作区是 CRLF，gofmt 会把
    # 每个文件都判为"需格式化"。对存疑文件做一次 LF 归一后再判，只有真正的格式问题
    # 才算失败（行尾差异不算）。
    $real = @()
    $tmpdir = Join-Path ([System.IO.Path]::GetTempPath()) ("wb2api-fmt-" + [guid]::NewGuid().ToString('n'))
    New-Item -ItemType Directory -Force -Path $tmpdir | Out-Null
    try {
        $tmp = Join-Path $tmpdir 'x.go'
        foreach ($rel in $flagged) {
            $src = [System.IO.File]::ReadAllText($rel)
            if ($src -notmatch "`r`n") { $real += $rel; continue }
            $norm = $src -replace "`r`n", "`n"
            [System.IO.File]::WriteAllText($tmp, $norm, (New-Object System.Text.UTF8Encoding($false)))
            $still = @(& $script:gofmt -l $tmp)
            if ($still) { $real += $rel }
        }
    }
    finally {
        Remove-Item -Recurse -Force $tmpdir -ErrorAction SilentlyContinue
    }

    if ($real) {
        Write-Host "以下文件未 gofmt：" -ForegroundColor Red
        $real | ForEach-Object { Write-Host "  $_" }
        throw "gofmt 检查未通过"
    }
    Write-Host 'gofmt: clean（已忽略 CRLF 行尾差异）'
}

function Build-All {
    # 顺序与 CI 发布口径一致
    $map = [ordered]@{ 'wb2api' = 'server'; 'signin_bin' = 'signin'; 'login' = 'login'; 'credit' = 'credit' }
    New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
    $prevCgo = $env:CGO_ENABLED
    $env:CGO_ENABLED = '0'
    try {
        foreach ($name in $map.Keys) {
            Write-Host "==> $name  (./cmd/$($map[$name]))"
            $out = Join-Path $BinDir "$name.exe"
            Invoke-Go @('build', '-trimpath', '-ldflags', '-s -w', '-o', $out, "./cmd/$($map[$name])")
        }
    }
    finally {
        $env:CGO_ENABLED = $prevCgo
    }
    Write-Host "built $(Get-AppVersion) -> $BinDir\"
}

Write-Host ("go: {0}  ({1})" -f $script:go, (& $script:go version)) -ForegroundColor DarkGray

switch ($Task) {
    'all' { Build-All }
    'build' { Build-All }

    'version' { Write-Host (Get-AppVersion) }

    'clean' {
        if (Test-Path $BinDir) { Remove-Item -Recurse -Force $BinDir }
        Write-Host "cleaned $BinDir"
    }

    'test' { Invoke-Go @('test', './...') }

    'vet' { Invoke-Go @('vet', './...') }

    'fmt-check' { Test-GoFmt }

    'race' { Invoke-Go @('test', '-race', './internal/pool/...', './internal/scheduler/...') }

    'check' {
        # 与 Makefile check 等价：fmt-check → vet → test
        Test-GoFmt
        Invoke-Go @('vet', './...')
        Invoke-Go @('test', './...')
        Write-Host 'check: all green' -ForegroundColor Green
    }
}
