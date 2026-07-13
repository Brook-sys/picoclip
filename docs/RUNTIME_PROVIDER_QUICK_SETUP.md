# Runtime Provider Quick Setup

PicoClip exposes a **Quick Setup** form for AI runtimes that support an OpenAI-compatible endpoint. The initial scope is one PicoClip-managed provider profile per runtime with three fields:

- Base URL
- API key
- Model

Supported runtimes:

| Runtime | Native files written | Managed entry |
| --- | --- | --- |
| Crush | `crush.json` | provider `picoclip-openai` plus `large`/`small` model aliases |
| PicoClaw | `config.json` and `.security.yml` | model `picoclip-default`; API keys remain in the mode-`0600` security file |
| Claurst | `settings.json` | active `openai` provider and `config.model` |

Bubblewrap is a sandbox runtime and does not expose provider Quick Setup.

## Preservation and concurrency

Quick Setup merges only the PicoClip-managed entry. Existing providers, models, MCP servers, tools, and advanced options are preserved. A revision fingerprint covers every native file touched by the form; a stale form is rejected with HTTP `409 Conflict` instead of overwriting concurrent advanced edits.

For runtimes added through **Use Existing**, PicoClip derives the native configuration paths when no stored `ConfigPath` exists.

## Secret behavior

- API keys are never returned in the Quick Setup view.
- A blank API key field preserves the current key.
- **Remove configured API key** is the explicit deletion action.
- The advanced editor displays secret-shaped values as `[REDACTED]` and restores their stored values when the placeholder is submitted unchanged.
- PicoClaw keeps model API keys in `.security.yml` with file mode `0600`.

Saving configuration does not contact an AI provider and does not consume tokens. **Test AI** remains a separate action and asks for confirmation before sending a real request.

Conflict responses use HTTP `409` and set `HX-Refresh: true` so HTMX reloads the Settings page with the latest revision instead of leaving a stale form stuck.

## Advanced configuration

The complete native configuration remains available under the collapsed **Advanced configuration** section. The web handler accepts only editable files explicitly returned by the runtime adapter and validates JSON/YAML before writing.

The first version intentionally leaves multiple managed providers, fallback chains, model discovery, custom headers, and presets to the advanced editor or future work.
