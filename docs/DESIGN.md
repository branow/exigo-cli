# exigo-cli design

## Scope of this iteration

Exigo exposes at least three overlapping access mechanisms (SOAP `.asmx`,
a REST surface, and a newer per-tenant "API SDK" layer). The live API docs
document both SOAP and, per operation, a REST binding — method, path, and
field names — for most of the surface. We have no sandbox credentials yet.

**The CLI targets the REST API.** The wire protocol lives entirely inside
`internal/exigoapi`; the command-line UX follows the usual REST-CLI
conventions (`gh`-style noun-verb, JSON-friendly output).

Each operation's own documentation page turned out to carry a full,
authoritative schema — exact request/response field names, types, and
notes — plus the operation's REST call sample. That's been crawled and
distilled twice:

- A full SOAP schema reference (per-field types and notes — still the best
  field documentation, since the REST docs reuse the same shapes) lives in
  `api-catalog/`, kept locally as a gitignored research artifact alongside
  the crawl itself and the generation scripts (`research/`, `scripts/`).
- The REST bindings are distilled from that crawl into
  `internal/exigoapi/catalog/rest-catalog.json` — the only committed
  artifact of the pipeline — and embedded in the binary.
  252 operations are catalogued; 246 have REST
  bindings, and 6 are marked "Rest call not available for this method yet"
  (`AuthorizeOnlyCreditCardToken`, `AuthorizeOnlyCreditCardTokenOnFile`,
  `ChargePriorAuthorization`, `CreateTableFilterSettings`,
  `ProcessTransaction`, `Validate`) — the CLI reports those with a typed
  error and exit code 3 rather than guessing an endpoint.

What's still unconfirmed is anything that requires an actual account: real
response *values*, rate limits, and the REST error envelope — the docs
never show a populated error response, so the business-error extraction in
`internal/exigoapi` is deliberately defensive (top-level `errors` array, or
a `result` object carrying `errors`/`error`/`message`, mirroring the SOAP
`Errors[]` convention) and unverified against a live tenant. Those need
sandbox credentials.

This iteration delivers:

- A tested REST client: operation-name routing via the embedded catalog,
  HTTP Basic auth, method-aware retry with backoff, and decoding of both
  HTTP-level and business-level errors.
- `exigo api <Operation>` — a generic operation-invoker escape hatch (same
  idea as `gh api`) that can call any catalogued operation by name with
  `-f key=value` fields or an `--input` JSON object, without the CLI
  needing a hardcoded schema per operation. Operation names complete from
  the catalog in shell completion.
- Auth, config, and profile management, so multiple Exigo tenants
  (sandbox vs. production, multiple clients) can be configured.
- The embedded REST catalog.

Resource-specific commands (`exigo customer get`, `exigo order list`, ...)
are the next milestone — the schemas are known, but they're still untested
against a live account, so committing to typed Go structs for 252
operations before that verification would be premature.

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

Noun-verb pattern, fixed verb vocabulary, mirroring the conventions of
`gh`/`stripe`/`aws`.

## Auth model

Exigo credentials are `LoginName` + `Password` + `Company` (tenant code),
not an API key or OAuth token. The REST surface authenticates at the
transport level with HTTP Basic auth, username `login@company` — unlike
SOAP, which needed a custom `ApiAuthentication` header. So:

- `exigo auth login` prompts for login name, password (hidden input),
  company, and the REST base URL (default
  `https://<company>-api.exigo.com/3.0` — the API routes per tenant via a
  company-prefixed hostname — overridable per profile for sandbox hosts);
  stores them under a named profile.
- Credentials are stored in the OS keychain (`zalando/go-keyring`) with a
  plaintext-file fallback (0600) when no keychain backend is available,
  clearly warned about at write time.
- Preferences (current profile, output format, base URL) live in
  `~/.config/exigo/config.yml` (`%AppData%\exigo\config.yml` on Windows);
  secrets never enter that file.
- Config precedence: flags > env vars (`EXIGO_*`, e.g. `EXIGO_BASE_URL`) >
  user config file > built-in defaults.

## Wire protocol

`exigo api <Operation>` looks the operation up in the embedded catalog
(case-insensitively) and sends fields as query parameters for GET
operations, or as a JSON body for POST/PUT/PATCH/DELETE. Field names are
canonicalized case-insensitively to the documented casing, so
`CustomerID` and `customerID` are interchangeable; undocumented names pass
through unchanged, since the catalog samples may be incomplete. `-f` values
parse as JSON literals when they look like it (`42` → number, `true` →
bool) and as strings otherwise; `--input` accepts an arbitrary nested JSON
object for requests like an order with detail lines.

Retry is method-aware: GET retries 429/5xx/network errors up to 4 attempts
with jittered exponential backoff; mutating methods retry only on 429 —
the server explicitly refused the request before processing it — never on
5xx or network errors, which may have already applied the change (e.g.
double-creating an order). A completed 2xx response is never retried, even
if its body reports business errors.

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
  exigoapi/                  REST client: catalog routing, Basic auth,
                              method-aware retry/backoff, error decoding
    catalog/                 embedded rest-catalog.json + lookup helpers
  iostreams/                 stdin/stdout/stderr + TTY/color detection
  output/                    table and JSON writers
```

Each package has one responsibility; commands depend on `exigoapi.Client`
as an interface so they're testable without a network call.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | Generic/unexpected failure, incl. business errors reported by a completed call |
| 2 | User cancelled (aborted a confirmation prompt) |
| 3 | Validation error (bad flags/args, unknown operation, or an operation with no REST binding — caught before any API call) |
| 4 | Authentication/authorization failure (not logged in, HTTP 401/403) |
| 5 | Resource not found (HTTP 404, or a business error mentioning "not found") |
| 6 | Rate-limited/unavailable (HTTP 429, or 429/5xx after retries exhausted) |

The business-error → exit-code mapping is conservative because the error
vocabulary is unconfirmed: only the one low-risk "not found" inference is
made; everything else is exit 1.

## Testing approach

- `internal/config`, `internal/exigoapi`, `internal/output`: unit tests,
  no network — `exigoapi` tests use `httptest.Server`.
- `internal/credentials`: tests against the plaintext fallback (an
  interface swap, not a real OS keychain in CI).
- `cmd/`: Cobra command tests via `Execute()` with `SetArgs`/`SetOut` and a
  fake `cmdutil.Factory` pointed at an `httptest.Server`.
- No live-API or end-to-end tests against real Exigo infrastructure — that
  is blocked on sandbox credentials (tracked as a follow-up).
