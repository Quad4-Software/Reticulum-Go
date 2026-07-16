// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

import 'dart:async';
import 'dart:convert';

import 'package:http/http.dart' as http;
import 'package:web_socket_channel/web_socket_channel.dart';

import 'errors.dart';
import 'models.dart';
import 'ws_connect_stub.dart'
    if (dart.library.io) 'ws_connect_io.dart' as ws_connect;

class ControlClient {
  ControlClient({
    required this.rpcKey,
    this.host = '127.0.0.1',
    this.port = 37430,
    http.Client? httpClient,
  }) : _http = httpClient ?? http.Client();

  final String host;
  final int port;
  final String rpcKey;
  final http.Client _http;

  Uri _httpUri(String path) => Uri(
        scheme: 'http',
        host: host,
        port: port,
        path: path,
      );

  Uri _wsUri(String path) => Uri(
        scheme: 'ws',
        host: host,
        port: port,
        path: path,
      );

  Map<String, String> get _headers => {
        'Authorization': 'Bearer $rpcKey',
        'Content-Type': 'application/json',
        'Accept': 'application/json',
      };

  Future<dynamic> _request(
    String method,
    String path, {
    Object? body,
  }) async {
    final uri = _httpUri(path);
    late http.Response response;
    final encoded = body == null ? null : jsonEncode(body);
    switch (method) {
      case 'GET':
        response = await _http.get(uri, headers: _headers);
      case 'POST':
        response = await _http.post(uri, headers: _headers, body: encoded);
      case 'DELETE':
        response = await _http.delete(uri, headers: _headers);
      default:
        throw ControlApiException('unsupported method $method');
    }
    if (response.statusCode >= 400) {
      var message = 'request failed';
      if (response.body.isNotEmpty) {
        try {
          final err = jsonDecode(response.body);
          if (err is Map && err['error'] is String) {
            message = err['error'] as String;
          } else {
            message = response.body;
          }
        } catch (_) {
          message = response.body;
        }
      }
      throw ControlApiException(message, statusCode: response.statusCode);
    }
    if (response.statusCode == 204 || response.body.isEmpty) {
      return null;
    }
    return jsonDecode(response.body);
  }

  Future<HealthStatus> health() async {
    final json = await _request('GET', '/v1/health') as Map<String, dynamic>;
    return HealthStatus.fromJson(json);
  }

  Future<NodeStatus> status() async {
    final json = await _request('GET', '/v1/status') as Map<String, dynamic>;
    return NodeStatus.fromJson(json);
  }

  Future<List<PathEntry>> paths() async {
    final decoded = await _request('GET', '/v1/paths');
    if (decoded is! List) {
      return const [];
    }
    return decoded
        .map((e) => PathEntry.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  Future<SessionInfo> createSession({String? identityPath}) async {
    final body = <String, dynamic>{};
    if (identityPath != null && identityPath.isNotEmpty) {
      body['identity_path'] = identityPath;
    }
    final json =
        await _request('POST', '/v1/sessions', body: body) as Map<String, dynamic>;
    return SessionInfo.fromJson(json);
  }

  Future<void> deleteSession(String sessionId) async {
    await _request('DELETE', '/v1/sessions/$sessionId');
  }

  Future<DestinationInfo> registerDestination(
    String sessionId, {
    required String appName,
    List<String> aspects = const [],
    bool acceptsLinks = false,
  }) async {
    final json = await _request(
      'POST',
      '/v1/sessions/$sessionId/destinations',
      body: {
        'app_name': appName,
        if (aspects.isNotEmpty) 'aspects': aspects,
        'accepts_links': acceptsLinks,
      },
    ) as Map<String, dynamic>;
    return DestinationInfo.fromJson(json);
  }

  Future<void> announce(
    String sessionId,
    String destinationHash, {
    List<int>? appData,
  }) async {
    await _request(
      'POST',
      '/v1/sessions/$sessionId/destinations/$destinationHash/announce',
      body: {
        if (appData != null) 'app_data': encodeBase64(appData),
      },
    );
  }

  Future<void> registerRequestHandler(
    String sessionId,
    String destinationHash, {
    required String path,
    String allow = 'all',
    List<String> allowedIdentities = const [],
  }) async {
    await _request(
      'POST',
      '/v1/sessions/$sessionId/destinations/$destinationHash/requests',
      body: {
        'path': path,
        if (allow.isNotEmpty) 'allow': allow,
        if (allowedIdentities.isNotEmpty)
          'allowed_identities': allowedIdentities,
      },
    );
  }

  Future<void> requestPath(String sessionId, String destinationHash) async {
    await _request(
      'POST',
      '/v1/sessions/$sessionId/path/request',
      body: {'destination_hash': destinationHash},
    );
  }

  Future<void> pause() async {
    await _request('POST', '/v1/lifecycle/pause');
  }

  Future<void> resume() async {
    await _request('POST', '/v1/lifecycle/resume');
  }

  Future<void> refreshPaths({List<String> destinationHashes = const []}) async {
    await _request(
      'POST',
      '/v1/lifecycle/refresh-paths',
      body: {
        if (destinationHashes.isNotEmpty) 'destinations': destinationHashes,
      },
    );
  }

  EventSession openEvents(String sessionId) {
    final channel = ws_connect.connectControlEvents(
      uri: _wsUri('/v1/sessions/$sessionId/events'),
      rpcKey: rpcKey,
    );
    return EventSession(channel);
  }

  void close() {
    _http.close();
  }
}

class EventSession {
  EventSession(this._channel) {
    _events = _channel.stream.map((message) {
      final decoded = jsonDecode(message as String) as Map<String, dynamic>;
      return ControlEvent.fromJson(decoded);
    }).asBroadcastStream();
  }

  final WebSocketChannel _channel;
  late final Stream<ControlEvent> _events;

  Stream<ControlEvent> get events => _events;

  void sendCommand(Map<String, dynamic> command) {
    _channel.sink.add(jsonEncode(command));
  }

  void subscribeAnnounces({String? filter}) {
    sendCommand({
      'type': 'subscribe_announces',
      if (filter != null) 'filter': filter,
    });
  }

  void linkOpen(String destinationHash) {
    sendCommand({
      'type': 'link.open',
      'destination_hash': destinationHash,
    });
  }

  void linkSend(String linkId, List<int> data) {
    sendCommand({
      'type': 'link.send',
      'link_id': linkId,
      'data': encodeBase64(data),
    });
  }

  void linkClose(String linkId) {
    sendCommand({
      'type': 'link.close',
      'link_id': linkId,
    });
  }

  void requestRespond(String requestId, {List<int>? data}) {
    sendCommand({
      'type': 'request.respond',
      'request_id': requestId,
      if (data != null) 'data': encodeBase64(data),
    });
  }

  Future<void> close() async {
    await _channel.sink.close();
  }
}
