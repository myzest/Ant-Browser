package backend

import (
	"ant-chrome/backend/internal/browser"
	"ant-chrome/backend/internal/fingerprint"
	"ant-chrome/backend/internal/logger"
	"ant-chrome/backend/internal/proxy"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type browserStartInput struct {
	ProfileID            string
	ExtraLaunchArgs      []string
	StartURLs            []string
	SkipDefaultStartURLs bool
	PreferVisibleWindow  bool
	ForceDirectProxy     bool
	RequireDebugBridge   bool
	TemporaryProxyID     string
	TemporaryProxyConfig string
}

type browserStartPlan struct {
	profile               *BrowserProfile
	chromeBinaryPath      string
	userDataDir           string
	args                  []string
	effectiveProxy        string
	exitIPCacheKey        string
	acquiredXrayBridgeKey string
	releaseXrayBridge     bool
	assignedDebugPort     int
	requireDebugBridge    bool
	startReadyTimeout     time.Duration
	startStableWindow     time.Duration
	maxStartAttempts      int
	totalReadyTimeout     time.Duration
}

var resolveFingerprintExitIP = proxy.ResolveExitIPWithCacheKey

func newBrowserStartInput(profileID string, extraLaunchArgs []string, startURLs []string, skipDefaultStartURLs bool, preferVisibleWindow bool, forceDirectProxy bool, requireDebugBridge bool, proxyID string, proxyConfig string) browserStartInput {
	normalizedExtraLaunchArgs := normalizeNonEmptyStrings(extraLaunchArgs)
	if preferVisibleWindow {
		normalizedExtraLaunchArgs = ensureNewWindowLaunchArg(normalizedExtraLaunchArgs)
	}

	return browserStartInput{
		ProfileID:            profileID,
		ExtraLaunchArgs:      normalizedExtraLaunchArgs,
		StartURLs:            normalizeNonEmptyStrings(startURLs),
		SkipDefaultStartURLs: skipDefaultStartURLs,
		PreferVisibleWindow:  preferVisibleWindow,
		ForceDirectProxy:     forceDirectProxy,
		RequireDebugBridge:   requireDebugBridge,
		TemporaryProxyID:     strings.TrimSpace(proxyID),
		TemporaryProxyConfig: strings.TrimSpace(proxyConfig),
	}
}

func (input browserStartInput) hasTemporaryProxy() bool {
	return strings.TrimSpace(input.TemporaryProxyID) != "" || strings.TrimSpace(input.TemporaryProxyConfig) != ""
}

func (plan *browserStartPlan) releaseBridgeIfNeeded(a *App) {
	if plan == nil || a == nil {
		return
	}
	if plan.releaseXrayBridge && plan.acquiredXrayBridgeKey != "" && a.xrayMgr != nil {
		a.xrayMgr.ReleaseBridge(plan.acquiredXrayBridgeKey)
	}
}

func (a *App) resolveBrowserStartProfile(input browserStartInput) (*BrowserProfile, bool, error) {
	log := logger.New("Browser")

	profile, exists := a.browserMgr.Profiles[input.ProfileID]
	if !exists {
		err := fmt.Errorf("实例启动失败：未找到实例配置（ID=%s）。请刷新列表后重试。", input.ProfileID)
		log.Error("实例不存在", logger.F("profile_id", input.ProfileID), logger.F("reason", err.Error()))
		return nil, false, err
	}

	if !profile.Running {
		return profile, false, nil
	}

	if !isBrowserProfileLive(profile, a.browserMgr.BrowserProcesses[input.ProfileID]) {
		log.Info("检测到实例运行状态已失效，准备重新启动",
			logger.F("profile_id", input.ProfileID),
			logger.F("pid", profile.Pid),
			logger.F("debug_port", profile.DebugPort),
		)
		a.markProfileStoppedLocked(input.ProfileID, profile)
		return profile, false, nil
	}

	if input.RequireDebugBridge && (profile.DebugPort <= 0 || !profile.DebugReady) {
		err := fmt.Errorf("实例已在无调试接管模式下运行；如需 Launch API、Cookie 或自动化，请先停止实例后再通过对应功能启动。")
		logMessage := "运行中实例缺少调试接管能力"
		if profile.DebugPort > 0 && !profile.DebugReady {
			err = fmt.Errorf("实例调试接口仍在就绪中；请稍后重试，或先停止实例后再通过对应功能启动。")
			logMessage = "运行中实例调试接口尚未就绪"
		}
		log.Warn(logMessage,
			logger.F("profile_id", input.ProfileID),
			logger.F("pid", profile.Pid),
			logger.F("debug_port", profile.DebugPort),
			logger.F("debug_ready", profile.DebugReady),
			logger.F("reason", err.Error()),
		)
		profile.LastError = err.Error()
		return profile, true, err
	}

	if input.PreferVisibleWindow {
		if err := a.openBrowserWindowForRunningProfile(profile, input.ExtraLaunchArgs, input.StartURLs); err != nil {
			startErr := fmt.Errorf("实例已在运行，但窗口唤起失败：%w", err)
			log.Error("运行中实例窗口唤起失败",
				logger.F("profile_id", input.ProfileID),
				logger.F("debug_port", profile.DebugPort),
				logger.F("error", err.Error()),
				logger.F("reason", startErr.Error()),
			)
			profile.LastError = startErr.Error()
			return profile, true, startErr
		}
	}

	if a.launchServer != nil && profile.DebugReady {
		a.launchServer.SetActiveProfile(profile)
	}
	a.emitBrowserInstanceStarted(profile, true)
	return profile, true, nil
}

func (a *App) prepareBrowserStartPlan(input browserStartInput, profile *BrowserProfile) (*browserStartPlan, error) {
	sanitizedProfileLaunchArgs, sanitizedExtraLaunchArgs, chromeBinaryPath, userDataDir, err := a.prepareBrowserLaunchContext(input, profile)
	if err != nil {
		return nil, err
	}

	effectiveProxy, exitIPCacheKey, acquiredXrayBridgeKey, releaseXrayBridge, err := a.resolveBrowserStartProxy(input, profile)
	if err != nil {
		return nil, err
	}

	startReadyTimeout, startStableWindow := a.browserStartTimingSettings()
	maxStartAttempts := browserStartAttemptCount()
	totalReadyTimeout := time.Duration(maxStartAttempts) * startReadyTimeout

	assignedDebugPort := 0
	if input.RequireDebugBridge {
		var err error
		assignedDebugPort, err = nextAvailablePort()
		if err != nil {
			startErr := fmt.Errorf("实例启动失败：本地调试端口分配失败。原因：%v。请关闭占用端口的程序后重试。", err)
			logger.New("Browser").Error("调试端口分配失败",
				logger.F("profile_id", input.ProfileID),
				logger.F("error", err.Error()),
				logger.F("reason", startErr.Error()),
			)
			profile.LastError = startErr.Error()
			return nil, startErr
		}
	}

	launchArgs := buildBrowserLaunchArgs(profile, userDataDir, assignedDebugPort, effectiveProxy, sanitizedProfileLaunchArgs, sanitizedExtraLaunchArgs, input.StartURLs, a.browserDefaultStartURLs(), input.SkipDefaultStartURLs, browserRestoreLastSession(a.config))
	launchArgs = a.resolveAutoWebRTCIPLaunchArgWithCacheKey(input.ProfileID, launchArgs, effectiveProxy, exitIPCacheKey)

	return &browserStartPlan{
		profile:               profile,
		chromeBinaryPath:      chromeBinaryPath,
		userDataDir:           userDataDir,
		args:                  launchArgs,
		effectiveProxy:        effectiveProxy,
		exitIPCacheKey:        exitIPCacheKey,
		acquiredXrayBridgeKey: acquiredXrayBridgeKey,
		releaseXrayBridge:     releaseXrayBridge,
		assignedDebugPort:     assignedDebugPort,
		requireDebugBridge:    input.RequireDebugBridge,
		startReadyTimeout:     startReadyTimeout,
		startStableWindow:     startStableWindow,
		maxStartAttempts:      maxStartAttempts,
		totalReadyTimeout:     totalReadyTimeout,
	}, nil
}

func (a *App) prepareBrowserLaunchContext(input browserStartInput, profile *BrowserProfile) ([]string, []string, string, string, error) {
	log := logger.New("Browser")

	sanitizedProfileLaunchArgs, managedProfileArgs := sanitizeManagedLaunchArgs(profile.LaunchArgs)
	sanitizedExtraLaunchArgs, managedExtraArgs := sanitizeManagedLaunchArgs(input.ExtraLaunchArgs)
	logManagedLaunchArgOverrides(log, input.ProfileID, "profile.launchArgs", managedProfileArgs)
	logManagedLaunchArgOverrides(log, input.ProfileID, "start.extraLaunchArgs", managedExtraArgs)

	proxyChanged := a.browserMgr.ApplyDefaults(profile)
	if proxyChanged {
		_ = a.browserMgr.SaveProfiles()
	}

	chromeBinaryPath, err := a.browserMgr.ResolveChromeBinary(profile)
	if err != nil {
		startErr := fmt.Errorf("实例启动失败：%w", err)
		log.Error("内核路径解析失败",
			logger.F("profile_id", input.ProfileID),
			logger.F("error", err.Error()),
			logger.F("reason", startErr.Error()),
		)
		profile.LastError = startErr.Error()
		return nil, nil, "", "", startErr
	}

	userDataDir := a.browserMgr.ResolveUserDataDir(profile)
	if err := os.MkdirAll(userDataDir, 0o755); err != nil {
		startErr := fmt.Errorf("实例启动失败：无法创建用户数据目录 %s。原因：%w。请检查目录权限或路径配置。", userDataDir, err)
		log.Error("用户数据目录创建失败",
			logger.F("profile_id", input.ProfileID),
			logger.F("dir", userDataDir),
			logger.F("error", err.Error()),
			logger.F("reason", startErr.Error()),
		)
		profile.LastError = startErr.Error()
		return nil, nil, "", "", startErr
	}

	if err := browser.EnsureDefaultBookmarks(userDataDir, a.BookmarkList()); err != nil {
		log.Error("默认书签写入失败", logger.F("error", err.Error()))
	}

	if !browserRestoreLastSession(a.config) {
		if err := browser.ClearSessionRestoreData(userDataDir); err != nil {
			sessionDir := filepath.Join(userDataDir, "Default", "Sessions")
			startErr := fmt.Errorf("实例启动失败：无法清理上次会话缓存 %s。原因：%w。请关闭占用该目录的浏览器进程后重试。", sessionDir, err)
			log.Error("会话恢复缓存清理失败",
				logger.F("profile_id", input.ProfileID),
				logger.F("dir", sessionDir),
				logger.F("error", err.Error()),
				logger.F("reason", startErr.Error()),
			)
			profile.LastError = startErr.Error()
			return nil, nil, "", "", startErr
		}
	}

	return sanitizedProfileLaunchArgs, sanitizedExtraLaunchArgs, chromeBinaryPath, userDataDir, nil
}

func buildBrowserLaunchArgs(profile *BrowserProfile, userDataDir string, debugPort int, effectiveProxy string, sanitizedProfileLaunchArgs []string, sanitizedExtraLaunchArgs []string, startURLs []string, defaultStartURLs []string, skipDefaultStartURLs bool, restoreLastSession bool) []string {
	args := []string{
		fmt.Sprintf("--user-data-dir=%s", userDataDir),
		"--disable-session-crashed-bubble",
	}
	if debugPort > 0 {
		args = append(args,
			fmt.Sprintf("--remote-debugging-port=%d", debugPort),
			"--remote-debugging-address=127.0.0.1",
		)
	}

	hasFingerprint := false
	for _, arg := range profile.FingerprintArgs {
		if strings.HasPrefix(arg, "--fingerprint=") {
			hasFingerprint = true
			break
		}
	}
	autoFingerprintArgs := []string{}
	if !hasFingerprint {
		autoFingerprintArgs = append(autoFingerprintArgs, fingerprint.SeedArg(profile.ProfileId))
	}

	if effectiveProxy == "direct://" {
		args = append(args, "--proxy-server=direct://")
	} else if effectiveProxy != "" {
		args = append(args, fmt.Sprintf("--proxy-server=%s", effectiveProxy))
	}

	fingerprintArgs := mergeLaunchArgs(autoFingerprintArgs, profile.FingerprintArgs, sanitizedProfileLaunchArgs, sanitizedExtraLaunchArgs)
	fingerprintArgs = ensureDefaultFingerprintNetworkArgs(fingerprintArgs, effectiveProxy)
	args = append(args, fingerprintArgs...)
	return appendLaunchTargets(args, startURLs, defaultStartURLs, skipDefaultStartURLs, restoreLastSession)
}

func (a *App) resolveAutoWebRTCIPLaunchArg(profileID string, args []string, effectiveProxy string) []string {
	return a.resolveAutoWebRTCIPLaunchArgWithCacheKey(profileID, args, effectiveProxy, "")
}

func (a *App) resolveAutoWebRTCIPLaunchArgWithCacheKey(profileID string, args []string, effectiveProxy string, cacheKey string) []string {
	index := -1
	valueIndex := -1
	for i, arg := range args {
		key, ok := singleValueLaunchArgKey(arg)
		value, candidateValueIndex := launchArgValueAt(args, i)
		if ok && key == "--fingerprint-webrtc-ip" && strings.EqualFold(strings.TrimSpace(value), "auto") {
			index = i
			valueIndex = candidateValueIndex
			break
		}
	}
	if index < 0 {
		return args
	}

	log := logger.New("Browser")
	if !shouldResolveFingerprintWebRTCIP(effectiveProxy) {
		log.Warn("WebRTC 出口 IP 自动解析已跳过：当前为直连",
			logger.F("profile_id", profileID),
		)
		return removeLaunchArgValueAt(args, index, valueIndex)
	}

	ip, err := resolveFingerprintExitIP(effectiveProxy, cacheKey, 5*time.Second)
	if err != nil || strings.TrimSpace(ip) == "" {
		if err != nil {
			log.Warn("WebRTC 出口 IP 自动解析失败，已移除 auto 参数",
				logger.F("profile_id", profileID),
				logger.F("proxy", proxy.RedactProxyURL(effectiveProxy)),
				logger.F("error", err.Error()),
			)
		}
		return removeLaunchArgValueAt(args, index, valueIndex)
	}

	args = replaceLaunchArgValueAt(args, index, valueIndex, "--fingerprint-webrtc-ip="+strings.TrimSpace(ip))
	log.Info("WebRTC 出口 IP 自动解析成功",
		logger.F("profile_id", profileID),
		logger.F("ip", ip),
	)
	return args
}

func launchArgValueAt(args []string, index int) (string, int) {
	if index < 0 || index >= len(args) {
		return "", index
	}
	arg := strings.TrimSpace(args[index])
	if _, value, ok := strings.Cut(arg, "="); ok {
		return value, index
	}
	if index+1 < len(args) {
		next := strings.TrimSpace(args[index+1])
		if next != "" && !strings.HasPrefix(next, "-") {
			return next, index + 1
		}
	}
	return "", index
}

func replaceLaunchArgValueAt(args []string, index int, valueIndex int, replacement string) []string {
	if index < 0 || index >= len(args) {
		return args
	}
	out := append([]string{}, args...)
	out[index] = replacement
	if valueIndex > index && valueIndex < len(out) {
		out = removeLaunchArgAt(out, valueIndex)
	}
	return out
}

func removeLaunchArgValueAt(args []string, index int, valueIndex int) []string {
	if index < 0 || index >= len(args) {
		return args
	}
	if valueIndex > index && valueIndex < len(args) {
		args = removeLaunchArgAt(args, valueIndex)
	}
	return removeLaunchArgAt(args, index)
}

func removeLaunchArgAt(args []string, index int) []string {
	if index < 0 || index >= len(args) {
		return args
	}
	out := make([]string, 0, len(args)-1)
	out = append(out, args[:index]...)
	out = append(out, args[index+1:]...)
	return out
}
