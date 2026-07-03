package backend

import (
	"ant-chrome/backend/internal/fingerprint"
	"ant-chrome/backend/internal/logger"
	"ant-chrome/backend/internal/proxy"
	"encoding/json"
	"fmt"
	"strings"
)

const temporaryDirectProxyID = "__direct__"

func (a *App) resolveBrowserStartProxy(input browserStartInput, profile *BrowserProfile) (string, string, string, bool, []string, error) {
	log := logger.New("Browser")
	proxies := a.getLatestProxies()
	profileID := input.ProfileID

	if input.ForceDirectProxy {
		log.Warn("按请求直连启动实例",
			logger.F("profile_id", profileID),
			logger.F("proxy_id", profile.ProxyId),
		)
		return "direct://", "", "", false, nil, nil
	}

	resolvedProxyID := strings.TrimSpace(profile.ProxyId)
	resolvedProxyConfig := strings.TrimSpace(profile.ProxyConfig)
	temporaryProxyRegionArgs := []string(nil)
	usingTemporaryProxy := input.hasTemporaryProxy()
	if usingTemporaryProxy {
		var err error
		resolvedProxyID, resolvedProxyConfig, err = resolveTemporaryBrowserStartProxy(input.TemporaryProxyID, input.TemporaryProxyConfig, proxies)
		if err != nil {
			startErr := fmt.Errorf("实例启动失败：%s", err.Error())
			profile.LastError = startErr.Error()
			log.Error("一次性代理配置无效",
				logger.F("profile_id", profileID),
				logger.F("temporary_proxy_id", input.TemporaryProxyID),
				logger.F("error", err.Error()),
				logger.F("reason", startErr.Error()),
			)
			return "", "", "", false, nil, startErr
		}
		temporaryProxyRegionArgs = temporaryProxyRegionLaunchArgs(resolveTemporaryProxyRegionContext(input, resolvedProxyID, resolvedProxyConfig, proxies))
	} else if resolvedProxyID != "" {
		for _, item := range proxies {
			if strings.EqualFold(item.ProxyId, resolvedProxyID) {
				resolvedProxyID = strings.TrimSpace(item.ProxyId)
				resolvedProxyConfig = strings.TrimSpace(item.ProxyConfig)
				break
			}
		}
	}

	log.Info("代理配置检查",
		logger.F("profile_id", profileID),
		logger.F("proxy_id", profile.ProxyId),
		logger.F("profile_proxy_config", proxy.RedactProxyURL(profile.ProxyConfig)),
		logger.F("temporary_proxy", usingTemporaryProxy),
		logger.F("temporary_proxy_id", input.TemporaryProxyID),
		logger.F("temporary_proxy_config", proxy.RedactProxyURL(input.TemporaryProxyConfig)),
		logger.F("resolved_proxy_config", proxy.RedactProxyURL(resolvedProxyConfig)),
	)
	if supported, errorMsg := proxy.ValidateProxyConfig(resolvedProxyConfig, proxies, resolvedProxyID); !supported {
		startErr := fmt.Errorf("实例启动失败：%s", errorMsg)
		profile.LastError = startErr.Error()
		log.Error("代理配置无效",
			logger.F("profile_id", profileID),
			logger.F("proxy_id", resolvedProxyID),
			logger.F("error", errorMsg),
			logger.F("reason", startErr.Error()),
		)
		return "", "", "", false, nil, startErr
	}

	exitIPCacheKey := browserStartProxyCacheKey(resolvedProxyID, resolvedProxyConfig)
	if proxy.IsSingBoxProtocol(resolvedProxyConfig) {
		socksURL, bridgeErr := a.singboxMgr.EnsureBridge(resolvedProxyConfig, proxies, resolvedProxyID)
		if bridgeErr != nil {
			startErr := fmt.Errorf("实例启动失败：代理桥接启动失败（sing-box）。原因：%v。请检查代理节点配置、sing-box 可执行文件是否存在，以及本地端口是否被占用。", bridgeErr)
			log.Error("代理桥接失败(sing-box)",
				logger.F("error", bridgeErr.Error()),
				logger.F("reason", startErr.Error()),
			)
			profile.LastError = startErr.Error()
			return "", "", "", false, nil, startErr
		}
		log.Info("sing-box 桥接成功", logger.F("socks_url", socksURL))
		return socksURL, exitIPCacheKey, "", false, temporaryProxyRegionArgs, nil
	}

	if proxy.RequiresBridge(resolvedProxyConfig, proxies, resolvedProxyID) || proxy.RequiresLocalProxyBridgeForBrowser(resolvedProxyConfig) {
		socksURL, bridgeKey, bridgeErr := a.xrayMgr.AcquireBridge(resolvedProxyConfig, proxies, resolvedProxyID)
		if bridgeErr != nil {
			startErr := fmt.Errorf("实例启动失败：代理桥接启动失败（xray）。原因：%v。请检查代理节点配置、xray 可执行文件是否存在，以及本地端口是否被占用。", bridgeErr)
			log.Error("代理桥接失败(xray)",
				logger.F("error", bridgeErr.Error()),
				logger.F("reason", startErr.Error()),
			)
			profile.LastError = startErr.Error()
			return "", "", "", false, nil, startErr
		}
		log.Info("xray 桥接成功", logger.F("socks_url", socksURL))
		return socksURL, exitIPCacheKey, bridgeKey, bridgeKey != "", temporaryProxyRegionArgs, nil
	}

	return resolvedProxyConfig, exitIPCacheKey, "", false, temporaryProxyRegionArgs, nil
}

func resolveTemporaryBrowserStartProxy(proxyID string, proxyConfig string, proxies []BrowserProxy) (string, string, error) {
	proxyID = strings.TrimSpace(proxyID)
	proxyConfig = strings.TrimSpace(proxyConfig)
	if proxyID == "" {
		return "", proxyConfig, nil
	}

	for _, item := range proxies {
		if strings.EqualFold(item.ProxyId, proxyID) {
			return strings.TrimSpace(item.ProxyId), strings.TrimSpace(item.ProxyConfig), nil
		}
	}
	if strings.EqualFold(proxyID, temporaryDirectProxyID) {
		return temporaryDirectProxyID, "direct://", nil
	}
	if proxyConfig != "" {
		return "", proxyConfig, nil
	}
	return "", "", fmt.Errorf("代理ID不存在（proxy id not found: %s），且未提供 proxyConfig", proxyID)
}

func browserStartProxyCacheKey(proxyID string, proxyConfig string) string {
	proxyID = strings.TrimSpace(proxyID)
	proxyConfig = strings.TrimSpace(proxyConfig)
	if proxyID != "" {
		return proxyID + "|" + proxyConfig
	}
	return proxyConfig
}

func resolveTemporaryProxyRegionContext(input browserStartInput, resolvedProxyID string, resolvedProxyConfig string, proxies []BrowserProxy) fingerprint.ValidationContext {
	if !input.hasTemporaryProxy() {
		return fingerprint.ValidationContext{}
	}
	if strings.EqualFold(strings.TrimSpace(resolvedProxyConfig), "direct://") {
		return fingerprint.ValidationContext{}
	}

	for _, proxyID := range []string{resolvedProxyID, input.TemporaryProxyID} {
		proxyID = strings.TrimSpace(proxyID)
		if proxyID == "" || strings.EqualFold(proxyID, temporaryDirectProxyID) {
			continue
		}
		for _, item := range proxies {
			if strings.EqualFold(strings.TrimSpace(item.ProxyId), proxyID) {
				return temporaryProxyRegionContextFromProxy(item)
			}
		}
	}
	for _, proxyConfig := range []string{resolvedProxyConfig, input.TemporaryProxyConfig} {
		proxyConfig = strings.TrimSpace(proxyConfig)
		if proxyConfig == "" || strings.EqualFold(proxyConfig, "direct://") {
			continue
		}
		for _, item := range proxies {
			if strings.EqualFold(strings.TrimSpace(item.ProxyConfig), proxyConfig) {
				return temporaryProxyRegionContextFromProxy(item)
			}
		}
	}
	return fingerprint.ValidationContext{}
}

func temporaryProxyRegionContextFromProxy(item BrowserProxy) fingerprint.ValidationContext {
	ctx := fingerprint.ValidationContext{
		ProxyCountry:  strings.TrimSpace(item.Country),
		ProxyLocale:   strings.TrimSpace(item.Locale),
		ProxyTimezone: strings.TrimSpace(item.Timezone),
	}
	if cached, ok := temporaryProxyRegionContextFromIPHealthJSON(item.LastIPHealthJSON); ok {
		ctx = mergeProxyRegionContext(ctx, cached)
	}
	if strings.TrimSpace(ctx.ProxyCountry) != "" || strings.TrimSpace(ctx.ProxyLocale) != "" || strings.TrimSpace(ctx.ProxyTimezone) != "" {
		locale, timezone, country := fingerprint.ProxyRegionDefaults(ctx.ProxyCountry, ctx.ProxyLocale, ctx.ProxyTimezone)
		ctx.ProxyCountry = country
		ctx.ProxyLocale = locale
		ctx.ProxyTimezone = timezone
	}
	return ctx
}

func temporaryProxyRegionContextFromIPHealthJSON(raw string) (fingerprint.ValidationContext, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fingerprint.ValidationContext{}, false
	}
	var result ProxyIPHealthResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil || !result.Ok {
		return fingerprint.ValidationContext{}, false
	}
	ctx := fingerprint.ValidationContext{
		ProxyCountry:  strings.TrimSpace(result.Country),
		ProxyLocale:   strings.TrimSpace(result.Locale),
		ProxyTimezone: strings.TrimSpace(result.Timezone),
	}
	return ctx, ctx.ProxyCountry != "" || ctx.ProxyLocale != "" || ctx.ProxyTimezone != ""
}

func temporaryProxyRegionLaunchArgs(ctx fingerprint.ValidationContext) []string {
	if strings.TrimSpace(ctx.ProxyCountry) == "" && strings.TrimSpace(ctx.ProxyLocale) == "" && strings.TrimSpace(ctx.ProxyTimezone) == "" {
		return nil
	}

	locale, timezone, country := fingerprint.ProxyRegionDefaults(ctx.ProxyCountry, ctx.ProxyLocale, ctx.ProxyTimezone)
	args := []string{}
	if locale != "" {
		args = append(args,
			"--lang="+locale,
			"--fingerprint-locale="+locale,
		)
		if acceptLanguage := fingerprint.AcceptLanguageDefaults(country, locale); acceptLanguage != "" {
			args = append(args, "--fingerprint-accept-language="+acceptLanguage)
		}
	}
	if timezone != "" {
		args = append(args,
			"--timezone="+timezone,
			"--fingerprint-timezone="+timezone,
		)
	}
	return args
}
