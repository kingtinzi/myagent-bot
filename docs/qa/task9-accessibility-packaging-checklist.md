# Task 9 Verification Notes

- [ ] Admin runtime-config page shows per-editor JSON validation and preview cards before save.
- [ ] Admin save button stays disabled until login succeeds and all JSON blocks parse.
- [ ] Launcher settings sidebar, group toggles, and model modal are keyboard reachable as buttons.
- [ ] Launcher settings exposes live status regions for toast and JSON validation feedback.
- [ ] Desktop chat login gate uses dialog semantics and announces auth/session errors.
- [ ] Desktop chat does not show another user's local history before auth state resolves.
- [ ] `scripts/build-release.sh` produces `launcher-chat.app/Contents/MacOS/` with `launcher-chat`, `picoclaw`, `picoclaw-launcher`, and `platform-server`.
- [ ] `docs/build-and-release.md` explains the `.app` bundle layout and first-run platform config behavior.
- [ ] macOS packaging docs and generated `README.txt` warn that external delivery still requires signing and notarization.
