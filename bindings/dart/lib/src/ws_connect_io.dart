// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

import 'package:web_socket_channel/io.dart';
import 'package:web_socket_channel/web_socket_channel.dart';

WebSocketChannel connectControlEvents({
  required Uri uri,
  required String rpcKey,
}) {
  return IOWebSocketChannel.connect(
    uri,
    headers: {'Authorization': 'Bearer $rpcKey'},
  );
}
