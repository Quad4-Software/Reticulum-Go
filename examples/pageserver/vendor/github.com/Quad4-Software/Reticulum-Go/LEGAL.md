# Legal

## License lineage

Reticulum-Go has been distributed under three licenses over its history.
The boundary commits below define which terms apply to which revision of the
source. For any checkout, the LICENSE file present at that commit is the
governing text; this file exists so recipients of older snapshots can map a
commit to its license without diffing LICENSE history.

| Range | License | Notes |
|-------|---------|-------|
| Through `ea0797de^` (2024-12-31 to 2025-12-28) | MIT | Earliest LICENSE text, added at `8df4039b` |
| `ea0797de` through `ca6228bf` (2025-12-28 to 2026-04-30) | 0BSD | `ea0797de` replaced MIT with 0BSD |
| `e3f8b415` and later (2026-04-30 onward) | Apache-2.0 | `e3f8b415` replaced the LICENSE text |

The 2026-04-30 transition was carried out as a commit series on that date:

- `ca6228bf` — SPDX headers in source files updated to Apache-2.0
- `9c09abe2` — NOTICE SPDX identifier updated to Apache-2.0
- `1d7c3e84` — README license text updated to Apache-2.0
- `f758ff3d` — CONTRIBUTING updated with signing requirements and CLA
- `e3f8b415` — LICENSE file replaced with Apache License 2.0

Because `ca6228bf` is the parent of `e3f8b415`, the practical boundary is
unambiguous: any commit at or before `ca6228bf` carries a LICENSE file
reading 0BSD, and `e3f8b415` onward carries Apache-2.0.

Full boundary SHAs:

```
ca6228bf6e8b33303e854ef9fa74b7c73087ace0  last commit under 0BSD
e3f8b415                                  first commit under Apache-2.0
ea0797de                                  first commit under 0BSD (was MIT before)
```

## Current terms

The project is licensed under Apache License 2.0. See LICENSE. Copyright
(c) 2024-2026 Quad4.io.

## Vendored code

Third-party source vendored under vendor/ remains under its own licenses,
itemized in NOTICE. Vendored license texts govern over this document.

## Contributions

See CONTRIBUTING.md for commit signing and CLA requirements.
