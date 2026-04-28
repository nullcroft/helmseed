# helmseed

> **Warning**: The `main` branch may be in a broken state. Use [releases/tags](https://github.com/nullcroft/helmseed/releases) to get a stable working version.

helmseed bootstraps golden-image Helm charts from a GitHub org or GitLab group into your application repository. It clones chart repos into `.helm/charts/`, manages a local cache to avoid redundant fetches, and generates Helm-compliant `Chart.yaml` and `Chart.lock` files.

## How it works

1. **List** -- queries your git provider for all repositories in the configured org/group.
2. **Filter** -- optionally narrows the list to repos matching a name prefix (e.g. `chart-`).
3. **Select** -- presents an interactive TUI where you pick which charts to include.
4. **Cache** -- shallow-clones selected repos into `~/.cache/helmseed/` with a configurable TTL. Subsequent runs skip repos whose cache entry is still fresh.
5. **Bootstrap** -- copies cached chart contents into `.helm/charts/` in your working directory, stripping `.git` metadata.
6. **Helm files** -- generates `.helm/Chart.yaml` (umbrella chart with dependencies) and `.helm/Chart.lock` (pinned versions).
7. **Update** -- force re-fetches all locked charts, overwrites `.helm/charts/`, and regenerates both Helm files.

## Installation

Requires Go 1.26+.

```sh
git clone https://github.com/nullcroft/helmseed.git
cd helmseed
make build
```

The binary is written to `./bin/helmseed`.

## Configuration

helmseed reads configuration from `helmseed.yaml` in the current directory. All values can also be set via environment variables with the `HELMSEED_` prefix (e.g. `HELMSEED_TOKEN`).

```yaml
# Git provider: "github" or "gitlab"
provider: github

# GitHub org name or GitLab group path
group: my-org

# Personal access token (required, can also use HELMSEED_TOKEN env var)
token: ghp_...

# GitLab only: override base URL for self-hosted instances
# base_url: https://gitlab.example.com

# Only include repos whose name starts with this prefix
# prefix: chart-

# How long cached repos are considered fresh (Go duration syntax)
# Defaults to 24h
# cache_ttl: 24h

# Output directory for charts (default: .helm)
# charts_dir: .helm

# Cache directory (must be absolute)
# Default: $XDG_CACHE_HOME/helmseed if set, otherwise ~/.cache/helmseed
# cache_dir: /var/cache/helmseed

# Helm chart metadata for generated Chart.yaml
# chart_name: my-app
# chart_description: Umbrella chart for my-app

# Skip interactive confirmation prompts (equivalent to --yes flag)
# non_interactive: false
```

### Required fields

| Field      | Description                                  |
|------------|----------------------------------------------|
| `provider` | `github` or `gitlab`                         |
| `group`    | GitHub organization or GitLab group path     |
| `token`    | Personal access token for API and clone auth |

> **Security note**: prefer the `HELMSEED_TOKEN` environment variable for the token, especially in CI or shared workstations. If you must place the token in `helmseed.yaml`, make sure that file is gitignored and never committed. The config file stores the token in plaintext.

### Optional fields

| Field            | Description                                        | Default            |
|-----------------|----------------------------------------------------|-------------------|
| `base_url`      | GitLab base URL for self-hosted instances         | none              |
| `prefix`        | Only include repos with names starting with this  | none              |
| `cache_ttl`    | Duration before a cached repo is considered stale | `24h`             |
| `charts_dir`    | Output directory for Helm charts            | `.helm`           |
| `cache_dir`    | Cache directory for cloned repos (must be absolute) | `$XDG_CACHE_HOME/helmseed` or `~/.cache/helmseed` |
| `chart_name`     | Name field in generated Chart.yaml         | `placeholder`     |
| `chart_description`| Description field in generated Chart.yaml| `placeholder`     |
| `non_interactive`| Skip confirmation prompts                  | `false`           |

## Usage

### List available repos

Print all repos in the configured org/group (after prefix filtering):

```sh
helmseed list
```

### Bootstrap charts

Interactively select repos and clone them into `.helm/charts/`:

```sh
helmseed bootstrap              # interactive TUI selection
helmseed bootstrap --local      # use file:// paths in Chart.lock
helmseed bootstrap --remote     # explicit remote URLs (default)
helmseed bootstrap --all, -a    # select all matching repos (skip TUI)
helmseed bootstrap --dry-run    # preview without executing
```

This opens a TUI multi-select. Use `j`/`k` or arrow keys to navigate, `space` to toggle, `a` to select all, and `enter` to confirm. Press `q` or `ctrl+c` to abort.

Charts already present in `.helm/charts/` are skipped. Both `.helm/Chart.yaml` and `.helm/Chart.lock` are written after bootstrap completes. When a `prefix` is configured, it is stripped from chart names in the output directories and Helm files.

### Update charts

Force re-fetch all charts listed in `.helm/Chart.lock` and overwrite the local copies:

```sh
helmseed update            # asks for confirmation
helmseed update --yes      # skips confirmation
```

This ignores the cache TTL, re-clones every locked chart, replaces the contents under `.helm/charts/`, and regenerates both `.helm/Chart.yaml` and `.helm/Chart.lock`.

### Global flags

All commands support these flags:

```sh
--yes, -y          # skip confirmation prompts
--dry-run, -d      # show what would be done without executing
--quiet, -q        # suppress non-essential output
--verbose, -v      # enable debug-level logging to stderr
--config, -c FILE  # specify config file (default: ./helmseed.yaml)
```

### Print version

```sh
helmseed version
```

## Cache

Cloned repos are cached at `<cache_dir>/<repo-name>/`. The cache directory is resolved in this order:

1. `cache_dir` from `helmseed.yaml` (if set, must be absolute)
2. `$XDG_CACHE_HOME/helmseed` (if `XDG_CACHE_HOME` is set and absolute)
3. `~/.cache/helmseed`

Each entry contains the repo files (with `.git` stripped) and a `meta.json` with clone metadata:

```json
{
  "cloned_at": "2026-04-22T10:00:00Z",
  "clone_url": "git@github.com:my-org/chart-postgres.git",
  "default_branch": "main",
  "commit": "abc123..."
}
```

Cache entries older than the configured TTL are re-cloned on the next `bootstrap`. The `update` command ignores TTL and always re-fetches.

## Helm files

Both files are written by `bootstrap` and `update` and should be committed to version control.

### `.helm/Chart.yaml`

```yaml
apiVersion: v2
name: placeholder
description: placeholder
version: 0.1.0
type: application
dependencies:
- name: postgres
  version: 1.2.0
  repository: https://github.com/my-org/chart-postgres
- name: redis
  version: 0.5.1
  repository: https://github.com/my-org/chart-redis
```

### `.helm/Chart.lock`

```yaml
generated: "2026-04-22T10:00:00.000000000Z"
digest: sha256:abc123def456...
dependencies:
- name: postgres
  repository: https://github.com/my-org/chart-postgres
  version: 1.2.0
- name: redis
  repository: https://github.com/my-org/chart-redis
  version: 0.5.1
```

## Makefile targets

| Target  | Description                                          |
|---------|------------------------------------------------------|
| `all`   | Run `clean`, `test`, `lint`, `build` (default target)|
| `build` | Compile binary to `./bin/helmseed`                   |
| `test`  | Run all tests                                        |
| `lint`  | Run `golangci-lint`                                  |
| `tidy`  | Run `go mod tidy`                                    |
| `clean` | Remove `./bin/`                                      |
| `purge` | `clean` + remove `./.helm/` and `~/.cache/helmseed`  |

## License

See LICENSE file for details.
