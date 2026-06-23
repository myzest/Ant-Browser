package backend

import (
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

func TestValidateFingerprintProfileReportsRedIssues(t *testing.T) {
	t.Parallel()

	app := NewApp("")
	report := app.ValidateFingerprintProfile(FingerprintValidateRequest{Args: []string{
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
