package backend

import (
	"fmt"
	"strings"

	"ant-chrome/backend/internal/browser"
	"ant-chrome/backend/internal/logger"
)

const (
	browserStartPreflightRiskLow    = "low"
	browserStartPreflightRiskMedium = "medium"
	browserStartPreflightRiskHigh   = "high"
)

type browserStartPreflightIssue struct {
	Kind    string   `json:"kind"`
	Source  string   `json:"source"`
	Args    []string `json:"args,omitempty"`
	Message string   `json:"message"`
}

type browserStartPreflightResult struct {
	FormatErrors        []browserStartPreflightIssue `json:"formatErrors,omitempty"`
	ManagedArgWarnings  []browserStartPreflightIssue `json:"managedArgWarnings,omitempty"`
	FingerprintWarnings []browserStartPreflightIssue `json:"fingerprintWarnings,omitempty"`
	RiskLevel           string                       `json:"riskLevel"`
}

type browserInstancePreflightResponse struct {
	ProfileID           string                       `json:"profileId"`
	ProfileName         string                       `json:"profileName"`
	Mode                string                       `json:"mode"`
	RiskLevel           string                       `json:"riskLevel"`
	Summary             string                       `json:"summary"`
	Warnings            []browserStartPreflightIssue `json:"warnings,omitempty"`
	ExplicitAlerts      []browserStartPreflightIssue `json:"explicitAlerts,omitempty"`
	RequireConfirm      bool                         `json:"requireConfirm"`
	FormatErrors        []browserStartPreflightIssue `json:"formatErrors,omitempty"`
	ManagedArgWarnings  []browserStartPreflightIssue `json:"managedArgWarnings,omitempty"`
	FingerprintWarnings []browserStartPreflightIssue `json:"fingerprintWarnings,omitempty"`
}

func (a *App) BrowserInstancePreflight(profileId string, mode string) browserInstancePreflightResponse {
	response := browserInstancePreflightResponse{
		ProfileID: strings.TrimSpace(profileId),
		Mode:      normalizeBrowserStartPreflightMode(mode),
		RiskLevel: browserStartPreflightRiskLow,
		Summary:   "未发现明显风险，可以继续启动。",
		Warnings:  []browserStartPreflightIssue{},
	}
	if a == nil || a.browserMgr == nil {
		response.RiskLevel = browserStartPreflightRiskHigh
		response.Summary = "浏览器管理器尚未初始化，无法完成启动前预检。"
		response.RequireConfirm = true
		response.ExplicitAlerts = []browserStartPreflightIssue{{
			Kind:    "preflight_error",
			Source:  "backend",
			Message: response.Summary,
		}}
		return response
	}

	a.browserMgr.Mutex.Lock()
	profile, exists := a.browserMgr.Profiles[response.ProfileID]
	if !exists || profile == nil {
		a.browserMgr.Mutex.Unlock()
		response.RiskLevel = browserStartPreflightRiskHigh
		response.Summary = fmt.Sprintf("未找到实例配置（ID=%s），请刷新列表后重试。", response.ProfileID)
		response.RequireConfirm = true
		response.ExplicitAlerts = []browserStartPreflightIssue{{
			Kind:    "preflight_error",
			Source:  "profile",
			Message: response.Summary,
		}}
		return response
	}
	profileSnapshot := *profile
	if profile.FingerprintArgs != nil {
		profileSnapshot.FingerprintArgs = append([]string{}, profile.FingerprintArgs...)
	}
	if profile.LaunchArgs != nil {
		profileSnapshot.LaunchArgs = append([]string{}, profile.LaunchArgs...)
	}
	response.ProfileName = profile.ProfileName
	a.browserMgr.Mutex.Unlock()

	input := newBrowserStartInput(response.ProfileID, nil, nil, false, false, response.Mode == "direct", "", "")
	preflight := buildBrowserStartPreflightResult(input, &profileSnapshot)
	return buildBrowserInstancePreflightResponse(response.ProfileID, response.ProfileName, response.Mode, preflight)
}

func normalizeBrowserStartPreflightMode(mode string) string {
	if strings.EqualFold(strings.TrimSpace(mode), "direct") {
		return "direct"
	}
	return "normal"
}

func buildBrowserInstancePreflightResponse(profileID string, profileName string, mode string, preflight *browserStartPreflightResult) browserInstancePreflightResponse {
	if preflight == nil {
		preflight = &browserStartPreflightResult{RiskLevel: browserStartPreflightRiskLow}
	}
	preflight.finalize()

	warnings := make([]browserStartPreflightIssue, 0, len(preflight.ManagedArgWarnings)+len(preflight.FingerprintWarnings))
	warnings = append(warnings, preflight.ManagedArgWarnings...)
	warnings = append(warnings, preflight.FingerprintWarnings...)

	explicitAlerts := append([]browserStartPreflightIssue{}, preflight.FormatErrors...)
	requireConfirm := preflight.RiskLevel == browserStartPreflightRiskHigh || len(explicitAlerts) > 0

	return browserInstancePreflightResponse{
		ProfileID:           strings.TrimSpace(profileID),
		ProfileName:         strings.TrimSpace(profileName),
		Mode:                normalizeBrowserStartPreflightMode(mode),
		RiskLevel:           preflight.RiskLevel,
		Summary:             browserStartPreflightSummary(preflight),
		Warnings:            warnings,
		ExplicitAlerts:      explicitAlerts,
		RequireConfirm:      requireConfirm,
		FormatErrors:        preflight.FormatErrors,
		ManagedArgWarnings:  preflight.ManagedArgWarnings,
		FingerprintWarnings: preflight.FingerprintWarnings,
	}
}

func browserStartPreflightSummary(preflight *browserStartPreflightResult) string {
	if preflight == nil {
		return "未发现明显风险，可以继续启动。"
	}
	switch preflight.RiskLevel {
	case browserStartPreflightRiskHigh:
		return "启动前预检发现格式错误，继续启动前需要处理或确认。"
	case browserStartPreflightRiskMedium:
		return "启动前预检发现需要关注的风险项，请确认后继续。"
	default:
		return "未发现明显风险，可以继续启动。"
	}
}

func buildBrowserStartPreflightResult(input browserStartInput, profile *BrowserProfile) *browserStartPreflightResult {
	result := &browserStartPreflightResult{RiskLevel: browserStartPreflightRiskLow}
	if profile == nil {
		return result
	}

	result.ManagedArgWarnings = append(result.ManagedArgWarnings,
		collectManagedLaunchArgWarnings("profile.launchArgs", profile.LaunchArgs)...,
	)
	result.ManagedArgWarnings = append(result.ManagedArgWarnings,
		collectManagedLaunchArgWarnings("start.extraLaunchArgs", input.ExtraLaunchArgs)...,
	)

	sanitizedProfileLaunchArgs, _ := sanitizeManagedLaunchArgs(profile.LaunchArgs)
	sanitizedExtraLaunchArgs, _ := sanitizeManagedLaunchArgs(input.ExtraLaunchArgs)

	result.FormatErrors = append(result.FormatErrors,
		collectLaunchArgFormatIssues("profile.fingerprintArgs", profile.FingerprintArgs)...,
	)
	result.FormatErrors = append(result.FormatErrors,
		collectLaunchArgFormatIssues("profile.launchArgs", sanitizedProfileLaunchArgs)...,
	)
	result.FormatErrors = append(result.FormatErrors,
		collectLaunchArgFormatIssues("start.extraLaunchArgs", sanitizedExtraLaunchArgs)...,
	)

	combinedArgs := make([]string, 0, len(profile.FingerprintArgs)+len(sanitizedProfileLaunchArgs)+len(sanitizedExtraLaunchArgs))
	combinedArgs = append(combinedArgs, profile.FingerprintArgs...)
	combinedArgs = append(combinedArgs, sanitizedProfileLaunchArgs...)
	combinedArgs = append(combinedArgs, sanitizedExtraLaunchArgs...)
	result.FingerprintWarnings = append(result.FingerprintWarnings,
		collectFingerprintConsistencyWarnings(combinedArgs)...,
	)

	result.finalize()
	return result
}

// buildBrowserStartPreflight 兼容旧调用点，复用当前预检构建逻辑。
func buildBrowserStartPreflight(input browserStartInput, profile *BrowserProfile) *browserStartPreflightResult {
	return buildBrowserStartPreflightResult(input, profile)
}

func collectManagedLaunchArgWarnings(source string, args []string) []browserStartPreflightIssue {
	if len(args) == 0 {
		return nil
	}

	warnings := make([]browserStartPreflightIssue, 0, 2)
	seen := map[string]struct{}{}

	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == "" {
			continue
		}

		spec, matched := matchManagedLaunchArg(arg)
		if !matched {
			continue
		}

		display := arg
		if spec.takesValue && !strings.Contains(arg, "=") && i+1 < len(args) {
			next := strings.TrimSpace(args[i+1])
			if next != "" && !strings.HasPrefix(next, "-") {
				display = display + " " + next
				i++
			}
		}

		key := strings.ToLower(source + "|" + display)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		warnings = append(warnings, browserStartPreflightIssue{
			Kind:    "managed_launch_arg_warning",
			Source:  source,
			Args:    []string{display},
			Message: fmt.Sprintf("系统将接管并忽略启动参数：%s", display),
		})
	}

	return warnings
}

func collectLaunchArgFormatIssues(source string, args []string) []browserStartPreflightIssue {
	if len(args) == 0 {
		return nil
	}

	issues := make([]browserStartPreflightIssue, 0, 2)
	seen := map[string]struct{}{}

	addIssue := func(kind string, arg string, message string) {
		key := strings.ToLower(source + "|" + kind + "|" + arg)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		issues = append(issues, browserStartPreflightIssue{
			Kind:    kind,
			Source:  source,
			Args:    []string{arg},
			Message: message,
		})
	}

	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == "" || !strings.HasPrefix(arg, "--") {
			continue
		}

		switch {
		case hasLaunchArgPrefix(arg, "--window-size"):
			value, ok := launchArgInlineOrSplitValue(args, i, "--window-size")
			if !ok || strings.TrimSpace(value) == "" {
				addIssue("format_error", arg, "window-size 参数格式错误，应为 --window-size=宽,高")
				continue
			}
			if _, _, ok := parseWindowWH(value); !ok {
				addIssue("format_error", arg, fmt.Sprintf("window-size 参数格式错误：%s", arg))
			}
		case hasLaunchArgPrefix(arg, "--lang"), hasLaunchArgPrefix(arg, "--accept-language"):
			flag := "--lang"
			if hasLaunchArgPrefix(arg, "--accept-language") {
				flag = "--accept-language"
			}
			value, ok := launchArgInlineOrSplitValue(args, i, flag)
			if !ok || len(normalizeAcceptLanguage(value)) == 0 {
				addIssue("format_error", arg, fmt.Sprintf("%s 参数格式错误，应为逗号分隔的语言列表", flag))
			}
		case hasLaunchArgPrefix(arg, "--fingerprint"):
			value, ok := launchArgInlineOrSplitValue(args, i, "--fingerprint")
			if !ok || strings.TrimSpace(value) == "" {
				addIssue("format_error", arg, "fingerprint 参数格式错误，应为非空整数")
				continue
			}
			if !isIntegerValue(value) {
				addIssue("format_error", arg, fmt.Sprintf("fingerprint 参数格式错误：%s", arg))
			}
		case hasLaunchArgPrefix(arg, "--user-agent"):
			value, ok := launchArgInlineOrSplitValue(args, i, "--user-agent")
			if !ok || strings.TrimSpace(value) == "" {
				addIssue("format_error", arg, "user-agent 参数格式错误，应提供非空字符串")
			}
		case hasLaunchArgPrefix(arg, "--fingerprint-platform"):
			value, ok := launchArgInlineOrSplitValue(args, i, "--fingerprint-platform")
			if !ok || strings.TrimSpace(value) == "" {
				addIssue("format_error", arg, "fingerprint-platform 参数格式错误，应提供非空平台值")
			}
		}
	}

	return issues
}

func collectFingerprintConsistencyWarnings(args []string) []browserStartPreflightIssue {
	if len(args) == 0 {
		return nil
	}

	uaHint := ""
	uaRaw := ""
	platformHint := ""
	platformRaw := ""

	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == "" || !strings.HasPrefix(arg, "--") {
			continue
		}

		switch {
		case hasLaunchArgPrefix(arg, "--user-agent"):
			value, ok := launchArgInlineOrSplitValue(args, i, "--user-agent")
			if !ok || strings.TrimSpace(value) == "" {
				continue
			}
			hint := camoufoxOSFromUserAgent(value)
			if hint == "" {
				continue
			}
			if uaHint == "" {
				uaHint = hint
				uaRaw = value
			} else if uaHint != hint {
				uaHint = "conflict"
			}
		case hasLaunchArgPrefix(arg, "--fingerprint-platform"):
			value, ok := launchArgInlineOrSplitValue(args, i, "--fingerprint-platform")
			if !ok || strings.TrimSpace(value) == "" {
				continue
			}
			hint := browser.AntCamoufoxOS(value)
			if platformHint == "" {
				platformHint = hint
				platformRaw = value
			} else if platformHint != hint {
				platformHint = "conflict"
			}
		}
	}

	if uaHint == "" || platformHint == "" || uaHint == platformHint {
		return nil
	}

	return []browserStartPreflightIssue{{
		Kind:    "fingerprint_consistency_warning",
		Source:  "profile.fingerprintArgs",
		Args:    []string{uaRaw, platformRaw},
		Message: fmt.Sprintf("user-agent 与 fingerprint-platform 指向不同平台：UA=%s, fingerprint-platform=%s；Camoufox 会按显式平台选池并按 UA 覆盖平台信息，建议统一。", uaRaw, platformRaw),
	}}
}

func parseWindowWH(value string) (int, int, bool) {
	parts := strings.Split(value, ",")
	if len(parts) != 2 {
		return 0, 0, false
	}
	w, ok := parsePositiveInt(parts[0])
	if !ok {
		return 0, 0, false
	}
	h, ok := parsePositiveInt(parts[1])
	if !ok {
		return 0, 0, false
	}
	return w, h, true
}

func normalizeAcceptLanguage(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		lang := p
		if idx := strings.IndexByte(lang, ';'); idx >= 0 {
			lang = lang[:idx]
		}
		lang = strings.TrimSpace(lang)
		if lang != "" {
			out = append(out, lang)
		}
	}
	return out
}

func parsePositiveInt(value string) (int, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	n := 0
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return 0, false
		}
		n = n*10 + int(ch-'0')
	}
	if n <= 0 {
		return 0, false
	}
	return n, true
}

func hasLaunchArgPrefix(arg string, prefix string) bool {
	argLower := strings.ToLower(strings.TrimSpace(arg))
	prefixLower := strings.ToLower(strings.TrimSpace(prefix))
	return argLower == prefixLower || strings.HasPrefix(argLower, prefixLower+"=")
}

func launchArgInlineOrSplitValue(args []string, idx int, flag string) (string, bool) {
	arg := strings.TrimSpace(args[idx])
	if arg == "" {
		return "", false
	}

	argLower := strings.ToLower(arg)
	flagLower := strings.ToLower(flag)

	if argLower == flagLower {
		if idx+1 >= len(args) {
			return "", false
		}
		next := strings.TrimSpace(args[idx+1])
		if next == "" || strings.HasPrefix(next, "-") {
			return "", false
		}
		return next, true
	}

	prefix := flagLower + "="
	if strings.HasPrefix(argLower, prefix) {
		return strings.TrimSpace(arg[len(flag)+1:]), true
	}

	return "", false
}

func isIntegerValue(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for i, ch := range value {
		if i == 0 && (ch == '+' || ch == '-') {
			continue
		}
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func (r *browserStartPreflightResult) finalize() {
	if r == nil {
		return
	}
	switch {
	case len(r.FormatErrors) > 0:
		r.RiskLevel = browserStartPreflightRiskHigh
	case len(r.ManagedArgWarnings)+len(r.FingerprintWarnings) > 0:
		r.RiskLevel = browserStartPreflightRiskMedium
	default:
		r.RiskLevel = browserStartPreflightRiskLow
	}
}

func (r *browserStartPreflightResult) blockingError(profileID string) error {
	if r == nil || len(r.FormatErrors) == 0 {
		return nil
	}

	parts := make([]string, 0, len(r.FormatErrors))
	for _, issue := range r.FormatErrors {
		parts = append(parts, fmt.Sprintf("%s: %s", issue.Source, issue.Message))
	}
	if profileID != "" {
		return fmt.Errorf("实例启动失败：启动前预检发现格式错误（profile_id=%s）：%s", profileID, strings.Join(parts, "; "))
	}
	return fmt.Errorf("实例启动失败：启动前预检发现格式错误：%s", strings.Join(parts, "; "))
}

func (r *browserStartPreflightResult) emit(profileID string) {
	if r == nil {
		return
	}

	log := logger.New("Browser")
	if len(r.ManagedArgWarnings) == 0 && len(r.FingerprintWarnings) == 0 {
		if len(r.FormatErrors) > 0 {
			log.Warn("启动前预检发现格式错误",
				logger.F("profile_id", profileID),
				logger.F("risk_level", r.RiskLevel),
				logger.F("format_errors", len(r.FormatErrors)),
			)
		}
		return
	}

	log.Warn("启动前预检发现风险",
		logger.F("profile_id", profileID),
		logger.F("risk_level", r.RiskLevel),
		logger.F("format_errors", len(r.FormatErrors)),
		logger.F("managed_arg_warnings", len(r.ManagedArgWarnings)),
		logger.F("fingerprint_warnings", len(r.FingerprintWarnings)),
	)
	for _, issue := range r.ManagedArgWarnings {
		log.Warn(issue.Message,
			logger.F("profile_id", profileID),
			logger.F("kind", issue.Kind),
			logger.F("source", issue.Source),
			logger.F("args", issue.Args),
		)
	}
	for _, issue := range r.FingerprintWarnings {
		log.Warn(issue.Message,
			logger.F("profile_id", profileID),
			logger.F("kind", issue.Kind),
			logger.F("source", issue.Source),
			logger.F("args", issue.Args),
		)
	}
}
