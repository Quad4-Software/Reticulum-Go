// SPDX-License-Identifier: LicenseRef-Reticulum
package rrc

import "log"

func recoverLog() {
	if r := recover(); r != nil {
		log.Printf("rrc: recovered: %v", r)
	}
}
