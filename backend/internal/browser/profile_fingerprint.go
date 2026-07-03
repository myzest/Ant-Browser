package browser

import (
	"encoding/json"
	"strconv"
	"strings"

	"ant-chrome/backend/internal/fingerprint"
)

func (m *Manager) defaultFingerprintArgsForProfile(profileID string, input []string, baseArgs []string, preferredPlatform string) []string {
	return m.defaultFingerprintArgsForProfileWithProxyRegion(profileID, input, baseArgs, preferredPlatform, fingerprint.ValidationContext{})
}

func (m *Manager) defaultFingerprintArgsForProfileWithProxyRegion(profileID string, input []string, baseArgs []string, preferredPlatform string, proxyCtx fingerprint.ValidationContext) []string {
	if len(input) > 0 {
		return ensureProfileFingerprintSeedIfMissing(profileID, input)
	}

	if len(baseArgs) == 0 && m != nil && m.Config != nil {
		baseArgs = m.Config.Browser.DefaultFingerprintArgs
	}
	baseArgs = applyProxyRegionToFingerprintBaseArgs(baseArgs, proxyCtx)
	platform := strings.TrimSpace(preferredPlatform)
	if platform == "" {
		platform = fingerprintValue(baseArgs, "--fingerprint-platform")
	}
	locale := firstFingerprintValue(baseArgs, "--fingerprint-locale", "--lang")
	timezone := firstFingerprintValue(baseArgs, "--fingerprint-timezone", "--timezone")
	country := strings.TrimSpace(proxyCtx.ProxyCountry)
	profile, err := fingerprint.Generate(fingerprint.LoadLibrary(), fingerprint.GenerateOptions{
		ProfileID: profileID,
		Platform:  platform,
		Country:   country,
		Locale:    locale,
		Timezone:  timezone,
		Seed:      fingerprint.StableSeed(profileID),
	})
	if err == nil {
		return ensureProfileFingerprintSeed(profileID, mergeFingerprintArgs(fingerprint.Args(profile), baseArgs))
	}
	return ensureProfileFingerprintSeed(profileID, withoutFingerprintSeed(baseArgs))
}

func (m *Manager) DefaultFingerprintArgsForProfile(profileID string, input []string, baseArgs []string, preferredPlatform string) []string {
	return m.defaultFingerprintArgsForProfile(profileID, input, baseArgs, preferredPlatform)
}

type proxyIPHealthFingerprintCache struct {
	Ok       bool   `json:"ok"`
	Country  string `json:"country"`
	Locale   string `json:"locale"`
	Timezone string `json:"timezone"`
}

func proxyFingerprintRegionContextFromResolvedProxy(resolved resolvedProfileProxyInput) fingerprint.ValidationContext {
	if !resolved.HasSelectedProxy {
		return fingerprint.ValidationContext{}
	}
	return proxyFingerprintRegionContextFromProxy(resolved.SelectedProxy)
}

func proxyFingerprintRegionContextFromProxy(proxy Proxy) fingerprint.ValidationContext {
	ctx := fingerprint.ValidationContext{
		ProxyCountry:  strings.TrimSpace(proxy.Country),
		ProxyLocale:   strings.TrimSpace(proxy.Locale),
		ProxyTimezone: strings.TrimSpace(proxy.Timezone),
	}
	if cached, ok := proxyFingerprintRegionContextFromIPHealthJSON(proxy.LastIPHealthJSON); ok {
		ctx = mergeProxyFingerprintRegionContext(ctx, cached)
	}
	if ctx.ProxyCountry != "" || ctx.ProxyLocale != "" || ctx.ProxyTimezone != "" {
		locale, timezone, country := fingerprint.ProxyRegionDefaults(ctx.ProxyCountry, ctx.ProxyLocale, ctx.ProxyTimezone)
		ctx.ProxyCountry = country
		ctx.ProxyLocale = locale
		ctx.ProxyTimezone = timezone
	}
	return ctx
}

func proxyFingerprintRegionContextFromIPHealthJSON(raw string) (fingerprint.ValidationContext, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fingerprint.ValidationContext{}, false
	}
	var cached proxyIPHealthFingerprintCache
	if err := json.Unmarshal([]byte(raw), &cached); err != nil || !cached.Ok {
		return fingerprint.ValidationContext{}, false
	}
	ctx := fingerprint.ValidationContext{
		ProxyCountry:  strings.TrimSpace(cached.Country),
		ProxyLocale:   strings.TrimSpace(cached.Locale),
		ProxyTimezone: strings.TrimSpace(cached.Timezone),
	}
	if ctx.ProxyCountry == "" && ctx.ProxyLocale == "" && ctx.ProxyTimezone == "" {
		return fingerprint.ValidationContext{}, false
	}
	if ctx.ProxyCountry != "" || ctx.ProxyLocale != "" || ctx.ProxyTimezone != "" {
		locale, timezone, country := fingerprint.ProxyRegionDefaults(ctx.ProxyCountry, ctx.ProxyLocale, ctx.ProxyTimezone)
		ctx.ProxyCountry = country
		ctx.ProxyLocale = locale
		ctx.ProxyTimezone = timezone
	}
	return ctx, true
}

func mergeProxyFingerprintRegionContext(base fingerprint.ValidationContext, cached fingerprint.ValidationContext) fingerprint.ValidationContext {
	if strings.TrimSpace(base.ProxyCountry) == "" {
		base.ProxyCountry = cached.ProxyCountry
	}
	if strings.TrimSpace(base.ProxyLocale) == "" {
		base.ProxyLocale = cached.ProxyLocale
	}
	if strings.TrimSpace(base.ProxyTimezone) == "" {
		base.ProxyTimezone = cached.ProxyTimezone
	}
	return base
}

func applyProxyRegionToFingerprintBaseArgs(baseArgs []string, proxyCtx fingerprint.ValidationContext) []string {
	if proxyCtx.ProxyCountry == "" && proxyCtx.ProxyLocale == "" && proxyCtx.ProxyTimezone == "" {
		return append([]string{}, baseArgs...)
	}
	locale, timezone, _ := fingerprint.ProxyRegionDefaults(proxyCtx.ProxyCountry, proxyCtx.ProxyLocale, proxyCtx.ProxyTimezone)
	out := append([]string{}, baseArgs...)
	if locale != "" {
		out = replaceOrAppendFingerprintBaseArg(out, "--lang", locale)
		out = replaceOrAppendFingerprintBaseArg(out, "--fingerprint-locale", locale)
		if acceptLanguage := fingerprint.AcceptLanguageDefaults(proxyCtx.ProxyCountry, locale); acceptLanguage != "" {
			out = replaceOrAppendFingerprintBaseArg(out, "--fingerprint-accept-language", acceptLanguage)
		}
	}
	if timezone != "" {
		out = replaceOrAppendFingerprintBaseArg(out, "--timezone", timezone)
		out = replaceOrAppendFingerprintBaseArg(out, "--fingerprint-timezone", timezone)
	}
	return out
}

func replaceOrAppendFingerprintBaseArg(args []string, key string, value string) []string {
	key = strings.ToLower(strings.TrimSpace(key))
	value = strings.TrimSpace(value)
	if key == "" || value == "" {
		return append([]string{}, args...)
	}
	out := append([]string{}, args...)
	for i := 0; i < len(out); i++ {
		arg := strings.TrimSpace(out[i])
		argKey := fingerprintArgKey(arg)
		if argKey != key {
			continue
		}
		out[i] = key + "=" + value
		if !strings.Contains(arg, "=") && i+1 < len(out) {
			next := strings.TrimSpace(out[i+1])
			if next != "" && !strings.HasPrefix(next, "-") {
				out = append(out[:i+1], out[i+2:]...)
			}
		}
		return out
	}
	return append(out, key+"="+value)
}

func platformFromFingerprintArgs(args []string) string {
	return fingerprintValue(args, "--fingerprint-platform")
}

func mergeFingerprintArgs(generated []string, base []string) []string {
	return mergeKnownFingerprintArgs(withoutFingerprintSeed(generated), withoutFingerprintSeed(base))
}

func ensureProfileFingerprintSeed(profileID string, args []string) []string {
	out := withoutFingerprintSeed(args)
	seed := fingerprint.SeedArg(profileID)
	return append([]string{seed}, out...)
}

func ensureProfileFingerprintSeedIfMissing(profileID string, args []string) []string {
	for _, arg := range args {
		if fingerprintArgKey(arg) == "--fingerprint" {
			values := fingerprint.ParseArgs(args)
			seed := strings.TrimSpace(values["--fingerprint"])
			if parsed, err := strconv.ParseInt(seed, 10, 64); err == nil && parsed > 0 {
				return append([]string{}, args...)
			}
			return ensureProfileFingerprintSeed(profileID, args)
		}
	}
	return append([]string{fingerprint.SeedArg(profileID)}, args...)
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
	key := strings.ToLower(strings.TrimSpace(arg))
	if key == "--accept-language" {
		return "--fingerprint-accept-language"
	}
	return key
}
