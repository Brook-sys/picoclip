#!/bin/sh
set -eu

# Keep automated E2E isolated from the developer's persistent PicoClip on 8088.
BASE_URL="${BASE_URL:-http://127.0.0.1:18088}"
export BASE_URL

port_from_url() {
  printf '%s\n' "$1" | sed -n 's|^[a-zA-Z][a-zA-Z0-9+.-]*://[^/:]*:\([0-9][0-9]*\).*|\1|p'
}

port_is_busy() {
  port="$1"
  if command -v python3 >/dev/null 2>&1; then
    python3 - "$port" <<'PY'
import socket
import sys

port = int(sys.argv[1])
with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
    sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    try:
        sock.bind(("127.0.0.1", port))
    except OSError:
        sys.exit(0)
    sys.exit(1)
PY
    return $?
  fi
  if command -v fuser >/dev/null 2>&1; then
    fuser "${port}/tcp" >/dev/null 2>&1
    return $?
  fi
  if command -v ss >/dev/null 2>&1; then
    ss -ltn | grep -q ":${port} "
    return $?
  fi
  return 1
}

if [ "${PLAYWRIGHT_REUSE_SERVER:-}" != "true" ]; then
  requested_port="$(port_from_url "$BASE_URL")"
  if [ -n "$requested_port" ] && port_is_busy "$requested_port"; then
    if [ "${PICOCLIP_E2E_DYNAMIC_PORT:-1}" = "1" ]; then
      for candidate in $(seq 18088 18188); do
        if ! port_is_busy "$candidate"; then
          BASE_URL="http://127.0.0.1:${candidate}"
          export BASE_URL
          break
        fi
      done
    else
      cat >&2 <<MSG
Port ${requested_port} from BASE_URL=${BASE_URL} is already in use.
Set PLAYWRIGHT_REUSE_SERVER=true to reuse it, or PICOCLIP_E2E_DYNAMIC_PORT=1 to select a free test port.
MSG
      exit 1
    fi
  fi
fi

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
