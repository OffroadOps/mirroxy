param(
    [string]$GoExe = 'go',
    [string]$Iscc = ''
)
$ErrorActionPreference = 'Stop'
if (-not $Iscc) {
    $taskIsccCandidates = @()
    if ($env:MIRROXY_ISCC) { $taskIsccCandidates += $env:MIRROXY_ISCC }
    foreach ($taskBase in @($env:ProgramFiles, $env:ProgramW6432, ${env:ProgramFiles(x86)}, $env:LOCALAPPDATA)) {
        if ($taskBase) { $taskIsccCandidates += (Join-Path $taskBase 'Inno Setup 7\ISCC.exe') }
    }
    $Iscc = $taskIsccCandidates | Where-Object { Test-Path -LiteralPath $_ } | Select-Object -First 1
    if (-not $Iscc) { throw 'Inno Setup 7 compiler not found. Install it, set MIRROXY_ISCC, or pass -Iscc <path to ISCC.exe>.' }
}
$taskRoot = Split-Path $PSScriptRoot
$taskQa = Join-Path $taskRoot ('work\installer-port-test-' + [guid]::NewGuid().ToString('N'))
$taskStage = Join-Path $taskQa 'stage'
$taskInstall = Join-Path $taskQa 'installed'
$taskAppIdSuffix = '-pt-' + [guid]::NewGuid().ToString('N').Substring(0, 12)
New-Item -ItemType Directory -Path $taskStage, $taskInstall | Out-Null
$env:GOTOOLCHAIN = 'local'
$env:GOFLAGS = '-buildvcs=false'
$env:CGO_ENABLED = '0'
Push-Location -LiteralPath $taskRoot
try {
    & $GoExe build -trimpath -ldflags '-s -w -H=windowsgui' -o (Join-Path $taskStage 'Mirroxy.exe') .
    if ($LASTEXITCODE -ne 0) { throw 'Test application build failed' }
    foreach ($taskVersion in @('0.6.1', '0.6.2')) {
        & $Iscc --quiet-progress "--define=MyAppVersion=$taskVersion" "--define=AppIdSuffix=$taskAppIdSuffix" "--define=SourceDir=$taskStage" "--define=OutputDir=$taskQa" "--define=OutputName=setup-$taskVersion" (Join-Path $PSScriptRoot 'Mirroxy.iss')
        if ($LASTEXITCODE -ne 0) { throw 'Test installer compilation failed' }
    }

    function Invoke-TestSetup([string]$Version, [string]$Label, [string[]]$ExtraArguments) {
        $taskArguments = @('/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', "/DIR=`"$taskInstall`"", "/LOG=`"$(Join-Path $taskQa ($Label + '.log'))`"") + $ExtraArguments
        $taskSetup = Start-Process -FilePath (Join-Path $taskQa "setup-$Version.exe") -ArgumentList $taskArguments -WindowStyle Hidden -PassThru
        if (-not $taskSetup.WaitForExit(60000)) {
            Stop-Process -Id $taskSetup.Id -Force
            throw "Test installer timed out: $Label"
        }
        $taskSetup.Refresh()
        return $taskSetup.ExitCode
    }

    function Read-TestPanelPort {
        $taskStdout = Join-Path $taskQa 'panel-port.stdout'
        $taskStderr = Join-Path $taskQa 'panel-port.stderr'
        $taskProbe = Start-Process -FilePath (Join-Path $taskInstall 'Mirroxy.exe') -ArgumentList @('-setup-panel-port', 'read', '-dir', ('"' + $taskInstall + '"')) -WindowStyle Hidden -RedirectStandardOutput $taskStdout -RedirectStandardError $taskStderr -PassThru -Wait
        if ($taskProbe.ExitCode -ne 0) { throw (Get-Content -LiteralPath $taskStderr -Raw) }
        return [int](Get-Content -LiteralPath $taskStdout -Raw).Trim()
    }

    if (Get-NetTCPConnection -State Listen -LocalPort 9527 -ErrorAction SilentlyContinue) {
        $taskExit = Invoke-TestSetup '0.6.1' 'default-9527-occupied' @()
        if ($taskExit -eq 0 -or (Test-Path -LiteralPath (Join-Path $taskInstall 'Mirroxy.exe'))) { throw 'Occupied default 9527 was not rejected' }
        Write-Host 'PASS: actual occupied default port 9527 is rejected'
    }
    $taskExit = Invoke-TestSetup '0.6.1' 'invalid-port' @('/PANELPORT=0')
    if ($taskExit -eq 0 -or (Test-Path -LiteralPath (Join-Path $taskInstall 'Mirroxy.exe'))) { throw 'Invalid port was not rejected' }
    Write-Host 'PASS: invalid panel port is rejected'

    # Keep the occupied socket open until Setup exits. No real programs are stopped.
    $taskOccupied = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, 0)
    $taskOccupied.ExclusiveAddressUse = $true
    $taskOccupied.Start()
    try {
        $taskPort = $taskOccupied.LocalEndpoint.Port
        if ($taskPort -gt 65000) { throw 'Test socket is outside allowed port range; rerun test' }
        $taskExit = Invoke-TestSetup '0.6.1' 'occupied' @("/PANELPORT=$taskPort")
        if ($taskExit -eq 0 -or (Test-Path -LiteralPath (Join-Path $taskInstall 'Mirroxy.exe')) -or (Test-Path -LiteralPath (Join-Path $taskInstall 'data'))) {
            throw 'Occupied port did not block installation before files/data were written'
        }
        Write-Host 'PASS: occupied port blocks installation without writing application/data'
    } finally { $taskOccupied.Stop() }

    $taskExit = Invoke-TestSetup '0.6.1' 'free-port' @("/PANELPORT=$taskPort")
    if ($taskExit -ne 0 -or (Read-TestPanelPort) -ne $taskPort) { throw 'Free-port installation or persistence failed' }
    Write-Host 'PASS: selecting another port installs successfully and persists the port'

    $taskDbHash = (Get-FileHash -LiteralPath (Join-Path $taskInstall 'data\Mirroxy.db')).Hash
    $taskKeyHash = (Get-FileHash -LiteralPath (Join-Path $taskInstall 'data\Mirroxy.key')).Hash
    $taskAppHash = (Get-FileHash -LiteralPath (Join-Path $taskInstall 'Mirroxy.exe')).Hash
    $taskUpgradeOccupied = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, $taskPort)
    $taskUpgradeOccupied.ExclusiveAddressUse = $true
    $taskUpgradeOccupied.Start()
    try {
        $taskExit = Invoke-TestSetup '0.6.2' 'upgrade-occupied' @()
        if ($taskExit -eq 0 -or (Get-FileHash -LiteralPath (Join-Path $taskInstall 'Mirroxy.exe')).Hash -ne $taskAppHash -or
            (Get-FileHash -LiteralPath (Join-Path $taskInstall 'data\Mirroxy.db')).Hash -ne $taskDbHash) { throw 'Upgrade did not recheck occupation before replacing files' }
        Write-Host 'PASS: upgrade rejects occupation before replacing application/data'
    } finally { $taskUpgradeOccupied.Stop() }
    $taskExit = Invoke-TestSetup '0.6.2' 'upgrade' @()
    if ($taskExit -ne 0 -or (Read-TestPanelPort) -ne $taskPort) { throw 'Upgrade did not retain custom panel port' }
    if ((Get-FileHash -LiteralPath (Join-Path $taskInstall 'data\Mirroxy.db')).Hash -ne $taskDbHash -or
        (Get-FileHash -LiteralPath (Join-Path $taskInstall 'data\Mirroxy.key')).Hash -ne $taskKeyHash) {
        throw 'Upgrade changed existing database or key'
    }
    Write-Host 'PASS: upgrade retains custom port, database, and encryption key'

    $taskExit = Invoke-TestSetup '0.6.2' 'same-version' @()
    if ($taskExit -eq 0 -or (Get-FileHash -LiteralPath (Join-Path $taskInstall 'data\Mirroxy.db')).Hash -ne $taskDbHash) { throw 'Same-version installer changed the installation' }
    Write-Host 'PASS: same-version installation exits without changing data'
    Write-Host "Test evidence: $taskQa"
} finally {
    $taskUninstall = Join-Path $taskInstall 'unins000.exe'
    if (Test-Path -LiteralPath $taskUninstall) {
        $taskCleanup = Start-Process -FilePath $taskUninstall -ArgumentList @('/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART') -WindowStyle Hidden -PassThru -Wait
        if ($taskCleanup.ExitCode -ne 0) { Write-Warning 'Test uninstaller could not remove isolated registration' }
    }
    Pop-Location
}
