# Legal

## License lineage

Reticulum-Go has been distributed under four licenses over its history.
The boundary commits below define which terms apply to which revision of the
source. For any checkout, the LICENSE file present at that commit is the
governing text; this file exists so recipients of older snapshots can map a
commit to its license without diffing LICENSE history.

| Range | License | Notes |
|-------|---------|-------|
| Through `ea0797de^` (2024-12-31 to 2025-12-28) | MIT | Earliest LICENSE text, added at `8df4039b` |
| `ea0797de` through `ca6228bf` (2025-12-28 to 2026-04-30) | 0BSD | `ea0797de` replaced MIT with 0BSD |
| `e3f8b415` through `5abea4ac` (2026-04-30 to 2026-09-18) | Apache-2.0 | `e3f8b415` replaced the LICENSE text |
| After `5abea4ac` (2026-09-18 onward) | Reticulum License | LICENSE replaced and SPDX headers updated in a single commit |

The 2026-04-30 transition was carried out as a commit series on that date:

- `ca6228bf` - SPDX headers in source files updated to Apache-2.0
- `9c09abe2` - NOTICE SPDX identifier updated to Apache-2.0
- `1d7c3e84` - README license text updated to Apache-2.0
- `f758ff3d` - CONTRIBUTING updated with signing requirements and CLA
- `e3f8b415` - LICENSE file replaced with Apache License 2.0

Because `ca6228bf` is the parent of `e3f8b415`, the practical boundary is
unambiguous: any commit at or before `ca6228bf` carries a LICENSE file
reading 0BSD, and `e3f8b415` onward carries Apache-2.0.

The 2026-09-18 transition to the Reticulum License replaced LICENSE and
updated all SPDX headers (`LicenseRef-Reticulum`), README, NOTICE,
CONTRIBUTING and docs in one commit. `5abea4ac` is the last commit under
Apache-2.0.

Full boundary SHAs:

```
5abea4ac                                  last commit under Apache-2.0
ca6228bf6e8b33303e854ef9fa74b7c73087ace0  last commit under 0BSD
e3f8b415                                  first commit under Apache-2.0
ea0797de                                  first commit under 0BSD (was MIT before)
```

## Why the Reticulum License

This implementation tracks the Reticulum protocol and the Python reference
implementation, which has been published under the Reticulum License since
RNS 0.9.4 (April 2025). The Reticulum License is a permissive license with
three conditions: no use in systems that purposefully harm human beings, no
use in AI/ML training datasets, and preservation of the copyright and
permission notices. Aligning this project's license with the reference
implementation keeps the terms unambiguous for portions of the codebase
that derive from post-0.9.4 upstream source. See
https://reticulum.network/manual/brandolinis.html for the upstream rationale.

## Current terms

The project is licensed under the Reticulum License. See LICENSE. Copyright
(c) 2024-2026 Quad4.io. Portions derived from the Reticulum Network Stack
reference implementation are Copyright (c) 2016-2026 Mark Qvist.

## Vendored code

Third-party source vendored under vendor/ remains under its own licenses,
itemized in NOTICE. Vendored license texts govern over this document.

## Contributions

See CONTRIBUTING.md for commit signing and CLA requirements.

## Legal contact

For any questions, corrections, or feedback about licensing, please contact legal@quad4.io.
