package backend

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"strings"
	"time"

	"ant-chrome/backend/internal/browser"
	"ant-chrome/backend/internal/logger"
	"encoding/json"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	camoufoxReadyMarker     = "CAMOUFOX_READY"
	camoufoxFailMarker      = "CAMOUFOX_FAIL"
	camoufoxLauncherStartup = 60 * time.Second
)

// camoufoxStartPayload 是 Node launcher 的输入文件，对应 camoufox_launcher.cjs。
type camoufoxStartPayload struct {
	// 节点运行时目录：launcher 会 require(runtimeDir/node_modules/playwright-core)
	RuntimeDir string `json:"runtimeDir"`
	// launcher 脚本所在目录，目前等于 RuntimeDir；保留字段以便将来分离
	LauncherDir string `json:"launcherDir,omitempty"`
	// 为 Linux 准备的 Camoufox fontconfig 路径；macOS/Windows 可空
	FontconfigPath string `json:"fontconfigPath,omitempty"`
	// 目标 OS 别名，仅用于分块大小与 linux 特殊处理
	OSName string `json:"osName,omitempty"`
	// Camoufox 启动参数：executablePath/userDataDir/proxy/headless/startUrls/CAMOU_CONFIG_*
	Config *browser.CamoufoxLaunchConfig `json:"config"`
}

// prepareCamoufoxStartPlan 为 Camoufox 内核构建启动计划。
// 流程：
//  1. 确保 automation runtime 已就绪（Node + playwright-core + launcher.cjs）
//  2. 解析代理 → Playwright proxy 字典
//  3. 以 profileId 算 seed（与原 Chromium 路线 buildBrowserLaunchArgs 对齐）
//  4. 从内嵌指纹池按 OS+seed 取一条指纹
//  5. 应用 profile.FingerprintArgs 覆盖
//  6. 编码 CAMOU_CONFIG_* 并写 launcher payload 临时文件
func (a *App) prepareCamoufoxStartPlan(input browserStartInput, profile *BrowserProfile, coreType string, camoufoxBinaryPath string, userDataDir string, sanitizedProfileLaunchArgs []string, sanitizedExtraLaunchArgs []string) (*browserStartPlan, error) {
	log := logger.New("Camoufox")

	ctx := context.Background()
	if a.automationMgr == nil {
		startErr := fmt.Errorf("实例启动失败：自动化运行时尚未初始化，无法拉起 Camoufox。 请先在 设置->自动化 中安装运行时。")
		if profile != nil {
			profile.LastError = startErr.Error()
		}
		return nil, startErr
	}
	if err := a.automationMgr.EnsureInstalled(ctx); err != nil {
		startErr := fmt.Errorf("实例启动失败：自动化运行时未就绪：%w", err)
		log.Error("自动化运行时未就绪", logger.F("profile_id", input.ProfileID), logger.F("error", err.Error()), logger.F("reason", startErr.Error()))
		if profile != nil {
			profile.LastError = startErr.Error()
		}
		return nil, startErr
	}
	state := a.automationMgr.CurrentState()
	if !state.Ready || strings.TrimSpace(state.NodePath) == "" || strings.TrimSpace(state.CamoufoxLauncherPath) == "" {
		startErr := fmt.Errorf("实例启动失败：自动化运行时缺少 Node 或 Camoufox 启动器。 请先在 设置->自动化 中安装运行时。")
		log.Error("Camoufox launcher 不可用", logger.F("profile_id", input.ProfileID), logger.F("ready", state.Ready), logger.F("node", state.NodePath), logger.F("launcher", state.CamoufoxLauncherPath))
		if profile != nil {
			profile.LastError = startErr.Error()
		}
		return nil, startErr
	}

	if err := os.MkdirAll(userDataDir, 0o755); err != nil {
		startErr := fmt.Errorf("实例启动失败：无法创建用户数据目录 %s。原因：%w。 请检查目录权限或路径配置。", userDataDir, err)
		log.Error("用户数据目录创建失败", logger.F("profile_id", input.ProfileID), logger.F("dir", userDataDir), logger.F("error", err.Error()))
		if profile != nil {
			profile.LastError = startErr.Error()
		}
		return nil, startErr
	}

	effectiveProxy, acquiredXrayBridgeKey, releaseXrayBridge, err := a.resolveBrowserStartProxy(input, profile)
	if err != nil {
		return nil, err
	}
	releaseAcquiredBridge := func() {
		if releaseXrayBridge && acquiredXrayBridgeKey != "" && a.xrayMgr != nil {
			a.xrayMgr.ReleaseBridge(acquiredXrayBridgeKey)
		}
	}
	proxy := buildCamoufoxProxy(effectiveProxy)

	startURLs := normalizeNonEmptyStrings(input.StartURLs)
	if len(startURLs) == 0 && !input.SkipDefaultStartURLs {
		startURLs = normalizeNonEmptyStrings(a.browserDefaultStartURLs())
	}
	if len(startURLs) == 0 && !browserRestoreLastSession(a.config) {
		startURLs = []string{"about:blank"}
	}

	overrides := camoufoxCombinedOverrides(profile, sanitizedProfileLaunchArgs, sanitizedExtraLaunchArgs)
	targetOS := antCamoufoxOSTarget(overrides)
	seed := camoufoxFingerprintSeed(profile.ProfileId, overrides)

	// 默认以有头模式启动 Camoufox；验收需要打开 playground，
	// 后续如需无头可在实例配置中显式控制。
	headless := false

	launchConfig, err := browser.BuildCamoufoxLaunchConfig(
		camoufoxBinaryPath,
		userDataDir,
		proxy,
		headless,
		startURLs,
		targetOS,
		seed,
		overrides,
		append(append([]string{}, sanitizedProfileLaunchArgs...), sanitizedExtraLaunchArgs...),
		nil,
	)
	if err != nil {
		releaseAcquiredBridge()
		startErr := fmt.Errorf("实例启动失败：%w", err)
		log.Error("Camoufox 启动参数构建失败", logger.F("profile_id", input.ProfileID), logger.F("error", err.Error()))
		if profile != nil {
			profile.LastError = startErr.Error()
		}
		return nil, startErr
	}

	payload := camoufoxStartPayload{
		RuntimeDir: state.RuntimeDir,
		OSName:     launchConfig.OSName,
		Config:     launchConfig,
	}
	payloadPath, err := writeCamoufoxStartPayload(payload)
	if err != nil {
		releaseAcquiredBridge()
		startErr := fmt.Errorf("实例启动失败：写入 Camoufox 启动参数失败：%w", err)
		if profile != nil {
			profile.LastError = startErr.Error()
		}
		return nil, startErr
	}

	startReadyTimeout, startStableWindow := a.browserStartTimingSettings()
	totalReadyTimeout := camoufoxLauncherStartup
	if startReadyTimeout > 0 {
		totalReadyTimeout = startReadyTimeout * 2
	}

	if profile != nil {
		profile.RuntimeProtocol = browser.RuntimeProtocolPlaywright
	}

	return &browserStartPlan{
		profile:               profile,
		chromeBinaryPath:      camoufoxBinaryPath,
		coreType:              coreType,
		runtimeProtocol:       browser.RuntimeProtocolPlaywright,
		camoufoxConfig:        launchConfig,
		camoufoxPayloadPath:   payloadPath,
		camoufoxNodePath:      state.NodePath,
		camoufoxLauncherPath:  state.CamoufoxLauncherPath,
		userDataDir:           userDataDir,
		effectiveProxy:        effectiveProxy,
		acquiredXrayBridgeKey: acquiredXrayBridgeKey,
		releaseXrayBridge:     releaseXrayBridge,
		startReadyTimeout:     startReadyTimeout,
		startStableWindow:     startStableWindow,
		maxStartAttempts:      1,
		totalReadyTimeout:     totalReadyTimeout,
	}, nil
}

// startCamoufoxProfileWithPlan 拉起 Camoufox Node launcher 并解析 stdout 协议，
// 成功后设置 PlaywrightEndpoint/RuntimeEndpoint 并调用 markProfileRunningLocked。
func (a *App) startCamoufoxProfileWithPlan(input browserStartInput, plan *browserStartPlan) (*BrowserProfile, error) {
	log := logger.New("Camoufox")
	profile := plan.profile

	readyCtx, cancel := context.WithTimeout(context.Background(), plan.totalReadyTimeout+10*time.Second)
	defer cancel()

	// 不能使用 exec.CommandContext 绑定 readyCtx：启动成功返回后 defer cancel() 会杀掉常驻 launcher。
	// Camoufox launcher 是实例本身的宿主进程，必须由 StopInstance/进程监控显式回收。
	cmd := exec.Command(plan.camoufoxNodePath, plan.camoufoxLauncherPath, plan.camoufoxPayloadPath)
	cmd.Dir = plan.userDataDir

	// stdout 单独取用于协议解析；stderr 合并到 stderrWriter 用于错误诊断。
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		startErr := fmt.Errorf("实例启动失败：创建 Camoufox 进程 stdout 失败：%w", err)
		log.Error("Camoufox 进程 stdout 失败", logger.F("profile_id", input.ProfileID), logger.F("error", err.Error()))
		if profile != nil {
			profile.LastError = startErr.Error()
		}
		_ = os.Remove(plan.camoufoxPayloadPath)
		return profile, startErr
	}
	var stderrBuf strings.Builder
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		startErr := fmt.Errorf("实例启动失败：无法启动 Camoufox 子进程：%w", err)
		log.Error("Camoufox 子进程启动失败", logger.F("profile_id", input.ProfileID), logger.F("error", err.Error()))
		if profile != nil {
			profile.LastError = startErr.Error()
		}
		_ = os.Remove(plan.camoufoxPayloadPath)
		return profile, startErr
	}

	wsEndpoint, readyErr := waitForCamoufoxReady(readyCtx, stdoutPipe, plan.totalReadyTimeout)
	if readyErr != nil {
		_ = killCamoufoxProcess(cmd)
		startErr := fmt.Errorf("实例启动失败：%w", readyErr)
		log.Error("Camoufox 启动未就绪",
			logger.F("profile_id", input.ProfileID),
			logger.F("binary", plan.chromeBinaryPath),
			logger.F("stderr", stderrBuf.String()),
			logger.F("error", readyErr.Error()),
		)
		if profile != nil {
			profile.LastError = startErr.Error()
		}
		_ = os.Remove(plan.camoufoxPayloadPath)
		return profile, startErr
	}

	pid := 0
	if cmd.Process != nil {
		pid = cmd.Process.Pid
	}

	// 保持 launcher 子进程常驻；释放 payload 临时文件
	_ = os.Remove(plan.camoufoxPayloadPath)
	plan.camoufoxPayloadPath = ""

	// 后台持续 drain launcher stdout，防止管道缓冲区满后 launcher 被阻塞死锁。
	// waitForCamoufoxReady 的读取 goroutine 在拿到 CAMOUFOX_READY 后已退出，
	// 但 launcher 进程仍常驻，如果后续向 stdout 写出数据无人读取会导致死锁。
	go func() { _, _ = io.Copy(io.Discard, stdoutPipe) }()

	if profile != nil {
		profile.RuntimeProtocol = browser.NormalizeRuntimeProtocol(profile.RuntimeProtocol)
		if profile.RuntimeProtocol == "" {
			profile.RuntimeProtocol = browser.RuntimeProtocolPlaywright
		}
		profile.PlaywrightEndpoint = wsEndpoint
		profile.RuntimeEndpoint = wsEndpoint
	}

	a.markProfileRunningLocked(input.ProfileID, profile, cmd, pid, 0, true, "")

	if plan.acquiredXrayBridgeKey != "" {
		a.bindProfileXrayBridge(input.ProfileID, plan.acquiredXrayBridgeKey)
		plan.releaseXrayBridge = false
	}

	log.Info("Camoufox 实例启动",
		logger.F("profile_id", input.ProfileID),
		logger.F("os", plan.camoufoxConfig.OSName),
		logger.F("pid", pid),
		logger.F("proxy", plan.effectiveProxy),
		logger.F("ws_endpoint", wsEndpoint),
	)

	a.emitBrowserInstanceStarted(profile, false)

	// 后台监管 launcher 子进程退出
	go a.waitCamoufoxLauncherProcess(input.ProfileID, cmd, wsEndpoint)

	return profile, nil
}

// waitCamoufoxLauncherProcess 会在 launcher 子进程退出时把 profile 标记为已停止。
func (a *App) waitCamoufoxLauncherProcess(profileID string, cmd *exec.Cmd, wsEndpoint string) {
	_ = cmd.Wait()
	profileName := profileID
	stoppedByMonitor := false
	a.browserMgr.Mutex.Lock()
	profile, exists := a.browserMgr.Profiles[profileID]
	if exists && profile != nil {
		profileName = profile.ProfileName
		if profile.PlaywrightEndpoint == wsEndpoint {
			a.markProfileStoppedLocked(profileID, profile)
			stoppedByMonitor = true
		}
	}
	a.browserMgr.Mutex.Unlock()
	logger.New("Camoufox").Info("Camoufox 进程退出", logger.F("profile_id", profileID), logger.F("profile_name", profileName))
	if stoppedByMonitor && a.ctx != nil {
		// 复用 Chromium 路线事件通道，与 runtime_events 保持一致。
		wailsruntime.EventsEmit(a.ctx, "browser:instance:stopped", profileID)
	}
}

// waitForCamoufoxReady 从 launcher stdout 逐行读取直到匹配 CAMOUFOX_READY <wsEndpoint>，
// 或收到 CAMOUFOX_FAIL <msg> 与超时。 出错时把 stderr 带回上层。
func waitForCamoufoxReady(ctx context.Context, stdout io.Reader, timeout time.Duration) (string, error) {
	readyC := make(chan string, 1)
	errC := make(chan string, 1)
	reader := bufio.NewReaderSize(stdout, 64*1024)

	go func() {
		for {
			line, err := reader.ReadString('\n')
			line = strings.TrimRight(line, "\r\n")
			if strings.HasPrefix(line, camoufoxReadyMarker+" ") {
				readyC <- strings.TrimSpace(strings.TrimPrefix(line, camoufoxReadyMarker+" "))
				return
			}
			if strings.HasPrefix(line, camoufoxFailMarker+" ") {
				errC <- strings.TrimSpace(strings.TrimPrefix(line, camoufoxFailMarker+" "))
				return
			}
			if err != nil {
				errC <- "Camoufox launcher 输出已结束，但未收到就绪信号。"
				return
			}
		}
	}()

	select {
	case ws := <-readyC:
		if strings.TrimSpace(ws) == "" {
			return "", fmt.Errorf("Camoufox launcher 返回空的 wsEndpoint")
		}
		return ws, nil
	case msg := <-errC:
		return "", fmt.Errorf("%s", msg)
	case <-time.After(timeout):
		return "", fmt.Errorf("Camoufox launcher 在 %s 内未就绪", formatBrowserWaitWindow(timeout))
	case <-ctx.Done():
		return "", fmt.Errorf("Camoufox launcher 上下文已取消")
	}
}

// killCamoufoxProcess 优雅终止 launcher 进程。
func killCamoufoxProcess(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	_ = cmd.Process.Signal(os.Interrupt)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
		return nil
	case <-time.After(3 * time.Second):
		_ = cmd.Process.Kill()
		return <-done
	}
}

// writeCamoufoxStartPayload 把 launcher 输入写入临时 JSON 文件。
func writeCamoufoxStartPayload(payload camoufoxStartPayload) (string, error) {
	tempDir := filepath.Join(os.TempDir(), "ant-browser-camoufox")
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return "", fmt.Errorf("创建 Camoufox 启动参数目录失败：%w", err)
	}
	f, err := os.CreateTemp(tempDir, "camoufox-*.json")
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err := json.NewEncoder(f).Encode(payload); err != nil {
		return "", err
	}
	return f.Name(), nil
}

// buildCamoufoxProxy 把 effectiveProxy 字符串 (http://user:pass@host:port/socks5://host:port/direct://)
// 转换成 Playwright 自动化代理字典；direct 返回 nil 以避免 Playwright 空代理差异。
// Playwright 的 proxy.server 不支持 URL 内嵌认证信息（user:pass@），
// 认证必须通过 username/password 单独设。
func buildCamoufoxProxy(effectiveProxy string) map[string]string {
	proxy := strings.TrimSpace(effectiveProxy)
	if proxy == "" || proxy == "direct://" || proxy == "direct" {
		return nil
	}

	// 尝试解析 URL，提取 server（去掉 userinfo）和认证字段
	parsed, err := url.Parse(proxy)
	if err != nil || parsed.Host == "" {
		// 非 URL 格式（如 "host:port"），直接作为 server
		return map[string]string{"server": proxy}
	}

	result := map[string]string{"server": fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host)}
	if parsed.User != nil {
		if u := parsed.User.Username(); u != "" {
			result["username"] = u
		}
		if pw, ok := parsed.User.Password(); ok && pw != "" {
			result["password"] = pw
		}
	}
	return result
}

// camoufoxCombinedOverrides 汇总原 Chromium 路线中会参与启动的三类参数：
// FingerprintArgs + Profile LaunchArgs + 一次性 ExtraLaunchArgs。
// Camoufox 只消费其中会影响指纹一致性的已知参数。
func camoufoxCombinedOverrides(profile *BrowserProfile, sanitizedProfileLaunchArgs []string, sanitizedExtraLaunchArgs []string) []string {
	items := []string{}
	if profile != nil {
		items = append(items, profile.FingerprintArgs...)
	}
	items = append(items, sanitizedProfileLaunchArgs...)
	items = append(items, sanitizedExtraLaunchArgs...)
	return normalizeNonEmptyStrings(items)
}

// antCamoufoxOSTarget 从参数中决定 Camoufox 指纹池 OS：
//  1. --fingerprint-platform 优先（与原 Chromium 路线一致）
//  2. 否则按 --user-agent 推断 OS，避免 UA 与 fonts/webgl/screen 池不一致
//  3. 最后按宿主 OS
func antCamoufoxOSTarget(args []string) string {
	for _, raw := range normalizeNonEmptyStrings(args) {
		arg := strings.TrimSpace(raw)
		if strings.HasPrefix(arg, "--fingerprint-platform=") {
			platform := strings.TrimSpace(strings.TrimPrefix(arg, "--fingerprint-platform="))
			return browser.AntCamoufoxOS(platform)
		}
	}
	for _, raw := range normalizeNonEmptyStrings(args) {
		arg := strings.TrimSpace(raw)
		if strings.HasPrefix(arg, "--user-agent=") {
			if osName := camoufoxOSFromUserAgent(strings.TrimSpace(strings.TrimPrefix(arg, "--user-agent="))); osName != "" {
				return osName
			}
		}
	}
	return browser.AntCamoufoxOS(goruntime.GOOS)
}

func camoufoxOSFromUserAgent(ua string) string {
	uaLower := strings.ToLower(ua)
	switch {
	case strings.Contains(uaLower, "windows nt"):
		return browser.CamoufoxTargetOSWindows
	case strings.Contains(uaLower, "macintosh"), strings.Contains(uaLower, "mac os x"):
		return browser.CamoufoxTargetOSMac
	case strings.Contains(uaLower, "linux"):
		return browser.CamoufoxTargetOSLinux
	default:
		return ""
	}
}

// camoufoxFingerprintSeed 与原 Chromium 路线对齐：
//   - 显式 --fingerprint=N 时优先使用 N
//   - 否则按 profileId 使用 buildBrowserLaunchArgs 同款 hash seed
func camoufoxFingerprintSeed(profileID string, args []string) int {
	for _, raw := range normalizeNonEmptyStrings(args) {
		arg := strings.TrimSpace(raw)
		if strings.HasPrefix(arg, "--fingerprint=") {
			value := strings.TrimSpace(strings.TrimPrefix(arg, "--fingerprint="))
			if parsed, err := strconv.Atoi(value); err == nil {
				if parsed < 0 {
					return -parsed
				}
				return parsed
			}
		}
	}
	return profileFingerprintSeed(profileID)
}

// profileFingerprintSeed 与原 Chromium 路线 buildBrowserLaunchArgs 相同的 seed 算法。
func profileFingerprintSeed(profileID string) int {
	seed := 0
	for _, ch := range profileID {
		seed = (seed << 5) - seed + int(ch)
	}
	if seed < 0 {
		seed = -seed
	}
	return seed
}
