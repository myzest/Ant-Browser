package backend

import (
	"ant-chrome/backend/internal/browser"
	"ant-chrome/backend/internal/config"
	"strings"
	"testing"
)

func TestBuildBrowserStartPreflightCollectsStructuredIssues(t *testing.T) {
	t.Parallel()

	input := newBrowserStartInput("profile-preflight", []string{"--remote-debugging-port=9222"}, nil, false, false, false, "", "")
	profile := &BrowserProfile{
		ProfileId:       "profile-preflight",
		FingerprintArgs: []string{"--user-agent=Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)", "--fingerprint-platform=windows", "--window-size=bad"},
		LaunchArgs:      []string{"--proxy-server", "http://127.0.0.1:9000"},
	}

	got := buildBrowserStartPreflight(input, profile)
	if got == nil {
		t.Fatal("expected preflight result")
	}
	if got.RiskLevel != browserStartPreflightRiskHigh {
		t.Fatalf("expected high risk level, got %q", got.RiskLevel)
	}
	if len(got.FormatErrors) != 1 {
		t.Fatalf("expected 1 format error, got %d", len(got.FormatErrors))
	}
	if len(got.ManagedArgWarnings) != 2 {
		t.Fatalf("expected 2 managed arg warnings, got %d", len(got.ManagedArgWarnings))
	}
	if len(got.FingerprintWarnings) != 1 {
		t.Fatalf("expected 1 fingerprint warning, got %d", len(got.FingerprintWarnings))
	}

	if got.FormatErrors[0].Kind != "format_error" || got.FormatErrors[0].Source != "profile.fingerprintArgs" {
		t.Fatalf("unexpected format error payload: %+v", got.FormatErrors[0])
	}
	if got.ManagedArgWarnings[0].Kind != "managed_launch_arg_warning" {
		t.Fatalf("unexpected managed warning payload: %+v", got.ManagedArgWarnings[0])
	}
	if got.FingerprintWarnings[0].Kind != "fingerprint_consistency_warning" {
		t.Fatalf("unexpected fingerprint warning payload: %+v", got.FingerprintWarnings[0])
	}

	if !strings.Contains(got.FingerprintWarnings[0].Message, "fingerprint-platform") {
		t.Fatalf("expected fingerprint warning message to mention platform mismatch, got %q", got.FingerprintWarnings[0].Message)
	}
}

func TestBuildBrowserStartPreflightKeepsLowRiskForCleanInput(t *testing.T) {
	t.Parallel()

	input := newBrowserStartInput("profile-clean", nil, nil, false, false, false, "", "")
	profile := &BrowserProfile{
		ProfileId:       "profile-clean",
		FingerprintArgs: []string{"--user-agent=Mozilla/5.0 (Windows NT 10.0; Win64; x64)", "--fingerprint-platform=windows", "--lang=en-US", "--window-size=1280,800"},
		LaunchArgs:      []string{"--disable-sync", "--no-first-run"},
	}

	got := buildBrowserStartPreflight(input, profile)
	if got == nil {
		t.Fatal("expected preflight result")
	}
	if got.RiskLevel != browserStartPreflightRiskLow {
		t.Fatalf("expected low risk level, got %q", got.RiskLevel)
	}
	if len(got.FormatErrors) != 0 || len(got.ManagedArgWarnings) != 0 || len(got.FingerprintWarnings) != 0 {
		t.Fatalf("expected clean preflight result, got %+v", got)
	}
}

func TestPrepareBrowserStartPlanRejectsPreflightFormatError(t *testing.T) {
	t.Parallel()

	app := NewApp(t.TempDir())
	app.config = config.DefaultConfig()
	app.browserMgr = browser.NewManager(app.config, t.TempDir())
	profile := &BrowserProfile{
		ProfileId:       "profile-blocked",
		ProfileName:     "Blocked",
		CoreId:          "",
		UserDataDir:     "blocked-user-data",
		FingerprintArgs: []string{"--window-size=oops"},
	}
	input := newBrowserStartInput(profile.ProfileId, nil, nil, false, false, false, "", "")

	plan, err := app.prepareBrowserStartPlan(input, profile)
	if err == nil {
		t.Fatalf("expected preflight error, got plan=%+v", plan)
	}
	if plan != nil {
		t.Fatalf("expected nil plan on preflight failure, got %+v", plan)
	}
	if profile.LastError == "" || !strings.Contains(profile.LastError, "启动前预检") {
		t.Fatalf("expected profile last error to mention preflight, got %q", profile.LastError)
	}
	if !strings.Contains(err.Error(), "格式错误") {
		t.Fatalf("expected error to mention format problem, got %q", err.Error())
	}
}
