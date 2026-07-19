// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// ignore_for_file: non_constant_identifier_names, library_private_types_in_public_api

import 'dart:ffi';

import 'types.dart';

typedef _VersionC = Pointer<Char> Function();
typedef _VersionDart = Pointer<Char> Function();

typedef _LastErrorC = Int32 Function(
  Pointer<Char> buf,
  Size bufLen,
  Pointer<Size> written,
);
typedef _LastErrorDart = int Function(
  Pointer<Char> buf,
  int bufLen,
  Pointer<Size> written,
);

typedef _NodeCreateC = Uint64 Function(Pointer<Char> configPath);
typedef _NodeCreateDart = int Function(Pointer<Char> configPath);

typedef _NodeOpC = Int32 Function(Uint64 node);
typedef _NodeOpDart = int Function(int node);

typedef _NodeSetIdentityC = Int32 Function(Uint64 node, Uint64 identity);
typedef _NodeSetIdentityDart = int Function(int node, int identity);

typedef _NodeRefreshPathsC = Int32 Function(
  Uint64 node,
  Pointer<Uint8> destHashes,
  Size count,
);
typedef _NodeRefreshPathsDart = int Function(
  int node,
  Pointer<Uint8> destHashes,
  int count,
);

typedef _IdentityGenerateC = Uint64 Function();
typedef _IdentityGenerateDart = int Function();

typedef _IdentityLoadC = Uint64 Function(Pointer<Char> path);
typedef _IdentityLoadDart = int Function(Pointer<Char> path);

typedef _IdentitySaveC = Int32 Function(Uint64 identity, Pointer<Char> path);
typedef _IdentitySaveDart = int Function(int identity, Pointer<Char> path);

typedef _IdentityDestroyC = Int32 Function(Uint64 identity);
typedef _IdentityDestroyDart = int Function(int identity);

typedef _IdentityHashC = Int32 Function(
  Uint64 identity,
  Pointer<Char> hexBuf,
  Size hexBufLen,
  Pointer<Size> written,
);
typedef _IdentityHashDart = int Function(
  int identity,
  Pointer<Char> hexBuf,
  int hexBufLen,
  Pointer<Size> written,
);

typedef _IdentityBytesC = Int32 Function(
  Uint64 identity,
  Pointer<Uint8> out,
  Size outLen,
  Pointer<Size> written,
);
typedef _IdentityBytesDart = int Function(
  int identity,
  Pointer<Uint8> out,
  int outLen,
  Pointer<Size> written,
);

typedef _IdentityFromPublicKeyC = Uint64 Function(
  Pointer<Uint8> pub,
  Size pubLen,
);
typedef _IdentityFromPublicKeyDart = int Function(
  Pointer<Uint8> pub,
  int pubLen,
);

typedef _IdentitySignC = Int32 Function(
  Uint64 identity,
  Pointer<Uint8> data,
  Size dataLen,
  Pointer<Uint8> sigOut,
  Size sigOutLen,
  Pointer<Size> written,
);
typedef _IdentitySignDart = int Function(
  int identity,
  Pointer<Uint8> data,
  int dataLen,
  Pointer<Uint8> sigOut,
  int sigOutLen,
  Pointer<Size> written,
);

typedef _IdentityVerifyC = Int32 Function(
  Uint64 identity,
  Pointer<Uint8> data,
  Size dataLen,
  Pointer<Uint8> sig,
  Size sigLen,
);
typedef _IdentityVerifyDart = int Function(
  int identity,
  Pointer<Uint8> data,
  int dataLen,
  Pointer<Uint8> sig,
  int sigLen,
);

typedef _RsgCreateC = Int32 Function(
  Uint64 identity,
  Pointer<Uint8> message,
  Size messageLen,
  Int32 embed,
  Pointer<Uint8> out,
  Size outLen,
  Pointer<Size> written,
);
typedef _RsgCreateDart = int Function(
  int identity,
  Pointer<Uint8> message,
  int messageLen,
  int embed,
  Pointer<Uint8> out,
  int outLen,
  Pointer<Size> written,
);

typedef _RsgValidateC = Int32 Function(
  Pointer<Uint8> rsg,
  Size rsgLen,
  Pointer<Uint8> message,
  Size messageLen,
  Pointer<Uint8> requiredSignerHash,
  Size requiredSignerHashLen,
);
typedef _RsgValidateDart = int Function(
  Pointer<Uint8> rsg,
  int rsgLen,
  Pointer<Uint8> message,
  int messageLen,
  Pointer<Uint8> requiredSignerHash,
  int requiredSignerHashLen,
);

typedef _RsgSignFileC = Int32 Function(
  Uint64 identity,
  Pointer<Char> path,
  Pointer<Uint8> out,
  Size outLen,
  Pointer<Size> written,
);
typedef _RsgSignFileDart = int Function(
  int identity,
  Pointer<Char> path,
  Pointer<Uint8> out,
  int outLen,
  Pointer<Size> written,
);

typedef _RsgVerifyFileC = Int32 Function(
  Pointer<Uint8> rsg,
  Size rsgLen,
  Pointer<Char> path,
  Pointer<Uint8> requiredSignerHash,
  Size requiredSignerHashLen,
);
typedef _RsgVerifyFileDart = int Function(
  Pointer<Uint8> rsg,
  int rsgLen,
  Pointer<Char> path,
  Pointer<Uint8> requiredSignerHash,
  int requiredSignerHashLen,
);

typedef _RsmVerifyC = Int32 Function(
  Pointer<Uint8> rsm,
  Size rsmLen,
  Pointer<Uint8> requiredSignerHash,
  Size requiredSignerHashLen,
  Pointer<Uint8> messageOut,
  Size messageOutLen,
  Pointer<Size> written,
);
typedef _RsmVerifyDart = int Function(
  Pointer<Uint8> rsm,
  int rsmLen,
  Pointer<Uint8> requiredSignerHash,
  int requiredSignerHashLen,
  Pointer<Uint8> messageOut,
  int messageOutLen,
  Pointer<Size> written,
);

typedef _DestinationCreateC = Uint64 Function(
  Uint64 node,
  Uint64 identity,
  Pointer<Char> appName,
  Pointer<Pointer<Char>> aspects,
  Size aspectCount,
  Int32 acceptsLinks,
);
typedef _DestinationCreateDart = int Function(
  int node,
  int identity,
  Pointer<Char> appName,
  Pointer<Pointer<Char>> aspects,
  int aspectCount,
  int acceptsLinks,
);

typedef _DestinationAnnounceC = Int32 Function(
  Uint64 destination,
  Pointer<Uint8> appData,
  Size appDataLen,
);
typedef _DestinationAnnounceDart = int Function(
  int destination,
  Pointer<Uint8> appData,
  int appDataLen,
);

typedef _DestinationHashC = Int32 Function(
  Uint64 destination,
  Pointer<Uint8> hashOut,
  Size hashOutLen,
  Pointer<Size> written,
);
typedef _DestinationHashDart = int Function(
  int destination,
  Pointer<Uint8> hashOut,
  int hashOutLen,
  Pointer<Size> written,
);

typedef _DestinationDestroyC = Int32 Function(Uint64 destination);
typedef _DestinationDestroyDart = int Function(int destination);

typedef _DestinationRegisterRequestHandlerC = Int32 Function(
  Uint64 destination,
  Pointer<Char> path,
);
typedef _DestinationRegisterRequestHandlerDart = int Function(
  int destination,
  Pointer<Char> path,
);

typedef _PathRequestC = Int32 Function(Uint64 node, Pointer<Uint8> destHash);
typedef _PathRequestDart = int Function(int node, Pointer<Uint8> destHash);

typedef _PathTableC = Int32 Function(
  Uint64 node,
  Pointer<RnsPathEntryNative> out,
  Size outCap,
  Pointer<Size> written,
  Int32 maxHops,
);
typedef _PathTableDart = int Function(
  int node,
  Pointer<RnsPathEntryNative> out,
  int outCap,
  Pointer<Size> written,
  int maxHops,
);

typedef _InterfacesC = Int32 Function(
  Uint64 node,
  Pointer<RnsInterfaceEntryNative> out,
  Size outCap,
  Pointer<Size> written,
);
typedef _InterfacesDart = int Function(
  int node,
  Pointer<RnsInterfaceEntryNative> out,
  int outCap,
  Pointer<Size> written,
);

typedef _LinkOpenC = Uint64 Function(Uint64 node, Pointer<Uint8> destHash);
typedef _LinkOpenDart = int Function(int node, Pointer<Uint8> destHash);

typedef _LinkSendC = Int32 Function(
  Uint64 link,
  Pointer<Uint8> data,
  Size dataLen,
);
typedef _LinkSendDart = int Function(
  int link,
  Pointer<Uint8> data,
  int dataLen,
);

typedef _LinkCloseC = Int32 Function(Uint64 link);
typedef _LinkCloseDart = int Function(int link);

typedef _LinkIdC = Int32 Function(
  Uint64 link,
  Pointer<Uint8> idOut,
  Size idOutLen,
  Pointer<Size> written,
);
typedef _LinkIdDart = int Function(
  int link,
  Pointer<Uint8> idOut,
  int idOutLen,
  Pointer<Size> written,
);

typedef _LinkRequestC = Int32 Function(
  Uint64 node,
  Uint64 link,
  Pointer<Char> path,
  Pointer<Uint8> data,
  Size dataLen,
  Int32 timeoutMs,
  Pointer<Uint8> requestIdOut,
  Size requestIdOutLen,
  Pointer<Size> written,
);
typedef _LinkRequestDart = int Function(
  int node,
  int link,
  Pointer<Char> path,
  Pointer<Uint8> data,
  int dataLen,
  int timeoutMs,
  Pointer<Uint8> requestIdOut,
  int requestIdOutLen,
  Pointer<Size> written,
);

typedef _RequestRespondC = Int32 Function(
  Uint64 node,
  Pointer<Uint8> requestId,
  Size requestIdLen,
  Pointer<Uint8> data,
  Size dataLen,
);
typedef _RequestRespondDart = int Function(
  int node,
  Pointer<Uint8> requestId,
  int requestIdLen,
  Pointer<Uint8> data,
  int dataLen,
);

typedef _LinkSendResourceC = Int32 Function(
  Uint64 link,
  Pointer<Uint8> data,
  Size dataLen,
  Pointer<Char> name,
);
typedef _LinkSendResourceDart = int Function(
  int link,
  Pointer<Uint8> data,
  int dataLen,
  Pointer<Char> name,
);

typedef _RequestRespondFileC = Int32 Function(
  Uint64 node,
  Pointer<Uint8> requestId,
  Size requestIdLen,
  Pointer<Char> filename,
  Pointer<Uint8> data,
  Size dataLen,
);
typedef _RequestRespondFileDart = int Function(
  int node,
  Pointer<Uint8> requestId,
  int requestIdLen,
  Pointer<Char> filename,
  Pointer<Uint8> data,
  int dataLen,
);

typedef _EventPollC = Int32 Function(
  Uint64 node,
  Pointer<RnsEventNative> event,
  Int32 timeoutMs,
);
typedef _EventPollDart = int Function(
  int node,
  Pointer<RnsEventNative> event,
  int timeoutMs,
);

typedef _EventCallbackC = Void Function(
  Pointer<RnsEventNative> event,
  Pointer<Void> userData,
);
typedef _SetEventCallbackC = Int32 Function(
  Uint64 node,
  Pointer<NativeFunction<_EventCallbackC>> callback,
  Pointer<Void> userData,
);
typedef _SetEventCallbackDart = int Function(
  int node,
  Pointer<NativeFunction<_EventCallbackC>> callback,
  Pointer<Void> userData,
);

class RnsBindings {
  RnsBindings(DynamicLibrary lib)
      : rns_version = lib.lookupFunction<_VersionC, _VersionDart>('rns_version'),
        rns_last_error =
            lib.lookupFunction<_LastErrorC, _LastErrorDart>('rns_last_error'),
        rns_node_create =
            lib.lookupFunction<_NodeCreateC, _NodeCreateDart>('rns_node_create'),
        rns_node_start =
            lib.lookupFunction<_NodeOpC, _NodeOpDart>('rns_node_start'),
        rns_node_stop =
            lib.lookupFunction<_NodeOpC, _NodeOpDart>('rns_node_stop'),
        rns_node_destroy =
            lib.lookupFunction<_NodeOpC, _NodeOpDart>('rns_node_destroy'),
        rns_node_set_identity =
            lib.lookupFunction<_NodeSetIdentityC, _NodeSetIdentityDart>(
          'rns_node_set_identity',
        ),
        rns_node_resume =
            lib.lookupFunction<_NodeOpC, _NodeOpDart>('rns_node_resume'),
        rns_node_pause =
            lib.lookupFunction<_NodeOpC, _NodeOpDart>('rns_node_pause'),
        rns_node_refresh_paths =
            lib.lookupFunction<_NodeRefreshPathsC, _NodeRefreshPathsDart>(
          'rns_node_refresh_paths',
        ),
        rns_identity_generate =
            lib.lookupFunction<_IdentityGenerateC, _IdentityGenerateDart>(
          'rns_identity_generate',
        ),
        rns_identity_load =
            lib.lookupFunction<_IdentityLoadC, _IdentityLoadDart>(
          'rns_identity_load',
        ),
        rns_identity_save =
            lib.lookupFunction<_IdentitySaveC, _IdentitySaveDart>(
          'rns_identity_save',
        ),
        rns_identity_destroy =
            lib.lookupFunction<_IdentityDestroyC, _IdentityDestroyDart>(
          'rns_identity_destroy',
        ),
        rns_identity_hash =
            lib.lookupFunction<_IdentityHashC, _IdentityHashDart>(
          'rns_identity_hash',
        ),
        rns_identity_hash_bytes =
            lib.lookupFunction<_IdentityBytesC, _IdentityBytesDart>(
          'rns_identity_hash_bytes',
        ),
        rns_identity_public_key =
            lib.lookupFunction<_IdentityBytesC, _IdentityBytesDart>(
          'rns_identity_public_key',
        ),
        rns_identity_from_public_key = lib.lookupFunction<
            _IdentityFromPublicKeyC, _IdentityFromPublicKeyDart>(
          'rns_identity_from_public_key',
        ),
        rns_identity_sign =
            lib.lookupFunction<_IdentitySignC, _IdentitySignDart>(
          'rns_identity_sign',
        ),
        rns_identity_verify =
            lib.lookupFunction<_IdentityVerifyC, _IdentityVerifyDart>(
          'rns_identity_verify',
        ),
        rns_rsg_create =
            lib.lookupFunction<_RsgCreateC, _RsgCreateDart>('rns_rsg_create'),
        rns_rsg_validate =
            lib.lookupFunction<_RsgValidateC, _RsgValidateDart>(
          'rns_rsg_validate',
        ),
        rns_rsg_sign_file =
            lib.lookupFunction<_RsgSignFileC, _RsgSignFileDart>(
          'rns_rsg_sign_file',
        ),
        rns_rsg_verify_file =
            lib.lookupFunction<_RsgVerifyFileC, _RsgVerifyFileDart>(
          'rns_rsg_verify_file',
        ),
        rns_rsm_verify =
            lib.lookupFunction<_RsmVerifyC, _RsmVerifyDart>('rns_rsm_verify'),
        rns_destination_create =
            lib.lookupFunction<_DestinationCreateC, _DestinationCreateDart>(
          'rns_destination_create',
        ),
        rns_destination_announce =
            lib.lookupFunction<_DestinationAnnounceC, _DestinationAnnounceDart>(
          'rns_destination_announce',
        ),
        rns_destination_hash =
            lib.lookupFunction<_DestinationHashC, _DestinationHashDart>(
          'rns_destination_hash',
        ),
        rns_destination_destroy =
            lib.lookupFunction<_DestinationDestroyC, _DestinationDestroyDart>(
          'rns_destination_destroy',
        ),
        rns_destination_register_request_handler = lib.lookupFunction<
            _DestinationRegisterRequestHandlerC,
            _DestinationRegisterRequestHandlerDart>(
          'rns_destination_register_request_handler',
        ),
        rns_path_request =
            lib.lookupFunction<_PathRequestC, _PathRequestDart>(
          'rns_path_request',
        ),
        rns_path_table =
            lib.lookupFunction<_PathTableC, _PathTableDart>('rns_path_table'),
        rns_interfaces =
            lib.lookupFunction<_InterfacesC, _InterfacesDart>('rns_interfaces'),
        rns_link_open =
            lib.lookupFunction<_LinkOpenC, _LinkOpenDart>('rns_link_open'),
        rns_link_send =
            lib.lookupFunction<_LinkSendC, _LinkSendDart>('rns_link_send'),
        rns_link_send_resource =
            lib.lookupFunction<_LinkSendResourceC, _LinkSendResourceDart>(
          'rns_link_send_resource',
        ),
        rns_link_close =
            lib.lookupFunction<_LinkCloseC, _LinkCloseDart>('rns_link_close'),
        rns_link_id =
            lib.lookupFunction<_LinkIdC, _LinkIdDart>('rns_link_id'),
        rns_link_request =
            lib.lookupFunction<_LinkRequestC, _LinkRequestDart>(
          'rns_link_request',
        ),
        rns_request_respond =
            lib.lookupFunction<_RequestRespondC, _RequestRespondDart>(
          'rns_request_respond',
        ),
        rns_request_respond_file =
            lib.lookupFunction<_RequestRespondFileC, _RequestRespondFileDart>(
          'rns_request_respond_file',
        ),
        rns_event_poll =
            lib.lookupFunction<_EventPollC, _EventPollDart>('rns_event_poll'),
        rns_set_event_callback =
            lib.lookupFunction<_SetEventCallbackC, _SetEventCallbackDart>(
          'rns_set_event_callback',
        );

  final _VersionDart rns_version;
  final _LastErrorDart rns_last_error;
  final _NodeCreateDart rns_node_create;
  final _NodeOpDart rns_node_start;
  final _NodeOpDart rns_node_stop;
  final _NodeOpDart rns_node_destroy;
  final _NodeSetIdentityDart rns_node_set_identity;
  final _NodeOpDart rns_node_resume;
  final _NodeOpDart rns_node_pause;
  final _NodeRefreshPathsDart rns_node_refresh_paths;
  final _IdentityGenerateDart rns_identity_generate;
  final _IdentityLoadDart rns_identity_load;
  final _IdentitySaveDart rns_identity_save;
  final _IdentityDestroyDart rns_identity_destroy;
  final _IdentityHashDart rns_identity_hash;
  final _IdentityBytesDart rns_identity_hash_bytes;
  final _IdentityBytesDart rns_identity_public_key;
  final _IdentityFromPublicKeyDart rns_identity_from_public_key;
  final _IdentitySignDart rns_identity_sign;
  final _IdentityVerifyDart rns_identity_verify;
  final _RsgCreateDart rns_rsg_create;
  final _RsgValidateDart rns_rsg_validate;
  final _RsgSignFileDart rns_rsg_sign_file;
  final _RsgVerifyFileDart rns_rsg_verify_file;
  final _RsmVerifyDart rns_rsm_verify;
  final _DestinationCreateDart rns_destination_create;
  final _DestinationAnnounceDart rns_destination_announce;
  final _DestinationHashDart rns_destination_hash;
  final _DestinationDestroyDart rns_destination_destroy;
  final _DestinationRegisterRequestHandlerDart
      rns_destination_register_request_handler;
  final _PathRequestDart rns_path_request;
  final _PathTableDart rns_path_table;
  final _InterfacesDart rns_interfaces;
  final _LinkOpenDart rns_link_open;
  final _LinkSendDart rns_link_send;
  final _LinkSendResourceDart rns_link_send_resource;
  final _LinkCloseDart rns_link_close;
  final _LinkIdDart rns_link_id;
  final _LinkRequestDart rns_link_request;
  final _RequestRespondDart rns_request_respond;
  final _RequestRespondFileDart rns_request_respond_file;
  final _EventPollDart rns_event_poll;
  final _SetEventCallbackDart rns_set_event_callback;
}
