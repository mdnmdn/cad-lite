; Cad-lite per-user installer. Build:
;   iscc /DMyAppVersion=1.2.3 /DMyArch=amd64 packaging\windows\cad-lite.iss
; Expects the exe at _tmp\dist\windows\cad-lite-windows-<arch>\cad-lite.exe (packaging/windows/bundle.sh).
#ifndef MyAppVersion
  #define MyAppVersion "0.0.0"
#endif
#ifndef MyArch
  #define MyArch "amd64"
#endif

[Setup]
AppId={{B3D7E0A4-5C1F-4E92-8A6B-2F9D41C7E5A8}
AppName=Cad-lite
AppVersion={#MyAppVersion}
AppPublisher=Synesthesia
DefaultDirName={localappdata}\Programs\Cad-lite
DefaultGroupName=Cad-lite
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=commandline
CloseApplications=yes
#if MyArch == "arm64"
ArchitecturesAllowed=arm64
ArchitecturesInstallIn64BitMode=arm64
#else
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
#endif
UninstallDisplayIcon={app}\cad-lite.exe
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
SourceDir=..\..
OutputDir=_tmp\dist\windows
OutputBaseFilename=cad-lite-{#MyAppVersion}-windows-{#MyArch}-setup

[Tasks]
Name: "desktopicon"; Description: "Create a &desktop icon"; GroupDescription: "Additional icons:"
Name: "dxfassoc"; Description: "Open .dxf files with Cad-lite"; GroupDescription: "File associations:"; Flags: unchecked

[Files]
Source: "_tmp\dist\windows\cad-lite-windows-{#MyArch}\cad-lite.exe"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{autoprograms}\Cad-lite"; Filename: "{app}\cad-lite.exe"
Name: "{autodesktop}\Cad-lite"; Filename: "{app}\cad-lite.exe"; Tasks: desktopicon

[Registry]
Root: HKCU; Subkey: "Software\Classes\.dxf\OpenWithProgids"; ValueType: string; ValueName: "cad-lite.dxf"; ValueData: ""; Flags: uninsdeletevalue; Tasks: dxfassoc
Root: HKCU; Subkey: "Software\Classes\cad-lite.dxf"; ValueType: string; ValueName: ""; ValueData: "DXF drawing"; Flags: uninsdeletekey; Tasks: dxfassoc
Root: HKCU; Subkey: "Software\Classes\cad-lite.dxf\DefaultIcon"; ValueType: string; ValueName: ""; ValueData: "{app}\cad-lite.exe,0"; Tasks: dxfassoc
Root: HKCU; Subkey: "Software\Classes\cad-lite.dxf\shell\open\command"; ValueType: string; ValueName: ""; ValueData: """{app}\cad-lite.exe"" -open ""%1"""; Tasks: dxfassoc

[Run]
Filename: "{app}\cad-lite.exe"; Description: "Launch Cad-lite"; Flags: nowait postinstall skipifsilent
