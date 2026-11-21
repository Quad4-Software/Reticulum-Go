# Language bindings

Host-language wrappers for reticulum-go-protocols. Prefer these packages when embedding RRC, MF, LXMF, or LXST codecs outside Go. Go applications should use `pkg/rrc`, `pkg/mf`, `pkg/lxmf`, and `pkg/lxst` directly.

## Integration paths

| Path | Use when | Spec |
|------|----------|------|
| **librrc** | App embeds RRC codec and hub/client in-process | [`include/rrc.h`](../include/rrc.h) |
| **libmf** | MF packet codec only | [`include/mf.h`](../include/mf.h) |
| **liblxmf** | LXMF message codec only | [`include/lxmf.h`](../include/lxmf.h) |
| **liblxst** | LXST telephony wire codec only | [`include/lxst.h`](../include/lxst.h) |

Build shared libraries:

```bash
task build-bindings    # librrc, libmf, liblxmf, liblxst into bin/
```

Artifacts land in `bin/` with public headers under `include/`.

## Packages in tree

| Directory | Integration | Notes |
|-----------|-------------|-------|
| [`python/`](python/) | librrc, libmf, liblxmf, liblxst | ctypes |
| [`rust/`](rust/) | librrc | Safe Rust over `extern` |
| [`java/`](java/) | librrc, liblxmf | JNA over C ABI (`io.quad4.rrc`, `io.quad4.lxmf`) |
| [`kotlin/`](kotlin/) | liblxmf | Thin Kotlin facade over Java JNA (`io.quad4.lxmf.kotlin`) |
| [`lua/`](lua/) | librrc | LuaJIT FFI |
| [`c/`](c/) | librrc, libmf, liblxst | Direct C examples |
| [`cpp/`](cpp/) | librrc, libmf, liblxst | CMake smoke tests |

Each RRC binding keeps demos under `bindings/<lang>/examples/` (`smoke`, `codec-roundtrip`, `hub-client`). Codec bindings add `mf-smoke` / `lxmf-smoke` / `lxst-smoke` (C) or `lxmf-roundtrip` / `lxst-roundtrip` (Python, Java, Kotlin).

## Build, test, examples

```bash
task build-bindings
task test-bindings     # python, rust, java, kotlin, lua, c, cpp
task test-codec        # mf/lxmf/lxst Go + Python/Java/Kotlin codec tests
task test-hub-client   # live UDP loopback hub-client (Go + all RRC bindings)
```

Per-language:

```bash
make -C bindings/<lang> test
make -C bindings/<lang> examples
```

## Adding or updating a binding

Follow [`SCAFFOLD`](SCAFFOLD). Mirror an existing package for idioms.

## Correctness bar

Every RRC binding must include:

1. ABI lock: call `version()` early and require `RRC_API_VERSION` from the header
2. Thin raw layer over `include/rrc.h`
3. Idiomatic ownership above that layer
4. Error mapping that preserves C codes
5. Automated tests covering version, envelope round-trip, node lifecycle, identity hash
6. Runnable `examples/smoke`
7. CI via `scripts/ci/run-<lang>-bindings.sh`

Live hub/client mesh tests use `examples/hub-client` over local UDP loopback.

MF, LXMF, and LXST codec bindings must round-trip pack/unpack in CI via `task test-codec`.
