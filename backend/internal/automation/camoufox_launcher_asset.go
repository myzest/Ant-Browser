package automation

import _ "embed"

const camoufoxLauncherFileName = "camoufox_launcher.cjs"

//go:embed assets/camoufox_launcher.cjs
var camoufoxLauncherScriptContent []byte

// CamoufoxLauncherScript 返回内嵌的 Camoufox launcher 脚本字节。
func CamoufoxLauncherScript() []byte {
	out := make([]byte, len(camoufoxLauncherScriptContent))
	copy(out, camoufoxLauncherScriptContent)
	return out
}
