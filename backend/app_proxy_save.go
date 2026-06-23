package backend

import (
	"ant-chrome/backend/internal/config"
	"ant-chrome/backend/internal/logger"
	"strings"
)

func (a *App) SaveBrowserProxies(proxies []BrowserProxy) error {
	log := logger.New("Browser")
	normalized := normalizeBrowserProxies(proxies)

	if a.browserMgr.ProxyDAO != nil {
		existing, err := a.browserMgr.ProxyDAO.List()
		if err != nil {
			log.Warn("读取现有代理失败，回退为整表重建", logger.F("error", err))
			a.config.Browser.Proxies = normalized
			if err := a.saveBrowserProxiesByRebuild(normalized, log); err != nil {
				return err
			}
			a.reconcileProfileProxyBindings()
			return nil
		}

		normalized = preserveBrowserProxyHealth(normalized, existing)
		a.config.Browser.Proxies = normalized

		existingByID := make(map[string]BrowserProxy, len(existing))
		for _, item := range existing {
			proxyID := strings.TrimSpace(item.ProxyId)
			if proxyID == "" {
				continue
			}
			existingByID[proxyID] = item
		}
		for _, item := range normalized {
			delete(existingByID, strings.TrimSpace(item.ProxyId))
		}
		for proxyID := range existingByID {
			if err := a.browserMgr.ProxyDAO.Delete(proxyID); err != nil {
				log.Error("删除已移除代理失败", logger.F("proxy_id", proxyID), logger.F("error", err))
				return err
			}
		}
		for _, item := range normalized {
			if err := a.browserMgr.ProxyDAO.Upsert(item); err != nil {
				log.Error("代理保存失败", logger.F("proxy_id", item.ProxyId), logger.F("error", err))
				return err
			}
		}
		log.Info("代理列表已保存到数据库", logger.F("count", len(normalized)))
		a.reconcileProfileProxyBindings()
		return nil
	}

	a.config.Browser.Proxies = normalized
	if err := config.SaveProxies(a.resolveAppPath("proxies.yaml"), normalized); err != nil {
		log.Error("代理列表保存失败", logger.F("error", err))
		return err
	}
	a.reconcileProfileProxyBindings()
	return nil
}

func (a *App) saveBrowserProxiesByRebuild(normalized []BrowserProxy, log *logger.Logger) error {
	if err := a.browserMgr.ProxyDAO.DeleteAll(); err != nil {
		log.Error("清空代理表失败", logger.F("error", err))
		return err
	}
	for _, item := range normalized {
		if err := a.browserMgr.ProxyDAO.Upsert(item); err != nil {
			log.Error("代理保存失败", logger.F("proxy_id", item.ProxyId), logger.F("error", err))
			return err
		}
	}
	return nil
}

func normalizeBrowserProxies(proxies []BrowserProxy) []BrowserProxy {
	normalized := make([]BrowserProxy, 0, len(proxies))
	for i, item := range proxies {
		proxyName := strings.TrimSpace(item.ProxyName)
		proxyConfig := strings.TrimSpace(item.ProxyConfig)
		if proxyName == "" || proxyConfig == "" {
			continue
		}
		proxyID := strings.TrimSpace(item.ProxyId)
		if proxyID == "" {
			proxyID = generateUUID()
		}
		sourceURL := strings.TrimSpace(item.SourceURL)
		sourceID := strings.TrimSpace(item.SourceID)
		sourceNamePrefix := strings.TrimSpace(item.SourceNamePrefix)
		sourceLastRefreshAt := strings.TrimSpace(item.SourceLastRefreshAt)
		sourceRefreshIntervalM := item.SourceRefreshIntervalM
		if sourceRefreshIntervalM < 0 {
			sourceRefreshIntervalM = 0
		}
		if sourceRefreshIntervalM > 24*60 {
			sourceRefreshIntervalM = 24 * 60
		}
		sourceAutoRefresh := item.SourceAutoRefresh && sourceURL != ""
		if sourceAutoRefresh && sourceRefreshIntervalM <= 0 {
			sourceRefreshIntervalM = 60
		}
		if !sourceAutoRefresh {
			sourceRefreshIntervalM = 0
		}
		if sourceURL == "" {
			sourceID = ""
			sourceNamePrefix = ""
			sourceLastRefreshAt = ""
			sourceAutoRefresh = false
			sourceRefreshIntervalM = 0
		}
		normalized = append(normalized, BrowserProxy{
			ProxyId:                proxyID,
			ProxyName:              proxyName,
			ProxyConfig:            proxyConfig,
			DnsServers:             strings.TrimSpace(item.DnsServers),
			GroupName:              strings.TrimSpace(item.GroupName),
			SourceID:               sourceID,
			SourceURL:              sourceURL,
			SourceNamePrefix:       sourceNamePrefix,
			SourceAutoRefresh:      sourceAutoRefresh,
			SourceRefreshIntervalM: sourceRefreshIntervalM,
			SourceLastRefreshAt:    sourceLastRefreshAt,
			SortOrder:              i,
		})
	}

	builtins := []BrowserProxy{
		{ProxyId: "__direct__", ProxyName: "直连（不走代理）", ProxyConfig: "direct://"},
	}
	for _, builtin := range builtins {
		found := false
		for _, item := range normalized {
			if item.ProxyId == builtin.ProxyId {
				found = true
				break
			}
		}
		if !found {
			normalized = append([]BrowserProxy{builtin}, normalized...)
		}
	}
	return normalized
}

func preserveBrowserProxyHealth(current []BrowserProxy, existing []BrowserProxy) []BrowserProxy {
	if len(current) == 0 || len(existing) == 0 {
		return current
	}
	existingByID := make(map[string]BrowserProxy, len(existing))
	for _, item := range existing {
		proxyID := strings.TrimSpace(item.ProxyId)
		if proxyID == "" {
			continue
		}
		if _, ok := existingByID[proxyID]; !ok {
			existingByID[proxyID] = item
		}
	}
	if len(existingByID) == 0 {
		return current
	}

	out := make([]BrowserProxy, len(current))
	copy(out, current)
	for i, item := range out {
		if existing, ok := existingByID[strings.TrimSpace(item.ProxyId)]; ok {
			out[i].LastLatencyMs = existing.LastLatencyMs
			out[i].LastTestOk = existing.LastTestOk
			out[i].LastTestedAt = existing.LastTestedAt
			out[i].LastIPHealthJSON = existing.LastIPHealthJSON
		}
	}
	return out
}
