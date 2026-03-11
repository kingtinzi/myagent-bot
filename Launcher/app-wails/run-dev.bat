@echo off
cd /d "%~dp0"
if exist "build\bin\launcher-chat-dev.exe" (
  "build\bin\launcher-chat-dev.exe"
) else if exist "launcher-chat.exe" (
  launcher-chat.exe
) else (
  echo 请先在此目录执行: go build -o launcher-chat.exe .
  echo 或: wails build
  pause
)
