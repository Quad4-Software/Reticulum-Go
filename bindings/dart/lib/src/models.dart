// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

import 'dart:convert';

class HealthStatus {
  const HealthStatus({
    required this.status,
    this.transportId,
    required this.transportUptimeSeconds,
  });

  factory HealthStatus.fromJson(Map<String, dynamic> json) {
    return HealthStatus(
      status: json['status'] as String? ?? '',
      transportId: json['transport_id'] as String?,
      transportUptimeSeconds:
          (json['transport_uptime_seconds'] as num?)?.toDouble() ?? 0,
    );
  }

  final String status;
  final String? transportId;
  final double transportUptimeSeconds;
}

class InterfaceStat {
  const InterfaceStat({
    required this.name,
    required this.type,
    required this.status,
    required this.rxBytes,
    required this.txBytes,
    required this.bitrate,
  });

  factory InterfaceStat.fromJson(Map<String, dynamic> json) {
    return InterfaceStat(
      name: json['name'] as String? ?? '',
      type: json['type'] as String? ?? '',
      status: json['status'] as bool? ?? false,
      rxBytes: (json['rx_bytes'] as num?)?.toInt() ?? 0,
      txBytes: (json['tx_bytes'] as num?)?.toInt() ?? 0,
      bitrate: (json['bitrate'] as num?)?.toInt() ?? 0,
    );
  }

  final String name;
  final String type;
  final bool status;
  final int rxBytes;
  final int txBytes;
  final int bitrate;
}

class NodeStatus {
  const NodeStatus({
    required this.transportId,
    required this.interfaces,
  });

  factory NodeStatus.fromJson(Map<String, dynamic> json) {
    final raw = json['interfaces'] as List<dynamic>? ?? const [];
    return NodeStatus(
      transportId: json['transport_id'] as String? ?? '',
      interfaces: raw
          .map((e) => InterfaceStat.fromJson(e as Map<String, dynamic>))
          .toList(),
    );
  }

  final String transportId;
  final List<InterfaceStat> interfaces;
}

class PathEntry {
  const PathEntry({
    required this.hash,
    required this.via,
    required this.hops,
    required this.expires,
    required this.interfaceName,
  });

  factory PathEntry.fromJson(Map<String, dynamic> json) {
    return PathEntry(
      hash: json['hash'] as String? ?? '',
      via: json['via'] as String? ?? '',
      hops: (json['hops'] as num?)?.toInt() ?? 0,
      expires: (json['expires'] as num?)?.toDouble() ?? 0,
      interfaceName: json['interface'] as String? ?? '',
    );
  }

  final String hash;
  final String via;
  final int hops;
  final double expires;
  final String interfaceName;
}

class SessionInfo {
  const SessionInfo({
    required this.sessionId,
    required this.identityHash,
  });

  factory SessionInfo.fromJson(Map<String, dynamic> json) {
    return SessionInfo(
      sessionId: json['session_id'] as String? ?? '',
      identityHash: json['identity_hash'] as String? ?? '',
    );
  }

  final String sessionId;
  final String identityHash;
}

class DestinationInfo {
  const DestinationInfo({required this.destinationHash});

  factory DestinationInfo.fromJson(Map<String, dynamic> json) {
    return DestinationInfo(
      destinationHash: json['destination_hash'] as String? ?? '',
    );
  }

  final String destinationHash;
}

sealed class ControlEvent {
  const ControlEvent(this.type);

  factory ControlEvent.fromJson(Map<String, dynamic> json) {
    final type = json['type'] as String? ?? '';
    switch (type) {
      case 'announce':
        return AnnounceEvent.fromJson(json);
      case 'link.established':
        return LinkEstablishedEvent.fromJson(json);
      case 'link.failed':
        return LinkFailedEvent.fromJson(json);
      case 'link.data':
        return LinkDataEvent.fromJson(json);
      case 'link.closed':
        return LinkClosedEvent.fromJson(json);
      case 'request.incoming':
        return RequestIncomingEvent.fromJson(json);
      default:
        return UnknownEvent(type, json);
    }
  }

  final String type;
}

class AnnounceEvent extends ControlEvent {
  const AnnounceEvent({
    required this.destinationHash,
    this.identityHash,
    this.appData,
    required this.hops,
  }) : super('announce');

  factory AnnounceEvent.fromJson(Map<String, dynamic> json) {
    return AnnounceEvent(
      destinationHash: json['destination_hash'] as String? ?? '',
      identityHash: json['identity_hash'] as String?,
      appData: _decodeOptionalBase64(json['app_data'] as String?),
      hops: (json['hops'] as num?)?.toInt() ?? 0,
    );
  }

  final String destinationHash;
  final String? identityHash;
  final List<int>? appData;
  final int hops;
}

class LinkEstablishedEvent extends ControlEvent {
  const LinkEstablishedEvent({
    required this.linkId,
    this.remoteHash,
  }) : super('link.established');

  factory LinkEstablishedEvent.fromJson(Map<String, dynamic> json) {
    return LinkEstablishedEvent(
      linkId: json['link_id'] as String? ?? '',
      remoteHash: json['remote_hash'] as String?,
    );
  }

  final String linkId;
  final String? remoteHash;
}

class LinkFailedEvent extends ControlEvent {
  const LinkFailedEvent({
    this.linkId,
    this.destinationHash,
    this.error,
  }) : super('link.failed');

  factory LinkFailedEvent.fromJson(Map<String, dynamic> json) {
    return LinkFailedEvent(
      linkId: json['link_id'] as String?,
      destinationHash: json['destination_hash'] as String?,
      error: json['error'] as String?,
    );
  }

  final String? linkId;
  final String? destinationHash;
  final String? error;
}

class LinkDataEvent extends ControlEvent {
  const LinkDataEvent({
    required this.linkId,
    required this.data,
  }) : super('link.data');

  factory LinkDataEvent.fromJson(Map<String, dynamic> json) {
    return LinkDataEvent(
      linkId: json['link_id'] as String? ?? '',
      data: base64.decode(json['data'] as String? ?? ''),
    );
  }

  final String linkId;
  final List<int> data;
}

class LinkClosedEvent extends ControlEvent {
  const LinkClosedEvent({required this.linkId}) : super('link.closed');

  factory LinkClosedEvent.fromJson(Map<String, dynamic> json) {
    return LinkClosedEvent(linkId: json['link_id'] as String? ?? '');
  }

  final String linkId;
}

class RequestIncomingEvent extends ControlEvent {
  const RequestIncomingEvent({
    required this.destinationHash,
    required this.linkId,
    required this.requestId,
    required this.path,
    this.data,
    this.remoteIdentityHash,
  }) : super('request.incoming');

  factory RequestIncomingEvent.fromJson(Map<String, dynamic> json) {
    return RequestIncomingEvent(
      destinationHash: json['destination_hash'] as String? ?? '',
      linkId: json['link_id'] as String? ?? '',
      requestId: json['request_id'] as String? ?? '',
      path: json['path'] as String? ?? '',
      data: _decodeOptionalBase64(json['data'] as String?),
      remoteIdentityHash: json['remote_identity_hash'] as String?,
    );
  }

  final String destinationHash;
  final String linkId;
  final String requestId;
  final String path;
  final List<int>? data;
  final String? remoteIdentityHash;
}

class UnknownEvent extends ControlEvent {
  const UnknownEvent(super.type, this.raw);

  final Map<String, dynamic> raw;
}

List<int>? _decodeOptionalBase64(String? value) {
  if (value == null || value.isEmpty) {
    return null;
  }
  return base64.decode(value);
}

String encodeBase64(List<int> data) => base64.encode(data);
