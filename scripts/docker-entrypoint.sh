#!/bin/sh
set -eu

runtime_user=picoclip
runtime_group=picoclip
app_root=/app
data_root=$app_root/data
workspaces_root=$app_root/workspaces

assert_no_parent_traversal() {
    path=$1
    case "/$path/" in
        */../*)
            printf 'picoclip entrypoint: refusing persistent path with parent traversal: %s\n' "$path" >&2
            exit 1
            ;;
    esac
}

assert_under_root() {
    allowed_root=$1
    path=$2
    assert_no_parent_traversal "$path"
    case "$path" in
        "$allowed_root"|"$allowed_root"/*) ;;
        *)
            printf 'picoclip entrypoint: persistent path must be under %s: %s\n' "$allowed_root" "$path" >&2
            exit 1
            ;;
    esac
}

assert_no_symlink_components() {
    path=$1
    current=
    old_ifs=$IFS
    IFS=/
    set -- $path
    IFS=$old_ifs
    for component do
        [ -n "$component" ] || continue
        current="$current/$component"
        if [ -L "$current" ]; then
            printf 'picoclip entrypoint: refusing symlink in persistent path: %s\n' "$current" >&2
            exit 1
        fi
    done
}

prepare_persistent_root() {
    root=$1
    if [ -L "$root" ]; then
        printf 'picoclip entrypoint: refusing symlink persistent root: %s\n' "$root" >&2
        exit 1
    fi
    mkdir -p "$root"
    if [ -L "$root" ]; then
        printf 'picoclip entrypoint: refusing symlink persistent root: %s\n' "$root" >&2
        exit 1
    fi
    chown -Rh "$runtime_user:$runtime_group" "$root"
}

current_uid=$(id -u)
picoclip_uid=$(id -u "$runtime_user")

if [ "$current_uid" = "0" ]; then
    db_path=${PICOCLIP_DB_PATH:-$data_root/picoclip.db}
    runtimes_path=${PICOCLIP_RUNTIMES:-$data_root/runtimes}
    workspaces_path=${PICOCLIP_WORKSPACES:-$workspaces_root}

    assert_under_root "$data_root" "$db_path"
    assert_under_root "$data_root" "$runtimes_path"
    assert_under_root "$workspaces_root" "$workspaces_path"
    assert_no_symlink_components "$db_path"
    assert_no_symlink_components "$runtimes_path"
    assert_no_symlink_components "$workspaces_path"

    prepare_persistent_root "$data_root"
    prepare_persistent_root "$workspaces_root"

    if command -v setpriv >/dev/null 2>&1; then
        exec setpriv --reuid "$runtime_user" --regid "$runtime_group" --init-groups "$@"
    fi
    if command -v gosu >/dev/null 2>&1; then
        exec gosu "$runtime_user:$runtime_group" "$@"
    fi
    printf 'picoclip entrypoint: setpriv or gosu is required to drop privileges\n' >&2
    exit 1
fi

if [ "$current_uid" != "$picoclip_uid" ]; then
    printf 'picoclip entrypoint: container must run as root or picoclip (uid %s), got uid %s\n' "$picoclip_uid" "$current_uid" >&2
    exit 1
fi

exec "$@"
