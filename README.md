# Giddy Up

Giddy Up is a way for Up Bank customers to view and track their own budget and spending via a Terminal User Interface (TUI). More to come.

## Security-first auth setup (no .env)

This project does not use `.env` files for secrets.

Set your PAT once with:

```bash
go run ./cmd/giddyup auth set
```

The command prompts for your PAT with hidden input and stores it in your OS credential store.
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

If you want custom key names:

```bash
GIDDYUP_KEYCHAIN_SERVICE=my-service GIDDYUP_KEYCHAIN_ACCOUNT=my-account go run ./cmd/giddyup
```

Do not pass PAT as a CLI argument (for example `--pat=...`), because command-line arguments can be exposed in shell history and process listings.

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
