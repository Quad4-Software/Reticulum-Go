// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

import 'dart:io';

import 'package:rns_control/ffi.dart';
import 'package:test/test.dart';

void main() {
  late Rns rns;

  setUpAll(() {
    final root = _repoRoot();
    final so = '$root/bin/librns.so';
    if (!File(so).existsSync()) {
      throw StateError(
        'missing $so (run task build-librns before FFI tests)',
      );
    }
    rns = Rns(libraryPath: so);
  });

  test('version matches API', () {
    expect(rns.version(), rnsApiVersion);
  });

  test('node lifecycle and event poll timeout', () {
    final node = rns.nodeCreate();
    expect(node, isNonZero);
    addTearDown(() => rns.nodeDestroy(node));

    expect(rns.nodeStart(node), RnsError.ok);

    final poll = rns.eventPoll(node, timeoutMs: 10);
    expect(poll.code, RnsError.timeout);
    expect(poll.event, isNull);

    expect(rns.nodeStop(node), RnsError.ok);
  });

  test('identity generate and hash', () {
    final id = rns.identityGenerate();
    expect(id, isNonZero);
    addTearDown(() => rns.identityDestroy(id));

    final hash = rns.identityHash(id);
    expect(hash, isNotNull);
    expect(hash!.length, 32);
    expect(RegExp(r'^[0-9a-f]+$').hasMatch(hash), isTrue);
  });

  test('invalid handle', () {
    final code = rns.nodeStart(0);
    expect(
      code,
      anyOf(RnsError.invalidHandle, RnsError.invalidArg, RnsError.internal),
    );
  });
}

String _repoRoot() {
  var dir = Directory.current;
  for (var i = 0; i < 6; i++) {
    final marker = File('${dir.path}/include/rns.h');
    if (marker.existsSync()) {
      return dir.path;
    }
    dir = dir.parent;
  }
  final env = Platform.environment['RNS_ROOT'];
  if (env != null && env.isNotEmpty) {
    return env;
  }
  throw StateError('could not locate repository root from ${Directory.current.path}');
}
