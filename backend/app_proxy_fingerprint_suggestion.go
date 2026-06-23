package backend

import (
	"encoding/json"
	"fmt"
	"strings"
)

type ProxyFingerprintSuggestionIssue struct {
	ID       string `json:"id"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
	Explicit bool   `json:"explicit"`
}

type ProxyFingerprintSuggestion struct {
	ProxyId        string                            `json:"proxyId"`
	ProxyName      string                            `json:"proxyName"`
	RiskLevel      string                            `json:"riskLevel"`
	Timezone       string                            `json:"timezone,omitempty"`
	Language       string                            `json:"language,omitempty"`
	Brand          string                            `json:"brand,omitempty"`
	Platform       string                            `json:"platform,omitempty"`
	Warnings       []ProxyFingerprintSuggestionIssue `json:"warnings,omitempty"`
	ExplicitAlerts []ProxyFingerprintSuggestionIssue `json:"explicitAlerts,omitempty"`
	RawIPHealth    *ProxyIPHealthResult              `json:"rawIPHealth,omitempty"`
}

func (a *App) SuggestFingerprintByProxy(proxyId string) ProxyFingerprintSuggestion {
	return a.BrowserProxySuggestFingerprint(proxyId)
}

func (a *App) findProxyByID(proxyId string) (BrowserProxy, bool) {
	proxyID := strings.TrimSpace(proxyId)
	if proxyID == "" {
		return BrowserProxy{}, false
	}
	for _, item := range a.getLatestProxies() {
		if strings.EqualFold(strings.TrimSpace(item.ProxyId), proxyID) {
			return item, true
		}
	}
	return BrowserProxy{}, false
}

func (a *App) getProxyIPHealth(proxyId string) *ProxyIPHealthResult {
	proxyInfo, ok := a.findProxyByID(proxyId)
	if !ok {
		return nil
	}
	if strings.TrimSpace(proxyInfo.LastIPHealthJSON) == "" {
		return nil
	}
	var result ProxyIPHealthResult
	if err := json.Unmarshal([]byte(proxyInfo.LastIPHealthJSON), &result); err != nil {
		return nil
	}
	if strings.TrimSpace(result.ProxyId) == "" {
		result.ProxyId = proxyInfo.ProxyId
	}
	if strings.TrimSpace(result.Source) == "" {
		result.Source = "ip_health"
	}
	return &result
}

func deriveProxyFingerprintRiskLevel(health *ProxyIPHealthResult) string {
	if health == nil {
		return "low"
	}
	switch {
	case !health.Ok:
		return "high"
	case health.FraudScore >= 80:
		return "high"
	case health.FraudScore >= 50:
		return "medium"
	default:
		return "low"
	}
}

func deriveProxyFingerprintBaseProfile(health *ProxyIPHealthResult) (timezone, language, brand, platform string) {
	if health == nil {
		return "Asia/Shanghai", "zh-CN", "Chrome", "windows"
	}
	switch {
	case countryIsCN(health.Country):
		timezone = "Asia/Shanghai"
		language = "zh-CN"
	case strings.EqualFold(strings.TrimSpace(health.Country), "US"), strings.EqualFold(strings.TrimSpace(health.Country), "United States"):
		timezone = "America/New_York"
		language = "en-US"
	case strings.EqualFold(strings.TrimSpace(health.Country), "JP"), strings.EqualFold(strings.TrimSpace(health.Country), "Japan"):
		timezone = "Asia/Tokyo"
		language = "ja-JP"
	default:
		timezone = "UTC"
		language = "en-US"
	}
	switch {
	case strings.Contains(strings.ToLower(health.AsOrganization), "apple"):
		brand = "Safari"
		platform = "mac"
	case strings.Contains(strings.ToLower(health.AsOrganization), "microsoft"):
		brand = "Edge"
		platform = "windows"
	default:
		brand = "Chrome"
		platform = "windows"
	}
	return
}

func countryIsCN(value string) bool {
	n := strings.TrimSpace(strings.ToLower(value))
	return n == "cn" || n == "china" || n == "中国" || n == "中华人民共和国"
}

func normalizeProxyFingerprintSuggestionWarnings(items []ProxyFingerprintSuggestionIssue) []ProxyFingerprintSuggestionIssue {
	if len(items) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]ProxyFingerprintSuggestionIssue, 0, len(items))
	for _, item := range items {
		item.ID = strings.TrimSpace(item.ID)
		item.Message = strings.TrimSpace(item.Message)
		if item.ID == "" || item.Message == "" {
			continue
		}
		key := item.ID + "|" + item.Message
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

func browserProxySuggestionFromHealth(proxyInfo BrowserProxy, health *ProxyIPHealthResult) ProxyFingerprintSuggestion {
	suggestion := ProxyFingerprintSuggestion{
		ProxyId:     strings.TrimSpace(proxyInfo.ProxyId),
		ProxyName:   strings.TrimSpace(proxyInfo.ProxyName),
		RiskLevel:   "unknown",
		RawIPHealth: health,
	}
	if health == nil {
		suggestion.RiskLevel = "low"
		suggestion.Timezone = "Asia/Shanghai"
		suggestion.Language = "zh-CN"
		suggestion.Brand = "Chrome"
		suggestion.Platform = "windows"
		suggestion.Warnings = append(suggestion.Warnings, ProxyFingerprintSuggestionIssue{
			ID:       "fallback-default",
			Message:  "未找到代理健康数据，已回退为默认国内画像建议。",
			Severity: "info",
		})
		suggestion.Warnings = normalizeProxyFingerprintSuggestionWarnings(suggestion.Warnings)
		return suggestion
	}
	suggestion.RiskLevel = deriveProxyFingerprintRiskLevel(health)
	suggestion.Timezone, suggestion.Language, suggestion.Brand, suggestion.Platform = deriveProxyFingerprintBaseProfile(health)
	if suggestion.Timezone == "" {
		suggestion.Timezone = "UTC"
	}
	if suggestion.Language == "" {
		suggestion.Language = "en-US"
	}
	if suggestion.Brand == "" {
		suggestion.Brand = "Chrome"
	}
	if suggestion.Platform == "" {
		suggestion.Platform = "windows"
	}
	if !health.Ok {
		suggestion.ExplicitAlerts = append(suggestion.ExplicitAlerts, ProxyFingerprintSuggestionIssue{
			ID:       "ip-health-failed",
			Message:  "代理 IP 健康检测失败，建议先完成测速/健康检查后再应用指纹建议。",
			Severity: "warning",
			Explicit: true,
		})
	}
	if health.FraudScore >= 80 {
		suggestion.ExplicitAlerts = append(suggestion.ExplicitAlerts, ProxyFingerprintSuggestionIssue{
			ID:       "high-fraud-score",
			Message:  fmt.Sprintf("当前代理风险分较高：%d。建议优先降低风险再启动。", health.FraudScore),
			Severity: "warning",
			Explicit: true,
		})
	}
	if strings.TrimSpace(health.Country) != "" && !strings.EqualFold(strings.TrimSpace(health.Country), "China") && !strings.EqualFold(strings.TrimSpace(health.Country), "中国") {
		suggestion.Warnings = append(suggestion.Warnings, ProxyFingerprintSuggestionIssue{
			ID:       "non-cn-country",
			Message:  fmt.Sprintf("代理出口国家为 %s，建议语言/时区与出口地保持一致。", strings.TrimSpace(health.Country)),
			Severity: "info",
		})
	}
	suggestion.Warnings = normalizeProxyFingerprintSuggestionWarnings(suggestion.Warnings)
	suggestion.ExplicitAlerts = normalizeProxyFingerprintSuggestionWarnings(suggestion.ExplicitAlerts)
	return suggestion
}

func (a *App) BrowserProxySuggestFingerprint(proxyId string) ProxyFingerprintSuggestion {
	proxyInfo, ok := a.findProxyByID(proxyId)
	if !ok {
		return ProxyFingerprintSuggestion{
			ProxyId:   strings.TrimSpace(proxyId),
			RiskLevel: "unknown",
			Warnings: []ProxyFingerprintSuggestionIssue{{
				ID:       "proxy-not-found",
				Message:  "未找到代理，无法生成指纹建议。",
				Severity: "error",
				Explicit: true,
			}},
		}
	}
	return a.suggestFingerprintByProxyInfo(proxyInfo)
}

func (a *App) suggestFingerprintByProxyInfo(proxyInfo BrowserProxy) ProxyFingerprintSuggestion {
	health := a.getProxyIPHealth(proxyInfo.ProxyId)
	return browserProxySuggestionFromHealth(proxyInfo, health)
}
