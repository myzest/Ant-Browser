package browser

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func decodeCamouConfigForTest(t *testing.T, env map[string]string) map[string]any {
	t.Helper()
	if len(env) == 0 {
		t.Fatal("expected camou config env")
	}
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var raw strings.Builder
	for _, key := range keys {
		raw.WriteString(env[key])
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw.String()), &out); err != nil {
		t.Fatalf("decode camou config: %v", err)
	}
	return out
}

func TestBuildCamoufoxLaunchConfigAppliesCommonFingerprintOverrides(t *testing.T) {
	cfg, err := BuildCamoufoxLaunchConfig(
		"/tmp/camoufox",
		t.TempDir(),
		nil,
		false,
		nil,
		CamoufoxTargetOSWindows,
		42,
		[]string{
			"--timezone=Asia/Shanghai",
			"--fingerprint-color-depth=30",
			"--fingerprint-hardware-concurrency=12",
			"--fingerprint-device-memory=16",
			"--fingerprint-do-not-track=true",
			"--fingerprint-touch-points=5",
			"--fingerprint-fonts=Arial, Microsoft YaHei, Arial, PingFang SC",
		},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("BuildCamoufoxLaunchConfig failed: %v", err)
	}

	decoded := decodeCamouConfigForTest(t, cfg.CamouConfig)
	if decoded["timezone"] != "Asia/Shanghai" {
		t.Fatalf("timezone = %#v, want Asia/Shanghai", decoded["timezone"])
	}
	if decoded["screen.colorDepth"] != float64(30) || decoded["screen.pixelDepth"] != float64(30) {
		t.Fatalf("color depth not applied: color=%#v pixel=%#v", decoded["screen.colorDepth"], decoded["screen.pixelDepth"])
	}
	if decoded["navigator.hardwareConcurrency"] != float64(12) {
		t.Fatalf("hardwareConcurrency = %#v, want 12", decoded["navigator.hardwareConcurrency"])
	}
	if decoded["navigator.deviceMemory"] != float64(16) {
		t.Fatalf("deviceMemory = %#v, want 16", decoded["navigator.deviceMemory"])
	}
	if decoded["navigator.doNotTrack"] != "1" {
		t.Fatalf("doNotTrack = %#v, want 1", decoded["navigator.doNotTrack"])
	}
	if decoded["navigator.maxTouchPoints"] != float64(5) {
		t.Fatalf("maxTouchPoints = %#v, want 5", decoded["navigator.maxTouchPoints"])
	}
	fonts, ok := decoded["fonts"].([]any)
	if !ok {
		t.Fatalf("fonts type = %T, want []any", decoded["fonts"])
	}
	gotFonts := make([]string, 0, len(fonts))
	for _, item := range fonts {
		gotFonts = append(gotFonts, item.(string))
	}
	wantFonts := []string{"Arial", "Microsoft YaHei", "PingFang SC"}
	if !reflect.DeepEqual(gotFonts, wantFonts) {
		t.Fatalf("fonts = %#v, want %#v", gotFonts, wantFonts)
	}
}

func TestApplyCamoufoxFingerprintOverridesIgnoresInvalidCommonValues(t *testing.T) {
	config := map[string]any{
		"screen.colorDepth":             24,
		"navigator.hardwareConcurrency": 8,
		"navigator.doNotTrack":          "0",
		"navigator.maxTouchPoints":      0,
		"fonts":                         []string{"Arial"},
		"navigator.deviceMemory":        8,
	}

	applyCamoufoxFingerprintOverrides(config, []string{
		"--fingerprint-color-depth=bad",
		"--fingerprint-hardware-concurrency=0",
		"--fingerprint-device-memory=-1",
		"--fingerprint-do-not-track=maybe",
		"--fingerprint-touch-points=-2",
		"--fingerprint-fonts=, ,",
	})

	if config["screen.colorDepth"] != 24 {
		t.Fatalf("invalid colorDepth should be ignored: %#v", config["screen.colorDepth"])
	}
	if config["navigator.hardwareConcurrency"] != 8 {
		t.Fatalf("invalid hardwareConcurrency should be ignored: %#v", config["navigator.hardwareConcurrency"])
	}
	if config["navigator.deviceMemory"] != 8 {
		t.Fatalf("invalid deviceMemory should be ignored: %#v", config["navigator.deviceMemory"])
	}
	if config["navigator.doNotTrack"] != "0" {
		t.Fatalf("invalid doNotTrack should be ignored: %#v", config["navigator.doNotTrack"])
	}
	if config["navigator.maxTouchPoints"] != 0 {
		t.Fatalf("invalid touchPoints should be ignored: %#v", config["navigator.maxTouchPoints"])
	}
}

func TestApplyCamoufoxFingerprintOverridesAcceptsSplitValues(t *testing.T) {
	config := map[string]any{}

	applyCamoufoxFingerprintOverrides(config, []string{
		"--timezone", "Asia/Shanghai",
		"--fingerprint-color-depth", "30",
		"--fingerprint-hardware-concurrency", "12",
		"--fingerprint-device-memory", "16",
		"--fingerprint-do-not-track", "false",
		"--fingerprint-touch-points", "5",
		"--fingerprint-fonts", "Arial, Microsoft YaHei",
	})

	if config["timezone"] != "Asia/Shanghai" {
		t.Fatalf("timezone = %#v, want Asia/Shanghai", config["timezone"])
	}
	if config["screen.colorDepth"] != 30 || config["screen.pixelDepth"] != 30 {
		t.Fatalf("color depth not applied: color=%#v pixel=%#v", config["screen.colorDepth"], config["screen.pixelDepth"])
	}
	if config["navigator.hardwareConcurrency"] != 12 {
		t.Fatalf("hardwareConcurrency = %#v, want 12", config["navigator.hardwareConcurrency"])
	}
	if config["navigator.deviceMemory"] != 16 {
		t.Fatalf("deviceMemory = %#v, want 16", config["navigator.deviceMemory"])
	}
	if config["navigator.doNotTrack"] != "0" {
		t.Fatalf("doNotTrack = %#v, want 0", config["navigator.doNotTrack"])
	}
	if config["navigator.maxTouchPoints"] != 5 {
		t.Fatalf("maxTouchPoints = %#v, want 5", config["navigator.maxTouchPoints"])
	}
	if !reflect.DeepEqual(config["fonts"], []string{"Arial", "Microsoft YaHei"}) {
		t.Fatalf("fonts = %#v, want Arial/Microsoft YaHei", config["fonts"])
	}
}

func TestNormalizeCamoufoxExtraArgsSkipsManagedFingerprintArgs(t *testing.T) {
	got := normalizeCamoufoxExtraArgs([]string{
		"--lang=en-US",
		"--accept-lang=zh-CN",
		"--timezone=Asia/Shanghai",
		"--window-size=1280,800",
		"--fingerprint=123",
		"--fingerprint-color-depth=30",
		"--fingerprint-webgl-vendor=Intel",
		"--proxy-server=http://127.0.0.1:8080",
		"-profile",
		"/tmp/manual-firefox-profile",
		"--profile=/tmp/manual-firefox-profile-2",
		"https://example.com",
		"--disable-web-security",
	})
	want := []string{"--disable-web-security"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeCamoufoxExtraArgs = %#v, want %#v", got, want)
	}
}
