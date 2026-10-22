$ErrorActionPreference = 'Stop'
# Tool locations are taken from the environment so the script works on any
# machine. Set ANDROID_JAVA_HOME / ANDROID_SDK / GRADLE_USER_HOME, or leave
# them empty to use whatever is already configured.
if ($env:ANDROID_JAVA_HOME) { $env:JAVA_HOME = $env:ANDROID_JAVA_HOME }
if ($env:ANDROID_SDK) { $env:ANDROID_HOME = $env:ANDROID_SDK }
if ($env:ANDROID_GRADLE_HOME) { $env:GRADLE_USER_HOME = $env:ANDROID_GRADLE_HOME }
Push-Location (Join-Path $PSScriptRoot '..')
try {
    if (-not (Test-Path 'app\libs\libXray.aar')) { & .\scripts\fetch-core.ps1 }
    & .\gradlew.bat :app:assembleDebug :app:assembleDebugAndroidTest :app:lintDebug --console=plain
    if ($LASTEXITCODE -ne 0) { throw 'Android build failed' }
} finally { Pop-Location }
