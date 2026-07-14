#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
entrypoint="$tmp/docker-entrypoint.sh"
sed "s|^app_root=/app$|app_root=$tmp/app|" "$repo_root/scripts/docker-entrypoint.sh" > "$entrypoint"
chmod +x "$entrypoint"

fake_bin="$tmp/bin"
state="$tmp/state"
data="$tmp/app/data"
workspaces="$tmp/app/workspaces"
mkdir -p "$fake_bin" "$state" "$data/runtimes/picoclaw/config" "$workspaces"
printf '%s\n' 'api_keys: {}' > "$data/runtimes/picoclaw/config/.security.yml"
chmod 0600 "$data/runtimes/picoclaw/config/.security.yml"

cat > "$fake_bin/id" <<'EOF'
#!/bin/sh
if [ "${1:-}" = "-u" ]; then
  if [ "$#" -gt 1 ]; then
    printf '%s\n' 100
  else
    printf '%s\n' "${ENTRYPOINT_TEST_UID:-0}"
  fi
  exit 0
fi
exec /usr/bin/id "$@"
EOF

cat > "$fake_bin/chown" <<'EOF'
#!/bin/sh
printf '%s\n' "$*" >> "$ENTRYPOINT_TEST_STATE/chown.log"
EOF

cat > "$fake_bin/setpriv" <<'EOF'
#!/bin/sh
[ "$1" = "--reuid" ]
[ "$3" = "--regid" ]
[ "$5" = "--init-groups" ]
printf '%s\n' "$2:$4" > "$ENTRYPOINT_TEST_STATE/drop-user"
shift 5
exec "$@"
EOF

cat > "$fake_bin/picoclip-test" <<'EOF'
#!/bin/sh
printf '%s\n' "$*" > "$ENTRYPOINT_TEST_STATE/command"
EOF
chmod +x "$fake_bin"/*

ENTRYPOINT_TEST_STATE="$state" \
PATH="$fake_bin:/usr/bin:/bin" \
PICOCLIP_DB_PATH="$data/picoclip.db" \
PICOCLIP_RUNTIMES="$data/runtimes" \
PICOCLIP_WORKSPACES="$workspaces" \
"$entrypoint" picoclip-test serve

grep -F -- "-Rh picoclip:picoclip $data" "$state/chown.log" >/dev/null
grep -F -- "-Rh picoclip:picoclip $workspaces" "$state/chown.log" >/dev/null
[ "$(cat "$state/drop-user")" = "picoclip:picoclip" ]
[ "$(cat "$state/command")" = "serve" ]

ENTRYPOINT_TEST_STATE="$state" \
PATH="$fake_bin:/usr/bin:/bin" \
PICOCLIP_RUNTIME_USER=root \
PICOCLIP_RUNTIME_GROUP=root \
PICOCLIP_DB_PATH="$data/picoclip.db" \
PICOCLIP_RUNTIMES="$data/runtimes" \
PICOCLIP_WORKSPACES="$workspaces" \
"$entrypoint" picoclip-test serve
[ "$(cat "$state/drop-user")" = "picoclip:picoclip" ]

if ENTRYPOINT_TEST_STATE="$state" \
    ENTRYPOINT_TEST_UID=200 \
    PATH="$fake_bin:/usr/bin:/bin" \
    PICOCLIP_DB_PATH="$data/picoclip.db" \
    PICOCLIP_RUNTIMES="$data/runtimes" \
    PICOCLIP_WORKSPACES="$workspaces" \
    "$entrypoint" picoclip-test wrong-user 2>"$state/wrong-user-error"; then
    printf '%s\n' 'expected a non-picoclip non-root UID to be rejected' >&2
    exit 1
fi
grep -F -- 'must run as root or picoclip' "$state/wrong-user-error" >/dev/null

ENTRYPOINT_TEST_STATE="$state" \
ENTRYPOINT_TEST_UID=100 \
PATH="$fake_bin:/usr/bin:/bin" \
PICOCLIP_DB_PATH="$data/picoclip.db" \
PICOCLIP_RUNTIMES="$data/runtimes" \
PICOCLIP_WORKSPACES="$workspaces" \
"$entrypoint" picoclip-test nonroot
[ "$(cat "$state/command")" = "nonroot" ]

: > "$state/chown.log"
if ENTRYPOINT_TEST_STATE="$state" \
    PATH="$fake_bin:/usr/bin:/bin" \
    PICOCLIP_DB_PATH="/etc/picoclip/picoclip.db" \
    PICOCLIP_RUNTIMES="$data/runtimes" \
    PICOCLIP_WORKSPACES="$workspaces" \
    "$entrypoint" picoclip-test serve 2>"$state/unsafe-error"; then
    printf '%s\n' 'expected unsafe system directory to be rejected' >&2
    exit 1
fi
grep -F -- 'persistent path must be under' "$state/unsafe-error" >/dev/null

if ENTRYPOINT_TEST_STATE="$state" \
    PATH="$fake_bin:/usr/bin:/bin" \
    PICOCLIP_DB_PATH="$data/../../../etc/picoclip.db" \
    PICOCLIP_RUNTIMES="$data/runtimes" \
    PICOCLIP_WORKSPACES="$workspaces" \
    "$entrypoint" picoclip-test serve 2>"$state/traversal-error"; then
    printf '%s\n' 'expected traversal into a system directory to be rejected' >&2
    exit 1
fi
grep -F -- 'refusing persistent path with parent traversal:' "$state/traversal-error" >/dev/null

ln -s /etc "$data/system-link"
: > "$state/chown.log"
if ENTRYPOINT_TEST_STATE="$state" \
    PATH="$fake_bin:/usr/bin:/bin" \
    PICOCLIP_DB_PATH="$data/system-link/picoclip.db" \
    PICOCLIP_RUNTIMES="$data/runtimes" \
    PICOCLIP_WORKSPACES="$workspaces" \
    "$entrypoint" picoclip-test serve 2>"$state/symlink-error"; then
    printf '%s\n' 'expected symlink into a system directory to be rejected' >&2
    exit 1
fi
grep -F -- 'refusing symlink in persistent path:' "$state/symlink-error" >/dev/null
[ ! -s "$state/chown.log" ]

if ENTRYPOINT_TEST_STATE="$state" \
    PATH="$fake_bin:/usr/bin:/bin" \
    PICOCLIP_DB_PATH="$tmp/outside/picoclip.db" \
    PICOCLIP_RUNTIMES="$data/runtimes" \
    PICOCLIP_WORKSPACES="$workspaces" \
    "$entrypoint" picoclip-test serve 2>"$state/outside-error"; then
    printf '%s\n' 'expected a persistent directory outside /app to be rejected' >&2
    exit 1
fi
grep -F -- 'persistent path must be under' "$state/outside-error" >/dev/null

for dockerfile in "$repo_root/Dockerfile" "$repo_root/Dockerfile.debian"; do
    grep -F -- 'COPY --chmod=0755 scripts/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh' "$dockerfile" >/dev/null
    grep -F -- 'ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh", "picoclip"]' "$dockerfile" >/dev/null
    if grep -F -- 'USER picoclip' "$dockerfile" >/dev/null; then
        printf '%s still bypasses root-time volume reconciliation\n' "$dockerfile" >&2
        exit 1
    fi
done
grep -F -- 'apk add --no-cache ca-certificates setpriv tzdata' "$repo_root/Dockerfile" >/dev/null
grep -F -- 'gosu' "$repo_root/Dockerfile.debian" >/dev/null

printf '%s\n' 'docker entrypoint legacy-volume test: PASS'
