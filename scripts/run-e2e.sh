#!/bin/sh
set -eu

BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
export BASE_URL

for candidate in chromium chromium-browser google-chrome chrome; do
  if command -v "$candidate" >/dev/null 2>&1; then
    export PLAYWRIGHT_CHROMIUM_EXECUTABLE="$(command -v "$candidate")"
    exec npx playwright test --config=e2e/playwright.config.ts "$@"
  fi
done

if [ -f /etc/alpine-release ]; then
  cat <<'MSG'
Skipping Playwright E2E: no Alpine-compatible Chromium executable found.

Install one of these on the host/container, then rerun:
  apk add chromium

Or set:
  PLAYWRIGHT_CHROMIUM_EXECUTABLE=/path/to/chromium

Go route/HTMX contract tests still run with `go test ./...`.
MSG
  exit 0
fi

exec npx playwright test --config=e2e/playwright.config.ts "$@"
