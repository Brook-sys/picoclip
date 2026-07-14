# PicoClip

_Read this in [Portuguese / Português](README.pt-BR.md)._

PicoClip is a lightweight, local-first agent orchestration engine. It was created as an alternative inspired by Paperclip, with extra focus on **lightness, extreme portability, and minimal hardware resource usage**.

The goal is to provide projects/workspaces, agents, tasks, runs, messages, delegation, permissions, skills, and APIs for agents while keeping the core simple, small, and easy to run almost anywhere.

## Important disclaimer: vibe coding

PicoClip is currently written entirely through **vibe coding**, with heavy AI-assisted development.

Because of that:

- use the project with care;
- we strongly discourage using it in production;
- architecture, APIs, UI flows, and implementation details may change quickly;
- large parts of the codebase can be rewritten or reorganized as the project evolves;
- you should review the code before running it in sensitive environments.

This does not mean the project is careless. It means the project is experimental, fast-moving, and intentionally open about how it is being built.

## Why PicoClip?

PicoClip is inspired by the idea of local agent orchestration, but it tries to stay extremely small and practical.

The main goals are:

- small Go binary;
- low RAM usage;
- local-first operation;
- simple modular architecture;
- pluggable drivers;
- pluggable storage;
- lightweight server-rendered UI with HTMX and Templ;
- useful APIs for both humans and agents;
- real permissions and capabilities instead of purely decorative roles;
- reusable skills as instruction/context packages.

## Current status

PicoClip is under active development.

It already includes a working web UI, SQLite persistence, task lifecycle management, agents, skills, projects/workspaces, runtime adapters, local administrative APIs, an Agent API for agent-driven workflows, diagnostics, Activity/SSE events, cancellation support, lock recovery, stalled-run detection, and scheduled retry wakeups with backoff.

That said, it is still early. Some behaviors are still being refined, and parts of the system may change significantly over time. The most important active hardening areas are retry classification, recovery visibility, permission enforcement, runtime event streaming, and operational dashboards.

## How PicoClip works

PicoClip is built around a small orchestration loop:

1. humans or agents create tasks;
2. tasks are assigned to agents and become runnable;
3. the dispatcher claims runnable tasks with checkout/lock metadata;
4. the runner executes the task through a runtime adapter;
5. runs produce events, messages, output, errors, and usage metadata;
6. the reconciler repairs stale locks, processes wakeups, detects stalled runs, and schedules retries when appropriate.

The system is intentionally local-first: the default storage is a local SQLite database, workspaces live on the local filesystem, and runtime adapters are local commands.

## Robustness model

PicoClip aims to fail visibly and recover conservatively. Current reliability features include:

- SQLite persistence by default;
- atomic task checkout and execution locks;
- stale lock recovery;
- runtime cancellation through adapters;
- stalled-run detection based on heartbeat/output timing;
- retry wakeups with exponential backoff;
- `retry.scheduled`, `run.timeout`, and `run.recovered` activity events;
- diagnostics for storage, runtime paths, workspace paths, and configured runtimes.

See [Robustness, Recovery, and Failure Learning](docs/ROBUSTNESS.md) for the detailed operational model.

Runtime providers can be configured from Settings with the adapter-aware [Runtime Provider Quick Setup](docs/RUNTIME_PROVIDER_QUICK_SETUP.md), while the full native configuration remains available in the advanced editor.

## Quick start

PicoClip is distributed as a single binary. It does not require an external database or heavy runtime services.

### First 15 minutes for contributors and agents

Before changing code, use the short onboarding path:

1. Read [AGENTS.md](AGENTS.md), [Project Map](docs/PROJECT_MAP.md), and [Documentation Policy](docs/DOCUMENTATION_POLICY.md).
2. Read the area guide for the change: [API Reference](docs/API_REFERENCE.md), [Robustness](docs/ROBUSTNESS.md), [Storage Architecture](docs/STORAGE.md), [Design System](docs/DESIGN.md), [Operations Runbook](docs/OPERATIONS.md), or [Development Guide](docs/DEVELOPMENT.md).
3. Run `git status --short` before editing and preserve local/untracked artifacts.
4. For autonomous maintenance, follow [Autonomous Improvement](docs/AUTONOMOUS_IMPROVEMENT.md) and the `picoclip` Kanban board before choosing a single ready card.
5. For behavior changes, use TDD, then run focused validation plus the proportional canonical gate (`make check-docs`, `make test-go`, or `make check`).

### Option 1: Run a prebuilt binary

Download the latest binary from the [GitHub Releases](https://github.com/Brook-sys/picoclip/releases) page for your platform.

Linux x64 example:

```sh
tar -xzf picoclip-v0.0.1-linux-amd64.tar.gz
chmod +x picoclip-v0.0.1-linux-amd64
./picoclip-v0.0.1-linux-amd64
```

macOS Apple Silicon example:

```sh
tar -xzf picoclip-v0.0.1-darwin-arm64.tar.gz
chmod +x picoclip-v0.0.1-darwin-arm64
./picoclip-v0.0.1-darwin-arm64
```

Windows example:

```powershell
Expand-Archive picoclip-v0.0.1-windows-amd64.zip
.\picoclip-v0.0.1-windows-amd64\picoclip-v0.0.1-windows-amd64.exe
```

Then open:

```text
http://127.0.0.1:8088
```

By default PicoClip listens on `127.0.0.1:8088`. To expose it on another interface, set `BIND` explicitly and place authentication or a trusted access proxy in front of the administrative API:

```sh
BIND=127.0.0.1 PORT=9090 ./picoclip-v0.0.1-linux-amd64
```

### Option 2: Run with Docker / Podman

Default Alpine-based image:

```sh
docker run --rm -p 8088:8088 \
  -v picoclip-data:/app/data \
  -v picoclip-workspaces:/app/workspaces \
  ghcr.io/brook-sys/picoclip:latest
```

If you need the Claurst runtime, use the Debian/glibc image variant because the official Claurst Linux binary does not run reliably on Alpine/musl:

```sh
docker run --rm -p 8088:8088 \
  -v picoclip-data:/app/data \
  -v picoclip-workspaces:/app/workspaces \
  ghcr.io/brook-sys/picoclip:latest-debian
```

Then open:

```text
http://127.0.0.1:8088
```

You can also use `podman run` with the same arguments.

The container starts as root only long enough to reconcile ownership of the fixed persistent roots `/app/data` and `/app/workspaces`, then executes PicoClip as the unprivileged `picoclip` user. Configured database/runtime paths must remain under `/app/data`, and workspace paths under `/app/workspaces`. This automatically repairs named volumes created by older images with a different UID, including runtime configuration files with mode `0600`. It does not remove or recreate the database, runtime installations, or workspace contents.

### Option 3: Run from source

Requirements:

- Go
- Git

```sh
git clone https://github.com/Brook-sys/picoclip.git
cd picoclip
make tools
make run
```

Then open:

```text
http://127.0.0.1:8088
```

Optional demo data:

```sh
make seed
```

Useful runtime configuration:

| Variable | Default | Purpose |
| --- | --- | --- |
| `BIND` | `127.0.0.1` | HTTP bind address. Containers set `0.0.0.0`; external exposure requires a trusted authentication/access layer. |
| `PORT` | `8080` in the binary, `8088` in the Makefile | HTTP port. |
| `PICOCLIP_STORAGE` | `sqlite` | `sqlite` or `memory`. Use `memory` only for temporary sessions/tests. |
| `PICOCLIP_DB_PATH` | `data/picoclip.db` | SQLite database path. |
| `PICOCLIP_WORKSPACES` | `workspaces` | Base directory for project/workspace folders. |
| `PICOCLIP_RUNTIMES` | `data/runtimes` | Base directory for runtime state. |
| `PICOCLIP_LOG_LEVEL` | `info` | Log level. |
| `PICOCLIP_DEBUG` | `false` | Enables debug behavior when `true` or `1`. |
| `CRUSH_PATH` | `crush` | Crush runtime executable. |
| `PICOCLAW_PATH` | `picoclaw` | PicoClaw runtime executable. |
| `CLAURST_PATH` | `claurst` | Claurst runtime executable. |

Development mode with live reload:

```sh
make dev
```

Build locally:

```sh
make build
./picoclip
```

Validate everything:

```sh
make check
```

## Roadmap

There is an active roadmap, and more features will be added gradually as the project matures.

See:

- [Project Map](docs/PROJECT_MAP.md)
- [Plugins](docs/PLUGINS.md)
- [Documentation Policy](docs/DOCUMENTATION_POLICY.md)
- [API Reference](docs/API_REFERENCE.md)
- [Operations Runbook](docs/OPERATIONS.md)
- [Roadmap](docs/ROADMAP.md)
- [Current State](docs/CURRENT_STATE.md)
- [Improvement Backlog](docs/IMPROVEMENT_BACKLOG.md)
- [Storage Architecture](docs/STORAGE.md)
- [Robustness, Recovery, and Failure Learning](docs/ROBUSTNESS.md)
- [Development Guide](docs/DEVELOPMENT.md)
- [Design System](docs/DESIGN.md)

## Contributing

Collaboration is very welcome.

This project is open-source in spirit and open to criticism, bug reports, feature suggestions, architectural feedback, and pull requests.

You can help by:

- opening issues for bugs;
- suggesting new features;
- reviewing design decisions;
- improving documentation;
- testing the project in different environments;
- submitting pull requests.

Because this is a vibe-coded project, external feedback is especially valuable. It helps keep the project grounded, useful, and safer to evolve.

## Production use

PicoClip is **not recommended for production use** at this stage.

If you decide to run it anyway, treat it as experimental software and review its behavior carefully.
