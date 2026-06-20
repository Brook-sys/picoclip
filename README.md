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

## Roadmap

There is an active roadmap, and more features will be added gradually as the project matures.

See:

- [Roadmap](docs/ROADMAP.md)
- [Current State](docs/CURRENT_STATE.md)
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
