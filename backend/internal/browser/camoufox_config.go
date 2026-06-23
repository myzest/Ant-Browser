package browser

/* Camoufox 配置编码。
 *
 * 运行时纯 Go/Node，零 Python 依赖：
 *   1. 从内嵌指纹池取一条 (navigator/screen/window/WebGL/canvas/fonts 已生成)
 *   2. 应用 Ant FingerprintArgs 覆盖：--user-agent / --lang / --window-size 等
 *   3. JSON 序列化 + 按 OS 分块 -> CAMOU_CONFIG_1 / CAMOU_CONFIG_2 / ...
 *   4. 交给 Node launcher 用 playwright-core.firefox.launchPersistentContext 拉起 Camoufox 二进制
 *
 * 分块规则对照 camoufox/utils.py:get_env_vars：
 *   windows -> 2047 字节
 *   macos/linux -> 32767 字节
 */

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// CamoufoxLaunchConfig 是传给 Node launcher 的完整负载。
type CamoufoxLaunchConfig struct {
	// Camoufox Firefox 可执行文件绝对路径。
	ExecutablePath string `json:"executablePath"`
	// 实例用户数据目录；launcher 以 launchPersistentContext(userDataDir, options) 使用。
	UserDataDir string `json:"userDataDir"`
	// 是否无头。
	Headless bool `json:"headless"`
	// Playwright proxy 字典 {server, username?, password?}，为空表示直连。
	Proxy map[string]string `json:"proxy,omitempty"`
	// 启动 URL 列表，由 Node launcher 在 context 就绪后 goto。
	StartURLs []string `json:"startUrls,omitempty"`
	// 透传给 Firefox/Camoufox 的非托管启动参数。
	ExtraArgs []string `json:"extraArgs,omitempty"`
	// CAMOU_CONFIG_* 环境变量字典。
	CamouConfig map[string]string `json:"camouConfig"`
	// Firefox 用户偏好。
	FirefoxUserPrefs map[string]any `json:"firefoxUserPrefs,omitempty"`
	// 目标 OS 别名（决定分块大小与下标）。
	OSName string `json:"osName"`
}

// camoufoxEnvChunkSize 对照 camoufox.utils.get_env_vars 的分块阈值。
func camoufoxEnvChunkSize(osName string) int {
	if strings.EqualFold(strings.TrimSpace(osName), CamoufoxTargetOSWindows) {
		return 2047
	}
	return 32767
}

// EncodeCamouConfig 把 config 映射序列化为 CAMOU_CONFIG_N 环境变量字典。
// 对照 camoufox/utils.py:get_env_vars 的分块规则。
func EncodeCamouConfig(config map[string]any, osName string) (map[string]string, error) {
	if config == nil {
		return nil, fmt.Errorf("camoufox config 为空")
	}
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(config); err != nil {
		return nil, fmt.Errorf("序列化 Camoufox 配置失败: %w", err)
	}
	// json.NewEncoder 追加一个换行；去掉，保持与 camoufox Python 端 orjson 一致。
	raw := strings.TrimRight(buf.String(), "\n")
	chunkSize := camoufoxEnvChunkSize(osName)
	if chunkSize <= 0 {
		chunkSize = 32767
	}
	text := string(raw)
	env := make(map[string]string, (len(text)+chunkSize-1)/chunkSize)
	for start, n := 0, 1; start < len(text); start, n = start+chunkSize, n+1 {
		end := start + chunkSize
		if end > len(text) {
			end = len(text)
		}
		env[fmt.Sprintf("CAMOU_CONFIG_%d", n)] = text[start:end]
	}
	return env, nil
}

// BuildCamoufoxLaunchConfig 组装一次 Camoufox 启动的完整负载。
// 它把指纹池条目 + Ant FingerprintArgs 覆盖 + 代理/headless/启动 URL 合并成 CamoufoxLaunchConfig。
func BuildCamoufoxLaunchConfig(
	executablePath string,
	userDataDir string,
	proxy map[string]string,
	headless bool,
	startURLs []string,
	targetOS string,
	seed int,
	overrides []string,
	extraArgs []string,
	firefoxUserPrefs map[string]any,
) (*CamoufoxLaunchConfig, error) {
	pool, err := LoadCamoufoxFingerprintPool()
	if err != nil {
		return nil, err
	}
	entry, err := pool.SelectCamoufoxFingerprint(targetOS, seed)
	if err != nil {
		return nil, err
	}

	// 合并 fingerprint + runtime 为单一 config 字典；运行时随机量 (canvas/font-spacing/webgl)
	// 全部已在构建期由 camoufox utils 生成并固化到池里，不再运行时重新随机。
	config := make(map[string]any, len(entry.Fingerprint)+len(entry.Runtime))
	for k, v := range entry.Fingerprint {
		config[k] = v
	}
	for k, v := range entry.Runtime {
		config[k] = v
	}

	applyCamoufoxFingerprintOverrides(config, overrides)

	camouEnv, err := EncodeCamouConfig(config, targetOS)
	if err != nil {
		return nil, err
	}

	prefs := firefoxUserPrefs
	if prefs == nil {
		// 池元数据里保留了一份默认 prefs（webgl.enable-webgl2 / webgl.force-enabled），直接复用避免遗漏。
		prefs = make(map[string]any, len(pool.FirefoxUserPrefs))
		for k, v := range pool.FirefoxUserPrefs {
			prefs[k] = v
		}
	}

	return &CamoufoxLaunchConfig{
		ExecutablePath:   executablePath,
		UserDataDir:      userDataDir,
		Headless:         headless,
		Proxy:            proxy,
		StartURLs:        startURLs,
		ExtraArgs:        normalizeCamoufoxExtraArgs(extraArgs),
		CamouConfig:      camouEnv,
		FirefoxUserPrefs: prefs,
		OSName:           targetOS,
	}, nil
}

// normalizeCamoufoxExtraArgs 只透传 Firefox/Camoufox 可能理解、且不被 Ant 上层接管的参数。
// 指纹/代理/profile/startURL/CDP 相关参数已经在配置字段中处理，不能再次进入 args。
func normalizeCamoufoxExtraArgs(args []string) []string {
	items := normalizeStringList(args)
	if len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, arg := range items {
		key := arg
		if idx := strings.IndexByte(arg, '='); idx >= 0 {
			key = arg[:idx]
		}
		keyLower := strings.ToLower(strings.TrimSpace(key))
		skip := false
		switch {
		case keyLower == "--user-agent",
			keyLower == "--accept-language",
			keyLower == "--accept-lang",
			keyLower == "--lang",
			keyLower == "--timezone",
			keyLower == "--window-size",
			keyLower == "--user-data-dir",
			keyLower == "--profile",
			keyLower == "-profile",
			keyLower == "--fingerprint",
			strings.HasPrefix(keyLower, "--fingerprint-"),
			keyLower == "--remote-debugging-port",
			keyLower == "--remote-debugging-address",
			keyLower == "--remote-debugging-pipe",
			keyLower == "--proxy-server",
			keyLower == "--disable-session-crashed-bubble",
			keyLower == "--new-window":
			skip = true
		case !strings.HasPrefix(arg, "-"):
			// 非 flag 参数通常是 URL/路径；启动 URL 已由 startUrls 统一处理。
			skip = true
		}
		if !skip {
			out = append(out, arg)
		}
	}
	return out
}

func normalizeStringList(args []string) []string {
	if len(args) == 0 {
		return nil
	}
	out := make([]string, 0, len(args))
	for _, item := range args {
		value := strings.TrimSpace(item)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

// applyCamoufoxFingerprintOverrides 把 Ant 的 FingerprintArgs (Chromium 命令行样式) 覆盖到 Camoufox config。
//
// 覆盖规则：
//
//	--user-agent=UA            -> navigator.userAgent + headers.User-Agent + 按 UA 推断并同步 oscpu/platform
//	--lang=xx 或 --accept-lang -> navigator.language/languages + headers.Accept-Language
//	--timezone=Area/City       -> timezone
//	--window-size=W,H          -> window.outerWidth/outerHeight
//	--fingerprint-color-depth=N -> screen.colorDepth + screen.pixelDepth
//	--fingerprint-hardware-concurrency=N -> navigator.hardwareConcurrency
//	--fingerprint-device-memory=N -> navigator.deviceMemory
//	--fingerprint-do-not-track=true/false -> navigator.doNotTrack
//	--fingerprint-touch-points=N -> navigator.maxTouchPoints
//	--fingerprint-fonts=A,B     -> fonts
//	--user-data-dir=...        -> 跳过（由 BuildCamoufoxLaunchConfig 顶层 userDataDir 统管）
//	--fingerprint-platform=... -> 跳过（已在 targetOS 决定时处理）
//	--fingerprint-brand=Chrome -> 忽略 (Firefox 不适用)
//	--fingerprint=<seed>      -> 跳过 (seed 在上层已用于池索引)
//	--remote-debugging-port/... -> 跳过 (CDP 专有)
//	--proxy-server=...         -> 跳过 (由顶层 proxy 统管)
//	其它 Firefox 能识别的 CLI 不在此覆盖 config，由 Node launcher 透传 args
func applyCamoufoxFingerprintOverrides(config map[string]any, fingerprintArgs []string) {
	args := normalizeStringList(fingerprintArgs)
	for i := 0; i < len(args); i++ {
		raw := args[i]
		arg := strings.TrimSpace(raw)
		if !strings.HasPrefix(arg, "--") {
			continue
		}
		key := arg
		value := ""
		if idx := strings.IndexByte(arg, '='); idx >= 0 {
			key = arg[:idx]
			value = arg[idx+1:]
		} else if camoufoxOverrideTakesValue(key) && i+1 < len(args) {
			next := strings.TrimSpace(args[i+1])
			if next != "" && !strings.HasPrefix(next, "-") {
				value = next
				i++
			}
		}
		switch {
		case key == "--user-agent":
			if value == "" {
				continue
			}
			config["navigator.userAgent"] = value
			config["headers.User-Agent"] = value
			// 按 UA 同步推断 oscpu/platform，避免 UA 与内部差异导致指纹不一致。
			oscpu, platform := inferFirefoxOSCPUAndPlatform(value)
			if oscpu != "" {
				config["navigator.oscpu"] = oscpu
			}
			if platform != "" {
				config["navigator.platform"] = platform
			}
		case key == "--accept-language", key == "--accept-lang", key == "--lang":
			langs := normalizeAcceptLanguage(value)
			if len(langs) == 0 {
				continue
			}
			config["navigator.language"] = langs[0]
			config["navigator.languages"] = langs
			// Camoufox headers.Accept-Language 不是池里的字段，手动补
			acceptParts := make([]string, 0, len(langs))
			for i, l := range langs {
				if i == 0 {
					acceptParts = append(acceptParts, l)
				} else if i == 1 {
					acceptParts = append(acceptParts, l+";q=0.9")
				} else {
					acceptParts = append(acceptParts, l+";q=0.8")
				}
			}
			config["headers.Accept-Language"] = strings.Join(acceptParts, ",")
		case key == "--window-size":
			w, h, ok := parseWindowWH(value)
			if !ok {
				continue
			}
			config["window.outerWidth"] = w
			config["window.outerHeight"] = h
			// 同时把屏幕尺寸与窗口外形对齐，避免 outerWidth > screen.width 导致不一致。
			if currentW, _ := config["screen.width"].(float64); currentW == 0 || currentW < float64(w) {
				config["screen.width"] = w
			}
			if currentH, _ := config["screen.height"].(float64); currentH == 0 || currentH < float64(h) {
				config["screen.height"] = h
			}
			// inner 估个近似值（Camoufox 默认 inner 由 padding 10px 计算）
			if w > 20 {
				config["screen.availWidth"] = w
			}
			if h > 100 {
				config["screen.availHeight"] = h - 40
			}
		case key == "--timezone":
			if value == "" {
				continue
			}
			config["timezone"] = value
		case key == "--fingerprint-color-depth":
			depth, ok := parsePositiveInt(value)
			if !ok {
				continue
			}
			config["screen.colorDepth"] = depth
			config["screen.pixelDepth"] = depth
		case key == "--fingerprint-hardware-concurrency":
			cores, ok := parsePositiveInt(value)
			if !ok {
				continue
			}
			config["navigator.hardwareConcurrency"] = cores
		case key == "--fingerprint-device-memory":
			memory, ok := parsePositiveInt(value)
			if !ok {
				continue
			}
			config["navigator.deviceMemory"] = memory
		case key == "--fingerprint-do-not-track":
			enabled, ok := parseBoolFlag(value)
			if !ok {
				continue
			}
			if enabled {
				config["navigator.doNotTrack"] = "1"
			} else {
				config["navigator.doNotTrack"] = "0"
			}
		case key == "--fingerprint-touch-points":
			points, ok := parseNonNegativeInt(value)
			if !ok {
				continue
			}
			config["navigator.maxTouchPoints"] = points
		case key == "--fingerprint-fonts":
			fonts := normalizeCommaList(value)
			if len(fonts) == 0 {
				continue
			}
			config["fonts"] = fonts
		case strings.HasPrefix(key, "--user-data-dir"),
			strings.HasPrefix(key, "--fingerprint-platform"),
			strings.HasPrefix(key, "--fingerprint-brand"),
			strings.HasPrefix(key, "--fingerprint="),
			strings.HasPrefix(key, "--remote-debugging-port"),
			strings.HasPrefix(key, "--remote-debugging-address"),
			strings.EqualFold(key, "--disable-session-crashed-bubble"),
			strings.HasPrefix(key, "--proxy-server"),
			strings.EqualFold(key, "--new-window"),
			strings.EqualFold(key, "--kiosk"):
			// 已在上层处理或不适用 Camoufox，跳过覆盖。
			continue
		default:
			// 其它未知参数不覆盖 config；若 Firefox 接受可由 Node launcher 透传到 args。
		}
	}
}

func camoufoxOverrideTakesValue(key string) bool {
	switch key {
	case "--user-agent",
		"--accept-language",
		"--accept-lang",
		"--lang",
		"--window-size",
		"--timezone",
		"--fingerprint-color-depth",
		"--fingerprint-hardware-concurrency",
		"--fingerprint-device-memory",
		"--fingerprint-do-not-track",
		"--fingerprint-touch-points",
		"--fingerprint-fonts":
		return true
	default:
		return false
	}
}

// parseWindowWH 解析 "--window-size=1280,800" 形式。
func parseWindowWH(value string) (int, int, bool) {
	parts := strings.Split(value, ",")
	if len(parts) != 2 {
		return 0, 0, false
	}
	w, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || w <= 0 {
		return 0, 0, false
	}
	h, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || h <= 0 {
		return 0, 0, false
	}
	return w, h, true
}

func parsePositiveInt(value string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

func parseNonNegativeInt(value string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

func parseBoolFlag(value string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on", "enable", "enabled":
		return true, true
	case "0", "false", "no", "off", "disable", "disabled":
		return false, true
	default:
		return false, false
	}
}

func normalizeCommaList(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		key := strings.ToLower(item)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

// normalizeAcceptLanguage 把 "zh-CN,zh;q=0.9,en" 或 "zh-CN" 转换成语言标签列表。
func normalizeAcceptLanguage(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		// 去掉 q 修饰
		lang := p
		if idx := strings.IndexByte(lang, ';'); idx >= 0 {
			lang = lang[:idx]
		}
		lang = strings.TrimSpace(lang)
		if lang != "" {
			out = append(out, lang)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// inferFirefoxOSCPUAndPlatform 按 Firefox UA 字符串推断 oscpu / navigator.platform。
// 映射表取自 browserforge/apify Firefox 指纹的真实形态，保证覆盖 UA 后指纹内部一致。
func inferFirefoxOSCPUAndPlatform(ua string) (oscpu string, platform string) {
	uaLower := strings.ToLower(ua)
	switch {
	case strings.Contains(uaLower, "windows nt"):
		oscpu = "Windows NT 10.0; Win64; x64"
		platform = "Win32"
	case strings.Contains(uaLower, "macintosh"), strings.Contains(uaLower, "mac os x"):
		oscpu = "Intel Mac OS X 10.15"
		platform = "MacIntel"
	case strings.Contains(uaLower, "linux"):
		oscpu = "Linux x86_64"
		platform = "Linux x86_64"
	}
	return
}
