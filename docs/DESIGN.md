# PicoClip Design System

The visual design is composed of simple layout primitives in `internal/adapters/web/ui.templ` and utility CSS in `app.css`.

## Core Components

- **Card / Panel**: General purpose container with borders and padding.
- **SectionHeader**: A standardized title section with a right-aligned action slot.
- **Badge / Status**: Semantic color indicators (`good`, `warn`, `bad`, `info`). Use `.pulse` for live tasks.
- **PropertyList**: Horizontal/grid description lists (`dl`) for key-value views.
- **Button / LinkButton**: Primary and `.secondary` interactions.
- **Form / FormField**: Unified labels, inputs, and validation wrappers.
- **EmptyState**: Visual feedback when lists are empty.
- **table-wrapper**: Required div wrapper around any `table` to handle horizontal overflow on mobile screens without collapsing rows.

## CSS Architecture

- **Variables**: Located in `:root`. Uses simple colors (`--bg`, `--surface`, `--accent`, `--border`, `--good`, etc). Supports Dark Mode via `[data-theme="dark"]`.
- **Layout**: The shell is built with CSS Grid (`.app-shell`).
- **Typography**: Inter (or system sans-serif).
- **Responsive**: 
  - `max-width: 980px`: Transitions sidebar to top nav.
  - `max-width: 680px`: Tightens padding and adjusts multi-column grids to 1 column.

## Modals & Popovers
Modals use `<dialog>` patterns conceptually but are managed via `.modal-backdrop` and explicit javascript helpers globally defined in `layout.templ` to toggle `.modal-open` on the body.

## Extensibility
Do not add ad-hoc classes. When styling a new view, compose `.panel`, `.stacked`, `.detail-grid`, and `.section-header`.
