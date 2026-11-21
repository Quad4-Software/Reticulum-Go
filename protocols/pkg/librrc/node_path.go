// SPDX-License-Identifier: LicenseRef-Reticulum
package librrc

func NodeHasPath(nodeHandle uint64, destHash []byte) (bool, int) {
	rec, err := nodeByHandle(nodeHandle)
	if err != nil {
		return false, setLastError(err)
	}
	if len(destHash) != 16 {
		return false, setLastError(errInvalidArg)
	}
	return rec.transport.HasPath(destHash), OK
}
