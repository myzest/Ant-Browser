package backend

import (
	"ant-chrome/backend/internal/browser"
	"ant-chrome/backend/internal/config"
	"errors"
	"testing"
)

func TestBuildProxyIPHealthResultPreservesErrorSourceMetadata(t *testing.T) {
	result := buildProxyIPHealthResult("proxy-1", map[string]interface{}{
		"_source":    "trace",
		"_targetUrl": "https://example.invalid/trace",
		"_parser":    "cloudflare_trace",
	}, errors.New("request failed"))

	if result.Source != "trace" {
		t.Fatalf("source = %q, want trace", result.Source)
	}
	if result.Error != "request failed" {
		t.Fatalf("error = %q, want request failed", result.Error)
	}
	rawError, _ := result.RawData["error"].(string)
	if rawError != "request failed" {
		t.Fatalf("raw error = %q, want request failed", rawError)
	}
	if got, _ := result.RawData["_targetUrl"].(string); got != "https://example.invalid/trace" {
		t.Fatalf("target url = %q, want trace url", got)
	}
}

func TestBuildProxyIPHealthResultIncludesGeoIPFields(t *testing.T) {
	t.Parallel()

	result := buildProxyIPHealthResult("proxy-geoip", map[string]interface{}{
		"_source":    "ip_health",
		"ip":         "203.0.113.7",
		"country":    "US",
		"region":     "California",
		"city":       "Los Angeles",
		"timezone":   "America/Los_Angeles",
		"locale":     "en-US",
		"fraudScore": 10,
	}, nil)

	if !result.Ok || result.Country != "US" || result.Timezone != "America/Los_Angeles" || result.Locale != "en-US" {
		t.Fatalf("geoip fields not preserved: %#v", result)
	}
}

func TestBuildProxyIPHealthResultBackfillsRegionDefaults(t *testing.T) {
	t.Parallel()

	result := buildProxyIPHealthResult("proxy-region-defaults", map[string]interface{}{
		"ip":      "203.0.113.8",
		"country": "US",
	}, nil)

	if result.Locale != "en-US" || result.Timezone != "America/New_York" {
		t.Fatalf("region defaults not backfilled: %#v", result)
	}
	if result.RawData["locale"] != "en-US" || result.RawData["timezone"] != "America/New_York" {
		t.Fatalf("region defaults not written to raw data: %#v", result.RawData)
	}
}

func TestPersistProxyIPHealthCalibratesCountryLocaleAndTimezone(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultConfig()
	dao := &proxyHealthProxyDAOStub{proxies: []browser.Proxy{
		{
			ProxyId:   "proxy-us-west",
			ProxyName: "US West",
			Country:   "UNITED STATES",
			Locale:    "zh-CN",
			Timezone:  "Asia/Shanghai",
		},
	}}
	app := NewApp("")
	app.config = cfg
	app.browserMgr = browser.NewManager(cfg, "")
	app.browserMgr.ProxyDAO = dao

	app.persistProxyIPHealthResult(buildProxyIPHealthResult("proxy-us-west", map[string]interface{}{
		"ip":       "203.0.113.10",
		"country":  "UNITED STATES",
		"locale":   "zh-CN",
		"timezone": "Asia/Shanghai",
	}, nil))

	got := dao.proxies[0]
	if got.Country != "US" || got.Locale != "en-US" || got.Timezone != "America/New_York" {
		t.Fatalf("proxy region should be calibrated, got %#v", got)
	}
	if dao.lastIPHealthJSON == "" {
		t.Fatal("expected IP health JSON to be persisted")
	}
}

func TestSaveBrowserProxiesCalibratesAllProxyTypesRegion(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultConfig()
	app := NewApp("")
	app.config = cfg
	app.browserMgr = browser.NewManager(cfg, "")

	proxies := []BrowserProxy{
		{ProxyId: "proxy-http", ProxyName: "HTTP", ProxyConfig: "http://127.0.0.1:8080", Country: "UNITED STATES", Locale: "zh-CN", Timezone: "Asia/Shanghai"},
		{ProxyId: "proxy-clash", ProxyName: "Clash", ProxyConfig: `name: us
type: vmess
server: example.com
port: 443`, Country: "United States", Locale: "zh-CN", Timezone: "Asia/Shanghai"},
		{ProxyId: "proxy-chain", ProxyName: "Chain", ProxyConfig: "chain+socks5://example", Country: "USA", Locale: "zh-CN", Timezone: "Asia/Shanghai"},
	}
	if err := app.SaveBrowserProxies(proxies); err != nil {
		t.Fatalf("SaveBrowserProxies returned error: %v", err)
	}

	for _, item := range app.config.Browser.Proxies {
		if item.ProxyId == "__direct__" {
			continue
		}
		if item.Country != "US" || item.Locale != "en-US" || item.Timezone != "America/New_York" {
			t.Fatalf("proxy %s should be calibrated regardless of type: %#v", item.ProxyId, item)
		}
	}
}

type proxyHealthProxyDAOStub struct {
	proxies          []browser.Proxy
	lastIPHealthJSON string
}

func (s *proxyHealthProxyDAOStub) List() ([]browser.Proxy, error) {
	return append([]browser.Proxy{}, s.proxies...), nil
}

func (s *proxyHealthProxyDAOStub) ListByGroup(groupName string) ([]browser.Proxy, error) {
	return nil, nil
}

func (s *proxyHealthProxyDAOStub) ListGroups() ([]string, error) {
	return nil, nil
}

func (s *proxyHealthProxyDAOStub) Upsert(proxy browser.Proxy) error {
	for i, item := range s.proxies {
		if item.ProxyId == proxy.ProxyId {
			s.proxies[i] = proxy
			return nil
		}
	}
	s.proxies = append(s.proxies, proxy)
	return nil
}

func (s *proxyHealthProxyDAOStub) Delete(proxyId string) error {
	return nil
}

func (s *proxyHealthProxyDAOStub) DeleteAll() error {
	return nil
}

func (s *proxyHealthProxyDAOStub) UpdateSpeedResult(proxyId string, ok bool, latencyMs int64, testedAt string) error {
	return nil
}

func (s *proxyHealthProxyDAOStub) UpdateIPHealthResult(proxyId string, healthJSON string) error {
	s.lastIPHealthJSON = healthJSON
	return nil
}
