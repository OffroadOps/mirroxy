$ErrorActionPreference = 'Stop'
$env:JAVA_HOME = 'D:\DevTools\jdk-21'
$env:ANDROID_HOME = 'D:\DevTools\Android\Sdk'
$env:GRADLE_USER_HOME = 'D:\DevTools\gradle-home'
Push-Location (Join-Path $PSScriptRoot '..')
try {
    if (-not (Test-Path 'app\libs\libXray.aar')) { & .\scripts\fetch-core.ps1 }
    & .\gradlew.bat :app:assembleDebug :app:assembleDebugAndroidTest :app:lintDebug --console=plain
    if ($LASTEXITCODE -ne 0) { throw 'Android build failed' }
} finally { Pop-Location }
