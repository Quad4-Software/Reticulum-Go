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

Minimum bar before merge:

1. Thin raw layer over `include/rns.h` or Control API routes
2. Idiomatic ownership and errors above that layer
3. Automated tests in `bindings/<lang>/tests/` (or language equivalent)
4. Working examples under `bindings/<lang>/examples/`
5. CI script under `scripts/ci/` wired from Task and GitHub Actions

## AI-assisted bindings

LLMs may be used to create or update bindings. Generated work is not accepted on authorship alone.

Every new or changed binding must include:

- Proper automated tests
- Validation against the current ABI (`RNS_API_VERSION` / `include/rns.h`) or Control API protocol
- Runnable examples under `bindings/<lang>/examples/`

**Interop verification is required.** Behavior must be checked against:

1. **Go** - same stack via `pkg/node` / `pkg/librns` and Go tests or examples
2. **Python reference RNS** - official `rns` / NomadNet-style peers where the feature is mesh-visible (announce, path, link, request, resource, pageserver)

Unit tests that only exercise the FFI shim are not enough for protocol-facing APIs. Prefer live or recorded interop that proves bytes and events match Go and Python RNS.

## Further reading

- [docs/en/librns.md](../docs/en/librns.md)
- [docs/en/control-api.md](../docs/en/control-api.md)
- [docs/en/examples.md](../docs/en/examples.md)
- [COMPATIBILITY.md](../COMPATIBILITY.md)
