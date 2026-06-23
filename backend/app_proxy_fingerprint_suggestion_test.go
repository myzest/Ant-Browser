package backend

import (
	"ant-chrome/backend/internal/browser"
	"ant-chrome/backend/internal/config"
	"encoding/json"
	"testing"
)

type proxySuggestionDAOStub struct {
	items []BrowserProxy
}

func (s *proxySuggestionDAOStub) List() ([]BrowserProxy, error) {
	return append([]BrowserProxy{}, s.items...), nil
}
func (s *proxySuggestionDAOStub) ListByGroup(string) ([]BrowserProxy, error)          { return nil, nil }
func (s *proxySuggestionDAOStub) ListGroups() ([]string, error)                       { return nil, nil }
func (s *proxySuggestionDAOStub) Upsert(proxy BrowserProxy) error                     { return nil }
func (s *proxySuggestionDAOStub) Delete(proxyId string) error                         { return nil }
func (s *proxySuggestionDAOStub) DeleteAll() error                                    { return nil }
func (s *proxySuggestionDAOStub) UpdateSpeedResult(string, bool, int64, string) error { return nil }
func (s *proxySuggestionDAOStub) UpdateIPHealthResult(string, string) error           { return nil }

func TestSuggestFingerprintByProxyUsesIPHealthMetadata(t *testing.T) {
	app := &App{config: config.DefaultConfig()}
	app.browserMgr = browser.NewManager(config.DefaultConfig(), "")
	app.browserMgr.ProxyDAO = &proxySuggestionDAOStub{items: []BrowserProxy{{
		ProxyId:          "proxy-cn",
		ProxyName:        "国内节点",
		ProxyConfig:      "http://example:8080",
		LastIPHealthJSON: `{"proxyId":"proxy-cn","ok":true,"fraudScore":12,"country":"China","asOrganization":"China Telecom"}`,
	}}}

	suggestion := app.SuggestFingerprintByProxy("proxy-cn")
	if suggestion.Timezone != "Asia/Shanghai" {
		t.Fatalf("timezone=%q, want Asia/Shanghai", suggestion.Timezone)
	}
	if suggestion.Language != "zh-CN" {
		t.Fatalf("language=%q, want zh-CN", suggestion.Language)
	}
	if suggestion.Platform != "windows" {
		t.Fatalf("platform=%q, want windows", suggestion.Platform)
	}
	if suggestion.RiskLevel != "low" {
		t.Fatalf("risk=%q, want low", suggestion.RiskLevel)
	}
	if len(suggestion.ExplicitAlerts) != 0 {
		t.Fatalf("unexpected explicit alerts: %+v", suggestion.ExplicitAlerts)
	}
}

func TestSuggestFingerprintByProxyFallsBackWithoutHealth(t *testing.T) {
	app := &App{config: config.DefaultConfig()}
	app.browserMgr = browser.NewManager(config.DefaultConfig(), "")
	app.browserMgr.ProxyDAO = &proxySuggestionDAOStub{items: []BrowserProxy{{
		ProxyId:     "proxy-x",
		ProxyName:   "节点X",
		ProxyConfig: "direct://",
	}}}

	suggestion := app.SuggestFingerprintByProxy("proxy-x")
	if suggestion.Timezone == "" || suggestion.Language == "" {
		t.Fatalf("fallback suggestion should fill base profile: %+v", suggestion)
	}
	if suggestion.RiskLevel != "low" {
		t.Fatalf("risk=%q, want low", suggestion.RiskLevel)
	}
}

func TestBrowserStartPreflightExposesRiskAndWarnings(t *testing.T) {
	app := &App{config: config.DefaultConfig()}
	app.browserMgr = browser.NewManager(config.DefaultConfig(), "")
	app.browserMgr.Profiles["profile-1"] = &BrowserProfile{
		ProfileId:   "profile-1",
		ProfileName: "测试配置",
		CoreId:      "default",
		FingerprintArgs: []string{
			"--user-agent=Mozilla/5.0 (Windows NT 10.0; Win64; x64)",
			"--fingerprint-platform=mac",
		},
		LaunchArgs:  []string{"--proxy-server=127.0.0.1:8080"},
		ProxyId:     "__direct__",
		ProxyConfig: "direct://",
	}

	res := app.BrowserInstancePreflight("profile-1", "normal")
	if res.RiskLevel == "" {
		t.Fatal("risk level should not be empty")
	}
	if res.Summary == "" {
		t.Fatal("summary should not be empty")
	}
	if len(res.Warnings) == 0 && len(res.ExplicitAlerts) == 0 {
		t.Fatal("expected warnings or explicit alerts")
	}
}

func TestProxyFingerprintSuggestionJSONRoundTrip(t *testing.T) {
	s := ProxyFingerprintSuggestion{ProxyId: "proxy-1", RiskLevel: "medium"}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var out ProxyFingerprintSuggestion
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if out.ProxyId != s.ProxyId || out.RiskLevel != s.RiskLevel {
		t.Fatalf("round trip mismatch: %+v", out)
	}
}
