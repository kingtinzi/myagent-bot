#ifndef MyAppVersion
  #define MyAppVersion "dev"
#endif
#ifndef MyOutputVersion
  #define MyOutputVersion MyAppVersion
#endif
#ifndef MyPackageDir
  #error MyPackageDir must point to a built PinchBot package directory.
#endif
#ifndef MyOutputDir
  #error MyOutputDir must point to the installer output directory.
#endif

[Setup]
AppId={{5A01079D-5F19-49CB-A33E-3F7A8BE3A6AA}
AppName=PinchBot
AppVersion={#MyAppVersion}
AppPublisher=PinchBot
DefaultDirName={localappdata}\Programs\PinchBot
DefaultGroupName=PinchBot
DisableProgramGroupPage=yes
DisableDirPage=yes
OutputDir={#MyOutputDir}
OutputBaseFilename=PinchBot-{#MyOutputVersion}-Windows-x86_64-Setup
Compression=lzma
SolidCompression=yes
WizardStyle=modern
LanguageDetectionMethod=none
UsePreviousLanguage=no
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
UninstallDisplayIcon={app}\launcher-chat.exe

[Languages]
Name: "chinesesimplified"; MessagesFile: "compiler:Default.isl,{#SourcePath}innosetup\ChineseSimplified.isl"
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked

[Files]
Source: "{#MyPackageDir}\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{autoprograms}\PinchBot"; Filename: "{app}\launcher-chat.exe"
Name: "{autodesktop}\PinchBot"; Filename: "{app}\launcher-chat.exe"; Tasks: desktopicon

[Run]
Filename: "{app}\launcher-chat.exe"; Description: "{cm:LaunchProgram,PinchBot}"; Flags: nowait postinstall skipifsilent
