$ErrorActionPreference = 'Stop'
$release = 'https://github.com/XTLS/libXray/releases/download/v26.10.10/libxray-android.zip'
$archive = 'D:\DevTools\downloads\libxray-android-v26.10.10.zip'
$expected = 'AAD01C82EB82E98702838F7B4AC3F33DBA1AE5FF4102538E5CA7F05BBAB57437'
New-Item -ItemType Directory -Force (Split-Path $archive) | Out-Null
if (-not (Test-Path -LiteralPath $archive)) {
    try { Invoke-WebRequest $release -OutFile $archive }
    catch { Invoke-WebRequest $release -OutFile $archive -Proxy 'http://127.0.0.1:10808' }
}
if ((Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash -ne $expected) { throw 'Official libXray archive SHA256 mismatch' }
Expand-Archive -LiteralPath $archive -DestinationPath 'D:\DevTools\libxray-26.10.10' -Force
$destination = Join-Path $PSScriptRoot '..\app\libs'
New-Item -ItemType Directory -Force $destination | Out-Null
Copy-Item -LiteralPath 'D:\DevTools\libxray-26.10.10\libxray-android\libXray.aar' -Destination $destination
