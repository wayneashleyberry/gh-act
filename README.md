<center>
<h1>gh-act</h1>

> ✨ A GitHub (gh) CLI extension to manage, update and pin your GitHub Actions.

<img width="1265" alt="Image" src="https://github.com/user-attachments/assets/9efbd0d5-f83e-4d65-98f0-abab889a39dd" />
</center>

### Why?

> _“Pinning an action to a full length commit SHA is currently the only way to use an action as an immutable release. Pinning to a particular SHA helps mitigate the risk of a bad actor adding a backdoor to the action's repository, as they would need to generate a SHA-1 collision for a valid Git object payload. When selecting a SHA, you should verify it is from the action's repository and not a repository fork.” — [Security hardening for GitHub Actions](https://docs.github.com/en/actions/security-guides/security-hardening-for-github-actions#using-third-party-actions)_

You should keep your GitHub Actions up to date, and pinned, but this makes them difficult to maintain unless you rely on [Dependabot](https://github.com/dependabot). This is where `act` comes in, it is a simple extension for the GitHub CLI which helps you keep third-party dependencies up to date.

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

#### Help

```sh
gh act help
```

```
NAME:
   act - The unofficial GitHub Actions package manager

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
