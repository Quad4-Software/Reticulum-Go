// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

const (
	AppName  = "git"
	Aspect   = "repositories"
	ProtoRNS = "rns://"

	PathList   = "/git/list"
	PathFetch  = "/git/fetch"
	PathPush   = "/git/push"
	PathDelete = "/git/delete"
	PathCreate = "/git/create"
	PathFork   = "/git/fork"
	PathSync   = "/git/sync"
	PathMirror = "/git/mirror"

	PathRelease = "/mgmt/release"
	PathWork    = "/mgmt/work"
	PathPerms   = "/mgmt/perms"

	ResOK         = 0x00
	ResDisallowed = 0x01
	ResInvalidReq = 0x02
	ResNotFound   = 0x03
	ResRemoteFail = 0xFF

	IdxRepository = 0x00
	IdxResultCode = 0x01
	IdxGroup      = 0x02

	DefaultRefBatchSize = 25
	DefaultPathTimeout  = 15
	DefaultLinkTimeout  = 15
)
