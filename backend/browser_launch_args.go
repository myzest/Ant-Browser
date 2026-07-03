package backend

import (
	"ant-chrome/backend/internal/logger"
	"ant-chrome/backend/internal/proxy"
	"strings"
)

type managedLaunchArgSpec struct {
	prefix     string
	takesValue bool
}

var managedLaunchArgSpecs = []managedLaunchArgSpec{
	{prefix: "--user-data-dir", takesValue: true},
	{prefix: "--remote-debugging-port", takesValue: true},
	{prefix: "--remote-debugging-address", takesValue: true},
	{prefix: "--remote-debugging-pipe", takesValue: false},
	{prefix: "--proxy-server", takesValue: true},
	{prefix: "--enable-automation", takesValue: false},
	{prefix: "--enable-unsafe-swiftshader", takesValue: false},
}

var singleValueLaunchArgPrefixes = []string{
	"--fingerprint",
	"--fingerprint-audio-noise",
	"--fingerprint-brand",
	"--fingerprint-canvas-noise",
	"--fingerprint-color-depth",
	"--fingerprint-device-memory",
	"--fingerprint-device-pixel-ratio",
	"--fingerprint-do-not-track",
	"--fingerprint-fonts",
	"--fingerprint-hardware-concurrency",
	"--fingerprint-locale",
	"--fingerprint-accept-language",
	"--fingerprint-media-devices",
	"--fingerprint-platform",
	"--fingerprint-screen-avail",
	"--fingerprint-timezone",
	"--fingerprint-touch-points",
	"--fingerprint-webgl-renderer",
	"--fingerprint-webgl-vendor",
	"--fingerprint-webrtc-ip",
	"--lang",
	"--timezone",
	"--window-position",
	"--webrtc-ip-handling-policy",
	"--window-size",
}

var singleValueLaunchArgAliases = map[string]string{
	"--accept-language": "--fingerprint-accept-language",
}

func sanitizeManagedLaunchArgs(args []string) ([]string, []string) {
	if len(args) == 0 {
		return nil, nil
	}

	sanitized := make([]string, 0, len(args))
	removed := make([]string, 0, 4)

	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == "" {
			continue
		}

		spec, matched := matchManagedLaunchArg(arg)
		if !matched {
			sanitized = append(sanitized, arg)
			continue
		}

		removed = appendUniqueString(removed, spec.prefix)
		if spec.takesValue && !strings.Contains(arg, "=") && i+1 < len(args) {
			next := strings.TrimSpace(args[i+1])
			if next != "" && !strings.HasPrefix(next, "-") {
				i++
			}
		}
	}

	return sanitized, removed
}

func matchManagedLaunchArg(arg string) (managedLaunchArgSpec, bool) {
	for _, spec := range managedLaunchArgSpecs {
		if strings.EqualFold(arg, spec.prefix) || strings.HasPrefix(strings.ToLower(arg), strings.ToLower(spec.prefix)+"=") {
			return spec, true
		}
	}
	return managedLaunchArgSpec{}, false
}

func mergeLaunchArgs(groups ...[]string) []string {
	if len(groups) == 0 {
		return nil
	}

	out := make([]string, 0)
	indexByKey := map[string]int{}
	for _, group := range groups {
		for i := 0; i < len(group); i++ {
			raw := group[i]
			arg := strings.TrimSpace(raw)
			if arg == "" {
				continue
			}
			key, ok := singleValueLaunchArgKey(arg)
			if ok {
				if argKey, value, hasValue := strings.Cut(arg, "="); hasValue {
					_ = argKey
					arg = key + "=" + strings.TrimSpace(value)
				} else if i+1 < len(group) {
					next := strings.TrimSpace(group[i+1])
					if next != "" && !strings.HasPrefix(next, "-") {
						arg = key + "=" + next
						i++
					}
				}
				if index, exists := indexByKey[key]; exists {
					out[index] = arg
					continue
				}
				indexByKey[key] = len(out)
			}
			out = append(out, arg)
		}
	}
	return out
}

func singleValueLaunchArgKey(arg string) (string, bool) {
	key := arg
	if eq := strings.Index(arg, "="); eq >= 0 {
		key = arg[:eq]
	}
	key = strings.ToLower(strings.TrimSpace(key))
	if canonical, ok := singleValueLaunchArgAliases[key]; ok {
		return canonical, true
	}
	for _, prefix := range singleValueLaunchArgPrefixes {
		if key == prefix {
			return key, true
		}
	}
	return "", false
}

func ensureDefaultFingerprintNetworkArgs(args []string, effectiveProxy string) []string {
	out := append([]string{}, args...)
	if !hasLaunchArgKey(out, "--webrtc-ip-handling-policy") {
		out = append(out, "--webrtc-ip-handling-policy=disable_non_proxied_udp")
	}
	if shouldResolveFingerprintWebRTCIP(effectiveProxy) && !hasLaunchArgKey(out, "--fingerprint-webrtc-ip") {
		out = append(out, "--fingerprint-webrtc-ip=auto")
	}
	return out
}

func hasLaunchArgKey(args []string, want string) bool {
	want = strings.ToLower(strings.TrimSpace(want))
	if canonical, ok := singleValueLaunchArgKey(want); ok {
		want = canonical
	}
	for _, arg := range args {
		key, ok := singleValueLaunchArgKey(arg)
		if ok && key == want {
			return true
		}
	}
	return false
}

func shouldResolveFingerprintWebRTCIP(effectiveProxy string) bool {
	value := strings.TrimSpace(effectiveProxy)
	return value != "" && !strings.EqualFold(value, "direct://")
}

func redactLaunchArgsForLog(args []string) string {
	if len(args) == 0 {
		return ""
	}
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == "" {
			continue
		}
		lower := strings.ToLower(arg)
		if strings.HasPrefix(lower, "--proxy-server=") {
			key, value, _ := strings.Cut(arg, "=")
			out = append(out, key+"="+proxy.RedactProxyURL(value))
			continue
		}
		if strings.EqualFold(arg, "--proxy-server") && i+1 < len(args) {
			next := strings.TrimSpace(args[i+1])
			if next != "" && !strings.HasPrefix(next, "-") {
				out = append(out, "--proxy-server="+proxy.RedactProxyURL(next))
				i++
				continue
			}
		}
		out = append(out, arg)
	}
	return strings.Join(out, " ")
}

func logManagedLaunchArgOverrides(log *logger.Logger, profileId string, source string, managedArgs []string) {
	if log == nil || len(managedArgs) == 0 {
		return
	}
	log.Warn("忽略由系统接管的浏览器启动参数",
		logger.F("profile_id", profileId),
		logger.F("source", source),
		logger.F("managed_args", managedArgs),
	)
}

func appendUniqueString(items []string, value string) []string {
	for _, item := range items {
		if strings.EqualFold(item, value) {
			return items
		}
	}
	return append(items, value)
}
