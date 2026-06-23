package browser

import (
	"ant-chrome/backend/internal/fingerprint"
	"strings"
)

func (m *Manager) defaultFingerprintArgsForProfile(profileID string, input []string, baseArgs []string, preferredPlatform string) []string {
	if len(input) > 0 {
		return append([]string{}, input...)
	}

	if len(baseArgs) == 0 && m != nil && m.Config != nil {
		baseArgs = m.Config.Browser.DefaultFingerprintArgs
	}
	platform := strings.TrimSpace(preferredPlatform)
	if platform == "" {
		platform = fingerprintValue(baseArgs, "--fingerprint-platform")
	}
	locale := firstFingerprintValue(baseArgs, "--fingerprint-locale", "--lang")
	timezone := firstFingerprintValue(baseArgs, "--fingerprint-timezone", "--timezone")
	profile, err := fingerprint.Generate(fingerprint.LoadLibrary(), fingerprint.GenerateOptions{
		ProfileID: profileID,
		Platform:  platform,
		Locale:    locale,
		Timezone:  timezone,
	})
	if err == nil {
		return mergeFingerprintArgs(fingerprint.Args(profile), baseArgs)
	}
	return withoutFingerprintSeed(baseArgs)
}

func platformFromFingerprintArgs(args []string) string {
	return fingerprintValue(args, "--fingerprint-platform")
}

func mergeFingerprintArgs(generated []string, base []string) []string {
	return mergeKnownFingerprintArgs(withoutFingerprintSeed(generated), withoutFingerprintSeed(base))
}

func mergeKnownFingerprintArgs(groups ...[]string) []string {
	out := make([]string, 0)
	indexByKey := map[string]int{}
	for _, group := range groups {
		for i := 0; i < len(group); i++ {
			arg := strings.TrimSpace(group[i])
			if arg == "" {
				continue
			}
			key := fingerprintArgKey(arg)
			if key != "" {
				if !strings.Contains(arg, "=") && i+1 < len(group) {
					next := strings.TrimSpace(group[i+1])
					if next != "" && !strings.HasPrefix(next, "-") {
						arg = key + "=" + next
						i++
					}
				}
				if index, ok := indexByKey[key]; ok {
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

func withoutFingerprintSeed(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		key := fingerprintArgKey(arg)
		if key == "--fingerprint" {
			if !strings.Contains(arg, "=") && i+1 < len(args) {
				next := strings.TrimSpace(args[i+1])
				if next != "" && !strings.HasPrefix(next, "-") {
					i++
				}
			}
			continue
		}
		if arg != "" {
			out = append(out, arg)
		}
	}
	return out
}

func firstFingerprintValue(args []string, keys ...string) string {
	for _, key := range keys {
		if value := fingerprintValue(args, key); value != "" {
			return value
		}
	}
	return ""
}

func fingerprintValue(args []string, key string) string {
	values := fingerprint.ParseArgs(args)
	return strings.TrimSpace(values[strings.ToLower(strings.TrimSpace(key))])
}

func fingerprintArgKey(arg string) string {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return ""
	}
	if before, _, ok := strings.Cut(arg, "="); ok {
		arg = before
	}
	if !strings.HasPrefix(arg, "--") {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(arg))
}
