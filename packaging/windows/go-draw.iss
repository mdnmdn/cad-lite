; go-draw per-user installer. Build:
;   iscc /DMyAppVersion=1.2.3 /DMyArch=amd64 packaging\windows\go-draw.iss
; Expects the exe at _tmp\dist\windows\go-draw-windows-<arch>\go-draw.exe (packaging/windows/bundle.sh).
#ifndef MyAppVersion
  #define MyAppVersion "0.0.0"
#endif
#ifndef MyArch
  #define MyArch "amd64"
#endif

[Setup]
AppId={{B3D7E0A4-5C1F-4E92-8A6B-2F9D41C7E5A8}
AppName=go-draw
AppVersion={#MyAppVersion}
AppPublisher=Synesthesia
DefaultDirName={localappdata}\Programs\go-draw
DefaultGroupName=go-draw
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
UninstallDisplayIcon={app}\go-draw.exe
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
SourceDir=..\..
OutputDir=_tmp\dist\windows
OutputBaseFilename=go-draw-{#MyAppVersion}-windows-{#MyArch}-setup

[Tasks]
Name: "desktopicon"; Description: "Create a &desktop icon"; GroupDescription: "Additional icons:"
Name: "dxfassoc"; Description: "Open .dxf files with go-draw"; GroupDescription: "File associations:"; Flags: unchecked

[Files]
Source: "_tmp\dist\windows\go-draw-windows-{#MyArch}\go-draw.exe"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{autoprograms}\go-draw"; Filename: "{app}\go-draw.exe"
Name: "{autodesktop}\go-draw"; Filename: "{app}\go-draw.exe"; Tasks: desktopicon

[Registry]
Root: HKCU; Subkey: "Software\Classes\.dxf\OpenWithProgids"; ValueType: string; ValueName: "go-draw.dxf"; ValueData: ""; Flags: uninsdeletevalue; Tasks: dxfassoc
Root: HKCU; Subkey: "Software\Classes\go-draw.dxf"; ValueType: string; ValueName: ""; ValueData: "DXF drawing"; Flags: uninsdeletekey; Tasks: dxfassoc
Root: HKCU; Subkey: "Software\Classes\go-draw.dxf\DefaultIcon"; ValueType: string; ValueName: ""; ValueData: "{app}\go-draw.exe,0"; Tasks: dxfassoc
Root: HKCU; Subkey: "Software\Classes\go-draw.dxf\shell\open\command"; ValueType: string; ValueName: ""; ValueData: """{app}\go-draw.exe"" -open ""%1"""; Tasks: dxfassoc

[Run]
Filename: "{app}\go-draw.exe"; Description: "Launch go-draw"; Flags: nowait postinstall skipifsilent
