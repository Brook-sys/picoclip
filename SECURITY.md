# Security Policy

## Supported versions

Security fixes are applied to the `main` branch and, when practical, to the latest published release. Older releases are not guaranteed to receive backports.

## Reporting a vulnerability

Please **do not open a public issue** for a suspected vulnerability.

Use GitHub's private vulnerability reporting form:

https://github.com/Brook-sys/picoclip/security/advisories/new

Include, when possible:

- affected version or commit;
- impact and attack scenario;
- reproduction steps or a minimal proof of concept;
- affected files or endpoints;
- suggested mitigation, if known.

You should receive an initial acknowledgement within 7 days. We will investigate, coordinate a fix, and discuss disclosure timing before publishing details.

## Scope

Reports are especially useful for:

- authentication or authorization bypasses;
- command, path, SQL, or template injection;
- server-side request forgery (SSRF);
- exposure of API keys, tokens, workspace data, or runtime configuration;
- sandbox escapes or execution outside configured workspaces;
- vulnerabilities in release artifacts or GitHub Actions workflows.

Reports that require an already-compromised administrator account, unsupported local modifications, or denial of service through intentionally unbounded local operator configuration may be handled as hardening rather than security vulnerabilities.

## Disclosure

Please allow reasonable time for a fix before public disclosure. We will credit reporters unless they prefer to remain anonymous.
