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

It already includes a working web UI, task lifecycle management, agents, skills, projects/workspaces, a local administrative API, and an Agent API for agent-driven workflows.

That said, it is still early. Some behaviors are still being refined, and parts of the system may change significantly over time.

## Quick start

PicoClip is distributed as a single binary. It does not require an external database or heavy runtime services.

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
http://127.0.0.1:8080
```

By default PicoClip listens on `0.0.0.0:8080`. You can change it with:

```sh
BIND=127.0.0.1 PORT=9090 ./picoclip-v0.0.1-linux-amd64
```

### Option 2: Run with Docker / Podman

Default Alpine-based image:

```sh
docker run --rm -p 8080:8080 \
  -v picoclip-data:/app/data \
  -v picoclip-workspaces:/app/workspaces \
  ghcr.io/brook-sys/picoclip:latest
```

If you need the Claurst runtime, use the Debian/glibc image variant because the official Claurst Linux binary does not run reliably on Alpine/musl:

```sh
docker run --rm -p 8080:8080 \
  -v picoclip-data:/app/data \
  -v picoclip-workspaces:/app/workspaces \
  ghcr.io/brook-sys/picoclip:latest-debian
```

Then open:

```text
http://127.0.0.1:8080
```

You can also use `podman run` with the same arguments.

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
http://127.0.0.1:8080
```

Optional demo data:

```sh
make seed
```

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

- [Roadmap](docs/ROADMAP.md)
- [Current State](docs/CURRENT_STATE.md)
- [Storage Architecture](docs/STORAGE.md)
- [Development Guide](docs/DEVELOPMENT.md)

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
