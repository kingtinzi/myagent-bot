# Bundled frontend libraries (chat Markdown)

These files are **committed** on purpose. The repository root `.gitignore` contains `vendor/`, which ignores **any** path named `vendor` — so `frontend/vendor/` never made it into Git, and Windows/clean clones had no `marked` / `DOMPurify`, breaking assistant message Markdown rendering.

| File | Version (approx.) | Source |
|------|-------------------|--------|
| `marked.min.js` | 11.1.1 | <https://cdn.jsdelivr.net/npm/marked@11.1.1/marked.min.js> |
| `purify.min.js` | 3.1.6 | <https://cdn.jsdelivr.net/npm/dompurify@3.1.6/dist/purify.min.js> |

To refresh: download the same paths (or bump versions in `index.html` if APIs change), then run Launcher tests.
