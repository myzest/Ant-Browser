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

func TestDefaultFingerprintArgsCanonicalizesAcceptLanguageAlias(t *testing.T) {
	t.Parallel()

	mgr := NewManager(config.DefaultConfig(), t.TempDir())
	args := mgr.defaultFingerprintArgsForProfile("profile-accept-language-alias", nil, []string{
		"--fingerprint-platform=windows",
		"--lang=en-US",
		"--fingerprint-locale=en-US",
		"--accept-language",
		"en-US,en;q=0.9",
	}, "")

	if browserTestContains(args, "--accept-language") || browserTestContains(args, "--accept-language=en-US,en;q=0.9") {
		t.Fatalf("accept-language alias should be canonicalized away, got %v", args)
	}
	if browserTestContains(args, "en-US,en;q=0.9") {
		t.Fatalf("split accept-language value should be consumed, got %v", args)
	}
	if !browserTestContains(args, "--fingerprint-accept-language=en-US,en;q=0.9") {
		t.Fatalf("canonical accept-language missing, got %v", args)
	}
}

func TestCreateProfileDefaultFingerprintFollowsBoundProxyRegion(t *testing.T) {
	mgr := newProfileProxyRegionTestManager(t)

	profile, err := mgr.Create(ProfileInput{
		ProfileName: "buyer-us",
		ProxyId:     "proxy-us",
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	for _, want := range []string{
		"--lang=en-US",
		"--fingerprint-locale=en-US",
		"--fingerprint-accept-language=en-US,en;q=0.9",
		"--timezone=America/Los_Angeles",
		"--fingerprint-timezone=America/Los_Angeles",
	} {
		if !browserTestContains(profile.FingerprintArgs, want) {
			t.Fatalf("proxy region args missing %q: %v", want, profile.FingerprintArgs)
		}
	}
}

func TestCreateProfileCustomProxyConfigDoesNotGuessFingerprintRegion(t *testing.T) {
	mgr := newProfileProxyRegionTestManager(t)

	profile, err := mgr.Create(ProfileInput{
		ProfileName: "buyer-custom",
		ProxyConfig: "http://127.0.0.1:18080",
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	for _, blocked := range []string{
		"--lang=en-US",
		"--fingerprint-locale=en-US",
		"--timezone=America/Los_Angeles",
		"--fingerprint-timezone=America/Los_Angeles",
	} {
		if browserTestContains(profile.FingerprintArgs, blocked) {
			t.Fatalf("custom proxy config should not infer proxy region, found %q in %v", blocked, profile.FingerprintArgs)
		}
	}
}

func TestUpdateProfileExplicitFingerprintArgsAreNotOverriddenByProxyRegion(t *testing.T) {
	mgr := newProfileProxyRegionTestManager(t)
	profile, err := mgr.Create(ProfileInput{
		ProfileName: "buyer",
		ProxyId:     "proxy-us",
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	explicitArgs := []string{
		"--fingerprint=12345",
		"--fingerprint-platform=linux",
		"--lang=ja-JP",
		"--fingerprint-locale=ja-JP",
		"--timezone=Asia/Tokyo",
		"--fingerprint-timezone=Asia/Tokyo",
	}
	updated, err := mgr.Update(profile.ProfileId, ProfileInput{
		ProfileName:     "buyer-updated",
		ProxyId:         "proxy-us",
		FingerprintArgs: explicitArgs,
	})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	for _, want := range explicitArgs {
		if !browserTestContains(updated.FingerprintArgs, want) {
			t.Fatalf("explicit args should be preserved, missing %q: %v", want, updated.FingerprintArgs)
		}
	}
	for _, blocked := range []string{"--lang=en-US", "--timezone=America/Los_Angeles"} {
		if browserTestContains(updated.FingerprintArgs, blocked) {
			t.Fatalf("proxy region should not override explicit args, found %q in %v", blocked, updated.FingerprintArgs)
		}
	}
}

func TestCopyProfileKeepsSourceFingerprintSemanticsButUsesNewSeed(t *testing.T) {
	mgr := NewManager(config.DefaultConfig(), t.TempDir())
	src := &Profile{
		ProfileId:   "source-profile",
		ProfileName: "source",
		FingerprintArgs: []string{
			"--fingerprint=11111",
			"--fingerprint-platform=linux",
			"--lang=ja-JP",
			"--fingerprint-locale=ja-JP",
			"--timezone=Asia/Tokyo",
			"--fingerprint-timezone=Asia/Tokyo",
		},
	}
	mgr.Profiles[src.ProfileId] = src

	copied, err := mgr.Copy(src.ProfileId, "copy")
	if err != nil {
		t.Fatalf("copy failed: %v", err)
	}

	for _, want := range []string{
		"--fingerprint-platform=linux",
		"--lang=ja-JP",
		"--fingerprint-locale=ja-JP",
		"--timezone=Asia/Tokyo",
		"--fingerprint-timezone=Asia/Tokyo",
	} {
		if !browserTestContains(copied.FingerprintArgs, want) {
			t.Fatalf("copy args missing %q: %v", want, copied.FingerprintArgs)
		}
	}
	if browserTestContains(copied.FingerprintArgs, "--fingerprint=11111") {
		t.Fatalf("copy should not carry source seed, got %v", copied.FingerprintArgs)
	}
	if !browserTestContains(copied.FingerprintArgs, "--fingerprint="+fingerprintStableSeedForTest(copied.ProfileId)) {
		t.Fatalf("copy should carry new profile seed, got %v", copied.FingerprintArgs)
	}
}

func newProfileProxyRegionTestManager(t *testing.T) *Manager {
	t.Helper()
	cfg := config.DefaultConfig()
	mgr := NewManager(cfg, t.TempDir())
	mgr.ProxyDAO = &proxyDAOStub{
		list: []Proxy{
			{
				ProxyId:     directProxyID,
				ProxyName:   "直连（不走代理）",
				ProxyConfig: "direct://",
			},
			{
				ProxyId:          "proxy-us",
				ProxyName:        "US",
				ProxyConfig:      "socks5://127.0.0.1:1080",
				Country:          "US",
				Locale:           "en-US",
				Timezone:         "America/Los_Angeles",
				LastIPHealthJSON: `{"ok":true,"country":"US","locale":"en-US","timezone":"America/New_York"}`,
			},
		},
	}
	return mgr
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
