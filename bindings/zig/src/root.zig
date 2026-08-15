//! Idiomatic Zig bindings for the librns C ABI.
//!
//! Link against bin/librns.so. Keep include/rns.h as the ABI source of truth.

const types = @import("types.zig");
const errors = @import("errors.zig");
const node_mod = @import("node.zig");
const identity_mod = @import("identity.zig");
const destination_mod = @import("destination.zig");
const path_mod = @import("path.zig");
const interfaces_mod = @import("interfaces.zig");
const link_mod = @import("link.zig");
const event_mod = @import("event.zig");
const rsg_mod = @import("rsg.zig");
const util_mod = @import("util.zig");

pub const c = @import("c.zig");

pub const api_version = types.api_version;
pub const hash_len = types.hash_len;

pub const Node = types.Node;
pub const Identity = types.Identity;
pub const Destination = types.Destination;
pub const Link = types.Link;
pub const Error = types.Error;
pub const EventKind = types.EventKind;
pub const Event = types.Event;
pub const PathEntry = types.PathEntry;
pub const InterfaceEntry = types.InterfaceEntry;
pub const EventCallback = types.EventCallback;

pub const version = errors.version;
pub const lastError = errors.lastError;
pub const errorString = errors.errorString;

pub const node = struct {
    pub const create = node_mod.create;
    pub const start = node_mod.start;
    pub const stop = node_mod.stop;
    pub const destroy = node_mod.destroy;
    pub const setIdentity = node_mod.setIdentity;
    pub const pause = node_mod.pause;
    pub const resumeNode = node_mod.resumeNode;
    pub const refreshPaths = node_mod.refreshPaths;
};

pub const identity = struct {
    pub const generate = identity_mod.generate;
    pub const load = identity_mod.load;
    pub const save = identity_mod.save;
    pub const destroy = identity_mod.destroy;
    pub const hash = identity_mod.hash;
    pub const hashBytes = identity_mod.hashBytes;
    pub const publicKey = identity_mod.publicKey;
    pub const fromPublicKey = identity_mod.fromPublicKey;
    pub const sign = identity_mod.sign;
    pub const verify = identity_mod.verify;
};

pub const destination = struct {
    pub const create = destination_mod.create;
    pub const announce = destination_mod.announce;
    pub const hash = destination_mod.hash;
    pub const destroy = destination_mod.destroy;
    pub const registerRequestHandler = destination_mod.registerRequestHandler;
};

pub const path = struct {
    pub const request = path_mod.request;
    pub const table = path_mod.table;
};

pub const interfaces = struct {
    pub const list = interfaces_mod.list;
    pub const name = interfaces_mod.name;
    pub const typeName = interfaces_mod.typeName;
};

pub const link = struct {
    pub const open = link_mod.open;
    pub const send = link_mod.send;
    pub const sendResource = link_mod.sendResource;
    pub const close = link_mod.close;
    pub const id = link_mod.id;
    pub const request = link_mod.request;
    pub const respond = link_mod.respond;
    pub const respondFile = link_mod.respondFile;
};

pub const event = struct {
    pub const poll = event_mod.poll;
    pub const setCallback = event_mod.setCallback;
    pub const appData = event_mod.appData;
    pub const path = event_mod.path;
    pub const errorMessage = event_mod.errorMessage;
    pub const linkId = event_mod.linkId;
    pub const destinationHash = event_mod.destinationHash;
    pub const identityHash = event_mod.identityHash;
    pub const requestId = event_mod.requestId;
};

pub const rsg = struct {
    pub const create = rsg_mod.create;
    pub const validate = rsg_mod.validate;
    pub const signFile = rsg_mod.signFile;
    pub const verifyFile = rsg_mod.verifyFile;
    pub const rsmVerify = rsg_mod.rsmVerify;
};

pub const util = struct {
    pub const hashToHex = util_mod.hashToHex;
    pub const hexToHash = util_mod.hexToHash;
    pub const cstringField = util_mod.cstringField;
};

pub const nodeCreate = node.create;
pub const nodeStart = node.start;
pub const nodeStop = node.stop;
pub const nodeDestroy = node.destroy;
pub const nodeSetIdentity = node.setIdentity;
pub const nodePause = node.pause;
pub const nodeResume = node.resumeNode;
pub const nodeRefreshPaths = node.refreshPaths;

pub const identityGenerate = identity.generate;
pub const identityLoad = identity.load;
pub const identitySave = identity.save;
pub const identityDestroy = identity.destroy;
pub const identityHash = identity.hash;
pub const identityHashBytes = identity.hashBytes;
pub const identityPublicKey = identity.publicKey;
pub const identityFromPublicKey = identity.fromPublicKey;
pub const identitySign = identity.sign;
pub const identityVerify = identity.verify;

pub const destinationCreate = destination.create;
pub const destinationAnnounce = destination.announce;
pub const destinationHash = destination.hash;
pub const destinationDestroy = destination.destroy;
pub const destinationRegisterRequestHandler = destination.registerRequestHandler;

pub const pathRequest = path.request;
pub const pathTable = path.table;
pub const interfacesList = interfaces.list;

pub const linkOpen = link.open;
pub const linkSend = link.send;
pub const linkSendResource = link.sendResource;
pub const linkClose = link.close;
pub const linkId = link.id;
pub const linkRequest = link.request;
pub const requestRespond = link.respond;
pub const requestRespondFile = link.respondFile;

pub const eventPoll = event.poll;
pub const setEventCallback = event.setCallback;
pub const eventAppData = event.appData;
pub const eventPath = event.path;
pub const eventErrorMessage = event.errorMessage;
pub const eventLinkId = event.linkId;
pub const eventDestinationHash = event.destinationHash;
pub const eventIdentityHash = event.identityHash;
pub const eventRequestId = event.requestId;

pub const rsgCreate = rsg.create;
pub const rsgValidate = rsg.validate;
pub const rsgSignFile = rsg.signFile;
pub const rsgVerifyFile = rsg.verifyFile;
pub const rsmVerify = rsg.rsmVerify;

pub const hashToHex = util.hashToHex;
pub const hexToHash = util.hexToHash;
