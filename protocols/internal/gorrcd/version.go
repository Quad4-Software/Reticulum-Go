// SPDX-License-Identifier: LicenseRef-Reticulum
package gorrcd

// Version is the gorrcd release string. Override at link time with
// -X github.com/Quad4-Software/reticulum-go-protocols/internal/gorrcd.Version=vX.Y.Z
var Version = "0.1.0"

// BuildDate is the UTC build timestamp. Override at link time with
// -X github.com/Quad4-Software/reticulum-go-protocols/internal/gorrcd.BuildDate=...
var BuildDate = ""

// VersionLine returns the version string shown in banners and --version.
func VersionLine() string {
	if BuildDate != "" {
		return Version + " (built " + BuildDate + ")"
	}
	return Version
}
