package backend

import (
	"ant-chrome/backend/internal/browser"
	"ant-chrome/backend/internal/config"
	"ant-chrome/backend/internal/fingerprint"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

func TestGenerateFingerprintProfileReturnsCoherentArgs(t *testing.T) {
	t.Parallel()

	app := NewApp("")
	result := app.GenerateFingerprintProfile(FingerprintGenerateRequest{
		ProfileID:           "profile-1",
		Platform:            "mac",
		Country:             "US",
		DeviceClass:         "laptop",
		PreserveUnknownArgs: true,
		CurrentArgs:         []string{"--custom-flag=value"},
	})
	if result.Health.Status != "green" {
		t.Fatalf("expected green health, got %#v", result.Health)
	}
	for _, want := range []string{
		"--fingerprint-platform=mac",
		"--lang=en-US",
		"--fingerprint-locale=en-US",
		"--timezone=America/New_York",
		"--fingerprint-timezone=America/New_York",
		"--webrtc-ip-handling-policy=disable_non_proxied_udp",
		"--fingerprint-webrtc-ip=auto",
		"--custom-flag=value",
	} {
		if !containsString(result.Args, want) {
			t.Fatalf("generated args missing %q: %v", want, result.Args)
		}
	}
}

func TestGenerateFingerprintProfileRegeneratesSeed(t *testing.T) {
	t.Parallel()

	app := NewApp("")
	first := app.GenerateFingerprintProfile(FingerprintGenerateRequest{
		ProfileID:      "profile-regenerate",
		Platform:       "windows",
		RegenerateSeed: true,
	})
	second := app.GenerateFingerprintProfile(FingerprintGenerateRequest{
		ProfileID:      "profile-regenerate",
		Platform:       "windows",
		RegenerateSeed: true,
	})
	firstSeed := fingerprintArgValue(first.Args, "--fingerprint")
	secondSeed := fingerprintArgValue(second.Args, "--fingerprint")
	if firstSeed == "" || secondSeed == "" {
		t.Fatalf("expected regenerated seeds, got first=%v second=%v", first.Args, second.Args)
	}
	if firstSeed == secondSeed {
		t.Fatalf("regenerated seed should change, got %q", firstSeed)
	}
}

func TestGenerateFingerprintProfileUsesProfileSeedForGeneratedFields(t *testing.T) {
	t.Parallel()

	profileID := "profile-seed-coherent"
	app := NewApp("")
	result := app.GenerateFingerprintProfile(FingerprintGenerateRequest{
		ProfileID: profileID,
		Platform:  "windows",
	})
	seed := fingerprintArgValue(result.Args, "--fingerprint")
	wantSeed := strconv.FormatInt(fingerprint.StableSeed(profileID), 10)
	if seed != wantSeed {
		t.Fatalf("expected stable profile seed %q, got %q in %v", wantSeed, seed, result.Args)
	}
	generated, err := fingerprint.Generate(fingerprint.LoadLibrary(), fingerprint.GenerateOptions{
		ProfileID: profileID,
		Platform:  "windows",
		Seed:      fingerprint.StableSeed(profileID),
	})
	if err != nil {
		t.Fatal(err)
	}
	wantRenderer := "--fingerprint-webgl-renderer=" + generated.WebGL.Renderer
	if !containsString(result.Args, wantRenderer) {
		t.Fatalf("expected generated fields to use profile seed, missing %q in %v", wantRenderer, result.Args)
	}
}

func TestGenerateFingerprintProfileReplacesInvalidCurrentSeed(t *testing.T) {
	t.Parallel()

	profileID := "profile-invalid-seed"
	app := NewApp("")
	result := app.GenerateFingerprintProfile(FingerprintGenerateRequest{
		ProfileID:   profileID,
		Platform:    "windows",
		CurrentArgs: []string{"--fingerprint=0"},
	})
	seed := fingerprintArgValue(result.Args, "--fingerprint")
	wantSeed := strconv.FormatInt(fingerprint.StableSeed(profileID), 10)
	if seed != wantSeed {
		t.Fatalf("expected invalid seed to be replaced with %q, got %q in %v", wantSeed, seed, result.Args)
	}
	if result.Health.Status != "green" {
		t.Fatalf("expected generated args to be healthy, got %#v", result.Health)
	}
}

func TestGenerateFingerprintProfileSystemModeIgnoresCurrentManualRegion(t *testing.T) {
	t.Parallel()

	app := NewApp("")
	result := app.GenerateFingerprintProfile(FingerprintGenerateRequest{
		ProfileID:  "profile-system",
		Platform:   "windows",
		RegionMode: "system",
		CurrentArgs: []string{
			"--lang=en-US",
			"--fingerprint-locale=en-US",
			"--timezone=America/New_York",
			"--fingerprint-timezone=America/New_York",
		},
	})
	for _, want := range []string{
		"--lang=zh-CN",
		"--fingerprint-locale=zh-CN",
		"--timezone=Asia/Shanghai",
		"--fingerprint-timezone=Asia/Shanghai",
	} {
		if !containsString(result.Args, want) {
			t.Fatalf("system mode should ignore current manual region, missing %q: %v", want, result.Args)
		}
	}
}

func TestGenerateFingerprintProfilePreservesUnknownSplitArgs(t *testing.T) {
	t.Parallel()

	app := NewApp("")
	result := app.GenerateFingerprintProfile(FingerprintGenerateRequest{
		ProfileID:           "profile-unknown",
		Platform:            "windows",
		PreserveUnknownArgs: true,
		CurrentArgs: []string{
			"--custom-flag", "value",
			"--fingerprint-platform", "mac",
			"--another-custom=kept",
		},
	})
	if !containsString(result.Args, "--custom-flag=value") {
		t.Fatalf("expected split unknown arg to be preserved as key=value, got %v", result.Args)
	}
	if !containsString(result.Args, "--another-custom=kept") {
		t.Fatalf("expected unknown key=value arg to be preserved, got %v", result.Args)
	}
	if containsString(result.Args, "value") {
		t.Fatalf("unexpected bare split value preserved: %v", result.Args)
	}
}

func TestGenerateFingerprintProfileFollowsProxyRegion(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultConfig()
	cfg.Browser.Proxies = []config.BrowserProxy{
		{
			ProxyId:     "proxy-us",
			ProxyName:   "US",
			ProxyConfig: "http://127.0.0.1:18080",
			Country:     "US",
			Locale:      "en-US",
			Timezone:    "America/New_York",
		},
	}
	app := NewApp("")
	app.config = cfg
	app.browserMgr = browser.NewManager(cfg, "")

	result := app.GenerateFingerprintProfile(FingerprintGenerateRequest{
		ProfileID:           "profile-proxy",
		Platform:            "windows",
		RegionMode:          "proxy",
		ProxyID:             "proxy-us",
		PreserveUnknownArgs: true,
	})
	for _, want := range []string{
		"--lang=en-US",
		"--fingerprint-locale=en-US",
		"--timezone=America/New_York",
		"--fingerprint-timezone=America/New_York",
	} {
		if !containsString(result.Args, want) {
			t.Fatalf("generated proxy region args missing %q: %v", want, result.Args)
		}
	}
	if result.Health.Status != "green" {
		t.Fatalf("expected proxy region health to be green, got %#v", result.Health)
	}
}

func TestGenerateFingerprintProfileUsesProxyIPHealthCache(t *testing.T) {
	t.Parallel()

	healthJSON, err := json.Marshal(ProxyIPHealthResult{
		ProxyId:  "proxy-us",
		Ok:       true,
		IP:       "203.0.113.42",
		Country:  "US",
		Region:   "California",
		City:     "Los Angeles",
		Timezone: "America/Los_Angeles",
		Locale:   "en-US",
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig()
	cfg.Browser.Proxies = []config.BrowserProxy{
		{
			ProxyId:          "proxy-us",
			ProxyName:        "US",
			ProxyConfig:      "http://127.0.0.1:18080",
			LastIPHealthJSON: string(healthJSON),
		},
	}
	app := NewApp("")
	app.config = cfg
	app.browserMgr = browser.NewManager(cfg, "")

	result := app.GenerateFingerprintProfile(FingerprintGenerateRequest{
		ProfileID: "profile-proxy-health",
		Platform:  "windows",
		CurrentArgs: []string{
			"--lang=zh-CN",
			"--fingerprint-locale=zh-CN",
			"--timezone=Asia/Shanghai",
			"--fingerprint-timezone=Asia/Shanghai",
		},
		RegionMode: "proxy",
		ProxyID:    "proxy-us",
	})
	for _, want := range []string{
		"--lang=en-US",
		"--fingerprint-locale=en-US",
		"--timezone=America/Los_Angeles",
		"--fingerprint-timezone=America/Los_Angeles",
		"--fingerprint-webrtc-ip=203.0.113.42",
	} {
		if !containsString(result.Args, want) {
			t.Fatalf("generated proxy health args missing %q: %v", want, result.Args)
		}
	}
	if result.Health.Status != "green" {
		t.Fatalf("expected proxy health generated args to be green, got %#v", result.Health)
	}
}

func TestValidateFingerprintProfileReportsRedIssues(t *testing.T) {
	t.Parallel()

	app := NewApp("")
	report := app.ValidateFingerprintProfile(FingerprintValidateRequest{Args: []string{
		"--fingerprint=111",
		"--fingerprint-brand=Firefox",
		"--fingerprint-platform=windows",
		"--fingerprint-webgl-vendor=Apple",
		"--fingerprint-webgl-renderer=Apple M1",
	}})
	if report.Status != "red" {
		t.Fatalf("expected red report, got %#v", report)
	}
	found := false
	for _, issue := range report.Issues {
		if strings.EqualFold(issue.Code, "non_chromium_brand") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected non_chromium_brand issue, got %#v", report.Issues)
	}
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func fingerprintArgValue(items []string, want string) string {
	prefix := want + "="
	for _, item := range items {
		if strings.HasPrefix(item, prefix) {
			return strings.TrimPrefix(item, prefix)
		}
	}
	return ""
}
