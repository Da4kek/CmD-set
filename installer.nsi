!include "MUI2.nsh"
!include "WinMessages.nsh"

Name "benchlog ${VERSION}"
OutFile "benchlog-${VERSION}-setup.exe"
InstallDir "$PROGRAMFILES64\benchlog"
RequestExecutionLevel admin

!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"

Section "Install"
  SetOutPath "$INSTDIR"
  File "dist/benchlog_windows_amd64_v1/benchlog.exe"
  WriteUninstaller "$INSTDIR\uninstall.exe"

  ; Add to system PATH so it works from any terminal
  ReadRegStr $0 HKLM "SYSTEM\CurrentControlSet\Control\Session Manager\Environment" "Path"
  WriteRegExpandStr HKLM "SYSTEM\CurrentControlSet\Control\Session Manager\Environment" "Path" "$0;$INSTDIR"
  SendMessage ${HWND_BROADCAST} ${WM_WININICHANGE} 0 "STR:Environment" /TIMEOUT=5000

  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\benchlog" "DisplayName" "benchlog"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\benchlog" "UninstallString" "$INSTDIR\uninstall.exe"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\benchlog" "DisplayVersion" "${VERSION}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\benchlog" "Publisher" "Da4kek"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\benchlog" "URLInfoAbout" "https://github.com/Da4kek/CmD-set"
SectionEnd

Section "Uninstall"
  Delete "$INSTDIR\benchlog.exe"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"
  DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\benchlog"
SectionEnd
