# Dart / Flutter bindings for Reticulum-Go

Package `rns_control` includes:

1. **librns FFI** (`package:rns_control/ffi.dart`): in-process mesh on Linux, Android, and Windows
2. **Control API client** (`package:rns_control/rns_control.dart`): HTTP and WebSocket to a running daemon

## FFI (Linux, Android, Windows)

```dart
import 'package:rns_control/ffi.dart';

final rns = Rns();
print(rns.version());

final node = rns.nodeCreate();
rns.nodeStart(node);
```

Load order for the shared library:

1. `libraryPath` argument
2. `RNS_LIB_PATH`
3. Platform defaults (`librns.so` on Android, search under `bin/` on Linux and Windows)

Build native libraries from the repository root:

```bash
task build-librns
# or cross-build
task build-librns-targets -- linux android windows
```

| Platform | Artifact |
|----------|----------|
| Linux | `bin/librns.so` |
| Android | `bin/android/<abi>/librns.so` (`arm64-v8a`, `armeabi-v7a`, `x86_64`) |
| Windows | `bin/windows/amd64/librns.dll` |

Flutter Android: copy the ABI folders into `android/app/src/main/jniLibs/`.

Flutter Windows: ship `librns.dll` next to the runner binary or set `RNS_LIB_PATH`.

Needs Android NDK (`ANDROID_NDK_HOME`) for Android builds, and mingw or Zig for Windows.

## Control API

```dart
import 'package:rns_control/rns_control.dart';

final client = ControlClient(rpcKey: rpcKey);
final health = await client.health();
```

Browser WebSocket cannot set the Authorization header required by the events endpoint.

## Test

```bash
task build-librns
task test-dart
# or
make -C bindings/dart test
```

See `docs/en/librns.md` and `docs/en/control-api.md` in the repository root.
