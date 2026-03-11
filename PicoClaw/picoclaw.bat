@echo off
REM Proxy for web_search (DuckDuckGo etc). Port 7897 = your system proxy; change if needed.
set HTTP_PROXY=http://127.0.0.1:7897
set HTTPS_PROXY=http://127.0.0.1:7897
"%~dp0build\picoclaw-windows-amd64.exe" %*
