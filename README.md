[ Experimenting with the idea ]

# lockie

Secret management for AI coding agents. Redact secrets in tool output and rehydrate them back when being used.

## Install

Pre-built binaries from GitHub Releases (no Go required):

```bash
curl -fsSL https://raw.githubusercontent.com/ujjalsharma100/lockie/main/scripts/install.sh | sh
```

Windows (PowerShell):

```powershell
iwr -useb https://raw.githubusercontent.com/ujjalsharma100/lockie/main/scripts/install.ps1 | iex
```

Pin a version:

```bash
LOCKIE_VERSION=v0.1.0 curl -fsSL .../install.sh | sh
```

## Quick start

```bash
lockie version
lockie install cursor --scope user
lockie install claude-code --scope user
lockie status
lockie add stripe STRIPE_SECRET_KEY   # example; value stored locally
```

The daemon auto-starts when hooks run. 

## From source (developers)

```bash
git clone https://github.com/ujjalsharma100/lockie.git
cd lockie
make build
./lockie version
```

Requires Go 1.23+.

## License

[Apache-2.0](LICENSE)
