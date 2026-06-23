package backend

import (
	"ant-chrome/backend/internal/browser"
	"ant-chrome/backend/internal/config"
	"sort"
	"testing"
)

type proxySaveDAOStub struct {
	items map[string]BrowserProxy
}

func newProxySaveDAOStub(initial []BrowserProxy) *proxySaveDAOStub {
	items := make(map[string]BrowserProxy, len(initial))
	for _, item := range initial {
		items[item.ProxyId] = item
	}
	return &proxySaveDAOStub{items: items}
}

func (s *proxySaveDAOStub) List() ([]BrowserProxy, error) {
	out := make([]BrowserProxy, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].SortOrder < out[j].SortOrder || (out[i].SortOrder == out[j].SortOrder && out[i].ProxyId < out[j].ProxyId)
	})
	return out, nil
}

func (s *proxySaveDAOStub) ListByGroup(string) ([]BrowserProxy, error) { return nil, nil }
func (s *proxySaveDAOStub) ListGroups() ([]string, error)              { return nil, nil }
func (s *proxySaveDAOStub) DeleteAll() error {
	s.items = map[string]BrowserProxy{}
	return nil
}
func (s *proxySaveDAOStub) Delete(proxyId string) error {
	delete(s.items, proxyId)
	return nil
}
func (s *proxySaveDAOStub) UpdateSpeedResult(string, bool, int64, string) error { return nil }
func (s *proxySaveDAOStub) UpdateIPHealthResult(string, string) error           { return nil }
func (s *proxySaveDAOStub) Upsert(proxy BrowserProxy) error {
	s.items[proxy.ProxyId] = proxy
	return nil
}

func TestSaveBrowserProxiesPreservesExistingHealthFields(t *testing.T) {
	app := &App{
		config: config.DefaultConfig(),
	}
	app.browserMgr = browser.NewManager(config.DefaultConfig(), "")
	app.browserMgr.ProxyDAO = newProxySaveDAOStub([]BrowserProxy{
		{
			ProxyId:          "__direct__",
			ProxyName:        "直连（不走代理）",
			ProxyConfig:      "direct://",
			LastLatencyMs:    11,
			LastTestOk:       true,
			LastTestedAt:     "2026-06-22T10:00:00Z",
			LastIPHealthJSON: `{"ip":"127.0.0.1","ok":true}`,
			SortOrder:        0,
		},
		{
			ProxyId:          "proxy-old",
			ProxyName:        "旧节点",
			ProxyConfig:      "http://old.example:8080",
			LastLatencyMs:    222,
			LastTestOk:       true,
			LastTestedAt:     "2026-06-22T11:00:00Z",
			LastIPHealthJSON: `{"ip":"10.0.0.8","ok":true}`,
			SortOrder:        1,
		},
		{
			ProxyId:          "proxy-drop",
			ProxyName:        "待删除",
			ProxyConfig:      "socks5://drop.example:1080",
			LastLatencyMs:    333,
			LastTestOk:       false,
			LastTestedAt:     "2026-06-22T12:00:00Z",
			LastIPHealthJSON: `{"ip":"10.0.0.9","ok":false}`,
			SortOrder:        2,
		},
	})

	err := app.SaveBrowserProxies([]BrowserProxy{
		{
			ProxyId:     "proxy-old",
			ProxyName:   "旧节点（已编辑）",
			ProxyConfig: "http://new.example:8080",
		},
		{
			ProxyId:     "proxy-new",
			ProxyName:   "新节点",
			ProxyConfig: "socks5://new.example:1080",
		},
	})
	if err != nil {
		t.Fatalf("SaveBrowserProxies returned error: %v", err)
	}

	got := app.browserMgr.ProxyDAO.(*proxySaveDAOStub).items
	if _, ok := got["proxy-drop"]; ok {
		t.Fatalf("deleted proxy still exists in DAO")
	}

	direct, ok := got["__direct__"]
	if !ok {
		t.Fatalf("builtin direct proxy missing after save")
	}
	if direct.LastLatencyMs != 11 || !direct.LastTestOk || direct.LastTestedAt != "2026-06-22T10:00:00Z" || direct.LastIPHealthJSON != `{"ip":"127.0.0.1","ok":true}` {
		t.Fatalf("builtin direct health fields were not preserved: %+v", direct)
	}

	updated, ok := got["proxy-old"]
	if !ok {
		t.Fatalf("edited proxy missing after save")
	}
	if updated.ProxyName != "旧节点（已编辑）" || updated.ProxyConfig != "http://new.example:8080" {
		t.Fatalf("edited proxy fields not saved: %+v", updated)
	}
	if updated.LastLatencyMs != 222 || !updated.LastTestOk || updated.LastTestedAt != "2026-06-22T11:00:00Z" || updated.LastIPHealthJSON != `{"ip":"10.0.0.8","ok":true}` {
		t.Fatalf("edited proxy health fields were lost: %+v", updated)
	}

	added, ok := got["proxy-new"]
	if !ok {
		t.Fatalf("new proxy missing after save")
	}
	if added.LastLatencyMs != 0 || added.LastTestOk || added.LastTestedAt != "" || added.LastIPHealthJSON != "" {
		t.Fatalf("new proxy should not inherit health fields: %+v", added)
	}

	configByID := map[string]BrowserProxy{}
	for _, item := range app.config.Browser.Proxies {
		configByID[item.ProxyId] = item
	}
	configUpdated, ok := configByID["proxy-old"]
	if !ok {
		t.Fatalf("edited proxy missing from in-memory config")
	}
	if configUpdated.LastLatencyMs != 222 || !configUpdated.LastTestOk || configUpdated.LastTestedAt != "2026-06-22T11:00:00Z" || configUpdated.LastIPHealthJSON != `{"ip":"10.0.0.8","ok":true}` {
		t.Fatalf("in-memory config lost preserved health fields: %+v", configUpdated)
	}
}
