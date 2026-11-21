// SPDX-License-Identifier: LicenseRef-Reticulum
package proto

var profileByName = map[string]int{
	"ulbw": ProfileBandwidthUltraLow,
	"vlbw": ProfileBandwidthVeryLow,
	"lbw":  ProfileBandwidthLow,
	"mq":   ProfileQualityMedium,
	"hq":   ProfileQualityHigh,
	"shq":  ProfileQualityMax,
	"ll":   ProfileLatencyLow,
	"ull":  ProfileLatencyUltraLow,
}

func LookupProfile(name string) (int, bool) {
	p, ok := profileByName[name]
	return p, ok
}

func KnownProfile(profile int) bool {
	return ProfileName(profile) != "unknown"
}

func ProfileFromName(name string) int {
	if p, ok := LookupProfile(name); ok {
		return p
	}
	return DefaultProfile
}

var modeByName = map[string]int{
	"full": ModeFullDuplex,
	"fdx":  ModeFullDuplex,
	"half": ModeHalfDuplex,
	"hdx":  ModeHalfDuplex,
	"ptt":  ModeHalfDuplex,
}

func LookupMode(name string) (int, bool) {
	m, ok := modeByName[name]
	return m, ok
}

func ModeFromName(name string) int {
	if m, ok := LookupMode(name); ok {
		return m
	}
	return DefaultMode
}

func ModeName(mode int) string {
	switch mode {
	case ModeFullDuplex:
		return "full"
	case ModeHalfDuplex:
		return "half"
	default:
		return "unknown"
	}
}

func ProfileName(profile int) string {
	switch profile {
	case ProfileBandwidthUltraLow:
		return "ulbw"
	case ProfileBandwidthVeryLow:
		return "vlbw"
	case ProfileBandwidthLow:
		return "lbw"
	case ProfileQualityMedium:
		return "mq"
	case ProfileQualityHigh:
		return "hq"
	case ProfileQualityMax:
		return "shq"
	case ProfileLatencyLow:
		return "ll"
	case ProfileLatencyUltraLow:
		return "ull"
	default:
		return "unknown"
	}
}
