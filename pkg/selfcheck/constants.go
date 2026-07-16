// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package selfcheck

import "time"

const (
	fileModePrivate  = 0o600
	dirModePrivate   = 0o700
	defaultTimeout   = 45 * time.Second
	daemonMaxTimeout = 30 * time.Second
	logTailLines     = 40
	logLineMaxChars  = 120
	logTailMaxChars  = 800
	detailMaxChars   = 200

	nameIdentityFile    = "identity/file"
	nameSandboxApply    = "sandbox/apply"
	nameIdentityKeyring = "identity/keyring"
	nameDaemonSmoke     = "daemon/sandbox-smoke"
)
