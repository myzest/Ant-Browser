package fingerprint

import (
	"strings"
	"testing"
)

func TestDefaultArgsForOS(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"windows": "--fingerprint-platform=windows",
		"darwin":  "--fingerprint-platform=mac",
		"linux":   "--fingerprint-platform=linux",
		"freebsd": "--fingerprint-platform=windows",
	}
	for goos, wantPlatform := range tests {
		got := DefaultArgsForOS(goos)
		if !contains(got, wantPlatform) {
			t.Fatalf("%s: missing platform %q in %v", goos, wantPlatform, got)
		}
		for _, required := range []string{
			"--fingerprint-brand=Chrome",
			"--webrtc-ip-handling-policy=disable_non_proxied_udp",
			"--fingerprint-webrtc-ip=auto",
			"--fingerprint-canvas-noise=true",
			"--fingerprint-audio-noise=true",
			"--fingerprint-touch-points=0",
		} {
			if !contains(got, required) {
				t.Fatalf("%s: missing %q in %v", goos, required, got)
			}
		}
		if !hasArgPrefix(got, "--fingerprint-screen-avail=") {
			t.Fatalf("%s: missing screen avail in %v", goos, got)
		}
		if !hasArgPrefix(got, "--fingerprint-device-pixel-ratio=") {
			t.Fatalf("%s: missing device pixel ratio in %v", goos, got)
		}
	}
}

func TestLoadLibraryFromData(t *testing.T) {
	t.Parallel()
	lib, err := LoadLibraryFromData()
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.profiles) < 7 {
		t.Fatalf("expected embedded profiles, got %d", len(lib.profiles))
	}
	if len(lib.regions) < 10 {
		t.Fatalf("expected embedded regions, got %d", len(lib.regions))
	}
	if len(lib.fonts) < 3 || len(lib.webgl) < 3 || len(lib.media) < 4 || len(lib.screen.Desktop) == 0 {
		t.Fatalf("expected embedded distribution data, got fonts=%d webgl=%d media=%d screen=%#v", len(lib.fonts), len(lib.webgl), len(lib.media), lib.screen)
	}
}

func TestGenerateIsStableForSameOptions(t *testing.T) {
	t.Parallel()
	lib := LoadLibrary()
	first, err := Generate(lib, GenerateOptions{ProfileID: "profile-a", Platform: "windows", DeviceClass: "office"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Generate(lib, GenerateOptions{ProfileID: "profile-a", Platform: "windows", DeviceClass: "office"})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("same options should generate same profile: %q != %q", first.ID, second.ID)
	}
}

func TestStableSeedUsesPositiveChromiumSeedRange(t *testing.T) {
	t.Parallel()

	for _, profileID := range []string{
		"",
		"profile-default",
		"profile-invalid-seed",
		"profile-invalid-input-seed",
		"very-long-profile-id-that-should-still-stay-inside-the-supported-seed-range",
	} {
		seed := StableSeed(profileID)
		if seed <= 0 || seed > MaxSeed {
			t.Fatalf("StableSeed(%q) = %d, want 1..%d", profileID, seed, MaxSeed)
		}
	}
}

func TestGenerateUsesDistributionData(t *testing.T) {
	t.Parallel()

	lib := &Library{
		profiles: []Profile{
			profile("custom_win", 1, PlatformWindows, "office", "800,600", 2, 2, WebGL{"Old", "Old GPU"}, []string{"Old Font"}, MediaDevices{9, 9, 9}),
		},
		regions: defaultRegions(),
		fonts: map[string]FontPool{
			PlatformWindows: {MarkerFonts: []string{"Segoe UI", "Calibri"}, OptionalFonts: []string{"Arial", "Verdana"}},
		},
		webgl: map[string][]WebGL{
			PlatformWindows: {{Vendor: "Intel", Renderer: "Intel(R) UHD Graphics 630"}},
		},
		screen: ScreenDistribution{
			Desktop:          []string{"1920,1080"},
			ColorDepth:       []int{24},
			DevicePixelRatio: []float64{1},
		},
		media: map[string][]MediaDevices{
			"office": {{Cameras: 1, Microphones: 1, Speakers: 1}},
		},
	}
	got, err := Generate(lib, GenerateOptions{ProfileID: "profile-distribution", Platform: "windows", DeviceClass: "office"})
	if err != nil {
		t.Fatal(err)
	}
	if got.WebGL.Renderer != "Intel(R) UHD Graphics 630" {
		t.Fatalf("expected generated WebGL from distribution, got %#v", got.WebGL)
	}
	if got.Screen.Width != 1920 || got.Screen.Height != 1080 {
		t.Fatalf("expected generated screen from distribution, got %#v", got.Screen)
	}
	if got.Media != (MediaDevices{Cameras: 1, Microphones: 1, Speakers: 1}) {
		t.Fatalf("expected generated media devices from distribution, got %#v", got.Media)
	}
	if !contains(got.Fonts, "Segoe UI") || !contains(got.Fonts, "Calibri") {
		t.Fatalf("expected marker fonts from distribution, got %#v", got.Fonts)
	}
}

func TestValidateArgsDetectsMismatches(t *testing.T) {
	t.Parallel()
	report := Health([]string{
		"--fingerprint=111",
		"--fingerprint-brand=Firefox",
		"--fingerprint-platform=windows",
		"--fingerprint-fonts=Arial,PingFang SC",
		"--fingerprint-webgl-vendor=Apple",
		"--fingerprint-webgl-renderer=Apple M1",
		"--fingerprint-webrtc-ip=not-an-ip",
	})
	if report.Status != "red" {
		t.Fatalf("expected red health report, got %#v", report)
	}
	if len(report.Issues) == 0 {
		t.Fatalf("expected validation issues")
	}
}

func TestValidateArgsWarnsMissingPlatform(t *testing.T) {
	t.Parallel()

	report := Health([]string{
		"--fingerprint=111",
		"--fingerprint-brand=Chrome",
		"--fingerprint-fonts=Arial,Calibri",
		"--fingerprint-webgl-vendor=Intel",
		"--fingerprint-webgl-renderer=Intel(R) UHD Graphics 630",
	})
	if report.Status != "yellow" {
		t.Fatalf("expected yellow health report for missing platform, got %#v", report)
	}
	if !hasIssue(report, "missing_platform") {
		t.Fatalf("expected missing_platform issue, got %#v", report.Issues)
	}
}

func TestValidateArgsChecksSeed(t *testing.T) {
	t.Parallel()

	missing := Health([]string{
		"--fingerprint-brand=Chrome",
		"--fingerprint-platform=windows",
		"--fingerprint-fonts=Arial,Calibri",
		"--fingerprint-webgl-vendor=Intel",
		"--fingerprint-webgl-renderer=Intel(R) UHD Graphics 630",
	})
	if missing.Status != "yellow" || !hasIssue(missing, "missing_seed") {
		t.Fatalf("expected missing seed warning, got %#v", missing)
	}

	invalid := Health([]string{
		"--fingerprint=abc",
		"--fingerprint-brand=Chrome",
		"--fingerprint-platform=windows",
		"--fingerprint-fonts=Arial,Calibri",
		"--fingerprint-webgl-vendor=Intel",
		"--fingerprint-webgl-renderer=Intel(R) UHD Graphics 630",
	})
	if invalid.Status != "red" || !hasIssue(invalid, "invalid_seed") {
		t.Fatalf("expected invalid seed red issue, got %#v", invalid)
	}
}

func TestValidateArgsWarnsRiskyLaunchArgs(t *testing.T) {
	t.Parallel()

	report := Health([]string{
		"--fingerprint=111",
		"--fingerprint-brand=Chrome",
		"--fingerprint-platform=windows",
		"--fingerprint-fonts=Arial,Calibri",
		"--fingerprint-webgl-vendor=Intel",
		"--fingerprint-webgl-renderer=Intel(R) UHD Graphics 630",
		"--headless=new",
		"--remote-debugging-port",
		"9222",
		"--disable-blink-features=AutomationControlled",
		"--use-gl=swiftshader",
	})
	if report.Status != "yellow" {
		t.Fatalf("expected yellow health report for risky launch args, got %#v", report)
	}
	for _, code := range []string{
		"risky_headless",
		"risky_remote_debugging",
		"risky_automation_controlled_override",
		"risky_swiftshader",
	} {
		if !hasIssue(report, code) {
			t.Fatalf("expected issue %q, got %#v", code, report.Issues)
		}
	}
}

func TestValidateArgsRejectsFirefoxUserAgent(t *testing.T) {
	t.Parallel()

	report := Health([]string{
		"--fingerprint=111",
		"--fingerprint-brand=Chrome",
		"--fingerprint-platform=windows",
		"--user-agent=Mozilla/5.0 Firefox/151.0",
		"--fingerprint-fonts=Arial,Calibri",
		"--fingerprint-webgl-vendor=Intel",
		"--fingerprint-webgl-renderer=Intel(R) UHD Graphics 630",
	})
	if report.Status != "red" || !hasIssue(report, "non_chromium_user_agent") {
		t.Fatalf("expected Firefox UA red issue, got %#v", report)
	}
}

func TestValidateArgsDetectsLocaleTimezoneRegionMismatch(t *testing.T) {
	t.Parallel()

	report := Health([]string{
		"--fingerprint=111",
		"--fingerprint-brand=Chrome",
		"--fingerprint-platform=windows",
		"--lang=en-US",
		"--fingerprint-locale=en-US",
		"--timezone=Asia/Shanghai",
		"--fingerprint-timezone=Asia/Shanghai",
		"--fingerprint-fonts=Arial,Calibri",
		"--fingerprint-webgl-vendor=Intel",
		"--fingerprint-webgl-renderer=Intel(R) UHD Graphics 630",
	})
	if report.Status != "yellow" || !hasIssue(report, "locale_timezone_region_mismatch") {
		t.Fatalf("expected locale/timezone mismatch warning, got %#v", report)
	}
}

func TestValidateArgsDetectsMalformedNumericValues(t *testing.T) {
	t.Parallel()

	report := Health([]string{
		"--fingerprint=111",
		"--fingerprint-brand=Chrome",
		"--fingerprint-platform=windows",
		"--window-size=bad",
		"--fingerprint-color-depth=deep",
		"--fingerprint-hardware-concurrency=many",
		"--fingerprint-device-memory=huge",
		"--fingerprint-touch-points=abc",
		"--fingerprint-media-devices=1,two,3",
	})
	if report.Status != "red" {
		t.Fatalf("expected red health report for malformed numeric values, got %#v", report)
	}
	for _, code := range []string{
		"invalid_window_size",
		"invalid_color_depth",
		"invalid_hardware_concurrency",
		"invalid_device_memory",
		"invalid_touch_points",
		"invalid_media_devices",
	} {
		if !hasIssue(report, code) {
			t.Fatalf("expected issue %q, got %#v", code, report.Issues)
		}
	}
}

func TestValidateArgsDetectsScreenPhysics(t *testing.T) {
	t.Parallel()

	report := Health([]string{
		"--fingerprint=111",
		"--fingerprint-brand=Chrome",
		"--fingerprint-platform=windows",
		"--window-size=1280,720",
		"--fingerprint-screen-avail=1600,900",
		"--fingerprint-device-pixel-ratio=bad",
		"--fingerprint-fonts=Arial,Calibri",
		"--fingerprint-webgl-vendor=Intel",
		"--fingerprint-webgl-renderer=Intel(R) UHD Graphics 630",
	})
	if report.Status != "red" {
		t.Fatalf("expected red health report for screen physics, got %#v", report)
	}
	for _, code := range []string{"screen_avail_exceeds_size", "invalid_device_pixel_ratio"} {
		if !hasIssue(report, code) {
			t.Fatalf("expected issue %q, got %#v", code, report.Issues)
		}
	}
}

func TestValidateArgsWithProxyRegionWarnsMismatch(t *testing.T) {
	t.Parallel()

	report := HealthWithContext([]string{
		"--fingerprint=111",
		"--fingerprint-brand=Chrome",
		"--fingerprint-platform=windows",
		"--lang=zh-CN",
		"--fingerprint-locale=zh-CN",
		"--timezone=Asia/Shanghai",
		"--fingerprint-timezone=Asia/Shanghai",
		"--fingerprint-fonts=Arial,Calibri",
		"--fingerprint-webgl-vendor=Intel",
		"--fingerprint-webgl-renderer=Intel(R) UHD Graphics 630",
	}, ValidationContext{ProxyCountry: "US"})
	if report.Status != "yellow" {
		t.Fatalf("expected yellow health report for proxy region mismatch, got %#v", report)
	}
	if !hasIssue(report, "proxy_locale_mismatch") || !hasIssue(report, "proxy_timezone_mismatch") {
		t.Fatalf("expected proxy locale/timezone mismatch, got %#v", report.Issues)
	}
}

func hasIssue(report HealthReport, code string) bool {
	for _, issue := range report.Issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func hasArgPrefix(items []string, prefix string) bool {
	for _, item := range items {
		if strings.HasPrefix(item, prefix) {
			return true
		}
	}
	return false
}
