# Giddy Up

**Unofficial product: Giddy Up is not affiliated with, endorsed by, sponsored by, or connected to Up in any way.**

<p align="center">
    <img width="464" height="560" alt="GiddyUp Logo" src="https://github.com/user-attachments/assets/cef765bd-1f9e-4c35-8aee-933d35f75f1c" />
  </p>

Giddy Up is a way for Up customers to view and track their own budget and spending via a Terminal User Interface (TUI). More to come.

## Up API acceptable use

This product is designed for personal, local use only:

- Personal use only: this software is for each user's personal usage.
- Your token only: each user must use their own Up Personal Access Token (PAT).
- No token sharing: do not share PATs with any third party, including partners/relatives.
- Reasonable request volume only: do not abuse, disrupt, or overload the API.
- No probing undocumented endpoints: only call documented API routes.
- No unauthorized data access: only access your own data.
- No commercial data extraction: do not extract data (for example merchant data) for commercial applications.
- User-responsible token security: users are responsible for securing their own PAT.
- No uptime guarantee: API availability may change and access may be suspended without notice.

Giddy Up does not run a backend service for user data. API calls are made directly from the user's machine to Up using the user's own PAT.

## Security-first auth setup (no .env)

This project does not use `.env` files for secrets.

Set your PAT once with:

```bash
go run ./cmd/giddyup auth set
```

The command prompts for your PAT with hidden input and stores it in your OS credential store.
`giddyup auth set` stores the PAT locally in the user's system keychain only. At no point is the PAT sent to any server or third-party service.
At runtime, PAT loading order is:

1. `UP_PAT` environment variable, if set.
2. System credential store (`service=giddyup`, `account=up_pat` by default).

Run the app (it reads Keychain automatically):

```bash
go run ./cmd/giddyup
```

Verify API connectivity:

```bash
go run ./cmd/giddyup ping
```

The command prints `connected successfully` only when `/util/ping` returns HTTP 200.

## Up API client layout

Routes are grouped by endpoint type under `internal/upapi`:

- `accounts.go`
- `attachments.go`
- `categories.go`
- `tags.go`
- `transactions.go`
- `utilities.go`

Paginated list routes send `page[size]=15`.

## Integration tests

Run integration tests for all non-webhook route groups:

```bash
go test -tags=integration ./internal/upapi -v
```

If you want custom key names:

```bash
GIDDYUP_KEYCHAIN_SERVICE=my-service GIDDYUP_KEYCHAIN_ACCOUNT=my-account go run ./cmd/giddyup
```

Do not pass PAT as a CLI argument (for example `--pat=...`), because command-line arguments can be exposed in shell history and process listings.

## Local database

Initialize local database storage:

```bash
go run ./cmd/giddyup
```

The app auto-initializes the database on normal startup if it does not already exist.

Delete local stored database data:

```bash
go run ./cmd/giddyup db wipe
```

Storage modes:

- `secure` mode only: encrypted-at-rest SQLite (SQLCipher), with DB key stored in system keychain.

Build requirement:

```bash
go build -tags sqlcipher ./cmd/giddyup
```

If the app is run on a build without SQLCipher support, startup fails with a clear error.

Optional DB path override:

```bash
GIDDYUP_DB_PATH=/custom/path/giddyup.db go run ./cmd/giddyup db wipe
```

Default DB path (when `GIDDYUP_DB_PATH` is not set):

- `<directory containing the giddyup executable>/giddyup.db`

## Pre-commit secret scanning

Install `gitleaks`:

```bash
brew install gitleaks
```

Enable repo-managed hooks:

```bash
./scripts/setup-hooks.sh
```

The pre-commit hook blocks commits when secrets are detected.
