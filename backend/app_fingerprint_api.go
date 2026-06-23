package backend

import (
	"ant-chrome/backend/internal/fingerprint"
	cryptorand "crypto/rand"
	"encoding/json"
	"math/big"
	"strconv"
	"strings"
	"time"
)

type FingerprintGenerateRequest struct {
	CurrentArgs         []string `json:"currentArgs"`
	ProfileID           string   `json:"profileId"`
	Platform            string   `json:"platform"`
	RegionMode          string   `json:"regionMode"`
	Country             string   `json:"country"`
	Locale              string   `json:"locale"`
	Timezone            string   `json:"timezone"`
	DeviceClass         string   `json:"deviceClass"`
	ProxyID             string   `json:"proxyId"`
	ProxyConfig         string   `json:"proxyConfig"`
	PreserveUnknownArgs bool     `json:"preserveUnknownArgs"`
	RegenerateSeed      bool     `json:"regenerateSeed"`
}

type FingerprintGenerateResult struct {
	Args     []string                      `json:"args"`
	Warnings []fingerprint.ValidationIssue `json:"warnings"`
	Health   fingerprint.HealthReport      `json:"health"`
	Summary  fingerprint.Summary           `json:"summary"`
}

type FingerprintValidateRequest struct {
	Args        []string `json:"args"`
	ProxyID     string   `json:"proxyId"`
	ProxyConfig string   `json:"proxyConfig"`
}

func (a *App) GenerateFingerprintProfile(request FingerprintGenerateRequest) FingerprintGenerateResult {
	current := fingerprint.ParseArgs(request.CurrentArgs)
	regionMode := fingerprint.NormalizeRegionMode(request.RegionMode)
	proxyCtx := a.fingerprintProxyRegionContext(request.ProxyID, request.ProxyConfig)
	platform := request.Platform
	if strings.TrimSpace(platform) == "" {
		platform = current["--fingerprint-platform"]
	}
	locale := request.Locale
	if regionMode == fingerprint.RegionModeProxy && strings.TrimSpace(locale) == "" {
		locale = proxyCtx.ProxyLocale
	}
	if strings.TrimSpace(locale) == "" && regionMode == fingerprint.RegionModeManual {
		locale = current["--fingerprint-locale"]
	}
	if strings.TrimSpace(locale) == "" && regionMode == fingerprint.RegionModeManual {
		locale = current["--lang"]
	}
	timezone := request.Timezone
	if regionMode == fingerprint.RegionModeProxy && strings.TrimSpace(timezone) == "" {
		timezone = proxyCtx.ProxyTimezone
	}
	if strings.TrimSpace(timezone) == "" && regionMode == fingerprint.RegionModeManual {
		timezone = current["--fingerprint-timezone"]
	}
	if strings.TrimSpace(timezone) == "" && regionMode == fingerprint.RegionModeManual {
		timezone = current["--timezone"]
	}

	seed := int64(0)
	seedArg := ""
	if request.RegenerateSeed {
		seed = newFingerprintSeed()
		seedArg = strconv.FormatInt(seed, 10)
	} else if currentSeed := strings.TrimSpace(current["--fingerprint"]); currentSeed != "" {
		if parsed, err := strconv.ParseInt(currentSeed, 10, 64); err == nil && parsed > 0 {
			seed = parsed
			seedArg = currentSeed
		}
	}
	if seed <= 0 && strings.TrimSpace(request.ProfileID) != "" {
		seed = fingerprint.StableSeed(request.ProfileID)
		seedArg = strconv.FormatInt(seed, 10)
	}
	if seed <= 0 {
		seed = newFingerprintSeed()
		seedArg = strconv.FormatInt(seed, 10)
	}

	country := request.Country
	if regionMode == fingerprint.RegionModeProxy && strings.TrimSpace(country) == "" {
		country = proxyCtx.ProxyCountry
	}
	generated, err := fingerprint.Generate(fingerprint.LoadLibrary(), fingerprint.GenerateOptions{
		ProfileID:   request.ProfileID,
		Platform:    platform,
		RegionMode:  regionMode,
		Country:     country,
		Locale:      locale,
		Timezone:    timezone,
		DeviceClass: request.DeviceClass,
		Seed:        seed,
	})
	if err != nil {
		health := fingerprint.HealthWithContext(request.CurrentArgs, proxyCtx)
		return FingerprintGenerateResult{
			Args:     append([]string{}, request.CurrentArgs...),
			Warnings: append([]fingerprint.ValidationIssue{{Code: "generate_failed", Severity: "red", Field: "fingerprint", Message: err.Error()}}, health.Issues...),
			Health:   health,
		}
	}

	args := fingerprint.Args(generated)
	if strings.TrimSpace(proxyCtx.ProxyIP) != "" {
		args = replaceOrAppendFingerprintArg(args, "--fingerprint-webrtc-ip", proxyCtx.ProxyIP)
	}
	if seedArg != "" {
		args = append([]string{"--fingerprint=" + seedArg}, args...)
	}
	if request.PreserveUnknownArgs {
		args = append(args, unknownFingerprintArgs(request.CurrentArgs)...)
	}
	health := fingerprint.HealthWithContext(args, proxyCtx)
	return FingerprintGenerateResult{
		Args:     args,
		Warnings: health.Issues,
		Health:   health,
		Summary:  fingerprint.SummaryFor(generated),
	}
}

func (a *App) ValidateFingerprintProfile(request FingerprintValidateRequest) fingerprint.HealthReport {
	return fingerprint.HealthWithContext(request.Args, a.fingerprintProxyRegionContext(request.ProxyID, request.ProxyConfig))
}

func unknownFingerprintArgs(args []string) []string {
	known := fingerprint.ParseArgs(fingerprint.DefaultArgsForOS("windows"))
	known["--fingerprint"] = ""
	out := make([]string, 0)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		key := strings.ToLower(strings.TrimSpace(arg))
		if before, _, ok := strings.Cut(key, "="); ok {
			key = before
		}
		if _, ok := known[key]; !ok && strings.HasPrefix(key, "--") {
			if !strings.Contains(strings.TrimSpace(arg), "=") && i+1 < len(args) {
				next := strings.TrimSpace(args[i+1])
				if next != "" && !strings.HasPrefix(next, "-") {
					out = append(out, strings.TrimSpace(arg)+"="+next)
					i++
					continue
				}
			}
			out = append(out, arg)
		}
	}
	return out
}

func newFingerprintSeed() int64 {
	n, err := cryptorand.Int(cryptorand.Reader, big.NewInt(fingerprint.MaxSeed))
	if err == nil {
		return n.Int64() + 1
	}
	seed := time.Now().UnixNano() % fingerprint.MaxSeed
	if seed < 0 {
		seed = -seed
	}
	if seed == 0 {
		seed = 1
	}
	return seed
}

func (a *App) fingerprintProxyRegionContext(proxyID string, proxyConfig string) fingerprint.ValidationContext {
	proxyID = strings.TrimSpace(proxyID)
	proxyConfig = strings.TrimSpace(proxyConfig)
	if proxyID == "" && proxyConfig == "" {
		return fingerprint.ValidationContext{}
	}
	for _, item := range a.getLatestProxiesSafe() {
		if proxyID != "" && strings.EqualFold(strings.TrimSpace(item.ProxyId), proxyID) {
			return proxyRegionContextFromProxy(item)
		}
		if proxyConfig != "" && strings.EqualFold(strings.TrimSpace(item.ProxyConfig), proxyConfig) {
			return proxyRegionContextFromProxy(item)
		}
	}
	if country := countryFromProxyText(proxyID + " " + proxyConfig); country != "" {
		return fingerprint.ValidationContext{ProxyCountry: country}
	}
	return fingerprint.ValidationContext{}
}

func replaceOrAppendFingerprintArg(args []string, key string, value string) []string {
	key = strings.ToLower(strings.TrimSpace(key))
	value = strings.TrimSpace(value)
	if key == "" || value == "" {
		return args
	}
	out := append([]string{}, args...)
	for i := 0; i < len(out); i++ {
		arg := strings.TrimSpace(out[i])
		argKey := strings.ToLower(arg)
		if before, _, ok := strings.Cut(argKey, "="); ok {
			argKey = before
		}
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

func proxyRegionContextFromProxy(item BrowserProxy) fingerprint.ValidationContext {
	ctx := fingerprint.ValidationContext{
		ProxyCountry:  strings.TrimSpace(item.Country),
		ProxyLocale:   strings.TrimSpace(item.Locale),
		ProxyTimezone: strings.TrimSpace(item.Timezone),
	}
	if cached, ok := proxyRegionContextFromIPHealthJSON(item.LastIPHealthJSON); ok {
		ctx = mergeProxyRegionContext(ctx, cached)
	}
	if ctx.ProxyCountry != "" && (ctx.ProxyLocale == "" || ctx.ProxyTimezone == "") {
		locale, timezone, _ := fingerprint.RegionDefaults(ctx.ProxyCountry, ctx.ProxyLocale, ctx.ProxyTimezone)
		if ctx.ProxyLocale == "" {
			ctx.ProxyLocale = locale
		}
		if ctx.ProxyTimezone == "" {
			ctx.ProxyTimezone = timezone
		}
	}
	return ctx
}

func proxyRegionContextFromIPHealthJSON(raw string) (fingerprint.ValidationContext, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fingerprint.ValidationContext{}, false
	}
	var result ProxyIPHealthResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return fingerprint.ValidationContext{}, false
	}
	if !result.Ok {
		return fingerprint.ValidationContext{}, false
	}
	ctx := fingerprint.ValidationContext{
		ProxyCountry:  strings.TrimSpace(result.Country),
		ProxyLocale:   strings.TrimSpace(result.Locale),
		ProxyTimezone: strings.TrimSpace(result.Timezone),
		ProxyIP:       strings.TrimSpace(result.IP),
	}
	if ctx.ProxyCountry != "" {
		locale, timezone, _ := fingerprint.RegionDefaults(ctx.ProxyCountry, ctx.ProxyLocale, ctx.ProxyTimezone)
		if ctx.ProxyLocale == "" {
			ctx.ProxyLocale = locale
		}
		if ctx.ProxyTimezone == "" {
			ctx.ProxyTimezone = timezone
		}
	}
	return ctx, ctx.ProxyCountry != "" || ctx.ProxyIP != ""
}

func mergeProxyRegionContext(base fingerprint.ValidationContext, cached fingerprint.ValidationContext) fingerprint.ValidationContext {
	if strings.TrimSpace(base.ProxyCountry) == "" {
		base.ProxyCountry = cached.ProxyCountry
	}
	if strings.TrimSpace(base.ProxyLocale) == "" {
		base.ProxyLocale = cached.ProxyLocale
	}
	if strings.TrimSpace(base.ProxyTimezone) == "" {
		base.ProxyTimezone = cached.ProxyTimezone
	}
	if strings.TrimSpace(base.ProxyIP) == "" {
		base.ProxyIP = cached.ProxyIP
	}
	return base
}

func (a *App) getLatestProxiesSafe() []BrowserProxy {
	if a == nil || a.browserMgr == nil || a.config == nil {
		return nil
	}
	return a.getLatestProxies()
}

func countryFromProxyText(value string) string {
	lower := strings.ToLower(value)
	for _, token := range strings.FieldsFunc(lower, func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	}) {
		switch token {
		case "us", "usa", "america", "unitedstates", "ny", "la":
			return "US"
		case "gb", "uk", "london":
			return "GB"
		case "de", "germany", "berlin":
			return "DE"
		case "fr", "france", "paris":
			return "FR"
		case "jp", "japan", "tokyo":
			return "JP"
		case "kr", "korea", "seoul":
			return "KR"
		case "sg", "singapore":
			return "SG"
		case "br", "brazil":
			return "BR"
		case "in", "india":
			return "IN"
		case "cn", "china":
			return "CN"
		}
	}
	return ""
}
