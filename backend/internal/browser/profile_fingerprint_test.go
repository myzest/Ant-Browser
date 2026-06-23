package browser

import (
	"ant-chrome/backend/internal/config"
	"ant-chrome/backend/internal/fingerprint"
	"strings"
	"testing"
)

func TestDefaultFingerprintArgsForProfilePreservesConfiguredSemantics(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultConfig()
	cfg.Browser.DefaultFingerprintArgs = []string{
		"--fingerprint=old-seed",
		"--fingerprint-brand=Chrome",
		"--fingerprint-platform=mac",
		"--lang=en-US",
		"--fingerprint-locale=en-US",
		"--timezone=America/New_York",
		"--fingerprint-timezone=America/New_York",
		"--webrtc-ip-handling-policy=default_public_interface_only",
		"--fingerprint-webrtc-ip=198.51.100.7",
		"--fingerprint-do-not-track=true",
	}
	mgr := NewManager(cfg, t.TempDir())

	args := mgr.defaultFingerprintArgsForProfile("profile-default", nil, nil, "")
	for _, want := range []string{
		"--fingerprint-platform=mac",
		"--lang=en-US",
		"--fingerprint-locale=en-US",
		"--timezone=America/New_York",
		"--fingerprint-timezone=America/New_York",
		"--webrtc-ip-handling-policy=default_public_interface_only",
		"--fingerprint-webrtc-ip=198.51.100.7",
		"--fingerprint-do-not-track=true",
	} {
		if !browserTestContains(args, want) {
			t.Fatalf("generated args missing %q: %v", want, args)
		}
	}
	if !browserTestContains(args, "--fingerprint="+fingerprintStableSeedForTest("profile-default")) {
		t.Fatalf("default profile args should carry profile seed, got %v", args)
	}
}

func TestDefaultFingerprintArgsForProfileUsesSourceArgsWhenCopying(t *testing.T) {
	t.Parallel()

	mgr := NewManager(config.DefaultConfig(), t.TempDir())
	srcArgs := []string{
		"--fingerprint=source-seed",
		"--fingerprint-platform=linux",
		"--lang=ja-JP",
		"--fingerprint-locale=ja-JP",
		"--timezone=Asia/Tokyo",
		"--fingerprint-timezone=Asia/Tokyo",
		"--fingerprint-webrtc-ip=auto",
	}

	args := mgr.defaultFingerprintArgsForProfile("profile-copy", nil, srcArgs, platformFromFingerprintArgs(srcArgs))
	for _, want := range []string{
		"--fingerprint-platform=linux",
		"--lang=ja-JP",
		"--fingerprint-locale=ja-JP",
		"--timezone=Asia/Tokyo",
		"--fingerprint-timezone=Asia/Tokyo",
		"--fingerprint-webrtc-ip=auto",
	} {
		if !browserTestContains(args, want) {
			t.Fatalf("copy args missing %q: %v", want, args)
		}
	}
	if browserTestContains(args, "--fingerprint=source-seed") {
		t.Fatalf("copy args should not carry source seed, got %v", args)
	}
	if !browserTestContains(args, "--fingerprint="+fingerprintStableSeedForTest("profile-copy")) {
		t.Fatalf("copy args should carry copied profile seed, got %v", args)
	}
}

func TestDefaultFingerprintArgsForProfileReplacesInvalidInputSeed(t *testing.T) {
	t.Parallel()

	mgr := NewManager(config.DefaultConfig(), t.TempDir())
	args := mgr.defaultFingerprintArgsForProfile("profile-invalid-input-seed", []string{
		"--fingerprint=0",
		"--fingerprint-platform=windows",
	}, nil, "")

	if browserTestContains(args, "--fingerprint=0") {
		t.Fatalf("invalid input seed should be replaced, got %v", args)
	}
	if !browserTestContains(args, "--fingerprint="+fingerprintStableSeedForTest("profile-invalid-input-seed")) {
		t.Fatalf("profile seed missing after invalid input replacement, got %v", args)
	}
}

func fingerprintStableSeedForTest(profileID string) string {
	return strings.TrimPrefix(fingerprint.SeedArg(profileID), "--fingerprint=")
}

func browserTestContains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func browserTestHasPrefix(items []string, prefix string) bool {
	for _, item := range items {
		if len(item) >= len(prefix) && item[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}
