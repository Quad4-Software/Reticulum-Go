// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

import 'dart:ffi';
import 'dart:io';

import 'package:ffi/ffi.dart';

import 'bindings.dart';
import 'types.dart';

export 'types.dart';

DynamicLibrary openRnsLibrary({String? path}) {
  if (path != null && path.isNotEmpty) {
    return DynamicLibrary.open(path);
  }
  final fromEnv = Platform.environment['RNS_LIB_PATH'];
  if (fromEnv != null && fromEnv.isNotEmpty) {
    return DynamicLibrary.open(fromEnv);
  }
  if (Platform.isAndroid) {
    return DynamicLibrary.open('librns.so');
  }
  if (Platform.isWindows) {
    for (final candidate in _windowsCandidates()) {
      if (File(candidate).existsSync()) {
        return DynamicLibrary.open(candidate);
      }
    }
    return DynamicLibrary.open('librns.dll');
  }
  if (Platform.isLinux) {
    for (final candidate in _linuxCandidates()) {
      if (File(candidate).existsSync()) {
        return DynamicLibrary.open(candidate);
      }
    }
    return DynamicLibrary.open('librns.so');
  }
  throw UnsupportedError(
    'librns FFI supports Linux, Android, and Windows. '
    'Got ${Platform.operatingSystem}',
  );
}

Iterable<String> _linuxCandidates() sync* {
  final root = Platform.environment['RNS_ROOT'];
  if (root != null) {
    yield '$root/bin/librns.so';
  }
  yield 'bin/librns.so';
  yield '../bin/librns.so';
  yield '../../bin/librns.so';
  yield '../../../bin/librns.so';
}

Iterable<String> _windowsCandidates() sync* {
  final root = Platform.environment['RNS_ROOT'];
  if (root != null) {
    yield '$root/bin/windows/amd64/librns.dll';
    yield '$root/bin/librns.dll';
  }
  yield 'bin/windows/amd64/librns.dll';
  yield 'bin/librns.dll';
  yield '../bin/windows/amd64/librns.dll';
  yield '../../bin/windows/amd64/librns.dll';
  yield '../../../bin/windows/amd64/librns.dll';
}

class Rns {
  Rns({String? libraryPath}) : _lib = openRnsLibrary(path: libraryPath) {
    _api = RnsBindings(_lib);
  }

  final DynamicLibrary _lib;
  late final RnsBindings _api;

  String version() {
    final ptr = _api.rns_version();
    if (ptr == nullptr) {
      return '';
    }
    return ptr.cast<Utf8>().toDartString();
  }

  String lastError() {
    final buf = calloc<Char>(512);
    final written = calloc<Size>();
    try {
      final code = _api.rns_last_error(buf, 512, written);
      if (code != RnsError.ok) {
        return '';
      }
      final n = written.value;
      if (n == 0) {
        return '';
      }
      return buf.cast<Utf8>().toDartString(length: n);
    } finally {
      calloc.free(buf);
      calloc.free(written);
    }
  }

  int nodeCreate([String configPath = '']) {
    final path = configPath.toNativeUtf8();
    try {
      return _api.rns_node_create(path.cast());
    } finally {
      malloc.free(path);
    }
  }

  int nodeStart(int node) => _api.rns_node_start(node);
  int nodeStop(int node) => _api.rns_node_stop(node);
  int nodeDestroy(int node) => _api.rns_node_destroy(node);
  int nodeSetIdentity(int node, int identity) =>
      _api.rns_node_set_identity(node, identity);
  int nodePause(int node) => _api.rns_node_pause(node);
  int nodeResume(int node) => _api.rns_node_resume(node);

  int nodeRefreshPaths(int node, [List<List<int>> destHashes = const []]) {
    if (destHashes.isEmpty) {
      return _api.rns_node_refresh_paths(node, nullptr, 0);
    }
    final flat = calloc<Uint8>(destHashes.length * rnsHashLen);
    try {
      var offset = 0;
      for (final hash in destHashes) {
        if (hash.length != rnsHashLen) {
          return RnsError.invalidArg;
        }
        for (var i = 0; i < rnsHashLen; i++) {
          flat[offset + i] = hash[i];
        }
        offset += rnsHashLen;
      }
      return _api.rns_node_refresh_paths(node, flat, destHashes.length);
    } finally {
      calloc.free(flat);
    }
  }

  int identityGenerate() => _api.rns_identity_generate();

  int identityLoad(String path) {
    final p = path.toNativeUtf8();
    try {
      return _api.rns_identity_load(p.cast());
    } finally {
      malloc.free(p);
    }
  }

  int identitySave(int identity, String path) {
    final p = path.toNativeUtf8();
    try {
      return _api.rns_identity_save(identity, p.cast());
    } finally {
      malloc.free(p);
    }
  }

  int identityDestroy(int identity) => _api.rns_identity_destroy(identity);

  String? identityHash(int identity) {
    final buf = calloc<Char>(64);
    final written = calloc<Size>();
    try {
      final code = _api.rns_identity_hash(identity, buf, 64, written);
      if (code != RnsError.ok) {
        return null;
      }
      return buf.cast<Utf8>().toDartString(length: written.value);
    } finally {
      calloc.free(buf);
      calloc.free(written);
    }
  }

  List<int>? identityHashBytes(int identity) {
    final out = calloc<Uint8>(rnsHashLen);
    final written = calloc<Size>();
    try {
      final code =
          _api.rns_identity_hash_bytes(identity, out, rnsHashLen, written);
      if (code != RnsError.ok || written.value != rnsHashLen) {
        return null;
      }
      return List<int>.from(out.asTypedList(rnsHashLen));
    } finally {
      calloc.free(out);
      calloc.free(written);
    }
  }

  List<int>? identityPublicKey(int identity) {
    final out = calloc<Uint8>(64);
    final written = calloc<Size>();
    try {
      final code = _api.rns_identity_public_key(identity, out, 64, written);
      if (code != RnsError.ok) {
        return null;
      }
      return List<int>.from(out.asTypedList(written.value));
    } finally {
      calloc.free(out);
      calloc.free(written);
    }
  }

  int identityFromPublicKey(List<int> pub) {
    if (pub.isEmpty) {
      return 0;
    }
    final buf = calloc<Uint8>(pub.length);
    try {
      buf.asTypedList(pub.length).setAll(0, pub);
      return _api.rns_identity_from_public_key(buf, pub.length);
    } finally {
      calloc.free(buf);
    }
  }

  List<int>? identitySign(int identity, List<int> data) {
    final dataPtr = data.isEmpty ? nullptr : calloc<Uint8>(data.length);
    final sig = calloc<Uint8>(64);
    final written = calloc<Size>();
    try {
      if (data.isNotEmpty) {
        dataPtr.asTypedList(data.length).setAll(0, data);
      }
      final code = _api.rns_identity_sign(
        identity,
        dataPtr,
        data.length,
        sig,
        64,
        written,
      );
      if (code != RnsError.ok) {
        return null;
      }
      return List<int>.from(sig.asTypedList(written.value));
    } finally {
      if (dataPtr != nullptr) {
        calloc.free(dataPtr);
      }
      calloc.free(sig);
      calloc.free(written);
    }
  }

  int identityVerify(int identity, List<int> data, List<int> signature) {
    final dataPtr = data.isEmpty ? nullptr : calloc<Uint8>(data.length);
    final sigPtr =
        signature.isEmpty ? nullptr : calloc<Uint8>(signature.length);
    try {
      if (data.isNotEmpty) {
        dataPtr.asTypedList(data.length).setAll(0, data);
      }
      if (signature.isNotEmpty) {
        sigPtr.asTypedList(signature.length).setAll(0, signature);
      }
      return _api.rns_identity_verify(
        identity,
        dataPtr,
        data.length,
        sigPtr,
        signature.length,
      );
    } finally {
      if (dataPtr != nullptr) {
        calloc.free(dataPtr);
      }
      if (sigPtr != nullptr) {
        calloc.free(sigPtr);
      }
    }
  }

  ({List<int>? blob, int code}) rsgCreate(
    int identity,
    List<int> message, {
    bool embed = true,
  }) {
    final msgPtr = message.isEmpty ? nullptr : calloc<Uint8>(message.length);
    final needed = calloc<Size>();
    try {
      if (message.isNotEmpty) {
        msgPtr.asTypedList(message.length).setAll(0, message);
      }
      var code = _api.rns_rsg_create(
        identity,
        msgPtr,
        message.length,
        embed ? 1 : 0,
        nullptr,
        0,
        needed,
      );
      if (code != RnsError.ok && code != RnsError.truncated) {
        return (blob: null, code: code);
      }
      if (needed.value == 0) {
        return (blob: null, code: RnsError.internal);
      }
      final out = calloc<Uint8>(needed.value);
      final written = calloc<Size>();
      try {
        code = _api.rns_rsg_create(
          identity,
          msgPtr,
          message.length,
          embed ? 1 : 0,
          out,
          needed.value,
          written,
        );
        if (code != RnsError.ok) {
          return (blob: null, code: code);
        }
        return (
          blob: List<int>.from(out.asTypedList(written.value)),
          code: code,
        );
      } finally {
        calloc.free(out);
        calloc.free(written);
      }
    } finally {
      if (msgPtr != nullptr) {
        calloc.free(msgPtr);
      }
      calloc.free(needed);
    }
  }

  int rsgValidate(
    List<int> rsg,
    List<int> message, [
    List<int> requiredSignerHash = const [],
  ]) {
    final rsgPtr = rsg.isEmpty ? nullptr : calloc<Uint8>(rsg.length);
    final msgPtr = message.isEmpty ? nullptr : calloc<Uint8>(message.length);
    final reqPtr = requiredSignerHash.isEmpty
        ? nullptr
        : calloc<Uint8>(requiredSignerHash.length);
    try {
      if (rsg.isNotEmpty) {
        rsgPtr.asTypedList(rsg.length).setAll(0, rsg);
      }
      if (message.isNotEmpty) {
        msgPtr.asTypedList(message.length).setAll(0, message);
      }
      if (requiredSignerHash.isNotEmpty) {
        reqPtr.asTypedList(requiredSignerHash.length).setAll(
              0,
              requiredSignerHash,
            );
      }
      return _api.rns_rsg_validate(
        rsgPtr,
        rsg.length,
        msgPtr,
        message.length,
        reqPtr,
        requiredSignerHash.length,
      );
    } finally {
      if (rsgPtr != nullptr) {
        calloc.free(rsgPtr);
      }
      if (msgPtr != nullptr) {
        calloc.free(msgPtr);
      }
      if (reqPtr != nullptr) {
        calloc.free(reqPtr);
      }
    }
  }

  int destinationCreate(
    int node, {
    int identity = 0,
    required String appName,
    List<String> aspects = const [],
    bool acceptsLinks = true,
  }) {
    final app = appName.toNativeUtf8();
    Pointer<Pointer<Char>> aspectPtrs = nullptr;
    final aspectNative = <Pointer<Utf8>>[];
    try {
      if (aspects.isNotEmpty) {
        aspectPtrs = calloc<Pointer<Char>>(aspects.length);
        for (var i = 0; i < aspects.length; i++) {
          final a = aspects[i].toNativeUtf8();
          aspectNative.add(a);
          aspectPtrs[i] = a.cast();
        }
      }
      return _api.rns_destination_create(
        node,
        identity,
        app.cast(),
        aspectPtrs,
        aspects.length,
        acceptsLinks ? 1 : 0,
      );
    } finally {
      malloc.free(app);
      for (final a in aspectNative) {
        malloc.free(a);
      }
      if (aspectPtrs != nullptr) {
        calloc.free(aspectPtrs);
      }
    }
  }

  int destinationAnnounce(int destination, [List<int>? appData]) {
    if (appData == null || appData.isEmpty) {
      return _api.rns_destination_announce(destination, nullptr, 0);
    }
    final buf = calloc<Uint8>(appData.length);
    try {
      buf.asTypedList(appData.length).setAll(0, appData);
      return _api.rns_destination_announce(destination, buf, appData.length);
    } finally {
      calloc.free(buf);
    }
  }

  List<int>? destinationHash(int destination) {
    final out = calloc<Uint8>(rnsHashLen);
    final written = calloc<Size>();
    try {
      final code =
          _api.rns_destination_hash(destination, out, rnsHashLen, written);
      if (code != RnsError.ok || written.value != rnsHashLen) {
        return null;
      }
      return List<int>.from(out.asTypedList(rnsHashLen));
    } finally {
      calloc.free(out);
      calloc.free(written);
    }
  }

  int destinationDestroy(int destination) =>
      _api.rns_destination_destroy(destination);

  int destinationRegisterRequestHandler(int destination, String path) {
    final p = path.toNativeUtf8();
    try {
      return _api.rns_destination_register_request_handler(
        destination,
        p.cast(),
      );
    } finally {
      malloc.free(p);
    }
  }

  int pathRequest(int node, List<int> destHash) {
    if (destHash.length != rnsHashLen) {
      return RnsError.invalidArg;
    }
    final buf = calloc<Uint8>(rnsHashLen);
    try {
      buf.asTypedList(rnsHashLen).setAll(0, destHash);
      return _api.rns_path_request(node, buf);
    } finally {
      calloc.free(buf);
    }
  }

  ({List<RnsPathEntryView> entries, int code}) pathTable(
    int node, {
    int capacity = 64,
    int maxHops = -1,
  }) {
    final out = calloc<RnsPathEntryNative>(capacity);
    final written = calloc<Size>();
    try {
      final code =
          _api.rns_path_table(node, out, capacity, written, maxHops);
      if (code != RnsError.ok) {
        return (entries: const [], code: code);
      }
      final n = written.value;
      final entries = <RnsPathEntryView>[];
      for (var i = 0; i < n; i++) {
        entries.add(RnsPathEntryView.fromNative(out[i]));
      }
      return (entries: entries, code: code);
    } finally {
      calloc.free(out);
      calloc.free(written);
    }
  }

  ({List<RnsInterfaceEntryView> entries, int code}) interfacesList(
    int node, {
    int capacity = 32,
  }) {
    final out = calloc<RnsInterfaceEntryNative>(capacity);
    final written = calloc<Size>();
    try {
      final code = _api.rns_interfaces(node, out, capacity, written);
      if (code != RnsError.ok && code != RnsError.truncated) {
        return (entries: const [], code: code);
      }
      final n = written.value;
      final entries = <RnsInterfaceEntryView>[];
      for (var i = 0; i < n; i++) {
        entries.add(RnsInterfaceEntryView.fromNative(out[i]));
      }
      return (entries: entries, code: code);
    } finally {
      calloc.free(out);
      calloc.free(written);
    }
  }

  int linkOpen(int node, List<int> destHash) {
    if (destHash.length != rnsHashLen) {
      return 0;
    }
    final buf = calloc<Uint8>(rnsHashLen);
    try {
      buf.asTypedList(rnsHashLen).setAll(0, destHash);
      return _api.rns_link_open(node, buf);
    } finally {
      calloc.free(buf);
    }
  }

  int linkSend(int link, List<int> data) {
    if (data.isEmpty) {
      return RnsError.invalidArg;
    }
    final buf = calloc<Uint8>(data.length);
    try {
      buf.asTypedList(data.length).setAll(0, data);
      return _api.rns_link_send(link, buf, data.length);
    } finally {
      calloc.free(buf);
    }
  }

  int linkSendResource(int link, List<int> data, String name) {
    if (data.isEmpty) {
      return RnsError.invalidArg;
    }
    final buf = calloc<Uint8>(data.length);
    final nameNative = name.toNativeUtf8();
    try {
      buf.asTypedList(data.length).setAll(0, data);
      return _api.rns_link_send_resource(
        link,
        buf,
        data.length,
        nameNative.cast(),
      );
    } finally {
      calloc.free(buf);
      malloc.free(nameNative);
    }
  }

  int linkClose(int link) => _api.rns_link_close(link);

  List<int>? linkId(int link) {
    final out = calloc<Uint8>(rnsHashLen);
    final written = calloc<Size>();
    try {
      final code = _api.rns_link_id(link, out, rnsHashLen, written);
      if (code != RnsError.ok || written.value != rnsHashLen) {
        return null;
      }
      return List<int>.from(out.asTypedList(rnsHashLen));
    } finally {
      calloc.free(out);
      calloc.free(written);
    }
  }

  ({List<int>? requestId, int code}) linkRequest(
    int node,
    int link,
    String path, {
    List<int>? data,
    int timeoutMs = 5000,
  }) {
    final pathNative = path.toNativeUtf8();
    final out = calloc<Uint8>(rnsHashLen);
    final written = calloc<Size>();
    Pointer<Uint8> dataPtr = nullptr;
    try {
      var dataLen = 0;
      if (data != null && data.isNotEmpty) {
        dataLen = data.length;
        dataPtr = calloc<Uint8>(dataLen);
        dataPtr.asTypedList(dataLen).setAll(0, data);
      }
      final code = _api.rns_link_request(
        node,
        link,
        pathNative.cast(),
        dataPtr,
        dataLen,
        timeoutMs,
        out,
        rnsHashLen,
        written,
      );
      if (code != RnsError.ok) {
        return (requestId: null, code: code);
      }
      return (
        requestId: List<int>.from(out.asTypedList(written.value)),
        code: code,
      );
    } finally {
      malloc.free(pathNative);
      calloc.free(out);
      calloc.free(written);
      if (dataPtr != nullptr) {
        calloc.free(dataPtr);
      }
    }
  }

  int requestRespond(int node, List<int> requestId, [List<int>? data]) {
    if (requestId.isEmpty) {
      return RnsError.invalidArg;
    }
    final idBuf = calloc<Uint8>(requestId.length);
    Pointer<Uint8> dataPtr = nullptr;
    try {
      idBuf.asTypedList(requestId.length).setAll(0, requestId);
      var dataLen = 0;
      if (data != null && data.isNotEmpty) {
        dataLen = data.length;
        dataPtr = calloc<Uint8>(dataLen);
        dataPtr.asTypedList(dataLen).setAll(0, data);
      }
      return _api.rns_request_respond(
        node,
        idBuf,
        requestId.length,
        dataPtr,
        dataLen,
      );
    } finally {
      calloc.free(idBuf);
      if (dataPtr != nullptr) {
        calloc.free(dataPtr);
      }
    }
  }

  int requestRespondFile(
    int node,
    List<int> requestId,
    String filename,
    List<int> data,
  ) {
    if (requestId.isEmpty) {
      return RnsError.invalidArg;
    }
    final idBuf = calloc<Uint8>(requestId.length);
    final nameNative = filename.toNativeUtf8();
    final dataPtr = calloc<Uint8>(data.length);
    try {
      idBuf.asTypedList(requestId.length).setAll(0, requestId);
      dataPtr.asTypedList(data.length).setAll(0, data);
      return _api.rns_request_respond_file(
        node,
        idBuf,
        requestId.length,
        nameNative.cast(),
        dataPtr,
        data.length,
      );
    } finally {
      calloc.free(idBuf);
      malloc.free(nameNative);
      calloc.free(dataPtr);
    }
  }

  ({RnsEventView? event, int code}) eventPoll(
    int node, {
    int timeoutMs = 0,
    int appDataCap = 4096,
  }) {
    final event = calloc<RnsEventNative>();
    Pointer<Uint8> appBuf = nullptr;
    try {
      if (appDataCap > 0) {
        appBuf = calloc<Uint8>(appDataCap);
        event.ref.app_data = appBuf;
        event.ref.app_data_cap = appDataCap;
      }
      final code = _api.rns_event_poll(node, event, timeoutMs);
      if (code != RnsError.ok) {
        return (event: null, code: code);
      }
      return (event: RnsEventView.fromNative(event.ref), code: code);
    } finally {
      if (appBuf != nullptr) {
        calloc.free(appBuf);
      }
      calloc.free(event);
    }
  }
}
