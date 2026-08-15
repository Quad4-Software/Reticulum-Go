// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

class ControlApiException implements Exception {
  ControlApiException(this.message, {this.statusCode});

  final String message;
  final int? statusCode;

  @override
  String toString() {
    if (statusCode == null) {
      return 'ControlApiException: $message';
    }
    return 'ControlApiException($statusCode): $message';
  }
}
