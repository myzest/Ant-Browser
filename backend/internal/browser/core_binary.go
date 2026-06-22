package browser

import (
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
)

// CoreExecutableCandidates 返回当前平台可接受的浏览器可执行文件候选名。
func CoreExecutableCandidates() []string {
	return CoreExecutableCandidatesForType(CoreTypeChromium)
}

// CoreExecutableCandidatesForType 返回指定内核类型在当前平台可接受的可执行文件候选名。
func CoreExecutableCandidatesForType(coreType string) []string {
	if NormalizeCoreType(coreType) == CoreTypeCamoufox {
		switch goruntime.GOOS {
		case "windows":
			return []string{"camoufox.exe"}
		case "linux":
			return []string{"camoufox-bin", "camoufox"}
		case "darwin":
			return []string{
				"Camoufox.app/Contents/MacOS/camoufox",
				"Camoufox.app/Contents/Resources/camoufox",
				"camoufox",
			}
		default:
			return []string{"camoufox"}
		}
	}
	switch goruntime.GOOS {
	case "windows":
		return []string{"chrome.exe"}
	case "linux":
		return []string{"chrome", "chrome-bin", "chrome.exe"}
	case "darwin":
		return []string{
			"Google Chrome.app/Contents/MacOS/Google Chrome",
			"Chromium.app/Contents/MacOS/Chromium",
			"chrome",
		}
	default:
		return []string{"chrome"}
	}
}

// FindCoreExecutable 在指定目录查找可执行文件，返回绝对路径和命中的候选名。
func FindCoreExecutable(baseDir string) (string, string, bool) {
	return FindCoreExecutableForType(baseDir, CoreTypeChromium)
}

// FindCoreExecutableForType 在指定目录查找指定类型内核的可执行文件。
func FindCoreExecutableForType(baseDir string, coreType string) (string, string, bool) {
	baseDir = strings.TrimSpace(baseDir)
	if baseDir == "" {
		return "", "", false
	}
	candidates := CoreExecutableCandidatesForType(coreType)
	if directPath, directCandidate, ok := findDirectCoreExecutable(baseDir, candidates); ok {
		return directPath, directCandidate, true
	}
	if bundlePath, bundleCandidate, ok := findAppBundleExecutable(baseDir, candidates); ok {
		return bundlePath, bundleCandidate, true
	}
	for _, candidate := range candidates {
		p := filepath.Join(baseDir, filepath.FromSlash(candidate))
		if _, err := os.Stat(p); err == nil {
			return p, candidate, true
		}
	}
	return "", "", false
}

func findDirectCoreExecutable(path string, candidates []string) (string, string, bool) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return "", "", false
	}

	normalized := filepath.ToSlash(filepath.Clean(path))
	for _, candidate := range candidates {
		candidatePath := filepath.ToSlash(candidate)
		if strings.HasSuffix(normalized, candidatePath) || filepath.Base(normalized) == filepath.Base(candidatePath) {
			return path, candidate, true
		}
	}

	return "", "", false
}

func findAppBundleExecutable(path string, candidates []string) (string, string, bool) {
	if goruntime.GOOS != "darwin" {
		return "", "", false
	}

	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return "", "", false
	}

	normalized := filepath.ToSlash(filepath.Clean(path))
	if !strings.HasSuffix(strings.ToLower(normalized), ".app") {
		return "", "", false
	}

	for _, candidate := range candidates {
		candidatePath := filepath.ToSlash(candidate)
		appMarker := ".app/"
		index := strings.Index(strings.ToLower(candidatePath), appMarker)
		if index < 0 {
			continue
		}
		if !strings.EqualFold(filepath.Base(normalized), filepath.Base(candidatePath[:index+len(".app")])) {
			continue
		}

		relativeExecutable := candidatePath[index+len(appMarker):]
		if relativeExecutable == "" {
			continue
		}

		p := filepath.Join(path, filepath.FromSlash(relativeExecutable))
		if _, err := os.Stat(p); err == nil {
			return p, candidate, true
		}
	}

	return "", "", false
}
