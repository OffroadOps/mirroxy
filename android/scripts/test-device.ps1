param([Parameter(Mandatory=$true)][string]$Serial, [switch]$Vpn)
$ErrorActionPreference = 'Stop'
$adb = 'D:\DevTools\Android\Sdk\platform-tools\adb.exe'
$root = Split-Path $PSScriptRoot
& $adb -s $Serial install -r (Join-Path $root 'app\build\outputs\apk\debug\app-debug.apk')
if ($LASTEXITCODE -ne 0) { throw 'App installation failed' }
& $adb -s $Serial install -r (Join-Path $root 'app\build\outputs\apk\androidTest\debug\app-debug-androidTest.apk')
if ($LASTEXITCODE -ne 0) { throw 'Test installation failed' }
if ($Vpn) {
    Write-Host 'Approve the Android VPN dialog manually. Only the test app is routed.'
    $result = & $adb -s $Serial shell am instrument -w -e class com.mirroxy.android.VpnIntegrationTest -e vpn true com.mirroxy.android.test/androidx.test.runner.AndroidJUnitRunner
} else {
    $result = & $adb -s $Serial shell am instrument -w -e class com.mirroxy.android.CoreIntegrationTest com.mirroxy.android.test/androidx.test.runner.AndroidJUnitRunner
}
$result | Write-Output
if (($result -join "`n") -notmatch 'OK \(\d+ tests?\)') { throw 'Instrumentation did not report a passing run; inspect the output above' }
