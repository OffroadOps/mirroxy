param([string]$Destination = 'E:\Mirroxy', [string]$GoExe = '')
$ErrorActionPreference = 'Stop'
Set-Location -LiteralPath $PSScriptRoot
if (-not $GoExe) {
    $taskKnownGo = 'D:\dev\go\workspace\pkg\mod\golang.org\toolchain@v0.0.1-go1.26.5.windows-amd64\bin\go.exe'
    if (Test-Path -LiteralPath $taskKnownGo) { $GoExe = $taskKnownGo } else { $GoExe = (Get-Command go.exe).Source }
}
$env:GOROOT = Split-Path (Split-Path $GoExe)
$env:GOTOOLCHAIN = 'local'
$env:CGO_ENABLED = '0'
if (-not (Test-Path -LiteralPath 'resource_windows_amd64.syso')) { throw 'Run python packaging/build-windows-resource.py to create the Windows application icon first' }
& $GoExe build -trimpath -ldflags '-s -w -H=windowsgui' -o (Join-Path $Destination 'Mirroxy.exe') .
if ($LASTEXITCODE -ne 0) { throw 'Build failed' }
& $GoExe build -trimpath -ldflags '-s -w -H=windowsgui' -o (Join-Path $Destination 'upgrade.exe') ./cmd/upgrade
if ($LASTEXITCODE -ne 0) { throw 'Upgrade helper build failed' }
Write-Host "Built: $(Join-Path $Destination 'Mirroxy.exe')"
Write-Host "Built: $(Join-Path $Destination 'upgrade.exe')"
