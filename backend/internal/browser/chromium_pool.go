package browser

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
)

//go:embed assets/chromium_fingerprints.json
var chromiumFingerprintsRaw []byte

// ChromiumFingerprintPool 是构建期固化的 Chromium 指纹池。
type ChromiumFingerprintPool struct {
	Version int                            `json:"version"`
	Meta    chromiumPoolMeta               `json:"meta"`
	Pools   map[string][]ChromiumPoolEntry `json:"pools"`
}

type chromiumPoolMeta struct {
	CountPerOS  map[string]int `json:"countPerOS"`
	OSList      []string       `json:"osList"`
	Generator   map[string]any `json:"generator"`
	Schema      string         `json:"schema"`
	GeneratedAt string         `json:"generatedAt"`
	Note        string         `json:"note"`
}

// ChromiumPoolEntry 是 Chromium 池中单条桌面画像。
type ChromiumPoolEntry struct {
	OS      string                     `json:"os"`
	Seed    int                        `json:"seed"`
	Hash    string                     `json:"hash"`
	Profile ChromiumFingerprintProfile `json:"profile"`
}

// ChromiumFingerprintProfile 表示 fingerprint-chromium 可通过 CLI 消费的字段。
type ChromiumFingerprintProfile struct {
	Brand               string   `json:"brand"`
	Platform            string   `json:"platform"`
	Lang                string   `json:"lang"`
	Timezone            string   `json:"timezone"`
	WindowSize          string   `json:"windowSize"`
	ColorDepth          int      `json:"colorDepth"`
	HardwareConcurrency int      `json:"hardwareConcurrency"`
	DeviceMemory        int      `json:"deviceMemory"`
	WebGLVendor         string   `json:"webglVendor"`
	WebGLRenderer       string   `json:"webglRenderer"`
	Fonts               []string `json:"fonts"`
	DoNotTrack          bool     `json:"doNotTrack"`
	TouchPoints         int      `json:"touchPoints"`
	WebRTCPolicy        string   `json:"webrtcPolicy"`
	CanvasNoise         bool     `json:"canvasNoise"`
	AudioNoise          bool     `json:"audioNoise"`
}

const (
	ChromiumTargetOSWindows = "windows"
	ChromiumTargetOSMac     = "mac"
	ChromiumTargetOSLinux   = "linux"
)

var (
	chromiumFingerprintPoolOnce sync.Once
	chromiumFingerprintPool     *ChromiumFingerprintPool
	chromiumFingerprintPoolErr  error
)

// LoadChromiumFingerprintPool 加载内嵌 Chromium 指纹池，仅解析一次。
func LoadChromiumFingerprintPool() (*ChromiumFingerprintPool, error) {
	chromiumFingerprintPoolOnce.Do(func() {
		var pool ChromiumFingerprintPool
		if err := json.Unmarshal(chromiumFingerprintsRaw, &pool); err != nil {
			chromiumFingerprintPoolErr = fmt.Errorf("解析 Chromium 指纹池失败: %w", err)
			return
		}
		chromiumFingerprintPool = &pool
	})
	return chromiumFingerprintPool, chromiumFingerprintPoolErr
}

// NormalizeChromiumFingerprintOS 把配置/宿主平台别名归一成 Chromium 指纹池 key。
func NormalizeChromiumFingerprintOS(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "mac", "macos", "darwin":
		return ChromiumTargetOSMac
	case "linux":
		return ChromiumTargetOSLinux
	default:
		return ChromiumTargetOSWindows
	}
}

// SelectChromiumFingerprint 按 OS 和 seed 从指纹池选取一条指纹。
func (p *ChromiumFingerprintPool) SelectChromiumFingerprint(osName string, seed int) (ChromiumPoolEntry, error) {
	if p == nil {
		return ChromiumPoolEntry{}, fmt.Errorf("Chromium 指纹池为空")
	}
	osKey := NormalizeChromiumFingerprintOS(osName)
	items, ok := p.Pools[osKey]
	if !ok || len(items) == 0 {
		return ChromiumPoolEntry{}, fmt.Errorf("指纹池缺省 OS=%s 条目", osKey)
	}
	idx := seed % len(items)
	if idx < 0 {
		idx = -idx
	}
	return items[idx], nil
}

// StableFingerprintSeed 与 Chromium 启动链旧逻辑保持一致：按 profileID 生成稳定正整数。
func StableFingerprintSeed(profileID string) int {
	seed := 0
	for _, char := range profileID {
		seed = (seed << 5) - seed + int(char)
	}
	if seed < 0 {
		seed = -seed
	}
	return seed
}

// ResolveChromiumFingerprintSeed 从用户启动参数中解析 seed，缺省时回退到 profileID 稳定 seed。
func ResolveChromiumFingerprintSeed(profileID string, args []string) int {
	items := normalizeStringList(args)
	for i := 0; i < len(items); i++ {
		arg := strings.TrimSpace(items[i])
		value := ""
		if strings.HasPrefix(arg, "--fingerprint=") {
			value = strings.TrimSpace(strings.TrimPrefix(arg, "--fingerprint="))
		} else if arg == "--fingerprint" && i+1 < len(items) {
			next := strings.TrimSpace(items[i+1])
			if next != "" && !strings.HasPrefix(next, "-") {
				value = next
				i++
			}
		}
		if value != "" {
			if parsed, err := strconv.Atoi(value); err == nil {
				return parsed
			}
		}
	}
	return StableFingerprintSeed(profileID)
}

// ResolveChromiumFingerprintOS 从用户启动参数中解析平台，缺省时按宿主 OS 归一。
func ResolveChromiumFingerprintOS(args []string, hostOS string) string {
	items := normalizeStringList(args)
	for i := 0; i < len(items); i++ {
		arg := strings.TrimSpace(items[i])
		value := ""
		if strings.HasPrefix(arg, "--fingerprint-platform=") {
			value = strings.TrimPrefix(arg, "--fingerprint-platform=")
		} else if arg == "--fingerprint-platform" && i+1 < len(items) {
			next := strings.TrimSpace(items[i+1])
			if next != "" && !strings.HasPrefix(next, "-") {
				value = next
				i++
			}
		}
		if strings.TrimSpace(value) != "" {
			return NormalizeChromiumFingerprintOS(value)
		}
	}
	return NormalizeChromiumFingerprintOS(hostOS)
}
