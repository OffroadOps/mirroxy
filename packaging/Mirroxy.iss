#preproc ispp

#ifndef MyAppVersion
  #error MyAppVersion must be supplied by the release builder
#endif
#ifndef SourceDir
  #error SourceDir must be supplied by the release builder
#endif
#ifndef OutputDir
  #error OutputDir must be supplied by the release builder
#endif
#ifndef OutputName
  #define OutputName "Mirroxy-windows-amd64"
#endif
#ifndef AppIdSuffix
  #define AppIdSuffix ""
#endif

#define MyAppName "Mirroxy"
#define MyAppPublisher "OffroadOps"
#define MyAppURL "https://ny-1s.enzonix.com/bucket-1776-2431/scrcpyproxy/index.html"
#define MyAppId "{{7D2BCE81-035C-4E1C-AAC1-58A51E667277}" + AppIdSuffix
#define MyAppRegistryId "{7D2BCE81-035C-4E1C-AAC1-58A51E667277}" + AppIdSuffix

[Setup]
AppId={#MyAppId}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
#ifdef OnlineBootstrap
AppVerName=Mirroxy Online Installer
#else
AppVerName={#MyAppName} {#MyAppVersion}
#endif
AppPublisher={#MyAppPublisher}
AppPublisherURL={#MyAppURL}
AppSupportURL={#MyAppURL}
AppUpdatesURL={#MyAppURL}
VersionInfoVersion={#MyAppVersion}
VersionInfoCompany={#MyAppPublisher}
VersionInfoDescription=Mirroxy Installer
VersionInfoProductName={#MyAppName}
VersionInfoProductVersion={#MyAppVersion}
DefaultDirName={localappdata}\Programs\Mirroxy
DefaultGroupName={#MyAppName}
DisableProgramGroupPage=yes
UsePreviousAppDir=yes
UsePreviousGroup=yes
UsePreviousLanguage=yes
PrivilegesRequired=lowest
SetupArchitecture=x64
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
MinVersion=10.0
OutputDir={#OutputDir}
OutputBaseFilename={#OutputName}
SetupIconFile=..\web\app.ico
UninstallDisplayIcon={app}\Mirroxy.exe
WizardStyle=modern dynamic
WizardSizePercent=110
Compression=lzma2/max
SolidCompression=yes
CloseApplications=yes
CloseApplicationsFilter=Mirroxy.exe,upgrade.exe,Mirroxy_core_A.exe,Mirroxy_core_B.exe
RestartApplications=no
SetupLogging=yes
ShowLanguageDialog=auto
LanguageDetectionMethod=uilanguage
ChangesAssociations=no
ChangesEnvironment=no
AllowNetworkDrive=no
AllowUNCPath=no

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"
Name: "chinesesimplified"; MessagesFile: ".\inno\ChineseSimplified.isl"

[CustomMessages]
english.LaunchMirroxy=Launch Mirroxy
chinesesimplified.LaunchMirroxy=运行 Mirroxy
english.SameVersion=Mirroxy %1 is already installed. No files or user data were changed.
chinesesimplified.SameVersion=Mirroxy %1 已安装，本次未修改任何程序文件或用户数据。
english.NewerVersion=A newer Mirroxy version (%1) is already installed. This installer will not downgrade it.
chinesesimplified.NewerVersion=已安装更高版本的 Mirroxy（%1），此安装包不会执行降级。
english.PanelPortTitle=Local dashboard port
chinesesimplified.PanelPortTitle=本机面板端口
english.PanelPortDescription=Choose an available TCP port for Mirroxy.
chinesesimplified.PanelPortDescription=为 Mirroxy 选择可用的 TCP 端口。
english.PanelPortHelp=The default is 9527. If it is occupied, close the program using it and retry, or enter another port. Upgrades keep the existing dashboard port.
chinesesimplified.PanelPortHelp=默认端口为 9527。若端口被占用，请关闭占用程序后重试，或输入其他端口。升级时默认保留已有面板端口。
english.PanelPortLabel=Dashboard port (1024-65000):
chinesesimplified.PanelPortLabel=面板端口（1024–65000）：
english.PanelPortInvalid=Enter a port between 1024 and 65000.
chinesesimplified.PanelPortInvalid=请输入 1024–65000 之间的端口。
english.PanelPortFailed=Cannot use dashboard port %1. Close the program using this port and retry, or choose another port. No application files have been replaced.%n%n%2
chinesesimplified.PanelPortFailed=无法使用面板端口 %1。请关闭占用该端口的程序后重试，或更换面板端口。尚未替换程序文件。%n%n%2
english.PanelSettingsFailed=Cannot read or save the dashboard port. Your existing data has been kept.%n%n%1
chinesesimplified.PanelSettingsFailed=无法读取或保存面板端口，已保留原有数据。%n%n%1

[Messages]
english.ConfirmUninstall=Do you want to remove Mirroxy? Your data folder, nodes, credentials, and traffic totals will be kept.
chinesesimplified.ConfirmUninstall=确定卸载 Mirroxy？data 数据目录、节点、凭据和流量记录将继续保留。

[Dirs]
Name: "{app}\data"; Flags: uninsneveruninstall

[Files]
#ifdef OnlineBootstrap
Source: "{#SourceDir}\Mirroxy-bootstrap.exe"; Flags: dontcopy
Source: "{code:GetPayloadDir}\*"; DestDir: "{app}"; Excludes: "data\*,data,bin\runtime\*"; ExternalSize: 1; Flags: external ignoreversion recursesubdirs createallsubdirs
Source: "{code:GetPayloadDir}\bin\runtime\*"; DestDir: "{app}\bin\runtime"; ExternalSize: 1; Flags: external ignoreversion onlyifdoesntexist
#else
Source: "{#SourceDir}\Mirroxy.exe"; DestName: "Mirroxy-setup-helper.exe"; Flags: dontcopy
Source: "{#SourceDir}\*"; DestDir: "{app}"; Excludes: "data\*;data"; Flags: ignoreversion recursesubdirs createallsubdirs
#endif

[Icons]
#if AppIdSuffix == ""
Name: "{group}\Mirroxy"; Filename: "{app}\Mirroxy.exe"; WorkingDir: "{app}"
Name: "{group}\{cm:UninstallProgram,Mirroxy}"; Filename: "{uninstallexe}"
Name: "{autodesktop}\Mirroxy"; Filename: "{app}\Mirroxy.exe"; WorkingDir: "{app}"
#endif

[Run]
Filename: "{app}\Mirroxy.exe"; WorkingDir: "{app}"; Description: "{cm:LaunchMirroxy}"; Flags: nowait postinstall skipifsilent; Check: PanelPortConfigured

[Code]
var
  PanelPortPage: TInputQueryWizardPage;
  PanelDirectory, PanelReadError: String;
  ExistingPanelPort: Integer;
  PanelSettingsSaved: Boolean;
#ifdef OnlineBootstrap
  ProxyPage: TInputQueryWizardPage;
  DownloadPage: TOutputMarqueeProgressWizardPage;
  PayloadDir, PayloadVersion: String;

function GetPayloadDir(Param: String): String;
begin
  Result := PayloadDir;
end;

function FetchLatest(var Detail: String): Boolean;
var
  ExitCode, I: Integer;
  Output: TExecOutput;
  Proxy, Parameters, Cache: String;
begin
  Result := PayloadDir <> '';
  if Result then Exit;
  Proxy := Trim(ExpandConstant('{param:DOWNLOADPROXY|' + ProxyPage.Values[0] + '}'));
  if (Pos('"', Proxy) > 0) or (Pos(#13, Proxy) > 0) or (Pos(#10, Proxy) > 0) then
  begin
    Detail := 'Invalid proxy address.';
    Exit;
  end;
  Cache := ExpandConstant('{tmp}\online-cache');
  Parameters := '--work "' + Cache + '"';
  if Proxy <> '' then Parameters := Parameters + ' --proxy "' + Proxy + '"';
  DownloadPage.SetText('Downloading and verifying the latest Mirroxy release...', 'This may take a few minutes. Existing files remain unchanged until verification succeeds.');
  DownloadPage.Show;
  try
    DownloadPage.Animate;
    Result := ExecAndCaptureOutput(ExpandConstant('{tmp}\Mirroxy-bootstrap.exe'), Parameters,
      ExpandConstant('{tmp}'), SW_HIDE, ewWaitUntilTerminated, ExitCode, Output);
    Result := Result and (ExitCode = 0) and not Output.Error;
  finally
    DownloadPage.Hide;
  end;
  if not Result then
  begin
    Detail := 'Failed to download or verify the release. No application files were changed.' + #13#10;
    for I := 0 to GetArrayLength(Output.StdErr) - 1 do Detail := Detail + Output.StdErr[I] + #13#10;
    Exit;
  end;
  PayloadDir := GetIniString('release', 'payload', '', Cache + '\release.ini');
  PayloadVersion := GetIniString('release', 'version', '', Cache + '\release.ini');
  Result := (PayloadVersion <> '') and FileExists(PayloadDir + '\Mirroxy.exe') and
    (Pos(Lowercase(Cache + '\payload-stage-'), Lowercase(PayloadDir)) = 1);
  if not Result then
  begin
    PayloadDir := '';
    Detail := 'Invalid verified release staging directory.';
  end;
end;
#endif

function PanelHelperPath: String;
begin
#ifdef OnlineBootstrap
  Result := PayloadDir + '\Mirroxy.exe';
#else
  Result := ExpandConstant('{tmp}\Mirroxy-setup-helper.exe');
#endif
end;

function RunPanelPortHelper(const Operation: String; var Detail: String): Boolean;
var
  ExitCode, I: Integer;
  Output: TExecOutput;
begin
  Detail := '';
  Result := ExecAndCaptureOutput(PanelHelperPath,
    '-setup-panel-port ' + Operation + ' -dir "' + ExpandConstant('{app}') +
    '" -panel ' + IntToStr(StrToIntDef(Trim(PanelPortPage.Values[0]), 0)),
    ExpandConstant('{tmp}'), SW_HIDE, ewWaitUntilTerminated, ExitCode, Output);
  if Result then
  begin
    Result := (ExitCode = 0) and not Output.Error;
    if Result then
    begin
      if GetArrayLength(Output.StdOut) > 0 then
        Detail := Trim(Output.StdOut[0]);
    end
    else
      for I := 0 to GetArrayLength(Output.StdErr) - 1 do
        Detail := Detail + Output.StdErr[I] + #13#10;
  end
  else
    Detail := SysErrorMessage(ExitCode);
  if not Result then
  begin
    if Detail = '' then
      Detail := 'Panel port helper failed (' + IntToStr(ExitCode) + ').';
    Log(Detail);
  end;
end;

procedure LoadPanelPort;
var
  Detail: String;
begin
  if (PanelDirectory = ExpandConstant('{app}')) and (PanelReadError = '') then
    Exit;
  PanelDirectory := ExpandConstant('{app}');
  PanelReadError := '';
  if not RunPanelPortHelper('read', Detail) then
    PanelReadError := FmtMessage(ExpandConstant('{cm:PanelSettingsFailed}'), [Detail])
  else
  begin
    ExistingPanelPort := StrToIntDef(Detail, 9527);
    PanelPortPage.Values[0] := ExpandConstant('{param:PANELPORT|' + IntToStr(ExistingPanelPort) + '}');
  end;
end;

procedure InitializeWizard;
begin
#ifdef OnlineBootstrap
  ExtractTemporaryFile('Mirroxy-bootstrap.exe');
  ProxyPage := CreateInputQueryPage(wpSelectDir, 'Download latest Mirroxy / 下载最新版',
    'This installer always downloads the latest published version.',
    'Optional HTTP/HTTPS proxy. Leave empty for direct access; this does not change your system proxy. / 可填写代理，例如 http://127.0.0.1:10808，留空直连，不修改系统代理。');
  ProxyPage.Add('Download proxy / 下载代理：', False);
  DownloadPage := CreateOutputMarqueeProgressPage('Downloading Mirroxy / 下载程序', 'Verify the latest release before installing.');
  PanelPortPage := CreateInputQueryPage(ProxyPage.ID,
#else
  ExtractTemporaryFile('Mirroxy-setup-helper.exe');
  PanelPortPage := CreateInputQueryPage(wpSelectDir,
#endif
    ExpandConstant('{cm:PanelPortTitle}'), ExpandConstant('{cm:PanelPortDescription}'),
    ExpandConstant('{cm:PanelPortHelp}'));
  PanelPortPage.Add(ExpandConstant('{cm:PanelPortLabel}'), False);
  PanelPortPage.Values[0] := '9527';
end;

procedure CurPageChanged(CurPageID: Integer);
begin
  if CurPageID = PanelPortPage.ID then
    LoadPanelPort;
end;

function ValidPanelPort(var Detail: String): Boolean;
var
  Port: Integer;
begin
  Detail := PanelReadError;
  if Detail <> '' then
  begin
    Result := False;
    Exit;
  end;
  Port := StrToIntDef(Trim(PanelPortPage.Values[0]), 0);
  Result := (Port >= 1024) and (Port <= 65000);
  if not Result then
    Detail := ExpandConstant('{cm:PanelPortInvalid}')
  else
    PanelPortPage.Values[0] := IntToStr(Port);
end;

function CheckPanelPort(const Operation: String; var Detail: String): Boolean;
var
  HelperDetail: String;
begin
  Result := ValidPanelPort(Detail);
  if Result then
  begin
    Result := RunPanelPortHelper(Operation, HelperDetail);
    if not Result then
      Detail := FmtMessage(ExpandConstant('{cm:PanelPortFailed}'), [PanelPortPage.Values[0], HelperDetail]);
  end;
end;

function NextButtonClick(CurPageID: Integer): Boolean;
var
  Detail: String;
begin
  Result := True;
#ifdef OnlineBootstrap
  if CurPageID = ProxyPage.ID then
  begin
    Result := FetchLatest(Detail);
    if not Result then SuppressibleMsgBox(Detail, mbError, MB_OK, IDOK);
    Exit;
  end;
#endif
  if CurPageID <> PanelPortPage.ID then
    Exit;
  // Only the executable in this exact installation can defer the check until
  // its graceful exit. Unrelated listeners are reported on the port page.
  Result := CheckPanelPort('check-upgrade', Detail);
  if not Result then
    SuppressibleMsgBox(Detail, mbError, MB_OK, IDOK);
end;

function PanelPortConfigured: Boolean;
begin
  Result := PanelSettingsSaved;
end;

procedure CurStepChanged(CurStep: TSetupStep);
var
  Detail: String;
begin
  if CurStep = ssPostInstall then
  begin
    PanelSettingsSaved := RunPanelPortHelper('write', Detail);
    if not PanelSettingsSaved then
      RaiseException(FmtMessage(ExpandConstant('{cm:PanelSettingsFailed}'), [Detail]));
#ifdef OnlineBootstrap
    if not RegWriteStringValue(HKCU64, 'Software\Microsoft\Windows\CurrentVersion\Uninstall\{#MyAppRegistryId}_is1', 'DisplayVersion', PayloadVersion) then
      RaiseException('Cannot record the installed application version.');
#endif
  end;
end;

function ReadInstalledVersion(var InstalledVersion: String): Boolean;
begin
  Result := RegQueryStringValue(HKCU64,
    'Software\Microsoft\Windows\CurrentVersion\Uninstall\{#MyAppRegistryId}_is1',
    'DisplayVersion', InstalledVersion);
end;

function ParseVersion(const Value: String; var Packed: Int64): Boolean;
var
  Normalized: String;
  I, Dots: Integer;
begin
  Normalized := Value;
  Dots := 0;
  for I := 1 to Length(Normalized) do
    if Normalized[I] = '.' then
      Dots := Dots + 1;
  while Dots < 3 do
  begin
    Normalized := Normalized + '.0';
    Dots := Dots + 1;
  end;
  Result := StrToVersion(Normalized, Packed);
end;

function AllowPackageVersion(const PackageVersion: String; var Detail: String): Boolean;
var
  InstalledVersion: String;
  InstalledPacked, PackagePacked: Int64;
  Comparison: Integer;
begin
  Result := True;
  if not ReadInstalledVersion(InstalledVersion) then
    Exit;
  if not ParseVersion(InstalledVersion, InstalledPacked) then
    Exit;
  if not ParseVersion(PackageVersion, PackagePacked) then
    Exit;
  Comparison := ComparePackedVersion(InstalledPacked, PackagePacked);
  if Comparison = 0 then
  begin
    Log('Same Mirroxy version is already installed; exiting without changes.');
    Detail := FmtMessage(ExpandConstant('{cm:SameVersion}'), [InstalledVersion]);
    Result := False;
  end
  else if Comparison > 0 then
  begin
    Log('A newer Mirroxy version is already installed; refusing downgrade.');
    Detail := FmtMessage(ExpandConstant('{cm:NewerVersion}'), [InstalledVersion]);
    Result := False;
  end;
end;

function InitializeSetup(): Boolean;
var
  Detail: String;
begin
#ifdef OnlineBootstrap
  Result := True;
#else
  Result := AllowPackageVersion('{#MyAppVersion}', Detail);
  if not Result and not WizardSilent then MsgBox(Detail, mbInformation, MB_OK);
#endif
end;

function PrepareToInstall(var NeedsRestart: Boolean): String;
var
  Detail: String;
begin
  Result := '';
#ifdef OnlineBootstrap
  if not FetchLatest(Result) then Exit;
  if not AllowPackageVersion(PayloadVersion, Result) then Exit;
#endif
  LoadPanelPort;
  if not ValidPanelPort(Result) then
    Exit;
  if FileExists(ExpandConstant('{app}\Mirroxy.exe')) then
  begin
    Log('Requesting a graceful Mirroxy shutdown before upgrade.');
    if not RunPanelPortHelper('stop', Detail) then
    begin
      Result := FmtMessage(ExpandConstant('{cm:PanelSettingsFailed}'), [Detail]);
      Exit;
    end;
  end;
  // Recheck immediately before replacing files, also in silent installations.
  CheckPanelPort('check', Result);
end;
