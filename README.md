# exigo-cli

A command-line interface for the [Exigo](https://www.exigo.com/)
direct-selling back-office REST API.

`exigo` can invoke any of the 252 operations documented in the Exigo API —
`GetCustomers`, `CreateOrder`, `CalculateOrder`, ... — by name, from your
terminal or a script, without writing any integration code.

- Credentials are stored in the OS keychain (macOS Keychain, Windows
  Credential Manager, Linux Secret Service), never in plain files.
- Named profiles switch between tenants and environments (production,
  sandbox).
- JSON output, meaningful exit codes, and non-interactive flags make it
  scriptable; shell completion covers commands and operation names.

## Installation

**Homebrew** (macOS and Linux):

```sh
brew install branow/tap/exigo
```

**Scoop** (Windows):

```powershell
scoop bucket add branow https://github.com/branow/scoop-bucket
scoop install exigo
```

**Linux packages**: `deb`, `rpm`, and `apk` packages are attached to the
[latest release](https://github.com/branow/exigo-cli/releases/latest), e.g.:

```sh
sudo dpkg -i exigo_*_linux_amd64.deb
```

**Shell script** (Linux and macOS; installs to `/usr/local/bin`, or
`~/.local/bin` when that is not writable):

```sh
curl -fsSL https://raw.githubusercontent.com/branow/exigo-cli/main/scripts/install.sh | sh
```

**Manual**: download the archive for your platform (macOS, Linux, Windows;
amd64 and arm64) from the
[latest release](https://github.com/branow/exigo-cli/releases/latest),
unpack it, and put the `exigo` binary on your `PATH`:

```sh
tar xzf exigo_*_darwin_arm64.tar.gz   # .zip on Windows
sudo mv exigo /usr/local/bin/
```

**Go** 1.25+:

```sh
go install github.com/branow/exigo-cli@latest   # installs as "exigo-cli"
```

or build from a checkout with `go build -o exigo .`

## Quick start

```sh
# Log in: prompts for login name, company, REST base URL, and password,
# then verifies the credentials against the API before storing them.
exigo auth login

# Call an operation
exigo api GetCustomers -f customerID=42
```

The base URL defaults to the production host
`https://<company>-api.exigo.com/3.0`; point it at a sandbox host at the
login prompt or with `--base-url`.

## Usage

### Calling API operations

```sh
exigo api --list                        # all 252 operation names
exigo api GetCustomers -f customerID=42
exigo api CreateCustomer -f firstName=Jane -f lastName=Doe -f email=jane@example.com
exigo api CreateOrder --input order.json
```

The operation name (case-insensitive) routes to its documented REST
endpoint: `-f key=value` fields become query parameters for GET operations
and the JSON request body otherwise. Values that look like JSON are typed
(`42` a number, `true` a boolean, `[1,2]` an array); everything else is a
string. `--input file.json` (or `--input -` for stdin) sends a full JSON
object — use it for nested requests such as an order's detail lines.

### Profiles and configuration

```sh
exigo auth login --profile sandbox --base-url https://sandboxapi1.exigo.com/3.0
exigo auth switch --profile sandbox
exigo auth status
exigo config list
exigo config set output json
```

Each profile stores a base URL, company, and preferred output format.
Settings resolve as flags > environment variables (`EXIGO_PROFILE`,
`EXIGO_BASE_URL`, `EXIGO_COMPANY`, `EXIGO_OUTPUT`) > config file
(`~/.config/exigo/config.yml`; `%AppData%\exigo\config.yml` on Windows) >
defaults.

### Scripting

Log in non-interactively and rely on exit codes:

```sh
echo "$EXIGO_PASSWORD" | exigo auth login --login-name svc --company ACME --password-stdin
```

| Exit code | Meaning |
|---|---|
| 0 | success |
| 1 | generic failure, including business errors reported by the API |
| 2 | cancelled by the user |
| 3 | validation error (bad flags, unknown operation) |
| 4 | authentication failure (not logged in, HTTP 401/403) |
| 5 | not found (HTTP 404) |
| 6 | rate-limited or unavailable (HTTP 429, 5xx after retries) |

## Development

```sh
go build ./...
go vet ./...
go test ./...
```

Architecture, command conventions, and design decisions are described in
[`docs/DESIGN.md`](docs/DESIGN.md). The embedded operation catalog
([`rest-catalog.json`](internal/exigoapi/catalog/rest-catalog.json)) is
generated from a crawl of the live Exigo API docs — treat it as a build
artifact, not something to edit by hand.

## License

[MIT](LICENSE)
