package fingerprint

import (
	"net/netip"
	"strconv"
	"strings"
)

func Validate(profile *Profile) []ValidationIssue {
	if profile == nil {
		return []ValidationIssue{{Code: "empty_profile", Severity: "red", Field: "profile", Message: "指纹 profile 为空"}}
	}
	return ValidateArgs(Args(profile))
}

func ValidateArgs(args []string) []ValidationIssue {
	values := ParseArgs(args)
	issues := make([]ValidationIssue, 0)
	rawPlatform := strings.TrimSpace(values["--fingerprint-platform"])
	platform := NormalizePlatform(rawPlatform)
	if rawPlatform == "" {
		issues = append(issues, issue("missing_platform", "yellow", "--fingerprint-platform", "缺少浏览器平台"))
	}
	brand := values["--fingerprint-brand"]
	if brand == "" {
		issues = append(issues, issue("missing_brand", "yellow", "--fingerprint-brand", "缺少浏览器品牌"))
	} else if strings.EqualFold(brand, "Firefox") || strings.EqualFold(brand, "Safari") {
		issues = append(issues, issue("non_chromium_brand", "red", "--fingerprint-brand", "Chromium 指纹不应使用 Firefox/Safari 品牌"))
	}
	if values["--lang"] != "" && values["--fingerprint-locale"] != "" && !strings.EqualFold(values["--lang"], values["--fingerprint-locale"]) {
		issues = append(issues, issue("locale_mismatch", "yellow", "--fingerprint-locale", "lang 与 fingerprint locale 不一致"))
	}
	if values["--timezone"] != "" && values["--fingerprint-timezone"] != "" && !strings.EqualFold(values["--timezone"], values["--fingerprint-timezone"]) {
		issues = append(issues, issue("timezone_mismatch", "yellow", "--fingerprint-timezone", "timezone 与 fingerprint timezone 不一致"))
	}
	if value := values["--window-size"]; value != "" {
		width, height, ok := parseStrictPair(value)
		if !ok {
			issues = append(issues, issue("invalid_window_size", "red", "--window-size", "窗口尺寸格式无效，应为 宽,高"))
		}
		if width < 800 || height < 600 {
			issues = append(issues, issue("window_too_small", "yellow", "--window-size", "窗口尺寸过小，容易形成异常 profile"))
		}
	}
	if value := values["--fingerprint-color-depth"]; value != "" {
		colorDepth, err := strconv.Atoi(value)
		if err != nil || colorDepth <= 0 {
			issues = append(issues, issue("invalid_color_depth", "red", "--fingerprint-color-depth", "色深不是合法正整数"))
		} else if colorDepth != 24 && colorDepth != 30 && colorDepth != 32 {
			issues = append(issues, issue("unusual_color_depth", "yellow", "--fingerprint-color-depth", "色深不是常见桌面取值"))
		}
	}
	if value := values["--fingerprint-hardware-concurrency"]; value != "" {
		concurrency, err := strconv.Atoi(value)
		if err != nil || concurrency <= 0 {
			issues = append(issues, issue("invalid_hardware_concurrency", "red", "--fingerprint-hardware-concurrency", "CPU 核心数不是合法正整数"))
		} else if concurrency > 64 {
			issues = append(issues, issue("unusual_hardware_concurrency", "yellow", "--fingerprint-hardware-concurrency", "CPU 核心数过高，容易形成异常 profile"))
		}
	}
	if value := values["--fingerprint-device-memory"]; value != "" {
		memory, err := strconv.Atoi(value)
		if err != nil || memory <= 0 {
			issues = append(issues, issue("invalid_device_memory", "red", "--fingerprint-device-memory", "设备内存不是合法正整数"))
		} else if memory > 128 {
			issues = append(issues, issue("unusual_device_memory", "yellow", "--fingerprint-device-memory", "设备内存过高，容易形成异常 profile"))
		}
	}
	fonts := values["--fingerprint-fonts"]
	if fonts == "" {
		issues = append(issues, issue("missing_fonts", "yellow", "--fingerprint-fonts", "缺少字体列表"))
	} else {
		issues = append(issues, validateFonts(platform, fonts)...)
	}
	vendor := values["--fingerprint-webgl-vendor"]
	renderer := values["--fingerprint-webgl-renderer"]
	if vendor == "" || renderer == "" {
		issues = append(issues, issue("missing_webgl", "yellow", "--fingerprint-webgl-renderer", "缺少 WebGL vendor/renderer"))
	} else if !webGLAllowed(platform, vendor, renderer) {
		issues = append(issues, issue("webgl_platform_mismatch", "red", "--fingerprint-webgl-renderer", "WebGL vendor/renderer 与平台不匹配"))
	}
	if value := values["--fingerprint-touch-points"]; value != "" {
		touch, err := strconv.Atoi(value)
		if err != nil || touch < 0 {
			issues = append(issues, issue("invalid_touch_points", "red", "--fingerprint-touch-points", "触摸点数不是合法非负整数"))
			touch = 0
		}
		if touch > 0 && (platform == PlatformWindows || platform == PlatformMac || platform == PlatformLinux) {
			issues = append(issues, issue("desktop_touch_points", "yellow", "--fingerprint-touch-points", "桌面 profile 默认不应暴露触摸点"))
		}
	}
	if value := values["--fingerprint-media-devices"]; value != "" {
		if !validMediaDevices(value) {
			issues = append(issues, issue("invalid_media_devices", "red", "--fingerprint-media-devices", "媒体设备格式无效，应为 摄像头,麦克风,扬声器"))
		}
	}
	if ip := values["--fingerprint-webrtc-ip"]; ip != "" && !strings.EqualFold(ip, "auto") {
		if _, err := netip.ParseAddr(ip); err != nil {
			issues = append(issues, issue("invalid_webrtc_ip", "red", "--fingerprint-webrtc-ip", "WebRTC IP 不是合法 IP 地址"))
		}
	}
	issues = append(issues, validateRiskyLaunchArgs(args)...)
	return issues
}

func validateRiskyLaunchArgs(args []string) []ValidationIssue {
	issues := make([]ValidationIssue, 0)
	seen := map[string]struct{}{}
	for i := 0; i < len(args); i++ {
		key, value, consumed := splitLaunchArg(args, i)
		if key == "" {
			continue
		}
		if consumed {
			i++
		}

		code := ""
		message := ""
		switch key {
		case "--headless":
			code = "risky_headless"
			message = "包含 headless 启动参数，外部检测站可能识别为自动化环境"
		case "--disable-gpu":
			code = "risky_disable_gpu"
			message = "包含禁用 GPU 参数，可能造成渲染栈异常"
		case "--no-sandbox":
			code = "risky_no_sandbox"
			message = "包含 no-sandbox 参数，可能暴露异常运行环境"
		case "--disable-web-security":
			code = "risky_disable_web_security"
			message = "包含禁用 Web 安全参数，可能暴露调试/篡改环境"
		case "--remote-debugging-port", "--remote-debugging-pipe":
			code = "risky_remote_debugging"
			message = "包含远程调试参数，外部检测站可能识别到 DevTools/CDP 环境"
		case "--enable-automation":
			code = "risky_enable_automation"
			message = "包含 Chromium 自动化标记，容易被识别为自动化环境"
		case "--disable-blink-features":
			if strings.Contains(value, "automationcontrolled") {
				code = "risky_automation_controlled_override"
				message = "包含 AutomationControlled 覆盖参数，可能触发自动化/篡改启发式检测"
			}
		case "--use-gl", "--use-angle":
			if strings.Contains(value, "swiftshader") {
				code = "risky_swiftshader"
				message = "包含 SwiftShader 渲染参数，可能造成 WebGL/GPU 指纹异常"
			}
		}
		if code == "" {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		issues = append(issues, issue(code, "yellow", key, message))
	}
	return issues
}

func splitLaunchArg(args []string, index int) (string, string, bool) {
	if index < 0 || index >= len(args) {
		return "", "", false
	}
	arg := strings.TrimSpace(args[index])
	if !strings.HasPrefix(arg, "--") {
		return "", "", false
	}
	if key, value, ok := strings.Cut(arg, "="); ok {
		return strings.ToLower(strings.TrimSpace(key)), strings.ToLower(strings.TrimSpace(value)), false
	}
	key := strings.ToLower(arg)
	if index+1 < len(args) {
		next := strings.TrimSpace(args[index+1])
		if next != "" && !strings.HasPrefix(next, "-") {
			return key, strings.ToLower(next), true
		}
	}
	return key, "", false
}

func parseStrictPair(value string) (int, int, bool) {
	left, right, ok := strings.Cut(strings.TrimSpace(value), ",")
	if !ok {
		return 0, 0, false
	}
	width, err := strconv.Atoi(strings.TrimSpace(left))
	if err != nil || width <= 0 {
		return 0, 0, false
	}
	height, err := strconv.Atoi(strings.TrimSpace(right))
	if err != nil || height <= 0 {
		return 0, 0, false
	}
	return width, height, true
}

func validMediaDevices(value string) bool {
	parts := strings.Split(value, ",")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || n < 0 || n > 16 {
			return false
		}
	}
	return true
}

func Health(args []string) HealthReport {
	issues := ValidateArgs(args)
	status := "green"
	for _, item := range issues {
		if item.Severity == "red" {
			status = "red"
			break
		}
		if item.Severity == "yellow" {
			status = "yellow"
		}
	}
	return HealthReport{Status: status, Issues: issues}
}

func validateFonts(platform string, fonts string) []ValidationIssue {
	lower := strings.ToLower(fonts)
	switch platform {
	case PlatformWindows:
		if strings.Contains(lower, "pingfang") || strings.Contains(lower, "menlo") {
			return []ValidationIssue{issue("windows_mac_fonts", "red", "--fingerprint-fonts", "Windows profile 包含 macOS marker fonts")}
		}
	case PlatformMac:
		if strings.Contains(lower, "segoe ui") || strings.Contains(lower, "calibri") {
			return []ValidationIssue{issue("mac_windows_fonts", "red", "--fingerprint-fonts", "macOS profile 包含 Windows marker fonts")}
		}
	case PlatformLinux:
		if strings.Contains(lower, "segoe ui") || strings.Contains(lower, "pingfang") {
			return []ValidationIssue{issue("linux_foreign_fonts", "yellow", "--fingerprint-fonts", "Linux profile 包含非 Linux 常见 marker fonts")}
		}
	}
	return nil
}

func webGLAllowed(platform string, vendor string, renderer string) bool {
	v := strings.ToLower(vendor)
	r := strings.ToLower(renderer)
	switch platform {
	case PlatformMac:
		return strings.Contains(v, "apple") || strings.Contains(r, "apple")
	case PlatformLinux:
		return strings.Contains(r, "mesa") || strings.Contains(v, "intel") || strings.Contains(v, "amd")
	case PlatformWindows:
		return strings.Contains(v, "intel") || strings.Contains(v, "nvidia") || strings.Contains(v, "amd")
	default:
		return false
	}
}

func issue(code string, severity string, field string, message string) ValidationIssue {
	return ValidationIssue{Code: code, Severity: severity, Field: field, Message: message}
}
