package fingerprint

import (
	"fmt"
	"hash/fnv"
	"math/rand"
	"sort"
	"strconv"
	"strings"
)

const (
	PlatformWindows = "windows"
	PlatformMac     = "mac"
	PlatformLinux   = "linux"
)

type GenerateOptions struct {
	ProfileID           string `json:"profileId"`
	Platform            string `json:"platform"`
	RegionMode          string `json:"regionMode"`
	Country             string `json:"country"`
	Locale              string `json:"locale"`
	Timezone            string `json:"timezone"`
	DeviceClass         string `json:"deviceClass"`
	Seed                int64  `json:"seed"`
	PreserveUnknown     bool   `json:"preserveUnknownArgs"`
	RegenerateSeed      bool   `json:"regenerateSeed"`
	IncludeExplicitSeed bool   `json:"includeExplicitSeed"`
}

type Profile struct {
	ID          string
	Weight      int
	Platform    string
	Brand       string
	Locale      string
	Timezone    string
	Screen      Screen
	Hardware    Hardware
	WebGL       WebGL
	Fonts       []string
	Media       MediaDevices
	Privacy     Privacy
	DeviceClass string
}

type Screen struct {
	Width      int
	Height     int
	ColorDepth int
}

type Hardware struct {
	HardwareConcurrency int
	DeviceMemory        int
	TouchPoints         int
}

type WebGL struct {
	Vendor   string
	Renderer string
}

type MediaDevices struct {
	Cameras     int
	Microphones int
	Speakers    int
}

type Privacy struct {
	DoNotTrack   bool
	WebRTCIP     string
	WebRTCPolicy string
	CanvasNoise  bool
	AudioNoise   bool
}

type ValidationIssue struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Field    string `json:"field"`
	Message  string `json:"message"`
}

type HealthReport struct {
	Status string            `json:"status"`
	Issues []ValidationIssue `json:"issues"`
}

type Summary struct {
	ProfileID           string `json:"profileId"`
	Platform            string `json:"platform"`
	Brand               string `json:"brand"`
	Locale              string `json:"locale"`
	Timezone            string `json:"timezone"`
	Resolution          string `json:"resolution"`
	WebGLVendor         string `json:"webglVendor"`
	WebGLRenderer       string `json:"webglRenderer"`
	HardwareConcurrency int    `json:"hardwareConcurrency"`
	DeviceMemory        int    `json:"deviceMemory"`
	TouchPoints         int    `json:"touchPoints"`
}

type Library struct {
	profiles []Profile
}

func LoadLibrary() *Library {
	return &Library{profiles: defaultProfiles()}
}

func StableSeed(profileID string) int64 {
	value := strings.TrimSpace(profileID)
	if value == "" {
		value = "ant-browser"
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(value))
	seed := int64(h.Sum32())
	if seed <= 0 {
		seed = 1
	}
	return seed
}

func DefaultArgsForOS(goos string) []string {
	platform := PlatformWindows
	switch strings.ToLower(strings.TrimSpace(goos)) {
	case "darwin":
		platform = PlatformMac
	case "linux":
		platform = PlatformLinux
	}
	profile, err := Generate(LoadLibrary(), GenerateOptions{
		ProfileID:   "default-" + platform,
		Platform:    platform,
		Country:     "CN",
		Locale:      "zh-CN",
		Timezone:    "Asia/Shanghai",
		DeviceClass: "office",
	})
	if err != nil {
		return nil
	}
	return Args(profile)
}

func Generate(lib *Library, opts GenerateOptions) (*Profile, error) {
	if lib == nil {
		lib = LoadLibrary()
	}
	platform := NormalizePlatform(opts.Platform)
	locale, timezone := localeTimezone(opts.Country, opts.Locale, opts.Timezone)
	deviceClass := normalizeDeviceClass(opts.DeviceClass)
	seed := opts.Seed
	if seed <= 0 {
		seed = StableSeed(opts.ProfileID + "|" + platform + "|" + locale + "|" + timezone + "|" + deviceClass)
	}

	candidates := make([]Profile, 0, len(lib.profiles))
	for _, profile := range lib.profiles {
		if profile.Platform != platform {
			continue
		}
		if deviceClass != "" && profile.DeviceClass != "" && profile.DeviceClass != deviceClass {
			continue
		}
		candidates = append(candidates, profile)
	}
	if len(candidates) == 0 {
		for _, profile := range lib.profiles {
			if profile.Platform == platform {
				candidates = append(candidates, profile)
			}
		}
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no fingerprint profiles for platform %q", platform)
	}

	picked := weightedPick(candidates, seed)
	picked.Locale = locale
	picked.Timezone = timezone
	if picked.Privacy.WebRTCPolicy == "" {
		picked.Privacy.WebRTCPolicy = "disable_non_proxied_udp"
	}
	if picked.Privacy.WebRTCIP == "" {
		picked.Privacy.WebRTCIP = "auto"
	}
	return &picked, nil
}

func Args(profile *Profile) []string {
	if profile == nil {
		return nil
	}
	args := []string{
		"--fingerprint-brand=" + defaultString(profile.Brand, "Chrome"),
		"--fingerprint-platform=" + NormalizePlatform(profile.Platform),
		"--lang=" + defaultString(profile.Locale, "zh-CN"),
		"--fingerprint-locale=" + defaultString(profile.Locale, "zh-CN"),
		"--timezone=" + defaultString(profile.Timezone, "Asia/Shanghai"),
		"--fingerprint-timezone=" + defaultString(profile.Timezone, "Asia/Shanghai"),
		"--window-size=" + strconv.Itoa(profile.Screen.Width) + "," + strconv.Itoa(profile.Screen.Height),
		"--fingerprint-color-depth=" + strconv.Itoa(defaultInt(profile.Screen.ColorDepth, 24)),
		"--fingerprint-hardware-concurrency=" + strconv.Itoa(defaultInt(profile.Hardware.HardwareConcurrency, 8)),
		"--fingerprint-device-memory=" + strconv.Itoa(defaultInt(profile.Hardware.DeviceMemory, 8)),
		"--fingerprint-canvas-noise=" + strconv.FormatBool(profile.Privacy.CanvasNoise),
		"--fingerprint-audio-noise=" + strconv.FormatBool(profile.Privacy.AudioNoise),
		"--fingerprint-webgl-vendor=" + profile.WebGL.Vendor,
		"--fingerprint-webgl-renderer=" + profile.WebGL.Renderer,
		"--fingerprint-fonts=" + strings.Join(profile.Fonts, ","),
		"--webrtc-ip-handling-policy=" + defaultString(profile.Privacy.WebRTCPolicy, "disable_non_proxied_udp"),
		"--fingerprint-webrtc-ip=" + defaultString(profile.Privacy.WebRTCIP, "auto"),
		"--fingerprint-do-not-track=" + strconv.FormatBool(profile.Privacy.DoNotTrack),
		"--fingerprint-touch-points=" + strconv.Itoa(profile.Hardware.TouchPoints),
		"--fingerprint-media-devices=" + strconv.Itoa(profile.Media.Cameras) + "," + strconv.Itoa(profile.Media.Microphones) + "," + strconv.Itoa(profile.Media.Speakers),
	}
	return args
}

func SummaryFor(profile *Profile) Summary {
	if profile == nil {
		return Summary{}
	}
	return Summary{
		ProfileID:           profile.ID,
		Platform:            NormalizePlatform(profile.Platform),
		Brand:               defaultString(profile.Brand, "Chrome"),
		Locale:              profile.Locale,
		Timezone:            profile.Timezone,
		Resolution:          strconv.Itoa(profile.Screen.Width) + "," + strconv.Itoa(profile.Screen.Height),
		WebGLVendor:         profile.WebGL.Vendor,
		WebGLRenderer:       profile.WebGL.Renderer,
		HardwareConcurrency: profile.Hardware.HardwareConcurrency,
		DeviceMemory:        profile.Hardware.DeviceMemory,
		TouchPoints:         profile.Hardware.TouchPoints,
	}
}

func NormalizePlatform(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "mac", "macos", "darwin":
		return PlatformMac
	case "linux":
		return PlatformLinux
	case "windows", "win", "win32", "":
		return PlatformWindows
	default:
		return PlatformWindows
	}
}

func ParseArgs(args []string) map[string]string {
	out := map[string]string{}
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == "" {
			continue
		}
		if key, value, ok := strings.Cut(arg, "="); ok {
			out[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
			continue
		}
		if strings.HasPrefix(arg, "--") && i+1 < len(args) {
			next := strings.TrimSpace(args[i+1])
			if next != "" && !strings.HasPrefix(next, "-") {
				out[strings.ToLower(arg)] = next
				i++
			}
		}
	}
	return out
}

func CanonicalArgs(args []string) []string {
	values := ParseArgs(args)
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key+"="+values[key])
	}
	return out
}

func weightedPick(candidates []Profile, seed int64) Profile {
	total := 0
	for _, candidate := range candidates {
		if candidate.Weight > 0 {
			total += candidate.Weight
		}
	}
	if total <= 0 {
		return candidates[int(seed)%len(candidates)]
	}
	r := rand.New(rand.NewSource(seed))
	n := r.Intn(total)
	for _, candidate := range candidates {
		if candidate.Weight <= 0 {
			continue
		}
		if n < candidate.Weight {
			return candidate
		}
		n -= candidate.Weight
	}
	return candidates[len(candidates)-1]
}

func localeTimezone(country string, locale string, timezone string) (string, string) {
	locale = strings.TrimSpace(locale)
	timezone = strings.TrimSpace(timezone)
	if locale != "" && timezone != "" {
		return locale, timezone
	}
	switch strings.ToUpper(strings.TrimSpace(country)) {
	case "US", "USA":
		if locale == "" {
			locale = "en-US"
		}
		if timezone == "" {
			timezone = "America/New_York"
		}
	case "GB", "UK":
		if locale == "" {
			locale = "en-GB"
		}
		if timezone == "" {
			timezone = "Europe/London"
		}
	case "DE":
		if locale == "" {
			locale = "de-DE"
		}
		if timezone == "" {
			timezone = "Europe/Berlin"
		}
	case "FR":
		if locale == "" {
			locale = "fr-FR"
		}
		if timezone == "" {
			timezone = "Europe/Paris"
		}
	case "JP":
		if locale == "" {
			locale = "ja-JP"
		}
		if timezone == "" {
			timezone = "Asia/Tokyo"
		}
	case "KR":
		if locale == "" {
			locale = "ko-KR"
		}
		if timezone == "" {
			timezone = "Asia/Seoul"
		}
	case "SG":
		if locale == "" {
			locale = "en-SG"
		}
		if timezone == "" {
			timezone = "Asia/Singapore"
		}
	case "BR":
		if locale == "" {
			locale = "pt-BR"
		}
		if timezone == "" {
			timezone = "America/Sao_Paulo"
		}
	case "IN":
		if locale == "" {
			locale = "en-IN"
		}
		if timezone == "" {
			timezone = "Asia/Kolkata"
		}
	default:
		if locale == "" {
			locale = "zh-CN"
		}
		if timezone == "" {
			timezone = "Asia/Shanghai"
		}
	}
	return locale, timezone
}

func normalizeDeviceClass(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "office", "laptop", "workstation", "gaming", "light_linux":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func defaultInt(value int, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}

func defaultProfiles() []Profile {
	return []Profile{
		profile("win_chrome_office_intel_1920", 120, PlatformWindows, "office", "1920,1080", 8, 8, WebGL{"Intel", "Intel(R) UHD Graphics 630"}, []string{"Arial", "Calibri", "Cambria Math", "Consolas", "Segoe UI", "Tahoma", "Times New Roman", "Verdana", "Microsoft YaHei"}, MediaDevices{1, 1, 1}),
		profile("win_chrome_laptop_intel_1366", 90, PlatformWindows, "laptop", "1366,768", 4, 4, WebGL{"Intel", "Intel(R) UHD Graphics 620"}, []string{"Arial", "Calibri", "Consolas", "Segoe UI", "Tahoma", "Times New Roman", "Verdana"}, MediaDevices{1, 1, 1}),
		profile("win_chrome_gaming_nvidia_2560", 40, PlatformWindows, "gaming", "2560,1440", 16, 16, WebGL{"NVIDIA", "NVIDIA GeForce RTX 3060"}, []string{"Arial", "Calibri", "Cambria Math", "Consolas", "Segoe UI", "Tahoma", "Times New Roman", "Verdana"}, MediaDevices{1, 2, 2}),
		profile("mac_chrome_laptop_apple_1440", 110, PlatformMac, "laptop", "1440,900", 8, 8, WebGL{"Apple", "Apple M1"}, []string{"Arial", "Helvetica", "Helvetica Neue", "Menlo", "Monaco", "PingFang SC", "Apple Color Emoji", "Times New Roman"}, MediaDevices{1, 1, 1}),
		profile("mac_chrome_workstation_apple_2560", 55, PlatformMac, "workstation", "2560,1440", 10, 16, WebGL{"Apple", "Apple M2"}, []string{"Arial", "Helvetica", "Helvetica Neue", "Menlo", "Monaco", "PingFang SC", "Apple Color Emoji", "Times New Roman"}, MediaDevices{1, 2, 2}),
		profile("linux_chrome_office_mesa_1920", 80, PlatformLinux, "office", "1920,1080", 8, 8, WebGL{"Intel", "Mesa Intel(R) UHD Graphics 620"}, []string{"Arimo", "Cousine", "Tinos", "Noto Sans", "Noto Sans CJK SC", "Noto Color Emoji"}, MediaDevices{0, 1, 1}),
		profile("linux_chrome_light_mesa_1366", 55, PlatformLinux, "light_linux", "1366,768", 4, 4, WebGL{"Intel", "Mesa Intel(R) HD Graphics 520"}, []string{"Arimo", "Cousine", "Tinos", "Noto Sans", "Noto Color Emoji"}, MediaDevices{0, 1, 1}),
	}
}

func profile(id string, weight int, platform string, deviceClass string, resolution string, concurrency int, memory int, webgl WebGL, fonts []string, media MediaDevices) Profile {
	width, height := parseResolution(resolution)
	return Profile{
		ID:          id,
		Weight:      weight,
		Platform:    platform,
		Brand:       "Chrome",
		Locale:      "zh-CN",
		Timezone:    "Asia/Shanghai",
		Screen:      Screen{Width: width, Height: height, ColorDepth: 24},
		Hardware:    Hardware{HardwareConcurrency: concurrency, DeviceMemory: memory, TouchPoints: 0},
		WebGL:       webgl,
		Fonts:       fonts,
		Media:       media,
		DeviceClass: deviceClass,
		Privacy: Privacy{
			WebRTCPolicy: "disable_non_proxied_udp",
			WebRTCIP:     "auto",
			CanvasNoise:  true,
			AudioNoise:   true,
		},
	}
}

func parseResolution(value string) (int, int) {
	left, right, ok := strings.Cut(strings.TrimSpace(value), ",")
	if !ok {
		return 1920, 1080
	}
	width, err := strconv.Atoi(strings.TrimSpace(left))
	if err != nil || width <= 0 {
		width = 1920
	}
	height, err := strconv.Atoi(strings.TrimSpace(right))
	if err != nil || height <= 0 {
		height = 1080
	}
	return width, height
}
