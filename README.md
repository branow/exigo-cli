# exigo-cli

A command-line interface for the [Exigo](https://www.exigo.com/) direct-selling
back-office API.

## Status

Early scaffold: authentication/profile management, config, and a generic
`exigo api <Operation>` escape hatch for the Exigo SOAP API are implemented
and tested against a mocked server. Resource-specific commands (`customer`,
`order`, etc.) are not yet built — see [`docs/DESIGN.md`](docs/DESIGN.md)
for why, and what's planned once sandbox credentials are available.

## Install / build

Requires Go 1.25+.

```sh
go build -o exigo .
```

## Usage

```sh
# Log in (prompts for login name, password, company, endpoint)
exigo auth login

# Or non-interactively, e.g. for CI
echo "$EXIGO_PASSWORD" | exigo auth login --login-name svc-account --company ACME --password-stdin

# Check who's logged in
exigo auth status

# Invoke any named SOAP operation directly
exigo api GetCustomers -f CustomerID=12345
exigo api CreateCustomer -f FirstName=Jane -f LastName=Doe -f Email=jane@example.com

# Manage preferences
exigo config list
exigo config set output json
```

Run `exigo --help` or `exigo <command> --help` for full usage, and see
`docs/DESIGN.md` for the exit-code scheme and command conventions.

## Development

```sh
go build ./...
go vet ./...
go test ./...
```

No live-API tests exist yet — they're blocked on real Exigo sandbox
credentials.
