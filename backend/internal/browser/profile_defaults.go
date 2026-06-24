package browser

import (
	"ant-chrome/backend/internal/fingerprint"
	"ant-chrome/backend/internal/logger"
	"slices"
	"strings"
)

const directProxyID = "__direct__"

var profileFingerprintDefaultBackfillKeys = []string{
	"--fingerprint-accept-language",
	"--fingerprint-do-not-track",
}

var profileFingerprintCompletionKeys = []string{
	"--fingerprint-brand",
	"--fingerprint-platform",
	"--lang",
	"--fingerprint-locale",
	"--timezone",
	"--fingerprint-timezone",
	"--window-size",
	"--fingerprint-screen-avail",
	"--fingerprint-device-pixel-ratio",
	"--fingerprint-color-depth",
	"--fingerprint-hardware-concurrency",
	"--fingerprint-device-memory",
	"--fingerprint-canvas-noise",
	"--fingerprint-audio-noise",
	"--fingerprint-webgl-vendor",
	"--fingerprint-webgl-renderer",
	"--fingerprint-fonts",
	"--webrtc-ip-handling-policy",
	"--fingerprint-webrtc-ip",
	"--fingerprint-touch-points",
	"--fingerprint-media-devices",
}

// ApplyDefaults 应用默认配置
func (m *Manager) ApplyDefaults(profile *Profile) bool {
	log := logger.New("Browser")
	fingerprintChanged := false
	shouldGenerateFingerprintDefaults := profile.FingerprintArgs == nil || len(profile.FingerprintArgs) == 0
	if profile.LaunchArgs == nil || len(profile.LaunchArgs) == 0 {
		profile.LaunchArgs = append([]string{}, m.Config.Browser.DefaultLaunchArgs...)
	}
	if strings.TrimSpace(profile.UserDataDir) == "" {
		profile.UserDataDir = profile.ProfileId
	}
	profile.CoreId = normalizeProfileCoreID(profile.CoreId)
	if profile.CoreId == "" {
		if defaultCore, ok := m.GetDefaultCore(); ok {
			profile.CoreId = defaultCore.CoreId
		}
	}

	proxyChanged := false
	bindChanged, boundInPool, bindMode := m.ResolveProfileProxyBinding(profile)
	if bindChanged {
		proxyChanged = true
	}
	if bindMode != "" && bindMode != "proxy_id" {
		log.Info("实例代理自动重关联",
			logger.F("profile_id", profile.ProfileId),
			logger.F("proxy_id", profile.ProxyId),
			logger.F("mode", bindMode),
		)
	}

	if strings.TrimSpace(profile.ProxyId) == "" {
		if proxy, ok := m.resolvePoolProxyByConfig(profile.ProxyConfig); ok {
			if BindProfileToProxy(profile, proxy, true) {
				proxyChanged = true
			}
			boundInPool = true
		} else if strings.TrimSpace(profile.ProxyConfig) == "" && m.bindProfileToDirectProxy(profile) {
			proxyChanged = true
			boundInPool = true
		}
	}

	if profile.ProxyId != "" && !boundInPool {
		missingProxyID := profile.ProxyId
		if strings.TrimSpace(profile.ProxyConfig) != "" {
			profile.ProxyId = ""
			proxyChanged = true
			if ClearProfileProxyBinding(profile) {
				proxyChanged = true
			}
			log.Warn("实例代理ID未找到，已改为使用实例代理配置",
				logger.F("profile_id", profile.ProfileId),
				logger.F("missing_proxy_id", missingProxyID),
			)
		} else if m.bindProfileToDirectProxy(profile) {
			proxyChanged = true
			log.Warn("实例代理未找到，已回退到直连",
				logger.F("profile_id", profile.ProfileId),
				logger.F("missing_proxy_id", missingProxyID),
			)
		}
	}

	if shouldGenerateFingerprintDefaults {
		profile.FingerprintArgs = m.defaultFingerprintArgsForProfileWithProxyRegion(profile.ProfileId, nil, nil, "", m.profileProxyFingerprintRegionContext(profile))
		fingerprintChanged = true
	} else if m.backfillProfileFingerprintDefaults(profile, m.profileProxyFingerprintRegionContext(profile)) {
		fingerprintChanged = true
	}

	return proxyChanged || fingerprintChanged
}

func (m *Manager) backfillProfileFingerprintDefaults(profile *Profile, proxyCtx fingerprint.ValidationContext) bool {
	if profile == nil || len(profile.FingerprintArgs) == 0 {
		return false
	}
	before := append([]string{}, profile.FingerprintArgs...)
	args := append([]string{}, profile.FingerprintArgs...)
	if shouldCompleteProfileFingerprintDefaults(args) {
		args = m.completeProfileFingerprintDefaults(profile.ProfileId, args, proxyCtx)
	}
	profile.FingerprintArgs = backfillMissingProfileFingerprintDefaults(args, m.profileFingerprintDefaultBackfillValues(args))
	return !slices.Equal(before, profile.FingerprintArgs)
}

func shouldCompleteProfileFingerprintDefaults(args []string) bool {
	for _, key := range profileFingerprintCompletionKeys {
		if strings.TrimSpace(fingerprintValue(args, key)) == "" {
			return true
		}
	}
	return false
}

func (m *Manager) completeProfileFingerprintDefaults(profileID string, args []string, proxyCtx fingerprint.ValidationContext) []string {
	baseArgs := append([]string{}, args...)
	defaultArgs := []string{}
	if m != nil && m.Config != nil {
		defaultArgs = m.Config.Browser.DefaultFingerprintArgs
	}
	defaultArgs = applyProxyRegionToFingerprintBaseArgs(defaultArgs, proxyCtx)
	if firstFingerprintValue(baseArgs, "--fingerprint-locale", "--lang") != "" &&
		profileFingerprintArgValue(baseArgs, "--fingerprint-accept-language") == "" {
		defaultArgs = withoutProfileFingerprintArgs(defaultArgs, "--fingerprint-accept-language")
	}

	platform := platformFromFingerprintArgs(baseArgs)
	if platform == "" {
		platform = platformFromFingerprintArgs(defaultArgs)
	}
	locale := firstFingerprintValue(baseArgs, "--fingerprint-locale", "--lang")
	timezone := firstFingerprintValue(baseArgs, "--fingerprint-timezone", "--timezone")
	country := strings.TrimSpace(proxyCtx.ProxyCountry)
	if locale == "" {
		locale = strings.TrimSpace(proxyCtx.ProxyLocale)
	}
	if timezone == "" {
		timezone = strings.TrimSpace(proxyCtx.ProxyTimezone)
	}
	if locale == "" {
		locale = firstFingerprintValue(defaultArgs, "--fingerprint-locale", "--lang")
	}
	if timezone == "" {
		timezone = firstFingerprintValue(defaultArgs, "--fingerprint-timezone", "--timezone")
	}

	generated, err := fingerprint.Generate(fingerprint.LoadLibrary(), fingerprint.GenerateOptions{
		ProfileID: profileID,
		Platform:  platform,
		Country:   country,
		Locale:    locale,
		Timezone:  timezone,
		Seed:      fingerprint.StableSeed(profileID),
	})
	if err != nil {
		return ensureProfileFingerprintSeedIfMissing(profileID, baseArgs)
	}
	return ensureProfileFingerprintSeedIfMissing(profileID, mergeKnownFingerprintArgs(fingerprint.Args(generated), defaultArgs, baseArgs))
}

func withoutProfileFingerprintArgs(args []string, keys ...string) []string {
	if len(args) == 0 || len(keys) == 0 {
		return append([]string{}, args...)
	}
	remove := map[string]struct{}{}
	for _, key := range keys {
		if canonical := fingerprintArgKey(key); canonical != "" {
			remove[canonical] = struct{}{}
		}
	}
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if _, ok := remove[fingerprintArgKey(arg)]; ok {
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

func (m *Manager) profileFingerprintDefaultBackfillValues(args []string) map[string]string {
	defaultArgs := []string{}
	if m != nil && m.Config != nil {
		defaultArgs = m.Config.Browser.DefaultFingerprintArgs
	}

	values := map[string]string{}
	if locale := firstFingerprintValue(args, "--fingerprint-locale", "--lang"); locale != "" {
		values["--fingerprint-accept-language"] = fingerprint.AcceptLanguageDefaults("", locale)
	}
	if values["--fingerprint-accept-language"] == "" {
		values["--fingerprint-accept-language"] = profileFingerprintArgValue(defaultArgs, "--fingerprint-accept-language")
	}
	if values["--fingerprint-accept-language"] == "" {
		values["--fingerprint-accept-language"] = fingerprint.AcceptLanguageDefaults("", "")
	}

	values["--fingerprint-do-not-track"] = profileFingerprintArgValue(defaultArgs, "--fingerprint-do-not-track")
	if values["--fingerprint-do-not-track"] == "" {
		values["--fingerprint-do-not-track"] = "false"
	}
	return values
}

func backfillMissingProfileFingerprintDefaults(args []string, defaultValues map[string]string) []string {
	out := make([]string, 0, len(args)+len(profileFingerprintDefaultBackfillKeys))
	indexByKey := map[string]int{}
	hasValueByKey := map[string]bool{}

	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == "" {
			continue
		}

		rawKey, value, ok, consumedNext := profileFingerprintArg(args, i)
		if !ok {
			out = append(out, arg)
			continue
		}
		key := canonicalProfileFingerprintBackfillKey(rawKey)
		if key == "" {
			out = append(out, arg)
			continue
		}
		if consumedNext {
			i++
		}
		isCanonical := rawKey == key
		if !isCanonical && hasValueByKey[key] {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		canonicalArg := key + "=" + value
		if index, exists := indexByKey[key]; exists {
			out[index] = canonicalArg
		} else {
			indexByKey[key] = len(out)
			out = append(out, canonicalArg)
		}
		hasValueByKey[key] = true
	}

	for _, key := range profileFingerprintDefaultBackfillKeys {
		if hasValueByKey[key] {
			continue
		}
		value := strings.TrimSpace(defaultValues[key])
		if value == "" {
			continue
		}
		out = append(out, key+"="+value)
	}
	return out
}

func profileFingerprintArgValue(args []string, want string) string {
	want = canonicalProfileFingerprintBackfillKey(want)
	if want == "" {
		return ""
	}
	value := ""
	hasCanonicalValue := false
	for i := 0; i < len(args); i++ {
		rawKey, candidate, ok, consumedNext := profileFingerprintArg(args, i)
		if consumedNext {
			i++
		}
		key := canonicalProfileFingerprintBackfillKey(rawKey)
		candidate = strings.TrimSpace(candidate)
		if !ok || key != want || candidate == "" {
			continue
		}
		if rawKey == key {
			value = candidate
			hasCanonicalValue = true
		} else if !hasCanonicalValue {
			value = candidate
		}
	}
	return value
}

func profileFingerprintArg(args []string, index int) (string, string, bool, bool) {
	if index < 0 || index >= len(args) {
		return "", "", false, false
	}
	arg := strings.TrimSpace(args[index])
	if arg == "" || !strings.HasPrefix(arg, "--") {
		return "", "", false, false
	}
	if key, value, ok := strings.Cut(arg, "="); ok {
		return strings.ToLower(strings.TrimSpace(key)), strings.TrimSpace(value), true, false
	}
	if index+1 < len(args) {
		next := strings.TrimSpace(args[index+1])
		if next != "" && !strings.HasPrefix(next, "-") {
			return strings.ToLower(arg), next, true, true
		}
	}
	return strings.ToLower(arg), "", true, false
}

func canonicalProfileFingerprintBackfillKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	switch key {
	case "--accept-language":
		return "--fingerprint-accept-language"
	case "--fingerprint-accept-language", "--fingerprint-do-not-track":
		return key
	default:
		return ""
	}
}

func (m *Manager) profileProxyFingerprintRegionContext(profile *Profile) fingerprint.ValidationContext {
	if m == nil || profile == nil {
		return fingerprint.ValidationContext{}
	}
	if proxy, ok := m.GetProxyByID(profile.ProxyId); ok {
		return proxyFingerprintRegionContextFromProxy(proxy)
	}
	if proxy, ok := m.resolvePoolProxyByConfig(profile.ProxyConfig); ok {
		return proxyFingerprintRegionContextFromProxy(proxy)
	}
	return fingerprint.ValidationContext{}
}

func (m *Manager) bindProfileToDirectProxy(profile *Profile) bool {
	if profile == nil {
		return false
	}
	if proxy, ok := m.GetProxyByID(directProxyID); ok {
		return BindProfileToProxy(profile, proxy, true)
	}

	changed := false
	if strings.TrimSpace(profile.ProxyId) != "" {
		profile.ProxyId = ""
		changed = true
	}
	if strings.TrimSpace(profile.ProxyConfig) != "" {
		profile.ProxyConfig = ""
		changed = true
	}
	if ClearProfileProxyBinding(profile) {
		changed = true
	}
	return changed
}

func (m *Manager) resolvePoolProxyByConfig(proxyConfig string) (Proxy, bool) {
	target := normalizeProxyBindValue(proxyConfig)
	if target == "" {
		return Proxy{}, false
	}
	proxies := m.listProxyCatalog()
	return uniqueProxyMatch(proxies, func(item Proxy) bool {
		return normalizeProxyBindValue(item.ProxyConfig) == target
	})
}

// copyKeywords 深拷贝 keywords map
func copyKeywords(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
