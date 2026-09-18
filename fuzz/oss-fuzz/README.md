# OSS-Fuzz integration

This directory holds the files submitted to google/oss-fuzz under
`projects/reticulum-go/`. They live here so the fuzz target list is versioned
with the code it builds.

## Submitting

1. Fork https://github.com/google/oss-fuzz.
2. Copy this directory to `projects/reticulum-go/` in the fork.
3. Open a pull request against google/oss-fuzz.

Acceptance criteria: OSS-Fuzz expects projects to be critical infrastructure
or to have a significant user base. Approval is manual and can take weeks.
If the project is declined, the same files work with ClusterFuzzLite in CI,
which keeps the continuous-fuzzing benefit without Google hosting.

## build.sh

`build.sh` runs inside the OSS-Fuzz base-builder-go container and calls
`compile_native_go_fuzzer` once per target. Arguments are:

```
compile_native_go_fuzzer <package-import-path> <FuzzFunction> <output-name>
```

The list covers wire parsers and other attacker-controlled input paths. When
a new `func Fuzz*` lands in pkg/, add a line here if the input is untrusted.

## Local check

ClusterFuzzLite or a plain `go test -fuzz` run exercises the same targets:

```bash
go test -fuzz=FuzzPacketUnpack -fuzztime=60s ./pkg/packet/
```

See also `scripts/ci/run-fuzz-guided.sh` for the in-repo guided fuzz runner.
