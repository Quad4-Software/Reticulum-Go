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

    test('parses request.response and command.error', () {
      final response = ControlEvent.fromJson({
        'type': 'request.response',
        'link_id': 'l1',
        'request_id': 'r1',
        'path': '/ping',
        'data': base64.encode(utf8.encode('pong')),
      });
      expect(response, isA<RequestResponseEvent>());
      expect(utf8.decode((response as RequestResponseEvent).data!), 'pong');

      final err = ControlEvent.fromJson({
        'type': 'command.error',
        'command': 'link.send',
        'error': 'unknown link_id',
      });
      expect(err, isA<CommandErrorEvent>());
      expect((err as CommandErrorEvent).command, 'link.send');
    });

    test('parses resource and identify events', () {
      final started = ControlEvent.fromJson({
        'type': 'resource.started',
        'link_id': 'l1',
      });
      expect(started, isA<ResourceStartedEvent>());

      final concluded = ControlEvent.fromJson({
        'type': 'resource.concluded',
        'link_id': 'l1',
        'success': true,
        'name': 'a.txt',
        'data': base64.encode(utf8.encode('x')),
      });
      expect(concluded, isA<ResourceConcludedEvent>());
      expect((concluded as ResourceConcludedEvent).name, 'a.txt');

      final identified = ControlEvent.fromJson({
        'type': 'link.remote_identified',
        'link_id': 'l1',
        'identity_hash': 'abcd',
      });
      expect(identified, isA<LinkRemoteIdentifiedEvent>());
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

    test('InterfaceStat maps integrity fields', () {
      final stat = InterfaceStat.fromJson({
        'name': 'udp',
        'type': 'UDP',
        'status': true,
        'rx_bytes': 1,
        'tx_bytes': 2,
        'bitrate': 3,
        'ifac_fail': 4,
        'hmac_fail': 5,
        'integrity_fail_rate': 0.25,
      });
      expect(stat.ifacFail, 4);
      expect(stat.hmacFail, 5);
      expect(stat.integrityFailRate, 0.25);
    });
  });
}
