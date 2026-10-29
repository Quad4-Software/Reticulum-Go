# Legal

## License lineage

reticulum-go-protocols was initialized with a 0BSD LICENSE file while its
source headers were mixed 0BSD and Apache-2.0, inherited from the
Reticulum-Go extraction. Reticulum-Go itself carried MIT, then 0BSD, then
Apache-2.0 before moving to the Reticulum License; see
https://github.com/Quad4-Software/Reticulum-Go LEGAL.md for that lineage.

| Range | License | Notes |
|-------|---------|-------|
| Through `10c8905` | 0BSD (LICENSE) with mixed 0BSD/Apache-2.0 headers | Headers carried over from the Reticulum-Go extraction |
| After `10c8905` | Reticulum License | LICENSE replaced and all SPDX headers unified to LicenseRef-Reticulum |

## Why the Reticulum License

This library implements application protocols on top of the Reticulum
protocol and tracks the Python reference implementation, which has been
published under the Reticulum License since RNS 0.9.4 (April 2025). The
Reticulum License is permissive with three conditions: no use in systems
that purposefully harm human beings, no use in AI/ML training datasets, and
preservation of the copyright and permission notices. Aligning with the
reference implementation keeps the terms unambiguous for derived portions.
See https://reticulum.network/manual/brandolinis.html for the upstream
rationale.

## Current terms

The project is licensed under the Reticulum License. See LICENSE. Copyright
(c) 2026 Quad4.io. Portions derived from the Reticulum Network Stack
reference implementation are Copyright (c) 2016-2026 Mark Qvist.

## Vendored code

Third-party source vendored under vendor/ remains under its own licenses.
Vendored license texts govern over this document.
