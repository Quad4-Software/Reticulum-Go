# Contributing

Contributions are welcome. Prefer sending changes as `.patch` files over Reticulum LXMF when you can. Pull requests and email are also fine. Reviews may take a few days.

## Quick start

1. Clone the repository. Dependencies live in `vendor/` (no network fetch for ordinary builds).
2. Install dev tools: `task bootstrap` (or `make bootstrap`).
3. Verify your environment: `task doctor` (or `make doctor`).
4. Enable git hooks: `task hooks:install` (or `make hooks-install`).
5. Branch from `dev` for feature work.
6. Before pushing: `task prepush` or full `task check`.

Primary automation is available via **Make** and **Task** (`make help`, `task --list`). Use whichever you prefer. Plain `go build` / `go test` also work with vendored modules.

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

Sign commits with GPG or SSH when practical (`git commit -S`), or use gitsign for keyless x509 signatures (`task gitsign:setup`; set `SIGSTORE_FULCIO_URL`, `SIGSTORE_REKOR_URL` and `SIGSTORE_OIDC_ISSUER` to sign against a private Sigstore stack). Use a real email address or LXMF address in the Git author field.

### Developer Certificate of Origin

Every commit must carry a `Signed-off-by:` trailer certifying the DCO and the contributor license grant below. Add it with `git commit -s`. The commit-msg hook enforces it and the `dco-signoff` CI job re-checks every PR commit.

Skip the commit-msg hook for one commit: `SKIP_COMMIT_MSG_HOOK=1 git commit ...`
Skip only the DCO check: `SKIP_DCO_HOOK=1 git commit ...`

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
- [ ] Every commit signed off (`git commit -s`, DCO + license grant)
- [ ] RSM hook skipped only when intentional (`SKIP_TREE_RSM_HOOK=1` with reason in PR)

## Git hooks

After `task hooks:install`:

| Hook | Runs |
|------|------|
| `pre-commit` | Staged Go fmt/vet, YAML, shellcheck, optional `reticulum-go.rsm` resign |
| `commit-msg` | Conventional commit format, DCO sign-off |
| `pre-push` | `task prepush` (fmt-check, vet, lint, test-short) |

Skip env vars:

| Variable | Skips |
|----------|-------|
| `SKIP_LINT_HOOK=1` | All pre-commit lint steps |
| `SKIP_GO_HOOK=1` | Staged Go fmt/vet |
| `SKIP_YAML_HOOK=1` | YAML checks |
| `SKIP_SHELLCHECK_HOOK=1` | shellcheck |
| `SKIP_TREE_RSM_HOOK=1` | RSM resign |
| `SKIP_COMMIT_MSG_HOOK=1` | commit-msg format and DCO |
| `SKIP_DCO_HOOK=1` | DCO sign-off only |
| `SKIP_PREPUSH=1` | pre-push checks |

See `SECURITY.md` for RSM signing and inventory details.

## CI overview

Required on pull requests (see `.github/workflows/ci.yml`):

| Job | Purpose |
|-----|---------|
| Lint | fmt-check, vet, revive, staticcheck, installer shellcheck |
| Test | Core Go tests, smoke, self-check |
| PR checks | Semantic PR title, DCO sign-off, signed-commit advisory |

Binding, OS matrix, legacy Windows, examples, and reproducibility jobs run on push to `dev`/`master`, or when relevant paths change on pull requests.

Advisory or scheduled: CodeQL, security workflow, sim-heavy, TinyGo, preview-release.

Local parity:

```bash
task ci          # fmt-check, vet, lint, staticcheck (CI lint job)
task check       # ci checks + test-short + gosec + vulncheck
task prepush     # fmt-check, vet, lint, test-short
```

## Contributor License Grant (CLA)

You keep the copyright in your contribution. The `Signed-off-by` trailer on each commit certifies both the DCO and this grant, so every commit carries the certification in permanent history, tied to the author identity.

By signing off a contribution, you certify that:

- You have the right to submit it and are not breaching any obligation to an employer, client, or third party.
- You grant **Quad4** a perpetual, worldwide, non-exclusive, irrevocable, royalty-free license to use, reproduce, modify, distribute, and sublicense the contribution as part of this project, on the condition that outbound distribution stays under the **Reticulum License** (see `LICENSE`) or a successor license adopted for this project that preserves its attribution, no-harm, and AI-training terms.
- You grant Quad4 and recipients of this project a perpetual, worldwide, non-exclusive, irrevocable, royalty-free patent license under the claims you own or control that are necessarily infringed by the contribution, to make, use, and distribute the contribution as part of this project.
- Where rights in the contribution cannot be licensed under applicable law, such as non-waivable moral rights, you agree not to assert them against Quad4 or recipients of this project to the maximum extent permitted.

For substantial contributions such as a new package, binding, or interface, we may also ask for a signed acceptance of this grant (GPG-signed email or LXMF message to the maintainer), which we keep on file under `LEGAL/`.

## Contact

Send issues, suggestions, patches, or feedback to:

- **Reticulum LXMF:** `f489752fbef161c64d65e385a4e9fc74` (Ivan, Lead Maintainer)
- **Email:** `team@quad4.io`

## AI-assisted contributions

Open-weight LLMs, preferably operated locally under a controlled harness, may assist with non-critical tasks such as commit messages, documentation, drafts, translations, tests, examples, language bindings, and CLI utilities. LLMs may create new bindings and examples, and update existing ones. Generated binding work is not accepted on authorship alone. See [bindings/README.md](bindings/README.md).

LLMs are strictly excluded from cryptography, key handling, protocol security logic, and any other security-sensitive development. All LLM output is reviewed and approved by a human. Design decisions and security-critical changes are made exclusively by humans.
