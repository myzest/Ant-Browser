package backend

import (
	appconfig "ant-chrome/backend/internal/config"
	"path/filepath"
	"testing"
)

func TestLoadConfigRestoresLocalLicenseState(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.yaml")

	cfg := appconfig.DefaultConfig()
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("写入测试配置失败: %v", err)
	}
	if err := saveLocalLicenseState(configPath, &localLicenseState{
		MaxProfileLimit: appconfig.DefaultMaxProfileLimit + appconfig.StandardCDKeyProfileBonus*2,
		UsedCDKeys:      []string{"ANT-AAAA-BBBB-CCCC-DDDD-EEEEEEEE", "ANT-1111-2222-3333-4444-FFFFFFFF"},
	}); err != nil {
		t.Fatalf("写入本机额度状态失败: %v", err)
	}

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig 失败: %v", err)
	}

	if loaded.App.MaxProfileLimit != appconfig.DefaultMaxProfileLimit+appconfig.StandardCDKeyProfileBonus*2 {
		t.Fatalf("本机额度状态未恢复: got=%d", loaded.App.MaxProfileLimit)
	}
	if len(loaded.App.UsedCDKeys) != 2 {
		t.Fatalf("兑换记录未恢复: %+v", loaded.App.UsedCDKeys)
	}
}

func TestLoadConfigSeedsLocalLicenseStateFromConfig(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.yaml")

	cfg := appconfig.DefaultConfig()
	cfg.App.MaxProfileLimit = appconfig.DefaultMaxProfileLimit + appconfig.StandardCDKeyProfileBonus
	cfg.App.UsedCDKeys = []string{"ANT-AAAA-BBBB-CCCC-DDDD-EEEEEEEE"}
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("写入测试配置失败: %v", err)
	}

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig 失败: %v", err)
	}
	if loaded.App.MaxProfileLimit != appconfig.DefaultMaxProfileLimit+appconfig.StandardCDKeyProfileBonus {
		t.Fatalf("LoadConfig 读取额度失败: got=%d", loaded.App.MaxProfileLimit)
	}

	state, exists, err := loadLocalLicenseState(configPath)
	if err != nil {
		t.Fatalf("读取本机额度状态失败: %v", err)
	}
	if !exists {
		t.Fatalf("应当从现有配置补建本机额度状态")
	}
	if state.MaxProfileLimit != appconfig.DefaultMaxProfileLimit+appconfig.StandardCDKeyProfileBonus {
		t.Fatalf("本机额度状态未补建: got=%d", state.MaxProfileLimit)
	}
	if len(state.UsedCDKeys) != 1 || state.UsedCDKeys[0] != "ANT-AAAA-BBBB-CCCC-DDDD-EEEEEEEE" {
		t.Fatalf("本机兑换记录未补建: %+v", state.UsedCDKeys)
	}
}
