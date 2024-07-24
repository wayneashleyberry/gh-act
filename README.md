## gh-act

> ✨ A GitHub (gh) CLI extension to manage, update and pin your GitHub Actions.

### Why?

> “Pinning an action to a full length commit SHA is currently the only way to use an action as an immutable release. Pinning to a particular SHA helps mitigate the risk of a bad actor adding a backdoor to the action's repository, as they would need to generate a SHA-1 collision for a valid Git object payload. When selecting a SHA, you should verify it is from the action's repository and not a repository fork.” — [Security hardening for GitHub Actions](https://docs.github.com/en/actions/security-guides/security-hardening-for-github-actions#using-third-party-actions)

You should keep your GitHub Actions up to date, and pinned, but this makes them difficult to maintain unless you rely on [Dependabot](https://github.com/dependabot). This is where `act` comes in, just like `npm` or `go mod` it is a simple command-line application which helps you keep third-party dependencies up to date.

### Installation

```
gh extension install wayneashleyberry/gh-act
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
