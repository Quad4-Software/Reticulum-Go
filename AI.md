# AI Provenance

Commits written with AI assistance carry provenance metadata at two levels.

## Commit trailers

Every AI-assisted commit ends with trailers:

```
Harness: Nullray
Model: Kimi K3
Method: Fireworks (ZDR)
```

- `Harness` is the agent/orchestration layer that drove the change.
- `Model` is the model that produced the code.
- `Method` is the inference path, including data-retention terms where
  relevant (for example ZDR = zero data retention).

`git log --format='%(trailers)'` or `git log --grep='^Model:'` audits them.
Trailers are metadata, not authorship; the human committer remains the
author of record.

## Per-commit notes

A machine-readable copy is attached on `refs/notes/ai-provenance`:

```
git log --show-notes=ai-provenance
git push origin refs/notes/ai-provenance
```

Each note is JSON:

```json
{"harness":"Nullray","model":"Kimi K3","method":"Fireworks (ZDR)","ts":"2026-10-01T09:00:00Z"}
```

Notes do not change commit SHAs and can be corrected after the fact
(`git notes --ref=ai-provenance add -f <sha>`).

## Setup on a machine

The hooks live in `.githooks/` and install once per clone:

```
sh scripts/ci/install-git-hooks.sh
```

Then configure the identity this machine reports:

```
git config --global ai.harness "Nullray"
git config --global ai.model   "Kimi K3"
git config --global ai.method  "Fireworks (ZDR)"
```

With the config present, every commit gains the trailers plus the note.
Unset the keys or use `SKIP_AI_HOOK=1 git commit ...` to opt out.

## Honest scope

This is self-reported provenance. It answers "which harness and model
produced this commit" for audit, debugging, and release review. It does not
cryptographically prove authorship and it is not an invitation to hide
agent use: if AI wrote it, say so in the trailers.
