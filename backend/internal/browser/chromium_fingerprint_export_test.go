package browser

import (
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestLoadChromiumFingerprintPoolSelectsStableEntry(t *testing.T) {
	t.Parallel()

	pool, err := LoadChromiumFingerprintPool()
	if err != nil {
		t.Fatalf("LoadChromiumFingerprintPool failed: %v", err)
	}
	first, err := pool.SelectChromiumFingerprint(ChromiumTargetOSWindows, 1)
	if err != nil {
		t.Fatalf("SelectChromiumFingerprint failed: %v", err)
	}
	windowsCount := len(pool.Pools[ChromiumTargetOSWindows])
	if windowsCount == 0 {
		t.Fatal("windows pool is empty")
	}
	second, err := pool.SelectChromiumFingerprint("win", 1+windowsCount)
	if err != nil {
		t.Fatalf("SelectChromiumFingerprint alias failed: %v", err)
	}
	if first.Hash != second.Hash {
		t.Fatalf("seed modulo selection unstable: first=%s second=%s", first.Hash, second.Hash)
	}

	mac, err := pool.SelectChromiumFingerprint("darwin", -1)
	if err != nil {
		t.Fatalf("SelectChromiumFingerprint mac alias failed: %v", err)
	}
	if mac.Profile.Platform != ChromiumTargetOSMac {
		t.Fatalf("mac platform = %q, want %q", mac.Profile.Platform, ChromiumTargetOSMac)
	}
}

func TestChromiumFingerprintProductionPoolIsExportable(t *testing.T) {
	t.Parallel()

	pool, err := LoadChromiumFingerprintPool()
	if err != nil {
		t.Fatalf("LoadChromiumFingerprintPool failed: %v", err)
	}
	wantCounts := map[string]int{
		ChromiumTargetOSWindows: 128,
		ChromiumTargetOSMac:     64,
		ChromiumTargetOSLinux:   64,
	}
	seenHashes := map[string]struct{}{}
	for osName, wantCount := range wantCounts {
		items := pool.Pools[osName]
		if len(items) != wantCount {
			t.Fatalf("%s pool count = %d, want %d", osName, len(items), wantCount)
		}
		for idx, entry := range items {
			if entry.OS != osName {
				t.Fatalf("%s[%d] os = %q", osName, idx, entry.OS)
			}
			if entry.Seed != idx {
				t.Fatalf("%s[%d] seed = %d, want %d", osName, idx, entry.Seed, idx)
			}
			if entry.Hash == "" {
				t.Fatalf("%s[%d] hash is empty", osName, idx)
			}
			if _, exists := seenHashes[entry.Hash]; exists {
				t.Fatalf("%s[%d] duplicate hash %s", osName, idx, entry.Hash)
			}
			seenHashes[entry.Hash] = struct{}{}

			args := ExportChromiumFingerprintArgs(entry, idx)
			if !containsString(args, "--fingerprint="+stringInt(idx)) {
				t.Fatalf("%s[%d] missing fingerprint arg in %#v", osName, idx, args)
			}
			for _, arg := range args {
				if strings.HasSuffix(arg, "=") {
					t.Fatalf("%s[%d] has empty arg %q", osName, idx, arg)
				}
				key := chromiumLaunchArgKey(arg)
				if !isChromiumFingerprintValueKey(key) {
					t.Fatalf("%s[%d] unsupported exported key %q from %q", osName, idx, key, arg)
				}
			}
		}
	}
}

func TestExportChromiumFingerprintArgs(t *testing.T) {
	t.Parallel()

	args := ExportChromiumFingerprintArgs(ChromiumPoolEntry{
		Profile: ChromiumFingerprintProfile{
			Brand:               "Chrome",
			Platform:            "windows",
			Lang:                "zh-CN",
			Timezone:            "Asia/Shanghai",
			WindowSize:          "1920,1080",
			ColorDepth:          24,
			HardwareConcurrency: 8,
			DeviceMemory:        8,
			CanvasNoise:         true,
			WebGLVendor:         "Intel",
			WebGLRenderer:       "Intel(R) UHD Graphics 630",
			AudioNoise:          true,
			Fonts:               []string{"Arial", "Microsoft YaHei", "Arial"},
			WebRTCPolicy:        "disable_non_proxied_udp",
			DoNotTrack:          false,
			TouchPoints:         0,
		},
	}, 123)

	wantContains := []string{
		"--fingerprint=123",
		"--fingerprint-brand=Chrome",
		"--fingerprint-platform=windows",
		"--lang=zh-CN",
		"--timezone=Asia/Shanghai",
		"--window-size=1920,1080",
		"--fingerprint-color-depth=24",
		"--fingerprint-hardware-concurrency=8",
		"--fingerprint-device-memory=8",
		"--fingerprint-canvas-noise=true",
		"--fingerprint-webgl-vendor=Intel",
		"--fingerprint-webgl-renderer=Intel(R) UHD Graphics 630",
		"--fingerprint-audio-noise=true",
		"--fingerprint-fonts=Arial,Microsoft YaHei,Arial",
		"--webrtc-ip-handling-policy=disable_non_proxied_udp",
		"--fingerprint-do-not-track=false",
		"--fingerprint-touch-points=0",
	}
	for _, want := range wantContains {
		if !containsString(args, want) {
			t.Fatalf("ExportChromiumFingerprintArgs missing %q in %#v", want, args)
		}
	}
}

func TestMergeChromiumFingerprintArgsLetsUserKeysWinAcrossArgSources(t *testing.T) {
	t.Parallel()

	poolArgs := []string{
		"--fingerprint=100",
		"--fingerprint-platform=windows",
		"--lang=zh-CN",
		"--window-size=1920,1080",
		"--timezone=Asia/Shanghai",
		"--fingerprint-webgl-vendor=Intel",
	}
	fingerprintArgs := []string{"--fingerprint=200", "--lang=en-US"}
	launchArgs := []string{"--window-size", "1366,768"}
	extraArgs := []string{"--fingerprint-webgl-vendor=NVIDIA"}

	got := MergeChromiumFingerprintArgs(poolArgs, fingerprintArgs, launchArgs, extraArgs)
	for _, removed := range []string{
		"--fingerprint=100",
		"--lang=zh-CN",
		"--window-size=1920,1080",
		"--fingerprint-webgl-vendor=Intel",
	} {
		if containsString(got, removed) {
			t.Fatalf("pool arg %q should yield to user args: %#v", removed, got)
		}
	}
	want := []string{"--fingerprint-platform=windows", "--timezone=Asia/Shanghai", "--fingerprint=200", "--lang=en-US"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("MergeChromiumFingerprintArgs = %#v, want %#v", got, want)
	}
}

func TestResolveChromiumFingerprintSeedAndOS(t *testing.T) {
	t.Parallel()

	args := []string{"--fingerprint=-42", "--fingerprint-platform=darwin"}
	if got := ResolveChromiumFingerprintSeed("profile-a", args); got != -42 {
		t.Fatalf("ResolveChromiumFingerprintSeed = %d, want -42", got)
	}
	if got := ResolveChromiumFingerprintOS(args, "linux"); got != ChromiumTargetOSMac {
		t.Fatalf("ResolveChromiumFingerprintOS = %q, want mac", got)
	}
	if got := ResolveChromiumFingerprintOS(nil, "linux"); got != ChromiumTargetOSLinux {
		t.Fatalf("ResolveChromiumFingerprintOS host fallback = %q, want linux", got)
	}

	splitArgs := []string{"--fingerprint", "77", "--fingerprint-platform", "linux"}
	if got := ResolveChromiumFingerprintSeed("profile-a", splitArgs); got != 77 {
		t.Fatalf("ResolveChromiumFingerprintSeed split value = %d, want 77", got)
	}
	if got := ResolveChromiumFingerprintOS(splitArgs, "darwin"); got != ChromiumTargetOSLinux {
		t.Fatalf("ResolveChromiumFingerprintOS split value = %q, want linux", got)
	}
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func containsPrefix(items []string, prefix string) bool {
	for _, item := range items {
		if strings.HasPrefix(item, prefix) {
			return true
		}
	}
	return false
}

func stringInt(value int) string {
	return strconv.Itoa(value)
}
