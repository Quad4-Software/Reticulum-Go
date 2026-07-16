// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

import 'dart:convert';
import 'dart:io';

import 'package:rns_control/rns_control.dart';

Future<void> main(List<String> args) async {
  final rpcKey = Platform.environment['RNS_RPC_KEY'] ??
      (args.isNotEmpty ? args.first : null);
  if (rpcKey == null || rpcKey.isEmpty) {
    stderr.writeln('usage: dart run example/rns_control_example.dart <rpc_key>');
    stderr.writeln('or set RNS_RPC_KEY');
    exitCode = 2;
    return;
  }

  final host = Platform.environment['RNS_CONTROL_HOST'] ?? '127.0.0.1';
  final port =
      int.parse(Platform.environment['RNS_CONTROL_PORT'] ?? '37430');

  final client = ControlClient(host: host, port: port, rpcKey: rpcKey);
  try {
    final health = await client.health();
    stdout.writeln('health: ${health.status} uptime=${health.transportUptimeSeconds}');

    final session = await client.createSession();
    stdout.writeln('session: ${session.sessionId}');

    final dest = await client.registerDestination(
      session.sessionId,
      appName: 'rns_control_example',
      aspects: const ['demo'],
    );
    stdout.writeln('destination: ${dest.destinationHash}');

    final events = client.openEvents(session.sessionId);
    events.subscribeAnnounces();
    await client.announce(
      session.sessionId,
      dest.destinationHash,
      appData: utf8.encode('dart-hello'),
    );

    await for (final event in events.events.timeout(
      const Duration(seconds: 3),
      onTimeout: (sink) => sink.close(),
    )) {
      stdout.writeln('event: ${event.type}');
      if (event is AnnounceEvent) {
        stdout.writeln('  dest=${event.destinationHash} hops=${event.hops}');
      }
    }

    await events.close();
    await client.deleteSession(session.sessionId);
  } finally {
    client.close();
  }
}
