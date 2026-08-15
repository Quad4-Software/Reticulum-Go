// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:http/http.dart' as http;
import 'package:test/test.dart';

import 'package:rns_control/rns_control.dart';

void main() {
  late HttpServer server;
  late ControlClient client;
  const rpcKey =
      '0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef';
  const destHash =
      'dddddddddddddddddddddddddddddddd';

  setUp(() async {
    server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    unawaited(_serveMock(server, rpcKey, destHash));
    client = ControlClient(
      host: '127.0.0.1',
      port: server.port,
      rpcKey: rpcKey,
      httpClient: http.Client(),
    );
  });

  tearDown(() async {
    client.close();
    await server.close(force: true);
  });

  test('health and session lifecycle against mock server', () async {
    final health = await client.health();
    expect(health.status, 'ok');

    final session = await client.createSession();
    expect(session.sessionId, 'session-1');
    expect(session.identityHash, isNotEmpty);

    final dest = await client.registerDestination(
      session.sessionId,
      appName: 'dart-rns',
      aspects: const ['chat'],
      acceptsLinks: true,
    );
    expect(dest.destinationHash, destHash);

    await client.announce(session.sessionId, dest.destinationHash);
    await client.registerRequestHandler(
      session.sessionId,
      dest.destinationHash,
      path: '/ping',
    );

    final events = client.openEvents(session.sessionId);
    final announceFuture = events.events
        .where((e) => e is AnnounceEvent)
        .cast<AnnounceEvent>()
        .first
        .timeout(const Duration(seconds: 2));
    events.subscribeAnnounces();
    final announce = await announceFuture;
    expect(announce.destinationHash, isNotEmpty);
    await events.close();

    await client.deleteSession(session.sessionId);
  });

  test('rejects bad bearer token', () async {
    final bad = ControlClient(
      host: '127.0.0.1',
      port: server.port,
      rpcKey: 'ff' * 32,
    );
    await expectLater(
      bad.health(),
      throwsA(isA<ControlApiException>()),
    );
    bad.close();
  });
}

Future<void> _serveMock(
  HttpServer server,
  String rpcKey,
  String destHash,
) async {
  await for (final request in server) {
    final auth = request.headers.value('authorization') ?? '';
    if (auth != 'Bearer $rpcKey') {
      request.response.statusCode = 401;
      request.response.headers.contentType = ContentType.json;
      request.response
          .write(jsonEncode({'error': 'missing or invalid bearer token'}));
      await request.response.close();
      continue;
    }

    if (WebSocketTransformer.isUpgradeRequest(request) &&
        request.uri.path.endsWith('/events')) {
      final socket = await WebSocketTransformer.upgrade(request);
      await for (final message in socket) {
        final cmd = jsonDecode(message as String) as Map<String, dynamic>;
        if (cmd['type'] == 'subscribe_announces') {
          socket.add(jsonEncode({
            'type': 'announce',
            'destination_hash': 'aa' * 16,
            'hops': 0,
          }));
        }
      }
      continue;
    }

    final body = await utf8.decoder.bind(request).join();
    Map<String, dynamic>? jsonBody;
    if (body.isNotEmpty) {
      jsonBody = jsonDecode(body) as Map<String, dynamic>;
    }

    request.response.headers.contentType = ContentType.json;
    final route = '${request.method} ${request.uri.path}';
    if (route == 'GET /v1/health') {
      request.response.write(jsonEncode({
        'status': 'ok',
        'transport_id': 'tt' * 16,
        'transport_uptime_seconds': 1.0,
      }));
    } else if (route == 'POST /v1/sessions') {
      request.response.write(jsonEncode({
        'session_id': 'session-1',
        'identity_hash': 'ii' * 16,
      }));
    } else if (route == 'DELETE /v1/sessions/session-1') {
      request.response.statusCode = 204;
    } else if (route == 'POST /v1/sessions/session-1/destinations') {
      if (jsonBody?['app_name'] != 'dart-rns') {
        request.response.statusCode = 400;
        request.response.write(jsonEncode({'error': 'bad app_name'}));
      } else {
        request.response.write(jsonEncode({'destination_hash': destHash}));
      }
    } else if (route ==
        'POST /v1/sessions/session-1/destinations/$destHash/announce') {
      request.response.statusCode = 204;
    } else if (route ==
        'POST /v1/sessions/session-1/destinations/$destHash/requests') {
      if (jsonBody?['path'] != '/ping') {
        request.response.statusCode = 400;
        request.response.write(jsonEncode({'error': 'bad path'}));
      } else {
        request.response.statusCode = 204;
      }
    } else {
      request.response.statusCode = 404;
      request.response.write(jsonEncode({'error': 'not found $route'}));
    }
    await request.response.close();
  }
}
