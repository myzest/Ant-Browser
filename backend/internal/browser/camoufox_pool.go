package browser

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

//go:embed assets/camoufox_fingerprints.json
var camoufoxFingerprintsRaw []byte

// CamoufoxFingerprintPool 构建期预生成的指纹池。
type CamoufoxFingerprintPool struct {
	Version          int                            `json:"version"`
	Meta             camoufoxPoolMeta               `json:"meta"`
	FirefoxUserPrefs map[string]any                 `json:"firefoxUserPrefs"`
	Pools            map[string][]CamoufoxPoolEntry `json:"pools"`
}

type camoufoxPoolMeta struct {
	CountPerOS int               `json:"countPerOS"`
	OSList     []string          `json:"osList"`
	Generator  map[string]string `json:"generator"`
}

// CamoufoxPoolEntry 指纹池中单条指纹。
type CamoufoxPoolEntry struct {
	OS          string         `json:"os"`
	Seed        int            `json:"seed"`
	Hash        string         `json:"hash"`
	Fingerprint map[string]any `json:"fingerprint"`
	Runtime     map[string]any `json:"runtime"`
}

var (
	camoufoxFingerprintPoolOnce sync.Once
	camoufoxFingerprintPool     *CamoufoxFingerprintPool
	camoufoxFingerprintPoolErr  error
)

// LoadCamoufoxFingerprintPool 加载内嵌指纹池，仅解析一次。
func LoadCamoufoxFingerprintPool() (*CamoufoxFingerprintPool, error) {
	camoufoxFingerprintPoolOnce.Do(func() {
		var pool CamoufoxFingerprintPool
		if err := json.Unmarshal(camoufoxFingerprintsRaw, &pool); err != nil {
			camoufoxFingerprintPoolErr = fmt.Errorf("解析 Camoufox 指纹池失败: %w", err)
			return
		}
		camoufoxFingerprintPool = &pool
	})
	return camoufoxFingerprintPool, camoufoxFingerprintPoolErr
}

// AntCamoufoxOS 把 Ant 的 --fingerprint-platform 值映射到指纹池的键。
// 映射规则与原 Chromium 路线 defaultFingerprintArgsForOS 对齐：
//
//	darwin/windows/linux -> windows/macos/linux
func AntCamoufoxOS(fingerprintPlatform string) string {
	switch strings.ToLower(strings.TrimSpace(fingerprintPlatform)) {
	case "mac", "macos", "darwin":
		return CamoufoxTargetOSMac
	case "linux":
		return CamoufoxTargetOSLinux
	default: // 包含 windows / pc
		return CamoufoxTargetOSWindows
	}
}

const (
	CamoufoxTargetOSWindows = "windows"
	CamoufoxTargetOSMac     = "macos"
	CamoufoxTargetOSLinux   = "linux"
)

// SelectCamoufoxFingerprint 按 OS 和 seed 从指纹池选取一条指纹。
// seed 取模保证落在池范围内；池缺失时返回错误。
func (p *CamoufoxFingerprintPool) SelectCamoufoxFingerprint(osName string, seed int) (CamoufoxPoolEntry, error) {
	osKey := strings.ToLower(strings.TrimSpace(osName))
	if osKey == "" {
		osKey = CamoufoxTargetOSWindows
	}
	items, ok := p.Pools[osKey]
	if !ok || len(items) == 0 {
		return CamoufoxPoolEntry{}, fmt.Errorf("指纹池缺省 OS=%s 条目", osKey)
	}
	idx := seed % len(items)
	if idx < 0 {
		idx = -idx
	}
	return items[idx], nil
}
