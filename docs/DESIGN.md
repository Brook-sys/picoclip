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

## Tokens

The CSS foundation lives in `internal/adapters/web/assets/app.css` and follows a three-layer token model inspired by Anvil2, implemented with plain CSS variables:

- Primitive tokens: `--a2-neutral-*`, `--a2-blue-*`, `--a2-green-*`, `--a2-red-*`, `--a2-yellow-*`.
- Semantic tokens: `--a2-background-color-primary`, `--a2-surface-color`, `--a2-surface-hover`, `--a2-foreground-color-primary`, `--a2-foreground-color-secondary`, `--a2-border-color-default`, `--a2-border-color-hover`, `--a2-focus-color`.
- Spacing tokens: `--space-1`, `--space-2`, `--space-3`, `--space-4`, `--space-5`, `--space-6`, `--space-8`.
- Shape/elevation tokens: `--radius-sm`, `--radius-md`, `--radius-lg`, `--shadow-sm`, `--shadow-md`.

Legacy variables such as `--bg`, `--surface`, `--text`, `--muted`, `--border`, `--good`, `--bad`, `--warn` and `--info` are mapped to the new token layer so old templates can migrate gradually.

## Templ Components

Defined in `internal/adapters/web/ui.templ` and `internal/adapters/web/icons.templ`:

- `Icon`: inline SVG icon component, no webfont or external dependency.
- `Button`, `ButtonLink`, `IconButton`: action primitives with `primary`, `secondary`, `ghost` and `danger` variants.
- `Card`, `CardEdit`: content containers.
- `SectionHeader`: section title, description and action slot.
- `PropertyList`, `PropertyItem`, `PropertyItemHTML`: detail rail metadata.
- `FieldLabel`, `FieldMessage`, `FormField`: accessible form wrapper and helper text.
- `TextField`, `TextareaField`, `SelectField`: styled form controls.
- `TagMultiSelectField`: chip-based tag editor backed by a lightweight custom element.
- `Chip`, `Badge`, `StatusBadge`: compact metadata/status display.
- `EmptyState`: icon-based empty content state.
- `Tabs`, `TabLink`: page-level tabs.
- `EntityRow`: compact linked list row with icon and arrow.

When adding UI, prefer composing these components before introducing new CSS.

## Iconography

Icons are inline SVGs rendered by `Icon(name, size)`. This avoids icon fonts, external requests and runtime dependencies. Add only icons that are actually used by the interface.

Current icon names include:

- `check`
- `x`
- `chevron-down`
- `chevron-right`
- `plus`
- `trash`
- `settings`
- `play`
- `stop`
- `users`
- `folder`
- `check-circle`
- `cpu`
- `zap`
- `book`
- `bar-chart`

## Interactive Components

The shell registers lightweight vanilla custom elements in `layout.templ`. These are intentionally small and compatible with HTMX swaps.

### `pc-tag-multiselect`

Use `TagMultiSelectField(name, selectedTags, availableTagsJSON)` for tag inputs.

Behavior:

- client-side filtering over the provided tag list;
- creates new tags inline when the typed value does not exist;
- renders selected values as chips;
- submits standard hidden inputs, so normal form handlers keep working;
- supports keyboard navigation with ArrowUp, ArrowDown, Enter, Escape and Backspace.

Server handlers should read repeated `tags` values before falling back to legacy newline textareas.

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
