package backend

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"

	"ant-chrome/backend/internal/logger"
)

const widevineHintFileName = "latest-component-updated-widevine-cdm"

func seedWidevineHintIfAvailable(profileID string, userDataDir string, chromeBinaryPath string) (string, bool) {
	if goruntime.GOOS != "linux" {
		return "", false
	}
	if widevineSeedingDisabled() || strings.TrimSpace(userDataDir) == "" {
		return "", false
	}
	cdmDir := resolveWidevineCDMDir(chromeBinaryPath)
	if cdmDir == "" {
		return "", false
	}

	hintDir := filepath.Join(userDataDir, "WidevineCdm")
	if err := os.MkdirAll(hintDir, 0o755); err != nil {
		logger.New("Browser").Warn("Widevine hint 目录创建失败",
			logger.F("profile_id", profileID),
			logger.F("dir", hintDir),
			logger.F("error", err.Error()),
		)
		return "", false
	}
	hintPath := filepath.Join(hintDir, widevineHintFileName)
	payload, err := json.Marshal(map[string]string{"Path": cdmDir})
	if err != nil {
		return "", false
	}
	if current, err := os.ReadFile(hintPath); err == nil && string(current) == string(payload) {
		return cdmDir, true
	}
	if err := os.WriteFile(hintPath, payload, 0o644); err != nil {
		logger.New("Browser").Warn("Widevine hint 写入失败",
			logger.F("profile_id", profileID),
			logger.F("path", hintPath),
			logger.F("error", err.Error()),
		)
		return "", false
	}
	logger.New("Browser").Info("Widevine hint 已写入",
		logger.F("profile_id", profileID),
		logger.F("cdm_dir", cdmDir),
	)
	return cdmDir, true
}

func widevineSeedingDisabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("ANT_BROWSER_WIDEVINE"))) {
	case "0", "false", "off", "no":
		return true
	}
	return false
}

func resolveWidevineCDMDir(chromeBinaryPath string) string {
	if custom := strings.TrimSpace(os.Getenv("ANT_BROWSER_WIDEVINE_CDM")); custom != "" {
		return validWidevineCDMDir(custom)
	}
	if custom := strings.TrimSpace(os.Getenv("CLOAKBROWSER_WIDEVINE_CDM")); custom != "" {
		return validWidevineCDMDir(custom)
	}
	if chromeBinaryPath != "" {
		if dir := validWidevineCDMDir(filepath.Join(filepath.Dir(chromeBinaryPath), "WidevineCdm")); dir != "" {
			return dir
		}
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		for _, candidate := range []string{
			filepath.Join(home, ".ant-browser", "WidevineCdm"),
			filepath.Join(home, ".cloakbrowser", "WidevineCdm"),
		} {
			if dir := validWidevineCDMDir(candidate); dir != "" {
				return dir
			}
		}
	}
	return ""
}

func validWidevineCDMDir(candidate string) string {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return ""
	}
	abs, err := filepath.Abs(candidate)
	if err != nil {
		abs = candidate
	}
	manifest := filepath.Join(abs, "manifest.json")
	info, err := os.Stat(manifest)
	if err != nil || info.IsDir() {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return abs
}

func widevineRuntimeWarning(cdmDir string, seeded bool) string {
	if goruntime.GOOS != "linux" {
		return ""
	}
	if seeded && strings.TrimSpace(cdmDir) != "" {
		return fmt.Sprintf("已为 persistent profile 写入 Widevine hint；请用 EME/codec 审计确认 Widevine、H.264/AAC、plugins/mimeTypes 状态一致（CDM=%s）。", cdmDir)
	}
	return "未检测到可用 Widevine CDM；请勿将 profile 伪装成支持 Widevine/DRM 的完整 Chrome 环境，EME/codec/plugins 状态需在审计中保持一致。"
}
