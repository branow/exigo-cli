# exigo-cli design

## Scope of this iteration

Exigo exposes at least three overlapping access mechanisms (SOAP `.asmx`,
an OData-style REST surface, and a newer per-tenant "API SDK" layer). Of
these, only the SOAP surface at `https://api.exigo.com/3.0/ExigoApi.asmx`
is publicly reachable and verifiable today — it is live, documents 140+
operations via its auto-generated service description page, and its WSDL
is fetchable (confirmed directly, not from search snippets). The OData/REST
docs domains have expired TLS certs and could not be verified at all (see
`research/exigo-api-research.md`, gitignored, for the raw findings). We
have no sandbox credentials yet.

**v1 therefore targets the confirmed SOAP API**, not the unconfirmed REST
surface — building against a guessed, unreachable schema would silently
ship wrong behavior, while the SOAP surface is real and testable today.
The CLI's command-line UX still follows REST-CLI conventions (`gh`-style
noun-verb, JSON-friendly output) even though the wire protocol underneath
is SOAP/XML — that distinction is entirely hidden inside `internal/exigoapi`.

v1 delivers:

- A correct, tested SOAP client (envelope construction, custom
  `ApiAuthentication` header, XML (de)serialization, `Errors[]` decoding)
  plus the auth/config/output foundation around it.
- `exigo api <Operation>` — a generic operation-invoker escape hatch (same
  idea as `gh api`) that can call *any* of the 140+ named SOAP operations
  by name with `-f key=value` fields, without the CLI needing a hardcoded
  schema per operation.
- Auth, config, and profile management, so multiple Exigo tenants
  (sandbox vs. production, multiple clients) can be configured.

Resource-specific commands (`exigo customer get`, `exigo order list`, ...)
are the next milestone once sandbox access confirms exact per-operation
request/response field shapes beyond what the WSDL's type names tell us.

## Command surface (v1)

```
exigo auth login [--profile NAME] [--login-name ...] [--company ...] [--base-url ...]
exigo auth logout [--profile NAME]
exigo auth status
exigo auth switch --profile NAME
exigo config get <key>
exigo config set <key> <value>
exigo config list
exigo api <Operation> [-f key=value ...] [--input FILE]
exigo completion bash|zsh|fish|powershell
exigo version
```

Noun-verb pattern, fixed verb vocabulary, mirrors `gh`/`stripe`/`aws` per
`research/cli-best-practices.md`.

## Auth model

Exigo credentials are `LoginName` + `Password` + `Company` (tenant code),
not an API key or OAuth token (see research doc §2). The SOAP surface
requires these three values in a custom `ApiAuthentication` SOAP header on
every request — **not** HTTP Basic auth (a documented gotcha: several
integrators tried Basic auth first and got confusing "operation not found"
errors until they built the header correctly). So:

- `exigo auth login` prompts for login name, password (hidden input),
  company, and the SOAP endpoint URL (default
  `https://api.exigo.com/3.0/ExigoApi.asmx`, overridable per profile for
  sandbox hosts); stores them under a named profile.
- Credentials are stored in the OS keychain (`zalando/go-keyring`) with a
  plaintext-file fallback (0600) when no keychain backend is available,
  clearly warned about at write time.
- Preferences (current profile, output format) live in
  `~/.config/exigo/config.yml`; secrets never enter that file.
- Config precedence: flags > env vars (`EXIGO_*`) > user config file >
  built-in defaults.

## Package layout

```
main.go                     entry point, calls cmd.Execute()
cmd/                         thin Cobra commands; no business logic
  root.go                    root command, persistent flags, factory wiring
  auth_login.go / auth_logout.go / auth_status.go / auth_switch.go
  config.go
  api.go
  completion.go
  version.go
internal/
  cmdutil/                   Factory struct: IOStreams + Config + ClientFn
  config/                    Config struct, load/save, profile resolution
  credentials/               keychain-backed store + plaintext fallback
  exigoapi/                  SOAP client: envelope + ApiAuthentication
                              header, retry/backoff, Errors[]/fault decoding
  iostreams/                 stdin/stdout/stderr + TTY/color detection
  output/                    table and JSON writers
```

Each package has one responsibility; commands depend on `exigoapi.Client`
as an interface so they're testable without a network call.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | Generic/unexpected failure |
| 2 | User cancelled (aborted a confirmation prompt) |
| 3 | Validation error (bad flags/args, caught before any API call) |
| 4 | Authentication/authorization failure |
| 5 | Resource not found (API-reported not-found error) |
| 6 | Rate-limited/retryable error (HTTP 429/5xx after retries exhausted) |

## Testing approach

- `internal/config`, `internal/exigoapi`, `internal/output`: unit tests,
  no network — `exigoapi` tests use `httptest.Server`.
- `internal/credentials`: tests against the plaintext fallback (an
  interface swap, not a real OS keychain in CI).
- `cmd/`: Cobra command tests via `Execute()` with `SetArgs`/`SetOut` and a
  fake `cmdutil.Factory` pointed at an `httptest.Server`.
- No live-API or end-to-end tests against real Exigo infrastructure — that
  is blocked on sandbox credentials (tracked as a follow-up).
