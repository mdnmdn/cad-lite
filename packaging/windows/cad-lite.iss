; Cad-lite installer: per-user by default, per-machine with /ALLUSERS. Build:
;   iscc /DMyAppVersion=1.2.3 /DMyArch=amd64 packaging\windows\cad-lite.iss
; Expects the exe at _tmp\dist\windows\cad-lite-windows-<arch>\cad-lite.exe (packaging/windows/bundle.sh).
; Shell integration (ProgID, .dxf/.dwg associations, "Open in Cad-lite") is written under HKA, which is
; HKCU in a per-user install and HKLM in a per-machine one; _docs/tech/operations.md lists every key.
; Every verb runs `"{app}\cad-lite.exe" "<path>"`: cmd/cad must open a path given as its first argument.
#ifndef MyAppVersion
  #define MyAppVersion "0.0.0"
#endif
#ifndef MyArch
  #define MyArch "amd64"
#endif
#define ProgID "CadLite.Drawing"
#define ExeName "cad-lite.exe"

[Setup]
AppId={{B3D7E0A4-5C1F-4E92-8A6B-2F9D41C7E5A8}
AppName=Cad-lite
AppVersion={#MyAppVersion}
AppPublisher=Synesthesia
DefaultDirName={autopf}\Cad-lite
DefaultGroupName=Cad-lite
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=commandline
CloseApplications=yes
ChangesAssociations=yes
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
Name: "dxfassoc"; Description: "Open .&dxf files with Cad-lite by default"; GroupDescription: "File associations:"
Name: "dwgassoc"; Description: "Open .d&wg files with Cad-lite by default"; GroupDescription: "File associations:"; Flags: unchecked
Name: "contextmenu"; Description: "Add ""Open in Cad-lite"" to the right-click menu of .dxf and .dwg files"; GroupDescription: "File associations:"

[Files]
Source: "_tmp\dist\windows\cad-lite-windows-{#MyArch}\{#ExeName}"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{autoprograms}\Cad-lite"; Filename: "{app}\{#ExeName}"
Name: "{autodesktop}\Cad-lite"; Filename: "{app}\{#ExeName}"; Tasks: desktopicon

; HKA is HKCU\Software\Classes in a per-user install and HKLM\Software\Classes in a per-machine one.
; {app}\cad-lite.exe is a stable path: the in-app updater swaps the file in place (updater.Install).
[Registry]
; Earlier installers registered the ProgID cad-lite.dxf with `-open "%1"`; replace it.
Root: HKCU; Subkey: "Software\Classes\.dxf\OpenWithProgids"; ValueType: none; ValueName: "cad-lite.dxf"; Flags: deletevalue dontcreatekey
Root: HKCU; Subkey: "Software\Classes\cad-lite.dxf"; ValueType: none; Flags: deletekey dontcreatekey

; The ProgID, always registered: the file type name, its icon and the open verb.
Root: HKA; Subkey: "Software\Classes\{#ProgID}"; ValueType: string; ValueName: ""; ValueData: "CAD drawing"; Flags: uninsdeletekey
Root: HKA; Subkey: "Software\Classes\{#ProgID}\DefaultIcon"; ValueType: string; ValueName: ""; ValueData: "{app}\{#ExeName},0"
Root: HKA; Subkey: "Software\Classes\{#ProgID}\shell"; ValueType: string; ValueName: ""; ValueData: "open"
Root: HKA; Subkey: "Software\Classes\{#ProgID}\shell\open"; ValueType: string; ValueName: "FriendlyAppName"; ValueData: "Cad-lite"
Root: HKA; Subkey: "Software\Classes\{#ProgID}\shell\open\command"; ValueType: string; ValueName: ""; ValueData: """{app}\{#ExeName}"" ""%1"""

; "Open with" for both extensions, always: lists Cad-lite without taking over the default.
Root: HKA; Subkey: "Software\Classes\.dxf\OpenWithProgids"; ValueType: string; ValueName: "{#ProgID}"; ValueData: ""; Flags: uninsdeletevalue uninsdeletekeyifempty
Root: HKA; Subkey: "Software\Classes\.dwg\OpenWithProgids"; ValueType: string; ValueName: "{#ProgID}"; ValueData: ""; Flags: uninsdeletevalue uninsdeletekeyifempty

; The application entry: "Open with" name, icon and the types it accepts.
Root: HKA; Subkey: "Software\Classes\Applications\{#ExeName}"; ValueType: string; ValueName: "FriendlyAppName"; ValueData: "Cad-lite"; Flags: uninsdeletekey
Root: HKA; Subkey: "Software\Classes\Applications\{#ExeName}\DefaultIcon"; ValueType: string; ValueName: ""; ValueData: "{app}\{#ExeName},0"
Root: HKA; Subkey: "Software\Classes\Applications\{#ExeName}\SupportedTypes"; ValueType: string; ValueName: ".dxf"; ValueData: ""
Root: HKA; Subkey: "Software\Classes\Applications\{#ExeName}\SupportedTypes"; ValueType: string; ValueName: ".dwg"; ValueData: ""
Root: HKA; Subkey: "Software\Classes\Applications\{#ExeName}\shell\open\command"; ValueType: string; ValueName: ""; ValueData: """{app}\{#ExeName}"" ""%1"""

; Settings > Default apps: the capabilities Windows offers the user to pick.
; The uninstaller walks entries in reverse, so the parents come first here.
Root: HKA; Subkey: "Software\Synesthesia"; Flags: uninsdeletekeyifempty
Root: HKA; Subkey: "Software\Synesthesia\Cad-lite"; Flags: uninsdeletekeyifempty
Root: HKA; Subkey: "Software\Synesthesia\Cad-lite\Capabilities"; ValueType: string; ValueName: "ApplicationName"; ValueData: "Cad-lite"; Flags: uninsdeletekey
Root: HKA; Subkey: "Software\Synesthesia\Cad-lite\Capabilities"; ValueType: string; ValueName: "ApplicationDescription"; ValueData: "2D drafting for DXF and DWG drawings"
Root: HKA; Subkey: "Software\Synesthesia\Cad-lite\Capabilities"; ValueType: string; ValueName: "ApplicationIcon"; ValueData: "{app}\{#ExeName},0"
Root: HKA; Subkey: "Software\Synesthesia\Cad-lite\Capabilities\FileAssociations"; ValueType: string; ValueName: ".dxf"; ValueData: "{#ProgID}"
Root: HKA; Subkey: "Software\Synesthesia\Cad-lite\Capabilities\FileAssociations"; ValueType: string; ValueName: ".dwg"; ValueData: "{#ProgID}"
Root: HKA; Subkey: "Software\RegisteredApplications"; ValueType: string; ValueName: "Cad-lite"; ValueData: "Software\Synesthesia\Cad-lite\Capabilities"; Flags: uninsdeletevalue

; Default program, per task. Unticking a task on a reinstall gives the default back only if it is
; still ours; the uninstaller does the same in CurUninstallStepChanged. A UserChoice the user made
; in Explorer still wins over these values (Windows does not let installers override it).
Root: HKA; Subkey: "Software\Classes\.dxf"; ValueType: string; ValueName: ""; ValueData: "{#ProgID}"; Tasks: dxfassoc
Root: HKA; Subkey: "Software\Classes\.dxf"; ValueType: none; ValueName: ""; Flags: deletevalue dontcreatekey; Tasks: not dxfassoc; Check: IsOurDefault('.dxf')
Root: HKA; Subkey: "Software\Classes\.dwg"; ValueType: string; ValueName: ""; ValueData: "{#ProgID}"; Tasks: dwgassoc
Root: HKA; Subkey: "Software\Classes\.dwg"; ValueType: none; ValueName: ""; Flags: deletevalue dontcreatekey; Tasks: not dwgassoc; Check: IsOurDefault('.dwg')

; "Open in Cad-lite" on .dxf and .dwg whatever program owns them (Windows 11: under "Show more options").
Root: HKA; Subkey: "Software\Classes\SystemFileAssociations\.dxf\shell\CadLite.Open"; ValueType: string; ValueName: ""; ValueData: "Open in Cad-lite"; Flags: uninsdeletekey; Tasks: contextmenu
Root: HKA; Subkey: "Software\Classes\SystemFileAssociations\.dxf\shell\CadLite.Open"; ValueType: string; ValueName: "Icon"; ValueData: "{app}\{#ExeName},0"; Tasks: contextmenu
Root: HKA; Subkey: "Software\Classes\SystemFileAssociations\.dxf\shell\CadLite.Open\command"; ValueType: string; ValueName: ""; ValueData: """{app}\{#ExeName}"" ""%1"""; Tasks: contextmenu
Root: HKA; Subkey: "Software\Classes\SystemFileAssociations\.dwg\shell\CadLite.Open"; ValueType: string; ValueName: ""; ValueData: "Open in Cad-lite"; Flags: uninsdeletekey; Tasks: contextmenu
Root: HKA; Subkey: "Software\Classes\SystemFileAssociations\.dwg\shell\CadLite.Open"; ValueType: string; ValueName: "Icon"; ValueData: "{app}\{#ExeName},0"; Tasks: contextmenu
Root: HKA; Subkey: "Software\Classes\SystemFileAssociations\.dwg\shell\CadLite.Open\command"; ValueType: string; ValueName: ""; ValueData: """{app}\{#ExeName}"" ""%1"""; Tasks: contextmenu
Root: HKA; Subkey: "Software\Classes\SystemFileAssociations\.dxf\shell\CadLite.Open"; ValueType: none; Flags: deletekey dontcreatekey; Tasks: not contextmenu
Root: HKA; Subkey: "Software\Classes\SystemFileAssociations\.dwg\shell\CadLite.Open"; ValueType: none; Flags: deletekey dontcreatekey; Tasks: not contextmenu

[UninstallDelete]
; The previous exe the updater keeps until the next start (updater.CleanupOld).
Type: files; Name: "{app}\{#ExeName}.old"

[Run]
Filename: "{app}\{#ExeName}"; Description: "Launch Cad-lite"; Flags: nowait postinstall skipifsilent

[Code]
// IsOurDefault reports whether Ext (".dxf") opens with Cad-lite's ProgID by default.
function IsOurDefault(Ext: String): Boolean;
var
  Value: String;
begin
  Result := RegQueryStringValue(HKA, 'Software\Classes\' + Ext, '', Value) and (CompareText(Value, '{#ProgID}') = 0);
end;

// ReleaseDefault removes Cad-lite as Ext's default program, leaving another program's claim alone.
procedure ReleaseDefault(Ext: String);
begin
  if IsOurDefault(Ext) then
    RegDeleteValue(HKA, 'Software\Classes\' + Ext, '');
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if CurUninstallStep = usUninstall then
  begin
    ReleaseDefault('.dxf');
    ReleaseDefault('.dwg');
  end;
end;
