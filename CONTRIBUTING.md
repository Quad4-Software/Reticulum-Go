# Contributing

Contributions are welcome. Prefer sending changes as `.patch` files over Reticulum LXMF when you can. Pull requests and email are also fine. Reviews may take a few days.

## Quick start

1. Clone the repository. Dependencies live in `vendor/` (no network fetch for ordinary builds).
2. Install dev tools: `task bootstrap` (or `make bootstrap`).
3. Verify your environment: `task doctor` (or `make doctor`).
4. Enable git hooks: `task hooks:install` (or `make hooks-install`).
5. Branch from `dev` for feature work.
6. Before pushing: `task prepush` or full `task check`.

Primary automation is **Task** (`task --list`). **Make** targets are aliases for common workflows.

Optional: use [mise](https://mise.jdx.dev/) (`mise install`) or the Dev Container (`.devcontainer/`) for pinned Go and Task versions.

## Branch workflow

- `dev` is the integration branch for ongoing work.
- `master` is release-stable.
- Open pull requests against `dev` unless you are fixing a release-only issue.

## Commit messages

We use [Conventional Commits](https://www.conventionalcommits.org/) (enforced by the `commit-msg` hook when hooks are installed).

### Format

```
<type>(<scope>): <short summary>

[optional body]

[optional footer]
```

### Types

| Type | Use for |
|------|---------|
| `feat` | New behavior or capability |
| `fix` | Bug fix |
| `refactor` | Code change without behavior change |
| `chore` | Tooling, deps, housekeeping |
| `docs` | Documentation only |
| `test` | Tests only |
| `ci` | CI and automation |
| `perf` | Performance improvement |
| `build` | Build system or packaging |

### Scope

Optional lowercase scope for the area touched: `transport`, `rnsgit`, `bindings/rust`, `cli`, `ci`, etc.

### Examples

```
feat(rnsgit): add mirror push with permission checks
fix(transport): defer IFAC until path resolved
chore(ci): pin staticcheck to v0.6.1
docs: expand development and testing guide
```

Sign commits with GPG or SSH when practical (`git commit -S`). Use a real email address or LXMF address in the Git author field.

Skip the commit-msg hook for one commit: `SKIP_COMMIT_MSG_HOOK=1 git commit ...`

## Changelog and compatibility

- User-facing changes: add an entry under the current `[unreleased]` section in `CHANGELOG.md` (Keep a Changelog style).
- Wire format, RPC, or Python RNS parity changes: update `COMPATIBILITY.md`.
- Preview unreleased notes from conventional commits: `task changelog-preview`.

## Pull request checklist

The PR template mirrors this list:

- [ ] `task prepush` or `task check` passes locally
- [ ] `CHANGELOG.md` updated when behavior or UX changes
- [ ] `COMPATIBILITY.md` updated when wire or API compatibility changes
- [ ] Tests added or extended for behavior changes
- [ ] PR title follows Conventional Commits (required for squash merges)
- [ ] RSM hook skipped only when intentional (`SKIP_TREE_RSM_HOOK=1` with reason in PR)

## Git hooks

After `task hooks:install`:

| Hook | Runs |
|------|------|
| `pre-commit` | Staged Go fmt/vet, YAML, shellcheck, optional `reticulum-go.rsm` resign |
| `commit-msg` | Conventional commit format |
| `pre-push` | `task prepush` (fmt-check, vet, lint, test-short) |

Skip env vars:

| Variable | Skips |
|----------|-------|
| `SKIP_LINT_HOOK=1` | All pre-commit lint steps |
| `SKIP_GO_HOOK=1` | Staged Go fmt/vet |
| `SKIP_YAML_HOOK=1` | YAML checks |
| `SKIP_SHELLCHECK_HOOK=1` | shellcheck |
| `SKIP_TREE_RSM_HOOK=1` | RSM resign |
| `SKIP_COMMIT_MSG_HOOK=1` | commit-msg format |
| `SKIP_PREPUSH=1` | pre-push checks |

See `SECURITY.md` for RSM signing and inventory details.

## CI overview

Required on pull requests (see `.github/workflows/ci.yml`):

| Job | Purpose |
|-----|---------|
| Lint | fmt-check, vet, revive, staticcheck, installer shellcheck |
| Test | Core Go tests, smoke, self-check |
| PR checks | Semantic PR title, signed-commit advisory |

Binding, OS matrix, legacy Windows, examples, and reproducibility jobs run on push to `dev`/`master`, or when relevant paths change on pull requests.

Advisory or scheduled: CodeQL, security workflow, sim-heavy, TinyGo, preview-release.

Local parity:

```bash
task ci          # fmt-check, vet, lint, staticcheck (CI lint job)
task check       # ci checks + test-short + gosec + vulncheck
task prepush     # fmt-check, vet, lint, test-short
```

## Contributor License Agreement (CLA)

By submitting a contribution, you agree that:

- You have the right to submit it and are not breaching any obligation to an employer, client, or third party.
- You assign to **Quad4** the copyright and related rights you hold in that contribution, or where assignment is not possible, grant Quad4 a perpetual, irrevocable, worldwide, royalty-free license (including the right to sublicense) to use, reproduce, modify, distribute, and prepare derivative works of the contribution.
- The contribution is provided for distribution under the **Apache License, Version 2.0** (see `LICENSE`) as part of this project.

## Contact

Send issues, suggestions, patches, or feedback to:

- **Reticulum LXMF:** `f489752fbef161c64d65e385a4e9fc74` (Ivan, Lead Maintainer)
- **Email:** `team@quad4.io`
