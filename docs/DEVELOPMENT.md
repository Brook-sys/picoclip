# PicoClip Development Guide

This guide describes the repeatable local development, debug, and test workflow for PicoClip.

## Project structure

- `cmd/picoclip/main.go` — application entrypoint.
- `internal/adapters/web/server.go` — route registration.
- `internal/adapters/web/html_handlers.go` — server-rendered HTML handlers and HTMX partial handlers.
- `internal/adapters/web/views.go` — HTML rendering helpers. The project currently uses manual Go rendering, not `.templ` files.
- `internal/adapters/web/assets/` — CSS and static assets.
- `internal/core/` — domain, ports, and application services.
- `internal/adapters/storage/memory/` — in-memory repositories.
- `e2e/` — Playwright browser tests.

## Runtime configuration

Defaults:

```sh
BIND=0.0.0.0
PORT=8080
```

Run manually:

```sh
BIND=0.0.0.0 PORT=8080 go run cmd/picoclip/main.go
```

## Install development tools

```sh
make tools
```

This installs:

- `templ` CLI
- `air` live reload CLI

Playwright is managed through npm/npx:

```sh
npm install
npx playwright install chromium
```

## templ workflow

The module includes `github.com/a-h/templ`, but there are currently no `.templ` files. The command is still part of the validation workflow so future templ files are generated consistently.

```sh
make templ-generate
```

Equivalent raw command:

```sh
templ generate
```

Expected current behavior: succeeds and produces no files when no `.templ` files exist.

## Live reload with air

```sh
make dev
```

or:

```sh
BIND=0.0.0.0 PORT=8080 air -c .air.toml
```

Air builds to `tmp/picoclip` and restarts when Go, CSS, JS, HTML, or templ files change.

## Build and validation

```sh
make fmt
make test-go
make vet
make build
```

Full check:

```sh
make check
```

`make check` runs:

1. `templ generate`
2. `gofmt -w cmd internal`
3. `go test ./...`
4. `go vet ./...`
5. `go build -o picoclip cmd/picoclip/main.go`
6. Playwright E2E tests

## Go route and handler tests

Go tests live next to the package they test. Current web tests are in:

```text
internal/adapters/web/server_test.go
```

Run:

```sh
make test-go
```

These tests cover:

- Agent task lifecycle API: create, checkout, block, comment/reopen.
- Task detail HTMX contract: no polling that swaps the entire body; live updates use `/partials/tasks/{id}`.

## Browser E2E tests with Playwright

Start the app in another terminal:

```sh
make dev
```

Then run:

```sh
make test-e2e
```

Headed mode:

```sh
make test-e2e-headed
```

The E2E runner uses a system browser when available. On Alpine, Playwright's downloaded Ubuntu fallback browser may not run because of native library incompatibilities. Install Chromium on the host/container or set `PLAYWRIGHT_CHROMIUM_EXECUTABLE`:

```sh
apk add chromium
PLAYWRIGHT_CHROMIUM_EXECUTABLE=/usr/bin/chromium make test-e2e
```

If no compatible browser is available on Alpine, `make test-e2e` prints a clear skip message and exits successfully; Go tests still cover route and HTMX contracts. The Playwright MCP browser requires a compatible Chrome/Chromium as well; if it reports `/opt/google/chrome/chrome` missing, install Chrome/Chromium in the environment before using MCP browser actions.

The E2E suite checks:

- Primary pages load with no console errors or failed requests.
- Agent/task creation works through the real UI.
- Task detail remains stable while HTMX polling runs.
- Comments update the live task fragment.
- Agent API supports a Paperclip-like lifecycle.

Artifacts:

- `e2e/test-results/`
- `e2e/playwright-report/`

## HTMX quality rules

Avoid polling containers that swap the whole page.

Bad:

```html
<section hx-get="/tasks/{id}" hx-trigger="every 2s" hx-target="body" hx-swap="outerHTML">
```

Good:

```html
<div id="task-live" hx-get="/partials/tasks/{id}" hx-trigger="every 3s" hx-swap="innerHTML">
```

Rules:

- Poll only small fragments.
- Keep forms, buttons, and modals outside polling containers.
- Use partial routes for live sections.
- Do not re-render `<body>` on timers.
- Check browser console after HTMX changes.

## Debug checklist

When changing UI or handlers:

1. Run `make test-go`.
2. Start app with `make dev`.
3. Open affected page in browser.
4. Check console messages.
5. Check Network tab or Playwright request failures.
6. Run `make test-e2e`.
7. Run `make check` before considering the change complete.

## Useful commands

```sh
make help
make kill-8080
make clean
```
