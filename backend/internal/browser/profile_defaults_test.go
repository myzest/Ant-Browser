package browser

import (
	"ant-chrome/backend/internal/config"
	"slices"
	"testing"
)

func TestApplyDefaultsDoesNotFallbackToDirectAfterPoolBindByProxyConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	mgr := NewManager(cfg, "")
	mgr.ProxyDAO = &proxyDAOStub{
		list: []Proxy{
			{ProxyId: directProxyID, ProxyName: "直连（不走代理）", ProxyConfig: "direct://"},
			{ProxyId: "pool-1", ProxyName: "节点-01", ProxyConfig: "socks5://127.0.0.1:1080"},
		},
	}

	profile := &Profile{
		ProfileId:   "pf-apply-defaults-1",
		ProxyId:     "",
		ProxyConfig: "socks5://127.0.0.1:1080",
	}

	changed := mgr.ApplyDefaults(profile)
	if !changed {
		t.Fatalf("expected proxy binding to change")
	}
	if profile.ProxyId != "pool-1" {
		t.Fatalf("expected proxyId to bind to pool-1, got=%q", profile.ProxyId)
	}
	if profile.ProxyId == directProxyID {
		t.Fatalf("expected not to fallback to direct proxy")
	}
}

func TestApplyDefaultsKeepsCustomProxyConfigWhenNotInPool(t *testing.T) {
	cfg := config.DefaultConfig()
	mgr := NewManager(cfg, "")
	mgr.ProxyDAO = &proxyDAOStub{
		list: []Proxy{
			{ProxyId: directProxyID, ProxyName: "直连（不走代理）", ProxyConfig: "direct://"},
			{ProxyId: "pool-2", ProxyName: "日本-01", ProxyConfig: "socks5://127.0.0.1:2080"},
		},
	}

	profile := &Profile{
		ProfileId:   "pf-apply-defaults-2",
		ProxyId:     "",
		ProxyConfig: "http://127.0.0.1:9090",
	}

	_ = mgr.ApplyDefaults(profile)
	if profile.ProxyId != "" {
		t.Fatalf("expected proxyId to stay empty for custom proxyConfig, got=%q", profile.ProxyId)
	}
	if profile.ProxyConfig != "http://127.0.0.1:9090" {
		t.Fatalf("expected proxyConfig to be preserved, got=%q", profile.ProxyConfig)
	}
}

func TestApplyDefaultsClearsMissingProxyIdButPreservesProxyConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	mgr := NewManager(cfg, "")
	mgr.ProxyDAO = &proxyDAOStub{
		list: []Proxy{
			{ProxyId: directProxyID, ProxyName: "直连（不走代理）", ProxyConfig: "direct://"},
		},
	}

	profile := &Profile{
		ProfileId:   "pf-apply-defaults-3",
		ProxyId:     "missing-proxy-id",
		ProxyConfig: "http://127.0.0.1:9090",
	}

	changed := mgr.ApplyDefaults(profile)
	if !changed {
		t.Fatalf("expected proxy binding to change when clearing missing proxyId")
	}
	if profile.ProxyId != "" {
		t.Fatalf("expected missing proxyId to be cleared, got=%q", profile.ProxyId)
	}
	if profile.ProxyConfig != "http://127.0.0.1:9090" {
		t.Fatalf("expected proxyConfig to be preserved, got=%q", profile.ProxyConfig)
	}
	if profile.ProxyId == directProxyID {
		t.Fatalf("expected not to fallback to direct proxy when proxyConfig is present")
	}
}

func TestApplyDefaultsFallsBackToDirectWhenProxyMissing(t *testing.T) {
	cfg := config.DefaultConfig()
	mgr := NewManager(cfg, "")
	mgr.ProxyDAO = &proxyDAOStub{
		list: []Proxy{
			{ProxyId: directProxyID, ProxyName: "直连（不走代理）", ProxyConfig: "direct://"},
		},
	}

	profile := &Profile{
		ProfileId:   "pf-apply-defaults-4",
		ProxyId:     "",
		ProxyConfig: "",
	}

	changed := mgr.ApplyDefaults(profile)
	if !changed {
		t.Fatalf("expected direct proxy fallback to change profile")
	}
	if profile.ProxyId != directProxyID {
		t.Fatalf("expected fallback to direct proxy id, got=%q", profile.ProxyId)
	}
	if profile.ProxyConfig != "direct://" {
		t.Fatalf("expected fallback proxy config to be direct://, got=%q", profile.ProxyConfig)
	}
}

func TestApplyDefaultsGeneratesCompleteFingerprintForEmptyArgs(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Browser.DefaultFingerprintArgs = []string{
		"--fingerprint-brand=Chrome",
		"--fingerprint-platform=windows",
		"--fingerprint-accept-language=zh-CN,zh;q=0.9",
		"--fingerprint-do-not-track=false",
	}
	mgr := NewManager(cfg, "")
	mgr.ProxyDAO = &proxyDAOStub{}

	profile := &Profile{
		ProfileId:   "pf-apply-defaults-empty-fingerprint",
		ProxyConfig: "http://127.0.0.1:9090",
	}

	changed := mgr.ApplyDefaults(profile)
	if !changed {
		t.Fatalf("expected empty fingerprint args to be generated and persisted")
	}
	for _, want := range []string{
		"--fingerprint=",
		"--fingerprint-platform=windows",
		"--lang=",
		"--fingerprint-locale=",
		"--timezone=",
		"--fingerprint-timezone=",
		"--fingerprint-accept-language=",
		"--fingerprint-webgl-vendor=",
		"--fingerprint-webgl-renderer=",
		"--fingerprint-fonts=",
		"--fingerprint-screen-avail=",
		"--fingerprint-do-not-track=false",
	} {
		if !browserTestHasPrefix(profile.FingerprintArgs, want) {
			t.Fatalf("expected generated fingerprint arg prefix %q, got=%v", want, profile.FingerprintArgs)
		}
	}
}

func TestApplyDefaultsGeneratesEmptyFingerprintFromBoundProxyRegion(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Browser.DefaultFingerprintArgs = []string{
		"--fingerprint-brand=Chrome",
		"--fingerprint-platform=windows",
		"--fingerprint-accept-language=zh-CN,zh;q=0.9",
		"--fingerprint-do-not-track=false",
	}
	mgr := NewManager(cfg, "")
	mgr.ProxyDAO = &proxyDAOStub{
		list: []Proxy{
			{
				ProxyId:     "proxy-us",
				ProxyName:   "US",
				ProxyConfig: "http://127.0.0.1:18080",
				Country:     "US",
				Locale:      "en-US",
				Timezone:    "America/Los_Angeles",
			},
		},
	}

	profile := &Profile{
		ProfileId:   "pf-apply-defaults-empty-fingerprint-proxy-region",
		ProxyId:     "proxy-us",
		ProxyConfig: "http://127.0.0.1:18080",
	}

	changed := mgr.ApplyDefaults(profile)
	if !changed {
		t.Fatalf("expected empty fingerprint args to be generated from bound proxy region")
	}
	for _, want := range []string{
		"--lang=en-US",
		"--fingerprint-locale=en-US",
		"--fingerprint-accept-language=en-US,en;q=0.9",
		"--timezone=America/Los_Angeles",
		"--fingerprint-timezone=America/Los_Angeles",
	} {
		if !slices.Contains(profile.FingerprintArgs, want) {
			t.Fatalf("expected proxy-region fingerprint arg %q, got=%v", want, profile.FingerprintArgs)
		}
	}
}

func TestApplyDefaultsCompletesLegacyMinimalFingerprintArgs(t *testing.T) {
	cfg := config.DefaultConfig()
	mgr := NewManager(cfg, "")
	mgr.ProxyDAO = &proxyDAOStub{}

	profile := &Profile{
		ProfileId: "pf-apply-defaults-legacy-minimal-fingerprint",
		FingerprintArgs: []string{
			"--fingerprint-brand=Chrome",
			"--fingerprint-platform=windows",
		},
		ProxyConfig: "http://127.0.0.1:9090",
	}

	changed := mgr.ApplyDefaults(profile)
	if !changed {
		t.Fatalf("expected legacy minimal fingerprint args to be completed")
	}
	for _, want := range []string{
		"--fingerprint=",
		"--fingerprint-platform=windows",
		"--lang=",
		"--fingerprint-locale=",
		"--timezone=",
		"--fingerprint-timezone=",
		"--fingerprint-accept-language=",
		"--fingerprint-webgl-vendor=",
		"--fingerprint-webgl-renderer=",
		"--fingerprint-fonts=",
		"--fingerprint-screen-avail=",
		"--fingerprint-do-not-track=",
	} {
		if !browserTestHasPrefix(profile.FingerprintArgs, want) {
			t.Fatalf("expected completed fingerprint arg prefix %q, got=%v", want, profile.FingerprintArgs)
		}
	}
}

func TestApplyDefaultsCompletesLegacyFingerprintWithoutReplacingValidSeed(t *testing.T) {
	cfg := config.DefaultConfig()
	mgr := NewManager(cfg, "")
	mgr.ProxyDAO = &proxyDAOStub{}

	profile := &Profile{
		ProfileId: "pf-apply-defaults-legacy-seed",
		FingerprintArgs: []string{
			"--fingerprint=12345",
			"--fingerprint-brand=Chrome",
			"--fingerprint-platform=linux",
			"--lang=ja-JP",
			"--fingerprint-locale=ja-JP",
			"--timezone=Asia/Tokyo",
			"--fingerprint-timezone=Asia/Tokyo",
		},
		ProxyConfig: "http://127.0.0.1:9090",
	}

	changed := mgr.ApplyDefaults(profile)
	if !changed {
		t.Fatalf("expected legacy fingerprint args to be completed")
	}
	if !slices.Contains(profile.FingerprintArgs, "--fingerprint=12345") {
		t.Fatalf("expected valid existing seed to be preserved, got=%v", profile.FingerprintArgs)
	}
	for _, want := range []string{
		"--fingerprint-platform=linux",
		"--lang=ja-JP",
		"--fingerprint-locale=ja-JP",
		"--timezone=Asia/Tokyo",
		"--fingerprint-timezone=Asia/Tokyo",
		"--fingerprint-accept-language=ja-JP,ja;q=0.9,en;q=0.8",
		"--fingerprint-fonts=",
		"--fingerprint-webgl-renderer=",
	} {
		if !browserTestHasPrefix(profile.FingerprintArgs, want) {
			t.Fatalf("expected completed fingerprint arg prefix %q, got=%v", want, profile.FingerprintArgs)
		}
	}
}

func TestApplyDefaultsBackfillsMissingAcceptLanguage(t *testing.T) {
	cfg := config.DefaultConfig()
	mgr := NewManager(cfg, "")
	mgr.ProxyDAO = &proxyDAOStub{}

	profile := &Profile{
		ProfileId: "pf-apply-defaults-accept-language",
		FingerprintArgs: []string{
			"--fingerprint=111",
			"--fingerprint-platform=windows",
			"--lang=en-US",
			"--fingerprint-locale=en-US",
			"--timezone=America/New_York",
			"--fingerprint-timezone=America/New_York",
			"--fingerprint-webgl-vendor=Google Inc.",
			"--fingerprint-webgl-renderer=ANGLE (Intel, Intel Iris OpenGL Engine, OpenGL 4.1)",
			"--fingerprint-fonts=Arial,Helvetica",
		},
		ProxyConfig: "http://127.0.0.1:9090",
	}

	changed := mgr.ApplyDefaults(profile)
	if !changed {
		t.Fatalf("expected fingerprint defaults to be backfilled")
	}
	if !slices.Contains(profile.FingerprintArgs, "--fingerprint-accept-language=en-US,en;q=0.9") {
		t.Fatalf("expected accept-language to follow profile locale, got=%v", profile.FingerprintArgs)
	}
	for _, want := range []string{
		"--fingerprint-platform=windows",
		"--lang=en-US",
		"--fingerprint-locale=en-US",
		"--timezone=America/New_York",
		"--fingerprint-timezone=America/New_York",
		"--fingerprint-webgl-vendor=Google Inc.",
		"--fingerprint-webgl-renderer=ANGLE (Intel, Intel Iris OpenGL Engine, OpenGL 4.1)",
		"--fingerprint-fonts=Arial,Helvetica",
	} {
		if !slices.Contains(profile.FingerprintArgs, want) {
			t.Fatalf("expected explicit fingerprint arg %q to be preserved, got=%v", want, profile.FingerprintArgs)
		}
	}
}

func TestApplyDefaultsDoesNotOverrideCanonicalAcceptLanguage(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Browser.DefaultFingerprintArgs = []string{
		"--fingerprint-accept-language=zh-CN,zh;q=0.9",
		"--fingerprint-do-not-track=false",
	}
	mgr := NewManager(cfg, "")
	mgr.ProxyDAO = &proxyDAOStub{}

	profile := &Profile{
		ProfileId: "pf-apply-defaults-keep-canonical",
		FingerprintArgs: []string{
			"--fingerprint=111",
			"--fingerprint-locale=en-US",
			"--fingerprint-accept-language=en-US,en;q=0.9",
		},
		ProxyConfig: "http://127.0.0.1:9090",
	}

	_ = mgr.ApplyDefaults(profile)
	if !slices.Contains(profile.FingerprintArgs, "--fingerprint-accept-language=en-US,en;q=0.9") {
		t.Fatalf("expected canonical accept-language to be preserved, got=%v", profile.FingerprintArgs)
	}
	if slices.Contains(profile.FingerprintArgs, "--fingerprint-accept-language=zh-CN,zh;q=0.9") {
		t.Fatalf("expected default accept-language not to override canonical value, got=%v", profile.FingerprintArgs)
	}
}

func TestApplyDefaultsCanonicalizesAcceptLanguageAlias(t *testing.T) {
	cfg := config.DefaultConfig()
	mgr := NewManager(cfg, "")
	mgr.ProxyDAO = &proxyDAOStub{}

	profile := &Profile{
		ProfileId: "pf-apply-defaults-accept-language-alias",
		FingerprintArgs: []string{
			"--fingerprint=111",
			"--fingerprint-locale=ja-JP",
			"--accept-language",
			"ja-JP,ja;q=0.9",
		},
		ProxyConfig: "http://127.0.0.1:9090",
	}

	changed := mgr.ApplyDefaults(profile)
	if !changed {
		t.Fatalf("expected accept-language alias to be canonicalized")
	}
	if slices.Contains(profile.FingerprintArgs, "--accept-language") || slices.Contains(profile.FingerprintArgs, "ja-JP,ja;q=0.9") {
		t.Fatalf("expected split accept-language alias to be consumed, got=%v", profile.FingerprintArgs)
	}
	if !slices.Contains(profile.FingerprintArgs, "--fingerprint-accept-language=ja-JP,ja;q=0.9") {
		t.Fatalf("expected canonical accept-language, got=%v", profile.FingerprintArgs)
	}
}

func TestApplyDefaultsBackfillsMissingDoNotTrack(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Browser.DefaultFingerprintArgs = []string{
		"--fingerprint-accept-language=zh-CN,zh;q=0.9",
		"--fingerprint-do-not-track=true",
	}
	mgr := NewManager(cfg, "")
	mgr.ProxyDAO = &proxyDAOStub{}

	profile := &Profile{
		ProfileId: "pf-apply-defaults-dnt",
		FingerprintArgs: []string{
			"--fingerprint=111",
			"--fingerprint-locale=zh-CN",
			"--fingerprint-accept-language=zh-CN,zh;q=0.9",
		},
		ProxyConfig: "http://127.0.0.1:9090",
	}

	changed := mgr.ApplyDefaults(profile)
	if !changed {
		t.Fatalf("expected do-not-track default to be backfilled")
	}
	if !slices.Contains(profile.FingerprintArgs, "--fingerprint-do-not-track=true") {
		t.Fatalf("expected do-not-track to be backfilled, got=%v", profile.FingerprintArgs)
	}
}
