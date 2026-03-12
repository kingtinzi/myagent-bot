# Task 9 Verification Notes

Verified locally on 2026-03-12 from `integration/platform-auth-billing`.

- [x] Admin runtime-config page shows per-editor JSON validation and preview cards before save.
- [x] Admin save button stays disabled until login succeeds and all JSON blocks parse.
- [x] Launcher settings sidebar, group toggles, and model modal are keyboard reachable as buttons.
- [x] Launcher settings exposes live status regions for toast and JSON validation feedback.
- [x] Desktop chat login gate uses dialog semantics and announces auth/session errors.
- [x] Desktop chat does not show another user's local history before auth state resolves.
- [x] `scripts/build-release.sh` produces `launcher-chat.app/Contents/MacOS/` with `launcher-chat`, `pinchbot`, `pinchbot-launcher`, and `platform-server`.
- [x] `docs/build-and-release.md` explains the `.app` bundle layout and first-run platform config behavior.
- [x] macOS packaging docs and generated `README.txt` warn that external delivery still requires signing and notarization.
