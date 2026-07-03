package fingerprint

import (
	"embed"
	"encoding/json"
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

	RegionModeManual = "manual"
	RegionModeProxy  = "proxy"
	RegionModeSystem = "system"

	MaxSeed = int64(2147483647)
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
	ID             string
	Weight         int
	Platform       string
	Brand          string
	Country        string
	Locale         string
	Timezone       string
	AcceptLanguage string
	Languages      []string
	Screen         Screen
	Hardware       Hardware
	WebGL          WebGL
	Fonts          []string
	Media          MediaDevices
	Privacy        Privacy
	ClientHints    ClientHints
	MediaCaps      MediaCapabilityProfile
	DeviceClass    string
}

type Screen struct {
	Width             int
	Height            int
	AvailWidth        int
	AvailHeight       int
	ColorDepth        int
	DevicePixelRatio  float64
	WindowChromeWidth int
	WindowChromeTop   int
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

type DeviceTemplate struct {
	ID          string
	Weight      int
	Platform    string
	DeviceClass string
	Screen      Screen
	Hardware    Hardware
	WebGL       WebGL
	Fonts       []string
	Media       MediaDevices
}

type Privacy struct {
	DoNotTrack   bool
	WebRTCIP     string
	WebRTCPolicy string
	CanvasNoise  bool
	AudioNoise   bool
}

// ClientHints is data-only for now. Ant-Browser does not emit UA-CH launch
// flags until the active Chromium core exposes a verified native capability.
type ClientHints struct {
	RuntimeCoverage string            `json:"runtimeCoverage"`
	Brands          []ClientHintBrand `json:"brands"`
	FullVersionList []ClientHintBrand `json:"fullVersionList"`
	Platform        string            `json:"platform"`
	PlatformVersion string            `json:"platformVersion"`
	Architecture    string            `json:"architecture"`
	Bitness         string            `json:"bitness"`
	Mobile          bool              `json:"mobile"`
	Model           string            `json:"model"`
	Notes           []string          `json:"notes,omitempty"`
}

type ClientHintBrand struct {
	Brand   string `json:"brand"`
	Version string `json:"version"`
}

type MediaCapabilityProfile struct {
	RuntimeCoverage string             `json:"runtimeCoverage"`
	ChromeMajor     string             `json:"chromeMajor"`
	PDFViewer       bool               `json:"pdfViewer"`
	WidevinePresent bool               `json:"widevinePresent"`
	EMESupported    bool               `json:"emeSupported"`
	Plugins         []PluginCapability `json:"plugins"`
	MimeTypes       []MimeCapability   `json:"mimeTypes"`
	Codecs          CodecCapabilities  `json:"codecs"`
	Notes           []string           `json:"notes,omitempty"`
}

type PluginCapability struct {
	Name        string `json:"name"`
	Filename    string `json:"filename"`
	Description string `json:"description"`
}

type MimeCapability struct {
	Type        string `json:"type"`
	Suffixes    string `json:"suffixes"`
	Description string `json:"description"`
}

type CodecCapabilities struct {
	H264 bool `json:"h264"`
	AAC  bool `json:"aac"`
	VP9  bool `json:"vp9"`
	AV1  bool `json:"av1"`
	HEVC bool `json:"hevc"`
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
	ProfileID           string   `json:"profileId"`
	Platform            string   `json:"platform"`
	Brand               string   `json:"brand"`
	Locale              string   `json:"locale"`
	Timezone            string   `json:"timezone"`
	AcceptLanguage      string   `json:"acceptLanguage,omitempty"`
	Languages           []string `json:"languages,omitempty"`
	Resolution          string   `json:"resolution"`
	WebGLVendor         string   `json:"webglVendor"`
	WebGLRenderer       string   `json:"webglRenderer"`
	HardwareConcurrency int      `json:"hardwareConcurrency"`
	DeviceMemory        int      `json:"deviceMemory"`
	TouchPoints         int      `json:"touchPoints"`
	ClientHintsCoverage string   `json:"clientHintsCoverage,omitempty"`
	RegionMode          string   `json:"regionMode,omitempty"`
	Country             string   `json:"country,omitempty"`
}

type Library struct {
	profiles        []Profile
	regions         map[string]Region
	fonts           map[string]FontPool
	webgl           map[string][]WebGL
	screen          ScreenDistribution
	media           map[string][]MediaDevices
	clientHints     map[string]ClientHints
	mediaCaps       map[string]MediaCapabilityProfile
	deviceTemplates []DeviceTemplate
}

type Region struct {
	Country        string   `json:"country"`
	Name           string   `json:"name"`
	Locales        []string `json:"locales"`
	Timezones      []string `json:"timezones"`
	AcceptLanguage string   `json:"acceptLanguage"`
	Languages      []string `json:"languages"`
}

type FontPool struct {
	MarkerFonts   []string `json:"markerFonts"`
	OptionalFonts []string `json:"optionalFonts"`
}

type ScreenDistribution struct {
	Desktop          []string  `json:"desktop"`
	Laptop           []string  `json:"laptop"`
	ColorDepth       []int     `json:"colorDepth"`
	DevicePixelRatio []float64 `json:"devicePixelRatio"`
}

//go:embed data/*.json
var fingerprintDataFS embed.FS

func LoadLibrary() *Library {
	lib, err := LoadLibraryFromData()
	if err != nil {
		return defaultLibrary()
	}
	return lib
}

func LoadLibraryFromData() (*Library, error) {
	profiles, err := loadProfilesFromData()
	if err != nil {
		return nil, err
	}
	regions, err := loadRegionsFromData()
	if err != nil {
		return nil, err
	}
	fonts, err := loadFontsFromData()
	if err != nil {
		return nil, err
	}
	webgl, err := loadWebGLFromData()
	if err != nil {
		return nil, err
	}
	screen, err := loadScreenFromData()
	if err != nil {
		return nil, err
	}
	media, err := loadMediaFromData()
	if err != nil {
		return nil, err
	}
	clientHints, err := loadClientHintsFromData()
	if err != nil {
		return nil, err
	}
	mediaCaps, err := loadMediaCapabilitiesFromData()
	if err != nil {
		return nil, err
	}
	return &Library{profiles: profiles, regions: regions, fonts: fonts, webgl: webgl, screen: screen, media: media, clientHints: clientHints, mediaCaps: mediaCaps, deviceTemplates: stableDeviceTemplates()}, nil
}

func StableSeed(profileID string) int64 {
	value := strings.TrimSpace(profileID)
	if value == "" {
		value = "ant-browser"
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(value))
	seed := int64(h.Sum32()%uint32(MaxSeed)) + 1
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
	if len(lib.regions) == 0 {
		lib.regions = defaultRegions()
	}
	if len(lib.fonts) == 0 {
		lib.fonts = defaultFontPools()
	}
	if len(lib.webgl) == 0 {
		lib.webgl = defaultWebGLPools()
	}
	if len(lib.screen.Desktop) == 0 && len(lib.screen.Laptop) == 0 {
		lib.screen = defaultScreenDistribution()
	}
	if len(lib.media) == 0 {
		lib.media = defaultMediaPools()
	}
	if len(lib.clientHints) == 0 {
		lib.clientHints = defaultClientHints()
	}
	if len(lib.mediaCaps) == 0 {
		lib.mediaCaps = defaultMediaCapabilities()
	}
	if lib.deviceTemplates == nil && isDefaultFingerprintDistribution(lib) {
		lib.deviceTemplates = stableDeviceTemplates()
	}
	platform := NormalizePlatform(opts.Platform)
	regionMode := NormalizeRegionMode(opts.RegionMode)
	locale, timezone, country := lib.localeTimezone(opts.Country, opts.Locale, opts.Timezone)
	if regionMode == RegionModeProxy {
		locale, timezone, country = lib.proxyRegionDefaults(opts.Country, opts.Locale, opts.Timezone)
	}
	if regionMode == RegionModeSystem && strings.TrimSpace(opts.Locale) == "" && strings.TrimSpace(opts.Timezone) == "" {
		locale, timezone = "zh-CN", "Asia/Shanghai"
		country = "CN"
	}
	deviceClass := normalizeDeviceClass(opts.DeviceClass)
	seed := opts.Seed
	if seed <= 0 {
		seed = StableSeed(opts.ProfileID + "|" + platform + "|" + locale + "|" + timezone + "|" + deviceClass + "|" + regionMode)
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
	picked.Country = country
	picked.Locale = locale
	picked.Timezone = timezone
	if deviceClass != "" {
		picked.DeviceClass = deviceClass
	}
	picked.AcceptLanguage, picked.Languages = lib.languageStack(country, locale)
	picked.ClientHints = pickClientHints(platform, picked.ClientHints, lib.clientHints)
	picked.MediaCaps = pickMediaCapabilities(platform, picked.MediaCaps, lib.mediaCaps)
	applyDistributionData(&picked, lib, seed)
	if picked.Screen.AvailWidth <= 0 {
		picked.Screen.AvailWidth = picked.Screen.Width
	}
	if picked.Screen.AvailHeight <= 0 {
		picked.Screen.AvailHeight = picked.Screen.Height - defaultWindowChromeTop(platform)
	}
	if picked.Screen.DevicePixelRatio <= 0 {
		picked.Screen.DevicePixelRatio = 1
	}
	if picked.Privacy.WebRTCPolicy == "" {
		picked.Privacy.WebRTCPolicy = "disable_non_proxied_udp"
	}
	if picked.Privacy.WebRTCIP == "" {
		picked.Privacy.WebRTCIP = "auto"
	}
	return &picked, nil
}

func applyDistributionData(profile *Profile, lib *Library, seed int64) {
	if profile == nil || lib == nil {
		return
	}
	platform := NormalizePlatform(profile.Platform)
	if template, ok := pickDeviceTemplate(lib.deviceTemplates, platform, profile.DeviceClass, seed); ok {
		applyDeviceTemplate(profile, template)
		return
	}
	profile.Fonts = pickFonts(platform, profile.Fonts, lib.fonts, seed)
	profile.WebGL = pickWebGL(platform, profile.DeviceClass, profile.WebGL, lib.webgl, seed)
	profile.Screen = pickScreen(platform, profile.DeviceClass, profile.Screen, lib.screen, seed)
	profile.Media = pickMedia(profile.DeviceClass, profile.Media, lib.media, seed)
}

func pickDeviceTemplate(templates []DeviceTemplate, platform string, deviceClass string, seed int64) (DeviceTemplate, bool) {
	if len(templates) == 0 {
		return DeviceTemplate{}, false
	}
	platform = NormalizePlatform(platform)
	deviceClass = normalizeDeviceClass(deviceClass)
	candidates := make([]DeviceTemplate, 0)
	for _, item := range templates {
		if NormalizePlatform(item.Platform) != platform {
			continue
		}
		if deviceClass != "" && normalizeDeviceClass(item.DeviceClass) != "" && normalizeDeviceClass(item.DeviceClass) != deviceClass {
			continue
		}
		candidates = append(candidates, item)
	}
	if len(candidates) == 0 && deviceClass != "" {
		for _, item := range templates {
			if NormalizePlatform(item.Platform) == platform {
				candidates = append(candidates, item)
			}
		}
	}
	if len(candidates) == 0 {
		return DeviceTemplate{}, false
	}
	return weightedPickDeviceTemplate(candidates, seed), true
}

func applyDeviceTemplate(profile *Profile, template DeviceTemplate) {
	if profile == nil {
		return
	}
	profile.DeviceClass = defaultString(profile.DeviceClass, template.DeviceClass)
	profile.Screen = template.Screen
	profile.Hardware.HardwareConcurrency = defaultInt(template.Hardware.HardwareConcurrency, profile.Hardware.HardwareConcurrency)
	profile.Hardware.DeviceMemory = defaultInt(template.Hardware.DeviceMemory, profile.Hardware.DeviceMemory)
	profile.Hardware.TouchPoints = template.Hardware.TouchPoints
	profile.WebGL = template.WebGL
	profile.Fonts = normalizeUniqueStrings(template.Fonts)
	profile.Media = template.Media
}

func isDefaultFingerprintDistribution(lib *Library) bool {
	if lib == nil {
		return false
	}
	return sameProfileIDs(lib.profiles, defaultProfiles()) &&
		sameFontPools(lib.fonts, defaultFontPools()) &&
		sameWebGLPools(lib.webgl, defaultWebGLPools()) &&
		sameScreenDistribution(lib.screen, defaultScreenDistribution()) &&
		sameMediaPools(lib.media, defaultMediaPools())
}

func sameProfileIDs(left []Profile, right []Profile) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if strings.TrimSpace(left[i].ID) != strings.TrimSpace(right[i].ID) ||
			NormalizePlatform(left[i].Platform) != NormalizePlatform(right[i].Platform) ||
			normalizeDeviceClass(left[i].DeviceClass) != normalizeDeviceClass(right[i].DeviceClass) {
			return false
		}
	}
	return true
}

func sameFontPools(left map[string]FontPool, right map[string]FontPool) bool {
	if len(left) != len(right) {
		return false
	}
	for platform, leftPool := range left {
		rightPool, ok := right[platform]
		if !ok || !stringSlicesEqual(leftPool.MarkerFonts, rightPool.MarkerFonts) || !stringSlicesEqual(leftPool.OptionalFonts, rightPool.OptionalFonts) {
			return false
		}
	}
	return true
}

func sameWebGLPools(left map[string][]WebGL, right map[string][]WebGL) bool {
	if len(left) != len(right) {
		return false
	}
	for platform, leftItems := range left {
		rightItems, ok := right[platform]
		if !ok || len(leftItems) != len(rightItems) {
			return false
		}
		for i := range leftItems {
			if !strings.EqualFold(strings.TrimSpace(leftItems[i].Vendor), strings.TrimSpace(rightItems[i].Vendor)) ||
				!strings.EqualFold(strings.TrimSpace(leftItems[i].Renderer), strings.TrimSpace(rightItems[i].Renderer)) {
				return false
			}
		}
	}
	return true
}

func sameScreenDistribution(left ScreenDistribution, right ScreenDistribution) bool {
	return stringSlicesEqual(left.Desktop, right.Desktop) &&
		stringSlicesEqual(left.Laptop, right.Laptop) &&
		intSlicesEqual(left.ColorDepth, right.ColorDepth) &&
		floatSlicesEqual(left.DevicePixelRatio, right.DevicePixelRatio)
}

func sameMediaPools(left map[string][]MediaDevices, right map[string][]MediaDevices) bool {
	if len(left) != len(right) {
		return false
	}
	for class, leftItems := range left {
		rightItems, ok := right[class]
		if !ok || len(leftItems) != len(rightItems) {
			return false
		}
		for i := range leftItems {
			if leftItems[i] != rightItems[i] {
				return false
			}
		}
	}
	return true
}

func stringSlicesEqual(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if strings.TrimSpace(left[i]) != strings.TrimSpace(right[i]) {
			return false
		}
	}
	return true
}

func intSlicesEqual(left []int, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func floatSlicesEqual(left []float64, right []float64) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
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
		"--fingerprint-accept-language=" + defaultString(profile.AcceptLanguage, buildAcceptLanguage(languagesForLocale(defaultString(profile.Locale, "zh-CN")))),
		"--timezone=" + defaultString(profile.Timezone, "Asia/Shanghai"),
		"--fingerprint-timezone=" + defaultString(profile.Timezone, "Asia/Shanghai"),
		"--window-size=" + strconv.Itoa(profile.Screen.Width) + "," + strconv.Itoa(profile.Screen.Height),
		"--fingerprint-screen-avail=" + strconv.Itoa(defaultInt(profile.Screen.AvailWidth, profile.Screen.Width)) + "," + strconv.Itoa(defaultInt(profile.Screen.AvailHeight, profile.Screen.Height-defaultWindowChromeTop(profile.Platform))),
		"--fingerprint-device-pixel-ratio=" + trimFloat(defaultFloat(profile.Screen.DevicePixelRatio, 1)),
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
		Country:             profile.Country,
		Locale:              profile.Locale,
		Timezone:            profile.Timezone,
		AcceptLanguage:      profile.AcceptLanguage,
		Languages:           append([]string{}, profile.Languages...),
		Resolution:          strconv.Itoa(profile.Screen.Width) + "," + strconv.Itoa(profile.Screen.Height),
		WebGLVendor:         profile.WebGL.Vendor,
		WebGLRenderer:       profile.WebGL.Renderer,
		HardwareConcurrency: profile.Hardware.HardwareConcurrency,
		DeviceMemory:        profile.Hardware.DeviceMemory,
		TouchPoints:         profile.Hardware.TouchPoints,
		ClientHintsCoverage: profile.ClientHints.RuntimeCoverage,
	}
}

func NormalizeRegionMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case RegionModeProxy:
		return RegionModeProxy
	case RegionModeSystem:
		return RegionModeSystem
	default:
		return RegionModeManual
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

func SeedArg(profileID string) string {
	return "--fingerprint=" + strconv.FormatInt(StableSeed(profileID), 10)
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

func weightedPickDeviceTemplate(candidates []DeviceTemplate, seed int64) DeviceTemplate {
	total := 0
	for _, candidate := range candidates {
		if candidate.Weight > 0 {
			total += candidate.Weight
		}
	}
	if total <= 0 {
		return candidates[int(seed)%len(candidates)]
	}
	r := rand.New(rand.NewSource(seed + 409))
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

func (lib *Library) localeTimezone(country string, locale string, timezone string) (string, string, string) {
	locale = strings.TrimSpace(locale)
	timezone = strings.TrimSpace(timezone)
	if locale != "" && timezone != "" {
		return locale, timezone, normalizeCountry(country, locale, timezone)
	}
	country = normalizeCountry(country, locale, timezone)
	region, ok := lib.regions[country]
	if !ok {
		region = defaultRegions()["CN"]
		country = "CN"
	}
	if locale == "" && len(region.Locales) > 0 {
		locale = region.Locales[0]
	}
	if timezone == "" && len(region.Timezones) > 0 {
		timezone = region.Timezones[0]
	}
	if locale == "" {
		locale = "zh-CN"
	}
	if timezone == "" {
		timezone = "Asia/Shanghai"
	}
	return locale, timezone, country
}

func (lib *Library) languageStack(country string, locale string) (string, []string) {
	locale = strings.TrimSpace(locale)
	country = normalizeCountry(country, locale, "")
	if lib != nil {
		if region, ok := lib.regions[country]; ok {
			if locale != "" && !languageTagListContains(region.Locales, locale) {
				languages := languagesForLocale(locale)
				return buildAcceptLanguage(languages), languages
			}
			acceptLanguage := strings.TrimSpace(region.AcceptLanguage)
			languages := normalizeLanguageTags(region.Languages)
			if len(languages) == 0 {
				languages = normalizeLanguageTags(region.Locales)
			}
			if acceptLanguage == "" {
				acceptLanguage = buildAcceptLanguage(languages)
			}
			if acceptLanguage != "" || len(languages) > 0 {
				return acceptLanguage, languages
			}
		}
	}
	languages := languagesForLocale(locale)
	if len(languages) == 0 {
		languages = []string{"zh-CN", "zh"}
	}
	return buildAcceptLanguage(languages), languages
}

func RegionDefaults(country string, locale string, timezone string) (string, string, string) {
	return LoadLibrary().localeTimezone(country, locale, timezone)
}

func ProxyRegionDefaults(country string, locale string, timezone string) (string, string, string) {
	return LoadLibrary().proxyRegionDefaults(country, locale, timezone)
}

func (lib *Library) proxyRegionDefaults(country string, locale string, timezone string) (string, string, string) {
	locale = strings.TrimSpace(locale)
	timezone = strings.TrimSpace(timezone)
	country = normalizeCountry(country, locale, timezone)
	region, ok := lib.regions[country]
	if !ok {
		region = defaultRegions()["CN"]
		country = "CN"
	}
	if locale == "" || !languageTagListContains(region.Locales, locale) {
		if len(region.Locales) > 0 {
			locale = region.Locales[0]
		} else {
			locale = "zh-CN"
		}
	}
	if timezone == "" || !stringListContainsFold(region.Timezones, timezone) {
		if len(region.Timezones) > 0 {
			timezone = region.Timezones[0]
		} else {
			timezone = "Asia/Shanghai"
		}
	}
	return locale, timezone, country
}

func AcceptLanguageDefaults(country string, locale string) string {
	acceptLanguage, _ := LoadLibrary().languageStack(country, locale)
	return acceptLanguage
}

func MediaCapabilitiesForPlatform(platform string) MediaCapabilityProfile {
	lib := LoadLibrary()
	return pickMediaCapabilities(platform, MediaCapabilityProfile{}, lib.mediaCaps)
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

func defaultFloat(value float64, fallback float64) float64 {
	if value <= 0 {
		return fallback
	}
	return value
}

func trimFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func defaultInt(value int, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}

func defaultWindowChromeTop(platform string) int {
	switch NormalizePlatform(platform) {
	case PlatformMac:
		return 74
	case PlatformLinux:
		return 80
	default:
		return 40
	}
}

func loadProfilesFromData() ([]Profile, error) {
	payload, err := fingerprintDataFS.ReadFile("data/profiles.json")
	if err != nil {
		return nil, err
	}
	var profiles []Profile
	if err := json.Unmarshal(payload, &profiles); err != nil {
		return nil, err
	}
	if len(profiles) == 0 {
		return nil, fmt.Errorf("fingerprint profile data is empty")
	}
	return profiles, nil
}

func loadRegionsFromData() (map[string]Region, error) {
	payload, err := fingerprintDataFS.ReadFile("data/locales.json")
	if err != nil {
		return nil, err
	}
	var items []Region
	if err := json.Unmarshal(payload, &items); err != nil {
		return nil, err
	}
	out := map[string]Region{}
	for _, item := range items {
		country := strings.ToUpper(strings.TrimSpace(item.Country))
		if country == "" {
			continue
		}
		item.Country = country
		item.Locales = normalizeLanguageTags(item.Locales)
		item.Languages = normalizeLanguageTags(item.Languages)
		if len(item.Languages) == 0 {
			item.Languages = append([]string{}, item.Locales...)
		}
		item.AcceptLanguage = normalizeAcceptLanguage(item.AcceptLanguage, item.Languages)
		out[country] = item
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("fingerprint locale data is empty")
	}
	return out, nil
}

func loadClientHintsFromData() (map[string]ClientHints, error) {
	payload, err := fingerprintDataFS.ReadFile("data/client_hints.json")
	if err != nil {
		return nil, err
	}
	var raw map[string]ClientHints
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, err
	}
	out := map[string]ClientHints{}
	for platform, hints := range raw {
		key := NormalizePlatform(platform)
		hints.RuntimeCoverage = strings.TrimSpace(hints.RuntimeCoverage)
		hints.Brands = normalizeClientHintBrands(hints.Brands)
		hints.FullVersionList = normalizeClientHintBrands(hints.FullVersionList)
		hints.Platform = strings.TrimSpace(hints.Platform)
		hints.PlatformVersion = strings.TrimSpace(hints.PlatformVersion)
		hints.Architecture = strings.TrimSpace(hints.Architecture)
		hints.Bitness = strings.TrimSpace(hints.Bitness)
		hints.Model = strings.TrimSpace(hints.Model)
		hints.Notes = normalizeUniqueStrings(hints.Notes)
		if hints.RuntimeCoverage == "" {
			hints.RuntimeCoverage = "data-only"
		}
		if len(hints.Brands) == 0 && len(hints.FullVersionList) == 0 {
			continue
		}
		out[key] = hints
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("fingerprint client hints data is empty")
	}
	return out, nil
}

func loadMediaCapabilitiesFromData() (map[string]MediaCapabilityProfile, error) {
	payload, err := fingerprintDataFS.ReadFile("data/media_capabilities.json")
	if err != nil {
		return nil, err
	}
	var raw map[string]MediaCapabilityProfile
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, err
	}
	out := map[string]MediaCapabilityProfile{}
	for platform, caps := range raw {
		key := NormalizePlatform(platform)
		caps = normalizeMediaCapabilityProfile(caps)
		if len(caps.Plugins) == 0 && len(caps.MimeTypes) == 0 {
			continue
		}
		out[key] = caps
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("fingerprint media capability data is empty")
	}
	return out, nil
}

func loadFontsFromData() (map[string]FontPool, error) {
	payload, err := fingerprintDataFS.ReadFile("data/fonts.json")
	if err != nil {
		return nil, err
	}
	var pools map[string]FontPool
	if err := json.Unmarshal(payload, &pools); err != nil {
		return nil, err
	}
	out := map[string]FontPool{}
	for platform, pool := range pools {
		key := NormalizePlatform(platform)
		if len(pool.MarkerFonts) == 0 {
			continue
		}
		out[key] = FontPool{
			MarkerFonts:   normalizeUniqueStrings(pool.MarkerFonts),
			OptionalFonts: normalizeUniqueStrings(pool.OptionalFonts),
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("fingerprint font data is empty")
	}
	return out, nil
}

func loadWebGLFromData() (map[string][]WebGL, error) {
	payload, err := fingerprintDataFS.ReadFile("data/webgl.json")
	if err != nil {
		return nil, err
	}
	var pools map[string][]WebGL
	if err := json.Unmarshal(payload, &pools); err != nil {
		return nil, err
	}
	out := map[string][]WebGL{}
	for platform, items := range pools {
		key := NormalizePlatform(platform)
		for _, item := range items {
			if strings.TrimSpace(item.Vendor) == "" || strings.TrimSpace(item.Renderer) == "" {
				continue
			}
			out[key] = append(out[key], WebGL{
				Vendor:   strings.TrimSpace(item.Vendor),
				Renderer: strings.TrimSpace(item.Renderer),
			})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("fingerprint webgl data is empty")
	}
	return out, nil
}

func loadScreenFromData() (ScreenDistribution, error) {
	payload, err := fingerprintDataFS.ReadFile("data/screen.json")
	if err != nil {
		return ScreenDistribution{}, err
	}
	var screen ScreenDistribution
	if err := json.Unmarshal(payload, &screen); err != nil {
		return ScreenDistribution{}, err
	}
	screen.Desktop = normalizeResolutionList(screen.Desktop)
	screen.Laptop = normalizeResolutionList(screen.Laptop)
	if len(screen.Desktop) == 0 && len(screen.Laptop) == 0 {
		return ScreenDistribution{}, fmt.Errorf("fingerprint screen data is empty")
	}
	if len(screen.ColorDepth) == 0 {
		screen.ColorDepth = []int{24}
	}
	if len(screen.DevicePixelRatio) == 0 {
		screen.DevicePixelRatio = []float64{1}
	}
	return screen, nil
}

func loadMediaFromData() (map[string][]MediaDevices, error) {
	payload, err := fingerprintDataFS.ReadFile("data/media_devices.json")
	if err != nil {
		return nil, err
	}
	var raw map[string][]string
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, err
	}
	out := map[string][]MediaDevices{}
	for key, values := range raw {
		class := normalizeDeviceClass(key)
		if class == "" && strings.TrimSpace(key) == "desktop" {
			class = "office"
		}
		if class == "" {
			continue
		}
		for _, value := range values {
			media, ok := parseMediaDevices(value)
			if ok {
				out[class] = append(out[class], media)
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("fingerprint media device data is empty")
	}
	return out, nil
}

func defaultLibrary() *Library {
	return &Library{
		profiles:        defaultProfiles(),
		regions:         defaultRegions(),
		fonts:           defaultFontPools(),
		webgl:           defaultWebGLPools(),
		screen:          defaultScreenDistribution(),
		media:           defaultMediaPools(),
		clientHints:     defaultClientHints(),
		mediaCaps:       defaultMediaCapabilities(),
		deviceTemplates: stableDeviceTemplates(),
	}
}

func defaultRegions() map[string]Region {
	return map[string]Region{
		"CN": {Country: "CN", Name: "China", Locales: []string{"zh-CN"}, Timezones: []string{"Asia/Shanghai"}, AcceptLanguage: "zh-CN,zh;q=0.9", Languages: []string{"zh-CN", "zh"}},
		"US": {Country: "US", Name: "United States", Locales: []string{"en-US"}, Timezones: []string{"America/New_York", "America/Los_Angeles", "America/Chicago"}, AcceptLanguage: "en-US,en;q=0.9", Languages: []string{"en-US", "en"}},
		"GB": {Country: "GB", Name: "United Kingdom", Locales: []string{"en-GB"}, Timezones: []string{"Europe/London"}, AcceptLanguage: "en-GB,en;q=0.9", Languages: []string{"en-GB", "en"}},
		"DE": {Country: "DE", Name: "Germany", Locales: []string{"de-DE"}, Timezones: []string{"Europe/Berlin"}, AcceptLanguage: "de-DE,de;q=0.9,en;q=0.8", Languages: []string{"de-DE", "de", "en"}},
		"FR": {Country: "FR", Name: "France", Locales: []string{"fr-FR"}, Timezones: []string{"Europe/Paris"}, AcceptLanguage: "fr-FR,fr;q=0.9,en;q=0.8", Languages: []string{"fr-FR", "fr", "en"}},
		"JP": {Country: "JP", Name: "Japan", Locales: []string{"ja-JP"}, Timezones: []string{"Asia/Tokyo"}, AcceptLanguage: "ja-JP,ja;q=0.9,en;q=0.8", Languages: []string{"ja-JP", "ja", "en"}},
		"KR": {Country: "KR", Name: "South Korea", Locales: []string{"ko-KR"}, Timezones: []string{"Asia/Seoul"}, AcceptLanguage: "ko-KR,ko;q=0.9,en;q=0.8", Languages: []string{"ko-KR", "ko", "en"}},
		"SG": {Country: "SG", Name: "Singapore", Locales: []string{"en-SG"}, Timezones: []string{"Asia/Singapore"}, AcceptLanguage: "en-SG,en;q=0.9", Languages: []string{"en-SG", "en"}},
		"BR": {Country: "BR", Name: "Brazil", Locales: []string{"pt-BR"}, Timezones: []string{"America/Sao_Paulo"}, AcceptLanguage: "pt-BR,pt;q=0.9,en;q=0.8", Languages: []string{"pt-BR", "pt", "en"}},
		"IN": {Country: "IN", Name: "India", Locales: []string{"en-IN"}, Timezones: []string{"Asia/Kolkata"}, AcceptLanguage: "en-IN,en;q=0.9", Languages: []string{"en-IN", "en"}},
	}
}

func defaultClientHints() map[string]ClientHints {
	return map[string]ClientHints{
		PlatformWindows: defaultClientHintsForPlatform("Windows", "10.0.0", "x86", "64"),
		PlatformMac:     defaultClientHintsForPlatform("macOS", "14.0.0", "arm", "64"),
		PlatformLinux:   defaultClientHintsForPlatform("Linux", "", "x86", "64"),
	}
}

func defaultClientHintsForPlatform(platform string, platformVersion string, architecture string, bitness string) ClientHints {
	return ClientHints{
		RuntimeCoverage: "data-only",
		Brands: []ClientHintBrand{
			{Brand: "Chromium", Version: "core-major"},
			{Brand: "Google Chrome", Version: "core-major"},
			{Brand: "Not/A)Brand", Version: "99"},
		},
		FullVersionList: []ClientHintBrand{
			{Brand: "Chromium", Version: "core-full-version"},
			{Brand: "Google Chrome", Version: "core-full-version"},
			{Brand: "Not/A)Brand", Version: "99.0.0.0"},
		},
		Platform:        platform,
		PlatformVersion: platformVersion,
		Architecture:    architecture,
		Bitness:         bitness,
		Mobile:          false,
		Notes:           []string{"Data model only; runtime UA-CH spoofing requires verified Chromium core support."},
	}
}

func defaultFontPools() map[string]FontPool {
	return map[string]FontPool{
		PlatformWindows: {
			MarkerFonts:   []string{"Segoe UI", "Calibri", "Cambria Math", "Consolas"},
			OptionalFonts: []string{"Arial", "Tahoma", "Times New Roman", "Verdana", "Microsoft YaHei", "SimSun", "SimHei"},
		},
		PlatformMac: {
			MarkerFonts:   []string{"Helvetica Neue", "PingFang SC", "Menlo", "Monaco"},
			OptionalFonts: []string{"Arial", "Helvetica", "Apple Color Emoji", "Times New Roman", "Hiragino Sans GB"},
		},
		PlatformLinux: {
			MarkerFonts:   []string{"Noto Sans", "Noto Color Emoji", "Arimo", "Cousine"},
			OptionalFonts: []string{"Tinos", "Noto Sans CJK SC", "Liberation Sans", "DejaVu Sans"},
		},
	}
}

func defaultWebGLPools() map[string][]WebGL {
	return map[string][]WebGL{
		PlatformWindows: {
			{Vendor: "Intel", Renderer: "Intel(R) UHD Graphics 630"},
			{Vendor: "Intel", Renderer: "Intel(R) UHD Graphics 620"},
			{Vendor: "NVIDIA", Renderer: "NVIDIA GeForce RTX 3060"},
			{Vendor: "AMD", Renderer: "AMD Radeon RX 580"},
		},
		PlatformMac: {
			{Vendor: "Apple", Renderer: "Apple M1"},
			{Vendor: "Apple", Renderer: "Apple M2"},
			{Vendor: "Apple", Renderer: "Apple GPU"},
		},
		PlatformLinux: {
			{Vendor: "Intel", Renderer: "Mesa Intel(R) UHD Graphics 620"},
			{Vendor: "Intel", Renderer: "Mesa Intel(R) HD Graphics 520"},
			{Vendor: "AMD", Renderer: "Mesa AMD Radeon Graphics"},
		},
	}
}

func defaultScreenDistribution() ScreenDistribution {
	return ScreenDistribution{
		Desktop:          []string{"1920,1080", "2560,1440", "1600,900"},
		Laptop:           []string{"1366,768", "1440,900", "1280,800"},
		ColorDepth:       []int{24, 30, 32},
		DevicePixelRatio: []float64{1, 1.25, 1.5, 2},
	}
}

func defaultMediaPools() map[string][]MediaDevices {
	return map[string][]MediaDevices{
		"office":      {{0, 1, 1}, {1, 1, 1}},
		"laptop":      {{1, 1, 1}},
		"workstation": {{1, 2, 2}},
		"gaming":      {{1, 2, 2}},
		"light_linux": {{0, 1, 1}},
	}
}

func defaultMediaCapabilities() map[string]MediaCapabilityProfile {
	return map[string]MediaCapabilityProfile{
		PlatformWindows: defaultMediaCapabilitiesForPlatform(true, true, CodecCapabilities{H264: true, AAC: true, VP9: true, AV1: true}),
		PlatformMac:     defaultMediaCapabilitiesForPlatform(true, true, CodecCapabilities{H264: true, AAC: true, VP9: true, AV1: true}),
		PlatformLinux:   defaultMediaCapabilitiesForPlatform(true, false, CodecCapabilities{VP9: true, AV1: true}),
	}
}

func defaultMediaCapabilitiesForPlatform(pdfViewer bool, widevine bool, codecs CodecCapabilities) MediaCapabilityProfile {
	return MediaCapabilityProfile{
		RuntimeCoverage: "capability-matrix",
		ChromeMajor:     "core-major",
		PDFViewer:       pdfViewer,
		WidevinePresent: widevine,
		EMESupported:    widevine,
		Plugins: []PluginCapability{
			{Name: "PDF Viewer", Filename: "internal-pdf-viewer", Description: "Portable Document Format"},
			{Name: "Chrome PDF Viewer", Filename: "internal-pdf-viewer", Description: "Portable Document Format"},
			{Name: "Chromium PDF Viewer", Filename: "internal-pdf-viewer", Description: "Portable Document Format"},
			{Name: "Microsoft Edge PDF Viewer", Filename: "internal-pdf-viewer", Description: "Portable Document Format"},
			{Name: "WebKit built-in PDF", Filename: "internal-pdf-viewer", Description: "Portable Document Format"},
		},
		MimeTypes: []MimeCapability{
			{Type: "application/pdf", Suffixes: "pdf", Description: "Portable Document Format"},
			{Type: "text/pdf", Suffixes: "pdf", Description: "Portable Document Format"},
		},
		Codecs: codecs,
		Notes:  []string{"Capability matrix only; runtime spoofing must be backed by Chromium core support or measured browser capabilities."},
	}
}

func stableDeviceTemplates() []DeviceTemplate {
	return []DeviceTemplate{
		deviceTemplate("win_laptop_1366_intel", 80, PlatformWindows, "laptop", "1366,768", "1366,728", 1, 24, 4, 4, WebGL{"Intel", "Intel(R) UHD Graphics 620"}, []string{"Segoe UI", "Calibri", "Cambria Math", "Consolas", "Arial", "Tahoma", "Times New Roman", "Verdana", "Microsoft YaHei"}, MediaDevices{1, 1, 1}),
		deviceTemplate("win_office_1920_intel", 120, PlatformWindows, "office", "1920,1080", "1920,1040", 1, 24, 8, 8, WebGL{"Intel", "Intel(R) UHD Graphics 630"}, []string{"Segoe UI", "Calibri", "Cambria Math", "Consolas", "Arial", "Tahoma", "Times New Roman", "Verdana", "Microsoft YaHei", "SimSun"}, MediaDevices{1, 1, 1}),
		deviceTemplate("win_workstation_2560_nvidia", 45, PlatformWindows, "workstation", "2560,1440", "2560,1400", 1, 24, 16, 16, WebGL{"NVIDIA", "NVIDIA GeForce RTX 3060"}, []string{"Segoe UI", "Calibri", "Cambria Math", "Consolas", "Arial", "Tahoma", "Times New Roman", "Verdana"}, MediaDevices{1, 2, 2}),
		deviceTemplate("win_gaming_2560_nvidia", 35, PlatformWindows, "gaming", "2560,1440", "2560,1400", 1, 24, 16, 16, WebGL{"NVIDIA", "NVIDIA GeForce RTX 3060"}, []string{"Segoe UI", "Calibri", "Cambria Math", "Consolas", "Arial", "Tahoma", "Times New Roman", "Verdana"}, MediaDevices{1, 2, 2}),
		deviceTemplate("mac_office_1440_apple", 105, PlatformMac, "office", "1440,900", "1440,826", 2, 24, 8, 8, WebGL{"Apple", "Apple GPU"}, []string{"Helvetica Neue", "PingFang SC", "Menlo", "Monaco", "Times New Roman", "Apple Color Emoji", "Helvetica"}, MediaDevices{1, 1, 1}),
		deviceTemplate("mac_retina_1440_apple", 130, PlatformMac, "laptop", "1440,900", "1440,826", 2, 24, 8, 8, WebGL{"Apple", "Apple GPU"}, []string{"Helvetica Neue", "PingFang SC", "Menlo", "Monaco", "Times New Roman", "Apple Color Emoji", "Helvetica"}, MediaDevices{1, 1, 1}),
		deviceTemplate("mac_retina_1512_apple", 95, PlatformMac, "laptop", "1512,982", "1512,900", 2, 24, 8, 8, WebGL{"Apple", "Apple GPU"}, []string{"Helvetica Neue", "PingFang SC", "Menlo", "Monaco", "Times New Roman", "Apple Color Emoji", "Helvetica"}, MediaDevices{1, 1, 1}),
		deviceTemplate("mac_workstation_1728_apple", 55, PlatformMac, "workstation", "1728,1117", "1728,1035", 2, 24, 10, 16, WebGL{"Apple", "Apple GPU"}, []string{"Helvetica Neue", "PingFang SC", "Menlo", "Monaco", "Times New Roman", "Apple Color Emoji", "Helvetica"}, MediaDevices{1, 2, 2}),
		deviceTemplate("mac_gaming_1728_apple", 25, PlatformMac, "gaming", "1728,1117", "1728,1035", 2, 24, 10, 16, WebGL{"Apple", "Apple GPU"}, []string{"Helvetica Neue", "PingFang SC", "Menlo", "Monaco", "Times New Roman", "Apple Color Emoji", "Helvetica"}, MediaDevices{1, 2, 2}),
		deviceTemplate("linux_laptop_1366_mesa", 70, PlatformLinux, "light_linux", "1366,768", "1366,728", 1, 24, 4, 4, WebGL{"Intel", "Mesa Intel(R) HD Graphics 520"}, []string{"Noto Sans", "Noto Color Emoji", "Arimo", "Cousine", "Tinos", "Noto Sans CJK SC", "Liberation Sans", "DejaVu Sans"}, MediaDevices{0, 1, 1}),
		deviceTemplate("linux_office_1920_mesa", 95, PlatformLinux, "office", "1920,1080", "1920,1040", 1, 24, 8, 8, WebGL{"Intel", "Mesa Intel(R) UHD Graphics 620"}, []string{"Noto Sans", "Noto Color Emoji", "Arimo", "Cousine", "Tinos", "Noto Sans CJK SC", "Liberation Sans", "DejaVu Sans"}, MediaDevices{0, 1, 1}),
		deviceTemplate("linux_workstation_1920_amd", 35, PlatformLinux, "workstation", "1920,1080", "1920,1040", 1, 24, 8, 8, WebGL{"AMD", "Mesa AMD Radeon Graphics"}, []string{"Noto Sans", "Noto Color Emoji", "Arimo", "Cousine", "Tinos", "Liberation Sans", "DejaVu Sans"}, MediaDevices{0, 1, 1}),
		deviceTemplate("linux_gaming_1920_amd", 25, PlatformLinux, "gaming", "1920,1080", "1920,1040", 1, 24, 8, 8, WebGL{"AMD", "Mesa AMD Radeon Graphics"}, []string{"Noto Sans", "Noto Color Emoji", "Arimo", "Cousine", "Tinos", "Liberation Sans", "DejaVu Sans"}, MediaDevices{0, 1, 1}),
	}
}

func deviceTemplate(id string, weight int, platform string, deviceClass string, resolution string, avail string, dpr float64, colorDepth int, concurrency int, memory int, webgl WebGL, fonts []string, media MediaDevices) DeviceTemplate {
	width, height := parseResolution(resolution)
	availWidth, availHeight := parseResolution(avail)
	if availWidth <= 0 {
		availWidth = width
	}
	if availHeight <= 0 || availHeight > height {
		availHeight = height - defaultWindowChromeTop(platform)
	}
	return DeviceTemplate{
		ID:          id,
		Weight:      weight,
		Platform:    NormalizePlatform(platform),
		DeviceClass: normalizeDeviceClass(deviceClass),
		Screen:      Screen{Width: width, Height: height, AvailWidth: availWidth, AvailHeight: availHeight, ColorDepth: colorDepth, DevicePixelRatio: dpr},
		Hardware:    Hardware{HardwareConcurrency: concurrency, DeviceMemory: memory, TouchPoints: 0},
		WebGL:       webgl,
		Fonts:       normalizeUniqueStrings(fonts),
		Media:       media,
	}
}

func normalizeCountry(country string, locale string, timezone string) string {
	value := strings.ToUpper(strings.TrimSpace(country))
	compactValue := strings.NewReplacer(" ", "", "-", "", "_", "", ".", "", ",", "").Replace(value)
	switch compactValue {
	case "US", "USA", "UNITEDSTATES", "UNITEDSTATESOFAMERICA", "AMERICA":
		return "US"
	case "GB", "UK", "UNITEDKINGDOM", "GREATBRITAIN", "BRITAIN", "ENGLAND":
		return "GB"
	case "DE", "DEU", "GERMANY", "DEUTSCHLAND":
		return "DE"
	case "FR", "FRA", "FRANCE":
		return "FR"
	case "JP", "JPN", "JAPAN":
		return "JP"
	case "KR", "KOR", "KOREA", "SOUTHKOREA", "REPUBLICOFKOREA":
		return "KR"
	case "SG", "SGP", "SINGAPORE":
		return "SG"
	case "BR", "BRA", "BRAZIL", "BRASIL":
		return "BR"
	case "IN", "IND", "INDIA":
		return "IN"
	case "CN", "CHN", "CHINA", "PRC", "PEOPLESREPUBLICOFCHINA":
		return "CN"
	case "":
	default:
		if len(compactValue) == 2 {
			return compactValue
		}
	}
	combined := strings.ToLower(strings.TrimSpace(locale) + "|" + strings.TrimSpace(timezone))
	switch {
	case strings.Contains(combined, "en-us") || strings.Contains(combined, "america/new_york") || strings.Contains(combined, "america/los_angeles") || strings.Contains(combined, "america/chicago"):
		return "US"
	case strings.Contains(combined, "en-gb") || strings.Contains(combined, "europe/london"):
		return "GB"
	case strings.Contains(combined, "de-de") || strings.Contains(combined, "europe/berlin"):
		return "DE"
	case strings.Contains(combined, "fr-fr") || strings.Contains(combined, "europe/paris"):
		return "FR"
	case strings.Contains(combined, "ja-jp") || strings.Contains(combined, "asia/tokyo"):
		return "JP"
	case strings.Contains(combined, "ko-kr") || strings.Contains(combined, "asia/seoul"):
		return "KR"
	case strings.Contains(combined, "en-sg") || strings.Contains(combined, "asia/singapore"):
		return "SG"
	case strings.Contains(combined, "pt-br") || strings.Contains(combined, "america/sao_paulo"):
		return "BR"
	case strings.Contains(combined, "en-in") || strings.Contains(combined, "asia/kolkata"):
		return "IN"
	default:
		return "CN"
	}
}

func pickFonts(platform string, current []string, pools map[string]FontPool, seed int64) []string {
	pool, ok := pools[NormalizePlatform(platform)]
	if !ok || len(pool.MarkerFonts) == 0 {
		return normalizeUniqueStrings(current)
	}
	out := normalizeUniqueStrings(pool.MarkerFonts)
	optional := normalizeUniqueStrings(pool.OptionalFonts)
	if len(optional) == 0 {
		return out
	}
	r := rand.New(rand.NewSource(seed + 101))
	count := len(optional)/2 + 1
	if count > len(optional) {
		count = len(optional)
	}
	perm := r.Perm(len(optional))
	for _, index := range perm[:count] {
		out = append(out, optional[index])
	}
	return normalizeUniqueStrings(out)
}

func pickWebGL(platform string, deviceClass string, current WebGL, pools map[string][]WebGL, seed int64) WebGL {
	items := pools[NormalizePlatform(platform)]
	if len(items) == 0 {
		return current
	}
	if deviceClass == "gaming" {
		for _, item := range items {
			if strings.Contains(strings.ToLower(item.Vendor+" "+item.Renderer), "nvidia") {
				return item
			}
		}
	}
	if deviceClass == "workstation" && NormalizePlatform(platform) == PlatformMac {
		for _, item := range items {
			if strings.Contains(strings.ToLower(item.Renderer), "m2") {
				return item
			}
		}
	}
	r := rand.New(rand.NewSource(seed + 211))
	return items[r.Intn(len(items))]
}

func pickScreen(platform string, deviceClass string, current Screen, distribution ScreenDistribution, seed int64) Screen {
	candidates := distribution.Desktop
	if deviceClass == "laptop" || deviceClass == "light_linux" {
		candidates = distribution.Laptop
	}
	if len(candidates) == 0 {
		candidates = append([]string{}, distribution.Desktop...)
		candidates = append(candidates, distribution.Laptop...)
	}
	if len(candidates) == 0 {
		return current
	}
	r := rand.New(rand.NewSource(seed + 307))
	width, height := parseResolution(candidates[r.Intn(len(candidates))])
	current.Width = width
	current.Height = height
	current.AvailWidth = width
	current.AvailHeight = height - defaultWindowChromeTop(platform)
	if current.AvailHeight <= 0 {
		current.AvailHeight = height
	}
	if len(distribution.ColorDepth) > 0 {
		current.ColorDepth = distribution.ColorDepth[r.Intn(len(distribution.ColorDepth))]
	}
	if len(distribution.DevicePixelRatio) > 0 {
		current.DevicePixelRatio = distribution.DevicePixelRatio[r.Intn(len(distribution.DevicePixelRatio))]
	}
	return current
}

func pickMedia(deviceClass string, current MediaDevices, pools map[string][]MediaDevices, seed int64) MediaDevices {
	class := normalizeDeviceClass(deviceClass)
	if class == "" {
		class = "office"
	}
	items := pools[class]
	if len(items) == 0 {
		return current
	}
	r := rand.New(rand.NewSource(seed + 401))
	return items[r.Intn(len(items))]
}

func pickClientHints(platform string, current ClientHints, pools map[string]ClientHints) ClientHints {
	hints, ok := pools[NormalizePlatform(platform)]
	if !ok {
		return current
	}
	return hints
}

func pickMediaCapabilities(platform string, current MediaCapabilityProfile, pools map[string]MediaCapabilityProfile) MediaCapabilityProfile {
	caps, ok := pools[NormalizePlatform(platform)]
	if !ok {
		return current
	}
	return caps
}

func normalizeMediaCapabilityProfile(caps MediaCapabilityProfile) MediaCapabilityProfile {
	caps.RuntimeCoverage = strings.TrimSpace(caps.RuntimeCoverage)
	if caps.RuntimeCoverage == "" {
		caps.RuntimeCoverage = "capability-matrix"
	}
	caps.ChromeMajor = strings.TrimSpace(caps.ChromeMajor)
	if caps.ChromeMajor == "" {
		caps.ChromeMajor = "core-major"
	}
	caps.Plugins = normalizePluginCapabilities(caps.Plugins)
	caps.MimeTypes = normalizeMimeCapabilities(caps.MimeTypes)
	caps.Notes = normalizeUniqueStrings(caps.Notes)
	return caps
}

func normalizePluginCapabilities(items []PluginCapability) []PluginCapability {
	seen := map[string]struct{}{}
	out := make([]PluginCapability, 0, len(items))
	for _, item := range items {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		key := strings.ToLower(name + "|" + strings.TrimSpace(item.Filename))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, PluginCapability{
			Name:        name,
			Filename:    strings.TrimSpace(item.Filename),
			Description: strings.TrimSpace(item.Description),
		})
	}
	return out
}

func normalizeMimeCapabilities(items []MimeCapability) []MimeCapability {
	seen := map[string]struct{}{}
	out := make([]MimeCapability, 0, len(items))
	for _, item := range items {
		mimeType := strings.ToLower(strings.TrimSpace(item.Type))
		if mimeType == "" {
			continue
		}
		if _, ok := seen[mimeType]; ok {
			continue
		}
		seen[mimeType] = struct{}{}
		out = append(out, MimeCapability{
			Type:        mimeType,
			Suffixes:    strings.TrimSpace(item.Suffixes),
			Description: strings.TrimSpace(item.Description),
		})
	}
	return out
}

func normalizeUniqueStrings(items []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func normalizeLanguageTags(items []string) []string {
	out := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		if before, _, ok := strings.Cut(value, ";"); ok {
			value = strings.TrimSpace(before)
		}
		value = strings.ReplaceAll(value, "_", "-")
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func languagesForLocale(locale string) []string {
	languages := normalizeLanguageTags([]string{locale})
	if len(languages) == 0 {
		return nil
	}
	base := languages[0]
	if before, _, ok := strings.Cut(base, "-"); ok {
		base = before
	}
	if base != "" && !strings.EqualFold(base, languages[0]) {
		languages = append(languages, base)
	}
	return normalizeLanguageTags(languages)
}

func stringListContainsFold(items []string, want string) bool {
	want = strings.TrimSpace(want)
	if want == "" {
		return false
	}
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), want) {
			return true
		}
	}
	return false
}

func languageTagListContains(items []string, want string) bool {
	want = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(want), "_", "-"))
	if want == "" {
		return false
	}
	for _, item := range normalizeLanguageTags(items) {
		if strings.ToLower(item) == want {
			return true
		}
	}
	return false
}

func normalizeAcceptLanguage(value string, languages []string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	return buildAcceptLanguage(languages)
}

func buildAcceptLanguage(languages []string) string {
	languages = normalizeLanguageTags(languages)
	if len(languages) == 0 {
		return ""
	}
	parts := make([]string, 0, len(languages))
	for index, language := range languages {
		if index == 0 {
			parts = append(parts, language)
			continue
		}
		q := 10 - index
		if q < 1 {
			q = 1
		}
		parts = append(parts, language+";q=0."+strconv.Itoa(q))
	}
	return strings.Join(parts, ",")
}

func normalizeClientHintBrands(items []ClientHintBrand) []ClientHintBrand {
	out := make([]ClientHintBrand, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		brand := strings.TrimSpace(item.Brand)
		version := strings.TrimSpace(item.Version)
		if brand == "" || version == "" {
			continue
		}
		key := strings.ToLower(brand)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, ClientHintBrand{Brand: brand, Version: version})
	}
	return out
}

func normalizeResolutionList(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		width, height := parseResolution(item)
		if width <= 0 || height <= 0 {
			continue
		}
		out = append(out, strconv.Itoa(width)+","+strconv.Itoa(height))
	}
	return normalizeUniqueStrings(out)
}

func parseMediaDevices(value string) (MediaDevices, bool) {
	parts := strings.Split(value, ",")
	if len(parts) != 3 {
		return MediaDevices{}, false
	}
	values := [3]int{}
	for i, part := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || n < 0 || n > 16 {
			return MediaDevices{}, false
		}
		values[i] = n
	}
	return MediaDevices{Cameras: values[0], Microphones: values[1], Speakers: values[2]}, true
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
	availHeight := height - defaultWindowChromeTop(platform)
	if availHeight <= 0 {
		availHeight = height
	}
	return Profile{
		ID:          id,
		Weight:      weight,
		Platform:    platform,
		Brand:       "Chrome",
		Country:     "CN",
		Locale:      "zh-CN",
		Timezone:    "Asia/Shanghai",
		Screen:      Screen{Width: width, Height: height, AvailWidth: width, AvailHeight: availHeight, ColorDepth: 24, DevicePixelRatio: 1},
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
