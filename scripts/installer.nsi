; ks-prank Windows Installer Script
; 使用 makensis 编译: makensis scripts/installer.nsi

!include "MUI2.nsh"

; ---- 基本信息 ----
Name "萌物·快手整蛊助手"
OutFile "..\build\ks-prank-setup.exe"
InstallDir "$PROGRAMFILES\ks-prank"
InstallDirRegKey HKLM "Software\ks-prank" "InstallDir"
RequestExecutionLevel admin

; ---- 界面配置 ----
!ifndef NOICON
    !define MUI_ICON "..\build\appicon.ico"
    !define MUI_UNICON "..\build\appicon.ico"
!endif
!define MUI_ABORTWARNING

; ---- 安装页面 ----
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

; ---- 卸载页面 ----
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

; ---- 语言 ----
!insertmacro MUI_LANGUAGE "SimpChinese"

; ---- 安装过程 ----
Section "主程序" SecMain
    SetOutPath "$INSTDIR"

    ; 复制文件
    File "..\build\bin\ks-prank.exe"
    File "..\config.yaml.example"

    ; 如果用户没有配置文件，从 example 复制一份
    IfFileExists "$INSTDIR\config.yaml" +2 0
    CopyFiles "$INSTDIR\config.yaml.example" "$INSTDIR\config.yaml"

    ; 写注册表（记住安装路径 + 卸载信息）
    WriteRegStr HKLM "Software\ks-prank" "InstallDir" "$INSTDIR"
    WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\ks-prank" "DisplayName" "萌物·快手整蛊助手"
    WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\ks-prank" "UninstallString" '"$INSTDIR\uninstall.exe"'
    WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\ks-prank" "InstallLocation" "$INSTDIR"
    WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\ks-prank" "NoModify" 1
    WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\ks-prank" "NoRepair" 1

    ; 创建卸载程序
    WriteUninstaller "$INSTDIR\uninstall.exe"

    ; 创建开始菜单快捷方式
    CreateDirectory "$SMPROGRAMS\萌物·快手整蛊助手"
    CreateShortcut "$SMPROGRAMS\萌物·快手整蛊助手\萌物·快手整蛊助手.lnk" "$INSTDIR\ks-prank.exe"
    CreateShortcut "$SMPROGRAMS\萌物·快手整蛊助手\卸载.lnk" "$INSTDIR\uninstall.exe"

    ; 创建桌面快捷方式
    CreateShortcut "$DESKTOP\萌物·快手整蛊助手.lnk" "$INSTDIR\ks-prank.exe"
SectionEnd

; ---- 卸载过程 ----
Section "Uninstall"
    ; 删除文件
    Delete "$INSTDIR\ks-prank.exe"
    Delete "$INSTDIR\config.yaml.example"
    Delete "$INSTDIR\uninstall.exe"

    ; 删除快捷方式
    Delete "$DESKTOP\萌物·快手整蛊助手.lnk"
    Delete "$SMPROGRAMS\萌物·快手整蛊助手\萌物·快手整蛊助手.lnk"
    Delete "$SMPROGRAMS\萌物·快手整蛊助手\卸载.lnk"
    RMDir "$SMPROGRAMS\萌物·快手整蛊助手"

    ; 删除安装目录（不删除用户配置 config.yaml）
    RMDir "$INSTDIR"

    ; 清理注册表
    DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\ks-prank"
    DeleteRegKey HKLM "Software\ks-prank"
SectionEnd
