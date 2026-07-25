# Language bindings

Host-language wrappers for Reticulum-Go. Prefer these packages when embedding the stack outside Go. Go applications should use `pkg/node` directly.

## Integration paths

| Path | Use when | Spec |
|------|----------|------|
| **librns** (in-process) | App embeds the mesh in the same process | [`include/rns.h`](../include/rns.h), [librns docs](../docs/en/librns.md) |
| **Control API** (out-of-process) | App talks to a running `reticulum-go` daemon | [control-api.md](../docs/en/control-api.md) |

Do not invent a third wire protocol. Application traffic belongs on peer destinations and links. The Control API is a local front end, not the mesh.

Build the shared library once:

```bash
task build-librns
```

Artifacts land in `bin/` (`librns.so` / `dylib` / `dll`) with the public header at `include/rns.h`.

## Packages in tree

| Directory | Integration | Notes |
|-----------|-------------|-------|
| [`c/`](c/) | librns C ABI | Examples only. Header is `include/rns.h`. |
| [`odin/`](odin/) | librns | ABI 1.5 reference wrap |
| [`zig/`](zig/) | librns | `@extern` wrappers |
| [`cpp/`](cpp/) | librns | C++17 RAII |
| [`rust/`](rust/) | librns | Safe Rust over `extern` |
| [`python/`](python/) | librns | ctypes |
| [`lua/`](lua/) | librns | LuaJIT FFI |
| [`swift/`](swift/) | librns | SwiftPM over C ABI |
| [`java/`](java/) | librns | JNA over C ABI |
| [`kotlin/`](kotlin/) | librns | Kotlin facade over Java JNA |
| [`dart/`](dart/) | librns FFI + Control API | Flutter-ready Control client |

Each package keeps demos under `bindings/<lang>/examples/` (typically `smoke`, `page-fetch`, `pageserver`). Go-only samples stay under [`examples/`](../examples/).

## Build, test, examples

```bash
task build-librns
task test-odin      # or test-zig, test-cpp, test-dart, test-rust, test-python, test-lua, test-swift, test-java, test-kotlin, test-c
make -C bindings/<lang> examples
```

CI runs package tests and example builds per language. See [development-and-testing](../docs/en/development-and-testing.md).

## Adding or updating a binding

Follow [`SCAFFOLD`](SCAFFOLD). Mirror an existing package for idioms. Required surface, ABI rules, and acceptance checks are documented there.

Minimum bar before merge is the Correctness and interop section below (ABI lock, tests, examples, CI, and Go plus Python peer checks for mesh-visible APIs).

## AI-assisted bindings

LLMs may be used to create or update bindings. Generated work is not accepted on authorship alone. See Correctness and interop below.

## Correctness and interop

A binding is not finished when it compiles. It must prove the host language talks to the same stack Go and Python RNS use, with the same ownership and error rules as `include/rns.h` or the Control API.

### Correctness bar

Every new or changed binding must include all of the following.

1. **ABI or protocol lock**
   - librns: call `version()` / `rns_version` early and require `RNS_API_VERSION` from the header the binding was written against
   - Control API: match routes and event or command names in `pkg/controlapi/protocol.go` and [control-api.md](../docs/en/control-api.md)
2. **Thin raw layer** over `include/rns.h` or Control `/v1` routes. No reimplemented wire protocol.
3. **Idiomatic ownership** above that layer: destroy on drop or defer, no silent handle copies, fixed 16-byte destination hashes at the safe API.
4. **Error mapping** that preserves C codes or Control failures (invalid handle, timeout, truncated buffers) instead of swallowing them.
5. **Automated tests** under `bindings/<lang>/tests/` (or language equivalent) covering at least:
   - version non-empty and ABI match
   - node create / start / stop / destroy
   - identity generate and hash length
   - destination create and hash length
   - event poll timeout returns TIMEOUT (librns)
6. **Runnable examples** under `bindings/<lang>/examples/` (typically `smoke`, and where feasible `page-fetch` / `pageserver`)
7. **CI** via `scripts/ci/run-<lang>-bindings.sh` and example build through `scripts/ci/run-binding-examples.sh`

Stronger tests (required when the binding exposes the feature): link open / send / close over local UDP or Auto, request register / incoming / respond, path table snapshot. Mirror patterns in `bindings/cpp/tests/` and SCAFFOLD.

### Interop bar

**Interop verification is required** for any API that is mesh-visible or shares crypto or identity bytes with peers.

Check against both stacks:

| Peer | What to prove | Typical approach |
|------|---------------|------------------|
| **Go** | Same node stack via `pkg/node` / `pkg/librns` | Match Go examples under `examples/` or binding demos against Go pageserver / page-fetch. Package tests in `pkg/librns` and live suites under `tests/interop/` |
| **Python RNS** | Official `rns` (1.4.1) / NomadNet-style peers | Live announce, path, link, request, resource, or pageserver against Python helpers in `tests/interop/py/`. Crossref vectors in `tests/crossref/` for byte-level crypto and packet parity |

What counts as interop evidence:

- Destination hashes, announce app data, link payloads, and request paths match byte-for-byte with Go and Python peers
- Event kinds and ordering match the C ABI / Control event model (announce, link established or failed, link data, request incoming or response)
- Page fetch / pageserver demos work against a Go or Python peer on the same config style used in [examples.md](../docs/en/examples.md)
- Where live mesh is impractical in CI, recorded vectors or loopback UDP two-node tests in the binding package are acceptable if they still exercise the real librns binary

What does **not** count:

- Unit tests that only load the shared library and call `version()`
- Mock FFI that never links `librns.so`
- Docs or screenshots without an automated or scripted peer check
- Claiming parity without running against Python RNS for features that leave the process (announce, path, link, request, resource, pageserver)

### How to run checks

```bash
task build-librns
task test-<lang>          # package tests + examples build
make -C bindings/<lang> examples

# Go / Python live mesh (optional local, CI where enabled)
RUN_LIVE_INTEROP=1 go test -v ./tests/interop/...
task test-crossref
```

Authoritative protocol status: [COMPATIBILITY.md](../COMPATIBILITY.md). Live harness notes: [development-and-testing.md](../docs/en/development-and-testing.md).

## Further reading

- [docs/en/librns.md](../docs/en/librns.md)
- [docs/en/control-api.md](../docs/en/control-api.md)
- [docs/en/examples.md](../docs/en/examples.md)
- [docs/en/development-and-testing.md](../docs/en/development-and-testing.md)
- [COMPATIBILITY.md](../COMPATIBILITY.md)
