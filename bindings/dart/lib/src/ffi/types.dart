// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// ignore_for_file: non_constant_identifier_names

import 'dart:ffi';
import 'dart:typed_data';

const rnsHashLen = 16;
const rnsApiVersion = '1.4';

abstract final class RnsError {
  static const ok = 0;
  static const invalidArg = 1;
  static const invalidHandle = 2;
  static const notFound = 3;
  static const state = 4;
  static const io = 5;
  static const internal = 6;
  static const timeout = 7;
  static const truncated = 8;

  static String name(int code) {
    switch (code) {
      case ok:
        return 'ok';
      case invalidArg:
        return 'invalid argument';
      case invalidHandle:
        return 'invalid handle';
      case notFound:
        return 'not found';
      case state:
        return 'invalid state';
      case io:
        return 'io error';
      case internal:
        return 'internal error';
      case timeout:
        return 'timeout';
      case truncated:
        return 'truncated';
      default:
        return 'unknown error';
    }
  }
}

abstract final class RnsEventKind {
  static const none = 0;
  static const announce = 1;
  static const linkEstablished = 2;
  static const linkFailed = 3;
  static const linkData = 4;
  static const linkClosed = 5;
  static const requestIncoming = 6;
  static const requestResponse = 7;
  static const requestFailed = 8;
  static const resourceStarted = 9;
  static const resourceConcluded = 10;
}

final class RnsEventNative extends Struct {
  @Int32()
  external int kind;

  @Array(rnsHashLen)
  external Array<Uint8> link_id;

  @Size()
  external int link_id_len;

  @Array(rnsHashLen)
  external Array<Uint8> destination_hash;

  @Size()
  external int destination_hash_len;

  @Array(rnsHashLen)
  external Array<Uint8> identity_hash;

  @Size()
  external int identity_hash_len;

  @Array(rnsHashLen)
  external Array<Uint8> request_id;

  @Size()
  external int request_id_len;

  @Uint8()
  external int hops;

  @Array(256)
  external Array<Uint8> path;

  @Int32()
  external int path_truncated;

  @Array(256)
  external Array<Uint8> error_message;

  @Int32()
  external int error_message_truncated;

  external Pointer<Uint8> app_data;

  @Size()
  external int app_data_len;

  @Size()
  external int app_data_cap;

  @Int32()
  external int app_data_truncated;
}

final class RnsPathEntryNative extends Struct {
  @Array(rnsHashLen)
  external Array<Uint8> hash;

  @Size()
  external int hash_len;

  @Array(rnsHashLen)
  external Array<Uint8> via;

  @Size()
  external int via_len;

  @Uint8()
  external int hops;

  @Array(64)
  external Array<Uint8> iface;

  @Double()
  external double timestamp;

  @Double()
  external double expires;
}

class RnsEventView {
  RnsEventView({
    required this.kind,
    required this.linkId,
    required this.destinationHash,
    required this.identityHash,
    required this.requestId,
    required this.hops,
    required this.path,
    required this.pathTruncated,
    required this.errorMessage,
    required this.errorMessageTruncated,
    required this.appData,
    required this.appDataTruncated,
  });

  factory RnsEventView.fromNative(RnsEventNative e) {
    return RnsEventView(
      kind: e.kind,
      linkId: _copyArray(e.link_id, e.link_id_len),
      destinationHash:
          _copyArray(e.destination_hash, e.destination_hash_len),
      identityHash: _copyArray(e.identity_hash, e.identity_hash_len),
      requestId: _copyArray(e.request_id, e.request_id_len),
      hops: e.hops,
      path: _cstringFromArray(e.path, 256),
      pathTruncated: e.path_truncated != 0,
      errorMessage: _cstringFromArray(e.error_message, 256),
      errorMessageTruncated: e.error_message_truncated != 0,
      appData: e.app_data == nullptr || e.app_data_len == 0
          ? null
          : List<int>.from(e.app_data.asTypedList(e.app_data_len)),
      appDataTruncated: e.app_data_truncated != 0,
    );
  }

  final int kind;
  final Uint8List linkId;
  final Uint8List destinationHash;
  final Uint8List identityHash;
  final Uint8List requestId;
  final int hops;
  final String path;
  final bool pathTruncated;
  final String errorMessage;
  final bool errorMessageTruncated;
  final List<int>? appData;
  final bool appDataTruncated;
}

class RnsPathEntryView {
  RnsPathEntryView({
    required this.hash,
    required this.via,
    required this.hops,
    required this.iface,
    required this.timestamp,
    required this.expires,
  });

  factory RnsPathEntryView.fromNative(RnsPathEntryNative e) {
    return RnsPathEntryView(
      hash: _copyArray(e.hash, e.hash_len),
      via: _copyArray(e.via, e.via_len),
      hops: e.hops,
      iface: _cstringFromArray(e.iface, 64),
      timestamp: e.timestamp,
      expires: e.expires,
    );
  }

  final Uint8List hash;
  final Uint8List via;
  final int hops;
  final String iface;
  final double timestamp;
  final double expires;
}

Uint8List _copyArray(Array<Uint8> arr, int len) {
  final n = len.clamp(0, rnsHashLen);
  final out = Uint8List(n);
  for (var i = 0; i < n; i++) {
    out[i] = arr[i];
  }
  return out;
}

String _cstringFromArray(Array<Uint8> arr, int maxLen) {
  final bytes = <int>[];
  for (var i = 0; i < maxLen; i++) {
    final b = arr[i];
    if (b == 0) {
      break;
    }
    bytes.add(b);
  }
  return String.fromCharCodes(bytes);
}
