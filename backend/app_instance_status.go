package backend

import (
	"ant-chrome/backend/internal/browser"
	"fmt"
	"strings"
)

func (a *App) BrowserInstanceStatus(profileId string) (*BrowserProfile, error) {
	a.browserMgr.Mutex.Lock()
	defer a.browserMgr.Mutex.Unlock()
	profile, exists := a.browserMgr.Profiles[profileId]
	if !exists {
		return nil, fmt.Errorf("profile not found")
	}
	snapshot := *profile
	if snapshot.RuntimeProtocol == "" {
		snapshot.RuntimeProtocol = a.browserMgr.RuntimeProtocolForProfile(&snapshot)
	}
	switch snapshot.RuntimeProtocol {
	case browser.RuntimeProtocolCDP:
		if snapshot.RuntimeEndpoint == "" && snapshot.DebugPort > 0 {
			snapshot.RuntimeEndpoint = fmt.Sprintf("http://127.0.0.1:%d", snapshot.DebugPort)
		}
		snapshot.PlaywrightEndpoint = ""
	case browser.RuntimeProtocolPlaywright:
		if snapshot.RuntimeEndpoint == "" {
			snapshot.RuntimeEndpoint = strings.TrimSpace(snapshot.PlaywrightEndpoint)
		}
		if snapshot.PlaywrightEndpoint == "" {
			snapshot.PlaywrightEndpoint = strings.TrimSpace(snapshot.RuntimeEndpoint)
		}
	}
	return &snapshot, nil
}

func (a *App) BrowserInstanceOpenUrl(profileId string, targetUrl string) bool {
	a.browserMgr.Mutex.Lock()
	profile, exists := a.browserMgr.Profiles[profileId]
	a.browserMgr.Mutex.Unlock()
	if !exists || !profile.Running {
		return false
	}
	return true
}

func (a *App) BrowserInstanceGetTabs(profileId string) []BrowserTab {
	return []BrowserTab{
		{TabId: "tab-1", Title: "新标签页", Url: "about:blank", Active: true},
		{TabId: "tab-2", Title: "示例站点", Url: "https://example.com", Active: false},
	}
}
