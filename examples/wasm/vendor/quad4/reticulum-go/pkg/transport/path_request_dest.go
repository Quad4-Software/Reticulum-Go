// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import "crypto/sha256"

// pathRequestDestinationHash is the truncated destination hash for
// rnstransport.path.request (Python Transport.pr_destination_hash).
func pathRequestDestinationHash() []byte {
	nameHashFull := sha256.Sum256([]byte("rnstransport.path.request"))
	finalHashFull := sha256.Sum256(nameHashFull[:10])
	return finalHashFull[:16]
}
