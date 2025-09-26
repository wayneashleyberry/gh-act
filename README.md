## gh-act

> ✨ A GitHub (gh) CLI extension to manage, update and pin your GitHub Actions.

[![Go Reference](https://pkg.go.dev/badge/github.com/wayneashleyberry/gh-act.svg)](https://pkg.go.dev/github.com/wayneashleyberry/gh-act)
[![Go Report Card](https://goreportcard.com/badge/github.com/wayneashleyberry/gh-act)](https://goreportcard.com/report/github.com/wayneashleyberry/gh-act)
[![Lint](https://github.com/wayneashleyberry/gh-act/actions/workflows/lint.yaml/badge.svg)](https://github.com/wayneashleyberry/gh-act/actions/workflows/lint.yaml)
[![Dependabot Updates](https://github.com/wayneashleyberry/gh-act/actions/workflows/dependabot/dependabot-updates/badge.svg)](https://github.com/wayneashleyberry/gh-act/actions/workflows/dependabot/dependabot-updates)
[![Release](https://github.com/wayneashleyberry/gh-act/actions/workflows/release.yaml/badge.svg)](https://github.com/wayneashleyberry/gh-act/actions/workflows/release.yaml)

### Why?

> _“Pinning an action to a full length commit SHA is currently the only way to use an action as an immutable release. Pinning to a particular SHA helps mitigate the risk of a bad actor adding a backdoor to the action's repository, as they would need to generate a SHA-1 collision for a valid Git object payload. When selecting a SHA, you should verify it is from the action's repository and not a repository fork.” — [Security hardening for GitHub Actions](https://docs.github.com/en/actions/security-guides/security-hardening-for-github-actions#using-third-party-actions)_

You should keep your GitHub Actions up to date, and pinned, but this makes them difficult to maintain unless you rely on [Dependabot](https://github.com/dependabot). This is where `act` comes in, it is a simple extension for the GitHub CLI which helps you keep third-party dependencies up to date.

### Branch Reference Support

`gh-act` now supports branch references (like `@main`, `@master`, etc.) when using the `--pin` flag. This is particularly useful for actions that don't follow semantic versioning or when you want to pin the latest stable version from the default branch.

**Features:**

- Automatically detects branch references in your workflow files
- Validates that the branch is the repository's default branch (for security)
- Resolves the branch to the latest tagged version when pinning
- Works with all standard commands (`update --pin`, `outdated`, `pin`)

**Example:**

```diff
- uses: actions/checkout@main
+ uses: actions/checkout@08c6903cd8c0fde910a37f88322edcfb5dd907a8 # v5.0.0
```

### Installation

Installation is a single command if you already have the [GitHub CLI](https://cli.github.com) installed:

```
gh extension install wayneashleyberry/gh-act
```

The [GitHub CLI](https://cli.github.com) also manages updates:

```
gh extension upgrade --all
```

### Usage

#### List actions

```sh
gh act ls
```

#### Pin actions

```sh
gh act pin
```

#### Update actions

```sh
gh act update --pin
```

#### Update actions with branch references

When you have actions using branch references (like `@main`), use the `--pin` flag to convert them to pinned versions:

```sh
# This will convert actions like:
# - uses: actions/checkout@main
# To pinned versions like:
# - uses: actions/checkout@08c6903cd8c0fde910a37f88322edcfb5dd907a8 # v5.0.0
gh act update --pin
```

#### Help

```sh
gh act help
```

```
NAME:
   act - Update, manage and pin your GitHub Actions

USAGE:
   act [global options] command [command options]

COMMANDS:
   ls        List used actions
   outdated  Check for outdated actions
   update    Update actions
   pin       Pin used actions
   help, h   Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --debug     Print debug logs (default: false)
   --help, -h  show help
```
