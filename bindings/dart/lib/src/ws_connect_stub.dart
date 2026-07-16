// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

import 'package:web_socket_channel/web_socket_channel.dart';

WebSocketChannel connectControlEvents({
  required Uri uri,
  required String rpcKey,
}) {
  throw UnsupportedError(
    'authenticated WebSocket events require dart:io '
    '(Flutter mobile or desktop). Browser WebSocket cannot set Authorization.',
  );
}
