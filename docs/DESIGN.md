# PicoClip Design System

PicoClip uses a lightweight server-rendered design system built with Templ, HTMX and plain CSS. The goal is to keep the UI attractive, consistent and easy to extend without adding a heavy SPA framework.

## Principles

- Server-rendered first.
- Components over ad-hoc markup.
- Small CSS surface with design tokens.
- HTMX partials for live or interactive regions.
- View models prepare page data before rendering templates.
- Prefer progressive disclosure for advanced controls.
- Destructive operations require clear danger styling and typed confirmation.

## Navigation Architecture

The shell groups navigation by product area:

- **Control**: Dashboard, Tasks, Runs, Activity.
- **Core**: Projects, Agents, Skills.
- **Admin**: Settings.

This keeps future additions predictable. New pages should be placed into one of these groups instead of appending random links.

## Layout Components

Core CSS/layout primitives:

- `.app-shell`: fixed sidebar + main content.
- `.sidebar`: grouped navigation.
- `.page-header`: breadcrumb, title, subtitle and action slot.
- `.content-grid`: primary + secondary column layout.
- `.detail-grid`: main column + right rail.
- `.grid-cols-2`, `.grid-cols-3`, `.grid-cols-4`: responsive metric/action grids.
- `.stack`, `.stack-sm`: vertical composition helpers.

## Templ Components

Defined in `internal/adapters/web/ui.templ`:

- `Card`
- `SectionHeader`
- `PropertyList`
- `PropertyItem`
- `PropertyItemHTML`
- `FormField`
- `Badge`
- `StatusBadge`
- `EmptyState`
- `Tabs`
- `TabLink`
- `EntityRow`

When adding UI, prefer composing these components before introducing new CSS.

## Task Detail Pattern

Task detail pages should use a `detail-grid`:

- main column: prompt, conversation, subtask tree, runs and activity;
- right rail: properties, workflow actions, delegation and danger zone;
- task comments remain separate from system activity;
- runs remain separate from comments and link to `/runs/{id}`.

## Live Observability

Activity now supports Server-Sent Events through `/sse/activity`. The Activity page keeps a standard server-rendered initial timeline and progressively enhances it with live events when `EventSource` is available.

## Runs Pattern

Runs are first-class observability records:

- `/runs` lists executions with status tabs;
- `/runs/{id}` shows output, error, input context and metadata;
- duration and attempt metadata belong in the run detail rail;
- running runs use a focused `/partials/runs/{id}` polling region instead of refreshing the full page.

## Dashboard Pattern

The dashboard should behave like an operational command center:

- top row: open work, running runs, blocked work and failed executions;
- left column: work that needs attention and active runs;
- right column: system health and recent activity;
- avoid hiding critical failures below the fold.

## Data Display

Use:

- `.table-wrapper` around every table.
- `StatusBadge` for lifecycle/status output.
- `Badge` for metadata and labels.
- `EntityRow` for compact linked lists.
- `PropertyList` for detail rails.

## Command Palette

The shell includes a lightweight command palette:

- open with `Ctrl+K` / `Cmd+K`;
- search tasks, agents, projects, skills and quick commands;
- results are powered by `/api/search`;
- use Enter to navigate to the selected result.

## Interaction Patterns

- Use HTMX for form submissions and partial refreshes.
- Avoid polling entire pages; poll fragments only.
- Use `hx-target="body" hx-swap="outerHTML"` for simple full-page form refreshes.
- Use dedicated `/partials/...` routes for live sections.
- Use toast feedback via the shell for non-GET HTMX requests.

## Modals

Modals are controlled by `data-open-modal` and `data-close-modal` from `layout.templ`.

Rules:

- Keep modals outside polling fragments.
- Escape closes all modals.
- The first focusable element receives focus when opened.
- Destructive modals should use `.border-danger`, `.text-danger`, and typed confirmation.

## Extensibility Rules

- Do not create one-off visual patterns for new features.
- Add a reusable component if the same shape appears twice.
- Keep page-specific CSS rare.
- Prefer ViewModels in Go for derived display data.
- Every new primary page should have a short E2E smoke check.
