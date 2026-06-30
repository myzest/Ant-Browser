package backend

import (
	"context"
	"os"
	"os/exec"
	goruntime "runtime"
	"strings"
	"time"
)

var hostFontListingForRuntimeWarning = defaultHostFontListingForRuntimeWarning
var hostGOOSForRuntimeWarning = func() string { return goruntime.GOOS }

func collectLaunchRuntimeWarnings(profileID string, args []string) []string {
	_ = profileID
	warnings := []string{}
	warnings = append(warnings, launchContextDisciplineWarnings(args)...)
	warnings = append(warnings, launchHostPlatformRuntimeWarnings(args)...)
	warnings = append(warnings, launchWebGLRuntimeWarnings(args)...)
	warnings = append(warnings, launchFontRuntimeWarnings(args)...)
	return uniqueRuntimeWarnings(warnings)
}

func launchContextDisciplineWarnings(args []string) []string {
	values := parseLaunchArgValues(args)
	warnings := []string{}
	if hasRuntimeWarningLaunchArgKey(args, "--user-agent") || hasRuntimeWarningLaunchArgKey(args, "--user-agent-string") {
		warnings = append(warnings, "检测到本次启动覆盖 User-Agent；请同时确认 UA-CH、Sec-CH-UA 请求头、platform、WebGL、codec 能力一致，避免只改 UA 字符串。")
		if !hasAnyLaunchArgPrefix(args, "--fingerprint-client-hints") && strings.TrimSpace(os.Getenv("ANT_BROWSER_UACH_RUNTIME")) == "" {
			warnings = append(warnings, "当前未检测到 UA-CH runtime 覆盖能力标记；navigator.userAgentData 与 Sec-CH-UA 请求头可能仍暴露真实 Chromium 身份。")
		}
	}
	if strings.Contains(strings.ToLower(values["--disable-blink-features"]), "automationcontrolled") {
		warnings = append(warnings, "检测到 AutomationControlled 覆盖参数；该参数本身可能成为自动化/篡改启发式信号，建议优先使用内核级 navigator.webdriver 处理。")
	}
	if hasRuntimeWarningLaunchArgKey(args, "--headless") {
		warnings = append(warnings, "检测到 headless 启动参数；高风控站点建议使用 headed 模式，并单独验证 screen/window/viewport/GPU/WebGPU 一致性。")
	}
	if hasRuntimeWarningLaunchArgKey(args, "--disable-gpu") {
		warnings = append(warnings, "检测到禁用 GPU 参数；可能造成 WebGL/WebGPU/Media 渲染栈异常。")
	}
	if strings.Contains(strings.ToLower(values["--use-gl"]), "swiftshader") || strings.Contains(strings.ToLower(values["--use-angle"]), "swiftshader") {
		warnings = append(warnings, "检测到 SwiftShader 渲染路径；请确认 WebGL/WebGPU renderer 与平台身份一致。")
	}
	return warnings
}

func launchHostPlatformRuntimeWarnings(args []string) []string {
	values := parseLaunchArgValues(args)
	platform := normalizeRuntimeWarningPlatform(values["--fingerprint-platform"])
	hostPlatform := normalizeRuntimeWarningGOOS(hostGOOSForRuntimeWarning())
	if platform == "" || hostPlatform == "" || platform == hostPlatform {
		return nil
	}
	return []string{platformHostMismatchRuntimeWarning(hostPlatform, platform)}
}

func launchWebGLRuntimeWarnings(args []string) []string {
	values := parseLaunchArgValues(args)
	platform := normalizeRuntimeWarningPlatform(values["--fingerprint-platform"])
	hostPlatform := normalizeRuntimeWarningGOOS(hostGOOSForRuntimeWarning())
	vendor := strings.TrimSpace(values["--fingerprint-webgl-vendor"])
	renderer := strings.TrimSpace(values["--fingerprint-webgl-renderer"])
	if vendor == "" && renderer == "" {
		return nil
	}

	combined := strings.ToLower(vendor + " " + renderer)
	if containsAnyFold(combined, virtualWebGLRuntimeTells) {
		return []string{"WebGL vendor/renderer 暴露虚拟化或软件渲染特征；Fingerprint Virtual Machine 信号可能升高，请优先使用真实 GPU 路径，避免 SwiftShader/llvmpipe/VirtualBox/VMware 等 renderer。"}
	}
	if hostPlatform == "mac" && platform != "" && platform != "mac" && !strings.Contains(combined, "apple") {
		return []string{"当前宿主为 macOS，但 WebGL 指纹声明为非 Apple GPU；这类跨平台 GPU 组合容易被 Virtual Machine/anti-fraud 模型识别，请确认内核已真实覆盖 WEBGL_debug_renderer_info，或改用 macOS/Apple GPU profile。"}
	}
	if platform == "mac" && !strings.Contains(combined, "apple") {
		return []string{"指纹平台声明为 macOS，但 WebGL vendor/renderer 不是 Apple GPU；请改用 Apple GPU renderer 或保持平台/GPU 一致。"}
	}
	return nil
}

func launchFontRuntimeWarnings(args []string) []string {
	values := parseLaunchArgValues(args)
	platform := normalizeRuntimeWarningPlatform(values["--fingerprint-platform"])
	fonts := strings.TrimSpace(values["--fingerprint-fonts"])
	if platform == "" || fonts == "" {
		return nil
	}

	hostPlatform := normalizeRuntimeWarningGOOS(hostGOOSForRuntimeWarning())
	if platform == "windows" && hostPlatform != "windows" && containsAnyFold(fonts, windowsFontRuntimeTells) {
		listing, ok := hostFontListingForRuntimeWarning()
		if ok && containsAnyFold(listing, windowsFontRuntimeTells) {
			return nil
		}
		if ok {
			return []string{"当前宿主环境未检测到常见 Windows 字体，但指纹声明为 Windows；字体 metrics/枚举可能与平台身份冲突，请安装 Segoe UI/Calibri 等字体或改用宿主平台指纹。"}
		}
		return []string{"当前宿主不是 Windows，但指纹声明为 Windows；无法完成字体运行时探测，请确认宿主/容器已安装 Windows marker fonts，避免字体枚举与平台身份冲突。"}
	}

	if platform == "mac" && hostPlatform != "mac" && containsAnyFold(fonts, macFontRuntimeTells) {
		return []string{"当前宿主不是 macOS，但指纹声明为 macOS；请确认字体、GPU、media codec 与 macOS 身份一致，否则建议改用宿主平台指纹。"}
	}
	return nil
}

var windowsFontRuntimeTells = []string{
	"Segoe UI",
	"Segoe UI Light",
	"Calibri",
	"Cambria",
	"Cambria Math",
	"Marlett",
	"MS UI Gothic",
	"Franklin Gothic",
}

var macFontRuntimeTells = []string{
	"San Francisco",
	"PingFang",
	"Menlo",
	"Helvetica Neue",
}

var virtualWebGLRuntimeTells = []string{
	"SwiftShader",
	"llvmpipe",
	"VirtualBox",
	"VMware",
	"Parallels",
	"QEMU",
	"Hyper-V",
	"virgl",
	"Microsoft Basic Render",
}

func parseLaunchArgValues(args []string) map[string]string {
	values := map[string]string{}
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if !strings.HasPrefix(arg, "--") {
			continue
		}
		key := arg
		value := ""
		if before, after, ok := strings.Cut(arg, "="); ok {
			key = before
			value = strings.TrimSpace(after)
		} else if i+1 < len(args) {
			next := strings.TrimSpace(args[i+1])
			if next != "" && !strings.HasPrefix(next, "-") {
				value = next
				i++
			}
		}
		key = strings.ToLower(strings.TrimSpace(key))
		if canonical, ok := singleValueLaunchArgKey(key); ok {
			key = canonical
		}
		values[key] = value
	}
	return values
}

func normalizeRuntimeWarningPlatform(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "windows", "win", "win32":
		return "windows"
	case "mac", "macos", "darwin":
		return "mac"
	case "linux":
		return "linux"
	default:
		return ""
	}
}

func normalizeRuntimeWarningGOOS(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "windows":
		return "windows"
	case "darwin":
		return "mac"
	case "linux":
		return "linux"
	default:
		return ""
	}
}

func platformHostMismatchRuntimeWarning(hostPlatform string, declaredPlatform string) string {
	return "当前宿主平台为 " + runtimeWarningPlatformLabel(hostPlatform) + "，但指纹声明为 " + runtimeWarningPlatformLabel(declaredPlatform) + "；跨平台 platform/GPU/字体/codec 组合容易推高 Fingerprint Virtual Machine 信号，请优先使用宿主平台 profile，或确认内核已完整覆盖相关运行时能力。"
}

func runtimeWarningPlatformLabel(platform string) string {
	switch normalizeRuntimeWarningPlatform(platform) {
	case "windows":
		return "Windows"
	case "mac":
		return "macOS"
	case "linux":
		return "Linux"
	default:
		return strings.TrimSpace(platform)
	}
}

func defaultHostFontListingForRuntimeWarning() (string, bool) {
	if _, err := exec.LookPath("fc-list"); err != nil {
		return "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "fc-list", ":", "family").Output()
	if err != nil || len(output) == 0 {
		return "", false
	}
	return string(output), true
}

func containsAnyFold(text string, needles []string) bool {
	lower := strings.ToLower(text)
	for _, needle := range needles {
		if strings.Contains(lower, strings.ToLower(strings.TrimSpace(needle))) {
			return true
		}
	}
	return false
}

func hasAnyLaunchArgPrefix(args []string, prefix string) bool {
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	if prefix == "" {
		return false
	}
	for _, arg := range args {
		key := strings.ToLower(strings.TrimSpace(arg))
		if before, _, ok := strings.Cut(key, "="); ok {
			key = before
		}
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

func hasRuntimeWarningLaunchArgKey(args []string, want string) bool {
	want = strings.ToLower(strings.TrimSpace(want))
	if want == "" {
		return false
	}
	for _, arg := range args {
		key := strings.ToLower(strings.TrimSpace(arg))
		if key == "" || !strings.HasPrefix(key, "--") {
			continue
		}
		if before, _, ok := strings.Cut(key, "="); ok {
			key = before
		}
		if key == want {
			return true
		}
	}
	return false
}

func uniqueRuntimeWarnings(warnings []string) []string {
	out := make([]string, 0, len(warnings))
	seen := map[string]struct{}{}
	for _, warning := range warnings {
		warning = strings.TrimSpace(warning)
		if warning == "" {
			continue
		}
		if _, ok := seen[warning]; ok {
			continue
		}
		seen[warning] = struct{}{}
		out = append(out, warning)
	}
	return out
}
