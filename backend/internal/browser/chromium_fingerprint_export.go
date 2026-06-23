package browser

import (
	"fmt"
	"strconv"
	"strings"
)

var chromiumFingerprintValueKeys = map[string]struct{}{
	"--fingerprint":                      {},
	"--fingerprint-brand":                {},
	"--fingerprint-platform":             {},
	"--lang":                             {},
	"--timezone":                         {},
	"--window-size":                      {},
	"--fingerprint-color-depth":          {},
	"--fingerprint-hardware-concurrency": {},
	"--fingerprint-device-memory":        {},
	"--fingerprint-canvas-noise":         {},
	"--fingerprint-webgl-vendor":         {},
	"--fingerprint-webgl-renderer":       {},
	"--fingerprint-audio-noise":          {},
	"--fingerprint-fonts":                {},
	"--webrtc-ip-handling-policy":        {},
	"--fingerprint-do-not-track":         {},
	"--fingerprint-touch-points":         {},
	"--fingerprint-media-devices":        {},
	"--user-agent":                       {},
	"--accept-language":                  {},
	"--accept-lang":                      {},
}

// ExportChromiumFingerprintArgs 把池条目导出为 fingerprint-chromium CLI 参数。
func ExportChromiumFingerprintArgs(entry ChromiumPoolEntry, seed int) []string {
	profile := entry.Profile
	args := []string{fmt.Sprintf("--fingerprint=%d", seed)}
	appendStringArg := func(key, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			args = append(args, key+"="+value)
		}
	}
	appendIntArg := func(key string, value int) {
		if value > 0 {
			args = append(args, fmt.Sprintf("%s=%d", key, value))
		}
	}
	appendNonNegativeIntArg := func(key string, value int) {
		if value >= 0 {
			args = append(args, fmt.Sprintf("%s=%d", key, value))
		}
	}
	appendBoolArg := func(key string, value bool) {
		args = append(args, key+"="+strconv.FormatBool(value))
	}

	appendStringArg("--fingerprint-brand", profile.Brand)
	appendStringArg("--fingerprint-platform", profile.Platform)
	appendStringArg("--lang", profile.Lang)
	appendStringArg("--timezone", profile.Timezone)
	appendStringArg("--window-size", profile.WindowSize)
	appendIntArg("--fingerprint-color-depth", profile.ColorDepth)
	appendIntArg("--fingerprint-hardware-concurrency", profile.HardwareConcurrency)
	appendIntArg("--fingerprint-device-memory", profile.DeviceMemory)
	appendBoolArg("--fingerprint-canvas-noise", profile.CanvasNoise)
	appendStringArg("--fingerprint-webgl-vendor", profile.WebGLVendor)
	appendStringArg("--fingerprint-webgl-renderer", profile.WebGLRenderer)
	appendBoolArg("--fingerprint-audio-noise", profile.AudioNoise)
	appendStringArg("--fingerprint-fonts", strings.Join(normalizeStringList(profile.Fonts), ","))
	appendStringArg("--webrtc-ip-handling-policy", profile.WebRTCPolicy)
	appendBoolArg("--fingerprint-do-not-track", profile.DoNotTrack)
	appendNonNegativeIntArg("--fingerprint-touch-points", profile.TouchPoints)
	return args
}

// BuildChromiumFingerprintPoolArgs 从内嵌池构建本次 Chromium 启动的池化指纹参数。
func BuildChromiumFingerprintPoolArgs(profileID string, hostOS string, userArgs []string) ([]string, error) {
	seed := ResolveChromiumFingerprintSeed(profileID, userArgs)
	osName := ResolveChromiumFingerprintOS(userArgs, hostOS)
	pool, err := LoadChromiumFingerprintPool()
	if err != nil {
		return nil, err
	}
	entry, err := pool.SelectChromiumFingerprint(osName, seed)
	if err != nil {
		return nil, err
	}
	return ExportChromiumFingerprintArgs(entry, seed), nil
}

// MergeChromiumFingerprintArgs 让池化参数只补用户没有显式设置的 fingerprint key。
func MergeChromiumFingerprintArgs(poolArgs []string, fingerprintArgs []string, launchArgs []string, extraArgs []string) []string {
	if len(poolArgs) == 0 {
		return normalizeStringList(fingerprintArgs)
	}
	userKeys := collectChromiumFingerprintArgKeys(fingerprintArgs, launchArgs, extraArgs)
	out := make([]string, 0, len(poolArgs)+len(fingerprintArgs))
	for _, arg := range normalizeStringList(poolArgs) {
		key := chromiumLaunchArgKey(arg)
		if key == "" {
			continue
		}
		if _, exists := userKeys[key]; exists {
			continue
		}
		out = append(out, arg)
	}
	out = append(out, normalizeStringList(fingerprintArgs)...)
	return out
}

func collectChromiumFingerprintArgKeys(argGroups ...[]string) map[string]struct{} {
	keys := map[string]struct{}{}
	for _, args := range argGroups {
		items := normalizeStringList(args)
		for i := 0; i < len(items); i++ {
			key := chromiumLaunchArgKey(items[i])
			if !isChromiumFingerprintValueKey(key) {
				continue
			}
			keys[key] = struct{}{}
			if !strings.Contains(items[i], "=") && i+1 < len(items) {
				next := strings.TrimSpace(items[i+1])
				if next != "" && !strings.HasPrefix(next, "-") {
					i++
				}
			}
		}
	}
	return keys
}

func chromiumLaunchArgKey(arg string) string {
	arg = strings.TrimSpace(arg)
	if arg == "" || !strings.HasPrefix(arg, "--") {
		return ""
	}
	key := arg
	if idx := strings.IndexByte(arg, '='); idx >= 0 {
		key = arg[:idx]
	}
	return strings.ToLower(strings.TrimSpace(key))
}

func isChromiumFingerprintValueKey(key string) bool {
	if key == "" {
		return false
	}
	if _, ok := chromiumFingerprintValueKeys[key]; ok {
		return true
	}
	return strings.HasPrefix(key, "--fingerprint-")
}
