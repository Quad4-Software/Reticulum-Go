# Contributing

Contributions are welcome. Prefer sending changes as `.patch` files over Reticulum LXMF when you can. Pull requests and email are also fine. Reviews may take a few days.

## Quick start

1. Clone the repository. Dependencies live in `vendor/` (no network fetch for ordinary builds).
2. Install dev tools: `make bootstrap`.
3. Verify your environment: `make doctor`.
4. Branch from `master` for feature work.
5. Before pushing: `make prepush` or full `make check`.

Automation is available via Make (`make help`). Plain `go build` / `go test` also work with vendored modules.

Optional: use [mise](https://mise.jdx.dev/) (`mise install`) or the Dev Container (`.devcontainer/`) for pinned Go and tool versions.

## Branch workflow

- `master` is the mainline branch. Open pull requests against `master`.
- `tinygo` is a long-lived port branch for TinyGo builds.

## Commit messages

We use [Conventional Commits](https://www.conventionalcommits.org/) for commit subjects and PR titles.

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

Sign commits with `git commit -S`. The preferred method is `rngcs`, which signs with a Reticulum identity instead of a PGP key (ships with `rns`, see `pip install rns`):

```bash
rnid -g ~/.rngit/client_identity              # once, creates your signing identity
git config gpg.format ssh
git config gpg.ssh.program rngcs
git config gpg.ssh.allowedsignersfile none
git config user.signingkey ~/.rngit/client_identity
git config user.email <your identity hash>    # rngcs binds the author to the signer
```

The author field must equal the identity hash, or an LXMF address you can prove. GPG, SSH, and gitsign signatures are also accepted (`make gitsign-setup`, set `SIGSTORE_FULCIO_URL`, `SIGSTORE_REKOR_URL` and `SIGSTORE_OIDC_ISSUER` for a private Sigstore stack).

### Developer Certificate of Origin

Every commit must carry a `Signed-off-by:` trailer certifying the [Developer Certificate of Origin](https://developercertificate.org/). Add it with `git commit -s`. The `dco-signoff` CI job re-checks every PR commit.

## Repository modules

The repository holds two Go modules:

| Path | Module | Build and test |
|------|--------|----------------|
| `/` | `git.quad4.io/reticulum/Reticulum-Go` | `make check` |
| `protocols/` | `github.com/Quad4-Software/reticulum-go-protocols` | `task protocols-build`, `task protocols-test` |

Changes under `protocols/` run their own CI job. The docs site is a Zensical build of `docs/en`; check doc edits with `task docs-build`, which the `docs` CI job and the docs deploy workflow also run.

## Changelog and compatibility

- User-facing changes: add an entry under the current `[unreleased]` section in `CHANGELOG.md` (Keep a Changelog style).
- Wire format, RPC, or Python RNS parity changes: update `COMPATIBILITY.md`.
- Preview unreleased notes from conventional commits: `make changelog-preview`.

## Pull request checklist

- [ ] `make prepush` or `make check` passes locally
- [ ] `CHANGELOG.md` updated when behavior or UX changes
- [ ] `COMPATIBILITY.md` updated when wire or API compatibility changes
- [ ] Tests added or extended for behavior changes
- [ ] PR title follows Conventional Commits (required for squash merges)
- [ ] Every commit signed off (`git commit -s`, DCO)

See `SECURITY.md` for RSM signing and inventory details.

## CI overview

Required on pull requests (see `.github/workflows/ci.yml`):

| Job | Purpose |
|-----|---------|
| Lint | fmt-check, vet, revive, staticcheck, installer shellcheck |
| Test | Core Go tests, smoke, self-check |
| PR checks | Semantic PR title, DCO sign-off, signed-commit advisory |

Binding, OS matrix, legacy Windows, examples, and reproducibility jobs run on push to `master`, or when relevant paths change on pull requests.

Advisory or scheduled: CodeQL, security workflow, sim-heavy, TinyGo, preview-release.

## Releasing

Tagged GitHub releases are built and signed by `.github/workflows/release-assets.yml`. The same assets are also published to the rngit repository at `rns://06a54b505bb67b25ef3f8097e8001edc/public/Reticulum-Go`:

1. Tag the release and let the GitHub release pipeline finish (assets, checksums, SBOMs, cosign bundles).
2. Download the assets with `gh release download <tag>` or point the script at a local `dist/` tree.
3. Run `task release-rngit TAG=<tag>` (or `sh scripts/release/rngit-release.sh <tag>`). The script writes rngit `.rsg` signatures and `manifest.rsm` for every asset and uploads them to the remote with the matching CHANGELOG section as release notes.
4. Preview everything locally first with `--dry-run` (passes `-L` to `rngit release create`, generating signatures and the manifest without uploading).

Local parity:

```bash
make fmt-check vet lint staticcheck  # CI lint job
make check                            # lint + test-short + vulncheck + gosec
make prepush                          # fmt-check, vet, lint, test-short
```

## Licensing

Contributions are licensed to the project and its recipients under the project's current license.

## Contact

Send issues, suggestions, patches, or feedback to:

- Reticulum LXMF: `f489752fbef161c64d65e385a4e9fc74` (Ivan, Lead Maintainer)
- Email: `team@quad4.io`

## AI-assisted contributions

Open-weight LLMs, preferably operated locally under a controlled harness, may assist with non-critical tasks such as commit messages, documentation, drafts, translations, tests, examples, language bindings, and CLI utilities. LLMs may create new bindings and examples, and update existing ones. Generated binding work is not accepted on authorship alone. See [bindings/README.md](bindings/README.md).

LLMs are strictly excluded from cryptography, key handling, protocol security logic, and any other security-sensitive development. All LLM output is reviewed and approved by a human. Design decisions and security-critical changes are made exclusively by humans.
