package browser

import "strings"

const (
	CoreTypeChromium = "chromium"
	CoreTypeCamoufox = "camoufox"

	RuntimeProtocolCDP        = "cdp"
	RuntimeProtocolPlaywright = "playwright"
)

func NormalizeCoreType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case CoreTypeCamoufox:
		return CoreTypeCamoufox
	default:
		return CoreTypeChromium
	}
}

func NormalizeRuntimeProtocol(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case RuntimeProtocolPlaywright:
		return RuntimeProtocolPlaywright
	default:
		return RuntimeProtocolCDP
	}
}

func RuntimeProtocolForCoreType(coreType string) string {
	if NormalizeCoreType(coreType) == CoreTypeCamoufox {
		return RuntimeProtocolPlaywright
	}
	return RuntimeProtocolCDP
}
