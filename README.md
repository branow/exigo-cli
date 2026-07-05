# exigo-cli

A command-line interface for the [Exigo](https://www.exigo.com/) direct-selling
back-office API.

## Status

Authentication/profile management, config, and a generic
`exigo api <Operation>` escape hatch for the Exigo REST API are implemented
and tested against a mocked server. 252 operations are catalogued from the
live API docs and embedded in the binary
([`internal/exigoapi/catalog/rest-catalog.json`](internal/exigoapi/catalog/rest-catalog.json));
246 have REST bindings, and 6 that the docs mark "Rest call not available
for this method yet" (`AuthorizeOnlyCreditCardToken`,
`AuthorizeOnlyCreditCardTokenOnFile`, `ChargePriorAuthorization`,
`CreateTableFilterSettings`, `ProcessTransaction`, `Validate`) are reported
with a clear error and exit code 3.
Resource-specific commands (`customer`, `order`, etc.) are not yet built —
see [`docs/DESIGN.md`](docs/DESIGN.md) for why, and what's planned once
sandbox credentials are available.

## Install / build

Requires Go 1.25+.

```sh
go build -o exigo .
```

## Usage

```sh
# Log in (prompts for login name, password, company, and the REST base URL,
# defaulting to https://<company>-api.exigo.com/3.0)
exigo auth login

# Or non-interactively, e.g. for CI
echo "$EXIGO_PASSWORD" | exigo auth login --login-name svc-account --company ACME --password-stdin

# Check who's logged in
exigo auth status

# Invoke any named REST operation directly. Field names use the documented
# camelCase but match case-insensitively; values parse as JSON when they
# look like it (42 → number, true → bool), else as strings. List all
# catalogued operation names with `exigo api --list`.
exigo api GetCustomers -f customerID=42
exigo api CreateCustomer -f firstName=Jane -f lastName=Doe -f email=jane@example.com

# Requests with nested values (e.g. an order's detail lines) via --input
exigo api CreateOrder --input order.json

# Manage preferences
exigo config list
exigo config set output json
```

The operation name routes to its REST endpoint via the embedded catalog:
GET operations send fields as query parameters, everything else sends a
JSON body. Requests authenticate with HTTP Basic auth as
`login@company`. The base URL is per profile — override the per-company
default with `--base-url` at login, `EXIGO_BASE_URL`, or config.

Run `exigo --help` or `exigo <command> --help` for full usage, and see
`docs/DESIGN.md` for the exit-code scheme and command conventions. Shell
completion (`exigo completion ...`) completes operation names from the
catalog.

## How the REST catalog is generated

[`internal/exigoapi/catalog/rest-catalog.json`](internal/exigoapi/catalog/rest-catalog.json)
— the method, path, and documented field names per operation — is distilled
from a crawl of the live Exigo API docs. The crawl output, the full SOAP
schema reference derived from it, and the generation scripts are local
research artifacts (`research/`, `api-catalog/`, `scripts/`, all
gitignored); only the distilled JSON is committed, embedded into the
binary via `go:embed`. Treat it as generated — regenerate from a fresh
crawl rather than editing it by hand.

## Development

```sh
go build ./...
go vet ./...
go test ./...
```

No live-API tests exist yet — they're blocked on real Exigo sandbox
credentials.
