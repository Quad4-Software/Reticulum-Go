// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

import 'dart:io';

import 'package:rns_control/ffi.dart';

void main() {
  final root = _repoRoot();
  final so = '$root/bin/librns.so';
  if (!File(so).existsSync()) {
    stderr.writeln('missing $so (run task build-librns)');
    exit(1);
  }

  final rns = Rns(libraryPath: so);
  if (rns.version() != rnsApiVersion) {
    stderr.writeln('unexpected version: ${rns.version()}');
    exit(1);
  }

  final node = rns.nodeCreate();
  if (node == 0) {
    stderr.writeln('nodeCreate failed');
    exit(1);
  }

  if (rns.nodeStart(node) != RnsError.ok) {
    stderr.writeln('nodeStart failed');
    rns.nodeDestroy(node);
    exit(1);
  }

  final poll = rns.eventPoll(node, timeoutMs: 10);
  if (poll.code != RnsError.timeout) {
    stderr.writeln('expected timeout poll on idle node');
    rns.nodeStop(node);
    rns.nodeDestroy(node);
    exit(1);
  }

  if (rns.nodeStop(node) != RnsError.ok ||
      rns.nodeDestroy(node) != RnsError.ok) {
    stderr.writeln('teardown failed');
    exit(1);
  }

  stdout.writeln('dart-smoke ok');
}

String _repoRoot() {
  var dir = Directory.current;
  for (var i = 0; i < 8; i++) {
    if (File('${dir.path}/include/rns.h').existsSync()) {
      return dir.path;
    }
    dir = dir.parent;
  }
  final env = Platform.environment['RNS_ROOT'];
  if (env != null && env.isNotEmpty) {
    return env;
  }
  throw StateError('could not locate repository root');
}
