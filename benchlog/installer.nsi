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
  File "benchlog.exe"
  File "busybox64.exe"
  WriteUninstaller "$INSTDIR\uninstall.exe"

  ; ── Shell launcher ─────────────────────────────────────────────────────────
  ; Opens BusyBox sh as a login shell. .profile auto-launches benchlog.
  ; Pressing q in benchlog drops to the shell prompt.
  FileOpen $0 "$INSTDIR\benchlog-shell.bat" w
  FileWrite $0 "@echo off$\r$\n"
  FileWrite $0 "title benchlog$\r$\n"
  FileWrite $0 "$\"$INSTDIR\busybox64.exe$\" sh -l$\r$\n"
  FileClose $0

  ; ── Write .profile if it doesn't exist ────────────────────────────────────
  IfFileExists "$PROFILE\.profile" skip_profile
    FileOpen $0 "$PROFILE\.profile" w
    FileWrite $0 "# benchlog shell$\n"
    FileWrite $0 "export TERM=xterm-256color$\n"
    FileWrite $0 "export COLORTERM=truecolor$\n"
    FileWrite $0 "benchlog$\n"
    FileWrite $0 "PS1='$$ '$\n"
    FileClose $0
  skip_profile:

  ; ── Add to system PATH ─────────────────────────────────────────────────────
  ReadRegStr $0 HKLM "SYSTEM\CurrentControlSet\Control\Session Manager\Environment" "Path"
  WriteRegExpandStr HKLM "SYSTEM\CurrentControlSet\Control\Session Manager\Environment" "Path" "$0;$INSTDIR"
  SendMessage ${HWND_BROADCAST} ${WM_WININICHANGE} 0 "STR:Environment" /TIMEOUT=5000

  ; ── Add/Remove Programs entry ─────────────────────────────────────────────
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\benchlog" "DisplayName" "benchlog"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\benchlog" "UninstallString" "$INSTDIR\uninstall.exe"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\benchlog" "DisplayVersion" "${VERSION}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\benchlog" "Publisher" "Da4kek"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\benchlog" "URLInfoAbout" "https://github.com/Da4kek/CmD-set"

  ; ── Start Menu shortcut → shell launcher ──────────────────────────────────
  CreateDirectory "$SMPROGRAMS\benchlog"
  CreateShortcut "$SMPROGRAMS\benchlog\benchlog.lnk" \
    "$INSTDIR\benchlog-shell.bat" "" "$INSTDIR\benchlog.exe" 0 \
    SW_SHOWNORMAL "" "Open benchlog research shell"

SectionEnd

Section "Uninstall"
  Delete "$INSTDIR\benchlog.exe"
  Delete "$INSTDIR\busybox64.exe"
  Delete "$INSTDIR\benchlog-shell.bat"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"
  Delete "$SMPROGRAMS\benchlog\benchlog.lnk"
  RMDir "$SMPROGRAMS\benchlog"
  DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\benchlog"
SectionEnd
