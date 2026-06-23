package fingerprint

import "testing"

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

func TestValidateArgsDetectsMismatches(t *testing.T) {
	t.Parallel()
	report := Health([]string{
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

func TestValidateArgsDetectsMalformedNumericValues(t *testing.T) {
	t.Parallel()

	report := Health([]string{
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
