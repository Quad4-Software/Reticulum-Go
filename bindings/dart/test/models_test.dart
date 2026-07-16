// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

import 'dart:convert';

import 'package:test/test.dart';

import 'package:rns_control/rns_control.dart';

void main() {
  group('ControlEvent parsing', () {
    test('parses announce', () {
      final event = ControlEvent.fromJson({
        'type': 'announce',
        'destination_hash': 'aabb',
        'identity_hash': 'ccdd',
        'app_data': base64.encode(utf8.encode('hi')),
        'hops': 1,
      });
      expect(event, isA<AnnounceEvent>());
      final announce = event as AnnounceEvent;
      expect(announce.destinationHash, 'aabb');
      expect(utf8.decode(announce.appData!), 'hi');
      expect(announce.hops, 1);
    });

    test('parses link.data', () {
      final event = ControlEvent.fromJson({
        'type': 'link.data',
        'link_id': 'link1',
        'data': base64.encode(utf8.encode('payload')),
      });
      expect(event, isA<LinkDataEvent>());
      final data = event as LinkDataEvent;
      expect(data.linkId, 'link1');
      expect(utf8.decode(data.data), 'payload');
    });

    test('parses request.incoming', () {
      final event = ControlEvent.fromJson({
        'type': 'request.incoming',
        'destination_hash': 'dest',
        'link_id': 'link',
        'request_id': 'req',
        'path': '/ping',
        'data': base64.encode(utf8.encode('hello')),
      });
      expect(event, isA<RequestIncomingEvent>());
      final incoming = event as RequestIncomingEvent;
      expect(incoming.path, '/ping');
      expect(utf8.decode(incoming.data!), 'hello');
    });

    test('unknown types are preserved', () {
      final event = ControlEvent.fromJson({'type': 'future.event', 'x': 1});
      expect(event, isA<UnknownEvent>());
      expect(event.type, 'future.event');
    });
  });

  group('model helpers', () {
    test('HealthStatus.fromJson', () {
      final health = HealthStatus.fromJson({
        'status': 'ok',
        'transport_id': 'abc',
        'transport_uptime_seconds': 12.5,
      });
      expect(health.status, 'ok');
      expect(health.transportId, 'abc');
      expect(health.transportUptimeSeconds, 12.5);
    });

    test('SessionInfo.fromJson', () {
      final session = SessionInfo.fromJson({
        'session_id': 's1',
        'identity_hash': 'id1',
      });
      expect(session.sessionId, 's1');
      expect(session.identityHash, 'id1');
    });
  });
}
