package backend

import (
	"ant-chrome/backend/internal/browser"
	"ant-chrome/backend/internal/config"
	"ant-chrome/backend/internal/launchcode"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	goruntime "runtime"
	"strings"
	"testing"
	"time"
)

func TestEnsureNewWindowLaunchArgAddsFlagOnce(t *testing.T) {
	t.Parallel()

	got := ensureNewWindowLaunchArg([]string{"--lang=en-US"})
	want := []string{"--lang=en-US", "--new-window"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ensureNewWindowLaunchArg 结果错误: got=%v want=%v", got, want)
	}

	got = ensureNewWindowLaunchArg([]string{"--new-window", "--lang=en-US"})
	want = []string{"--new-window", "--lang=en-US"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ensureNewWindowLaunchArg 不应重复追加: got=%v want=%v", got, want)
	}
}

func TestShouldPreferVisibleWindowForStartWithParams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		startURLs []string
		want      bool
	}{
		{
			name:      "nil start URLs",
			startURLs: nil,
			want:      false,
		},
		{
			name:      "empty start URLs",
			startURLs: []string{},
			want:      false,
		},
		{
			name:      "blank start URLs",
			startURLs: []string{"  ", "\t"},
			want:      false,
		},
		{
			name:      "valid start URL",
			startURLs: []string{"https://finance.sina.com.cn"},
			want:      true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := shouldPreferVisibleWindowForStartWithParams(tt.startURLs); got != tt.want {
				t.Fatalf("shouldPreferVisibleWindowForStartWithParams() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsBrowserProfileLive(t *testing.T) {
	t.Parallel()

	ln := mustListenLoopback(t)
	defer ln.Close()

	profile := &BrowserProfile{
		Running:   true,
		DebugPort: listenerPort(t, ln),
	}
	if !isBrowserProfileLive(profile, nil) {
		t.Fatal("期望存活中的调试端口被识别为运行中实例")
	}

	if isBrowserProfileLive(&BrowserProfile{Running: true, DebugPort: 0}, nil) {
		t.Fatal("debugPort=0 不应被识别为运行中实例")
	}
}

func TestIsBrowserProfileLiveKeepsPendingDebugProcessAlive(t *testing.T) {
	t.Parallel()

	cmd := longLivedCommand(2 * time.Second)
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动长生命周期测试进程失败: %v", err)
	}
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	}()

	profile := &BrowserProfile{
		Running:    true,
		Pid:        cmd.Process.Pid,
		DebugPort:  0,
		DebugReady: false,
	}
	if !isBrowserProfileLive(profile, cmd) {
		t.Fatal("期望调试接口未就绪但进程仍存活时识别为运行中实例")
	}
}

func TestRequireDebugBridgeReportsPendingDebugReadySeparately(t *testing.T) {
	cmd := longLivedCommand(2 * time.Second)
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动长生命周期测试进程失败: %v", err)
	}
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	}()

	app := NewApp("")
	app.browserMgr = browser.NewManager(config.DefaultConfig(), "")
	app.browserMgr.Profiles = map[string]*BrowserProfile{
		"profile-pending-debug": {
			ProfileId:  "profile-pending-debug",
			Running:    true,
			Pid:        cmd.Process.Pid,
			DebugPort:  freeLoopbackPort(t),
			DebugReady: false,
		},
	}
	app.browserMgr.BrowserProcesses = map[string]*exec.Cmd{
		"profile-pending-debug": cmd,
	}

	input := newBrowserStartInput("profile-pending-debug", nil, nil, false, false, false, true, "", "")
	profile, handled, err := app.resolveBrowserStartProfile(input)
	if err == nil {
		t.Fatal("期望调试接口未就绪时返回错误")
	}
	if !handled {
		t.Fatal("期望运行中实例被处理")
	}
	if profile == nil || !strings.Contains(profile.LastError, "调试接口仍在就绪中") {
		t.Fatalf("期望 LastError 提示调试接口仍在就绪中，实际 profile=%+v err=%v", profile, err)
	}
	if strings.Contains(err.Error(), "无调试接管模式") {
		t.Fatalf("调试端口已分配但未就绪时不应提示无调试接管模式: %v", err)
	}
}

func TestGetDebugPortDistinguishesNoBridgeFromPending(t *testing.T) {
	app := NewApp("")
	app.browserMgr = browser.NewManager(config.DefaultConfig(), "")
	app.browserMgr.Profiles = map[string]*BrowserProfile{
		"profile-no-bridge": {
			ProfileId:  "profile-no-bridge",
			Running:    true,
			DebugPort:  0,
			DebugReady: false,
		},
		"profile-pending": {
			ProfileId:  "profile-pending",
			Running:    true,
			DebugPort:  9222,
			DebugReady: false,
		},
	}

	_, err := app.getDebugPort("profile-no-bridge")
	if err == nil || !strings.Contains(err.Error(), "无调试接管模式") {
		t.Fatalf("期望无调试桥错误，实际=%v", err)
	}

	_, err = app.getDebugPort("profile-pending")
	if err == nil || !strings.Contains(err.Error(), "调试接口尚未就绪") {
		t.Fatalf("期望调试接口未就绪错误，实际=%v", err)
	}
}

func TestWaitBrowserDebugPortStableKeepsListeningPort(t *testing.T) {
	t.Parallel()

	server := startDevToolsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/json/version":
			_, _ = w.Write([]byte(`{"Browser":"Chrome/142.0","webSocketDebuggerUrl":"ws://127.0.0.1/devtools/browser"}`))
		case "/json/list":
			_, _ = w.Write([]byte(`[{"id":"page-1"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	if _, err := waitBrowserDebugPortStable(server.port, "", time.Second, 250*time.Millisecond, nil); err != nil {
		t.Fatalf("waitBrowserDebugPortStable 返回错误: %v", err)
	}
}

func TestWaitBrowserDebugPortStableRejectsEphemeralPort(t *testing.T) {
	t.Parallel()

	server := startDevToolsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/json/version":
			_, _ = w.Write([]byte(`{"Browser":"Chrome/142.0","webSocketDebuggerUrl":"ws://127.0.0.1/devtools/browser"}`))
		case "/json/list":
			_, _ = w.Write([]byte(`[{"id":"page-1"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	port := server.port
	time.AfterFunc(120*time.Millisecond, func() {
		_ = server.Close()
	})

	_, err := waitBrowserDebugPortStable(port, "", time.Second, 400*time.Millisecond, nil)
	if err == nil {
		t.Fatal("期望短暂就绪后关闭的端口被判定为失败")
	}
}

func TestWaitBrowserDebugPortStableRejectsPlainTCPPort(t *testing.T) {
	t.Parallel()

	ln := mustListenLoopback(t)
	defer ln.Close()

	_, err := waitBrowserDebugPortStable(listenerPort(t, ln), "", 700*time.Millisecond, 250*time.Millisecond, nil)
	if err == nil {
		t.Fatal("期望仅开放 TCP 端口但无 DevTools HTTP 时启动失败")
	}
}

func TestWaitBrowserDebugPortStableDiscoversPortFromStderr(t *testing.T) {
	t.Parallel()

	server := startDevToolsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/json/version":
			_, _ = w.Write([]byte(`{"Browser":"Chrome/142.0","webSocketDebuggerUrl":"ws://127.0.0.1/devtools/browser"}`))
		case "/json/list":
			_, _ = w.Write([]byte(`[{"id":"page-1"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cmd := stderrPortCommand(server.port, 2*time.Second)
	monitor, err := newBrowserProcessMonitor(cmd)
	if err != nil {
		t.Fatalf("初始化浏览器进程监控失败: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动测试命令失败: %v", err)
	}
	monitor.Start()

	debugPort, err := waitBrowserDebugPortStable(0, "", 2*time.Second, 250*time.Millisecond, monitor)
	if err != nil {
		t.Fatalf("期望从 stderr 自动发现调试端口，实际错误: %v", err)
	}
	if debugPort != server.port {
		t.Fatalf("期望发现调试端口 %d，实际=%d", server.port, debugPort)
	}
}

func TestWaitBrowserDebugPortStableDiscoversPortFromDevToolsFile(t *testing.T) {
	t.Parallel()

	server := startDevToolsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/json/version":
			_, _ = w.Write([]byte(`{"Browser":"Chrome/142.0","webSocketDebuggerUrl":"ws://127.0.0.1/devtools/browser"}`))
		case "/json/list":
			_, _ = w.Write([]byte(`[{"id":"page-1"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	userDataDir := t.TempDir()
	writeDevToolsActivePortFile(t, userDataDir, server.port)

	debugPort, err := waitBrowserDebugPortStable(0, userDataDir, time.Second, 250*time.Millisecond, nil)
	if err != nil {
		t.Fatalf("期望从 DevToolsActivePort 自动发现调试端口，实际错误: %v", err)
	}
	if debugPort != server.port {
		t.Fatalf("期望发现调试端口 %d，实际=%d", server.port, debugPort)
	}
}

func TestWaitBrowserDebugPortStableReturnsProcessExitDetail(t *testing.T) {
	t.Parallel()

	cmd := stderrFailingCommand("missing libEGL.dll")
	monitor, err := newBrowserProcessMonitor(cmd)
	if err != nil {
		t.Fatalf("初始化浏览器进程监控失败: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动测试命令失败: %v", err)
	}
	monitor.Start()

	startedAt := time.Now()
	_, err = waitBrowserDebugPortStable(0, "", 2*time.Second, 250*time.Millisecond, monitor)
	if err == nil {
		t.Fatal("期望启动前退出被判定为失败")
	}
	if time.Since(startedAt) >= 2*time.Second {
		t.Fatalf("期望在超时前返回进程退出错误，实际耗时=%s", time.Since(startedAt))
	}

	var exitErr *browserStartupExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("期望 browserStartupExitError，实际=%T %v", err, err)
	}
	if !strings.Contains(exitErr.Detail(), "missing libEGL.dll") {
		t.Fatalf("期望 stderr 细节被捕获，实际=%q", exitErr.Detail())
	}
}

func TestWaitBrowserProcessStableReturnsProcessExitDetail(t *testing.T) {
	t.Parallel()

	cmd := stderrFailingCommand("manual chrome failed quickly")
	monitor, err := newBrowserProcessMonitor(cmd)
	if err != nil {
		t.Fatalf("初始化浏览器进程监控失败: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动测试命令失败: %v", err)
	}
	monitor.Start()

	err = waitBrowserProcessStable(monitor, 500*time.Millisecond)
	if err == nil {
		t.Fatal("期望手动启动稳定检测返回进程退出错误")
	}

	var exitErr *browserStartupExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("期望 browserStartupExitError，实际=%T %v", err, err)
	}
	if !strings.Contains(exitErr.Detail(), "manual chrome failed quickly") {
		t.Fatalf("期望 stderr 细节被捕获，实际=%q", exitErr.Detail())
	}
}

func TestWaitBrowserProcessStableAcceptsLiveProcess(t *testing.T) {
	t.Parallel()

	cmd := longLivedCommand(1500 * time.Millisecond)
	monitor, err := newBrowserProcessMonitor(cmd)
	if err != nil {
		t.Fatalf("初始化浏览器进程监控失败: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动测试命令失败: %v", err)
	}
	monitor.Start()

	if err := waitBrowserProcessStable(monitor, 250*time.Millisecond); err != nil {
		t.Fatalf("期望存活进程通过稳定检测，实际错误: %v", err)
	}
	_ = cmd.Process.Kill()
	_ = monitor.Wait()
}

func TestWaitBrowserDebugPortStableAllowsDebugPortAfterLauncherExit(t *testing.T) {
	t.Parallel()

	port := freeLoopbackPort(t)
	cmd := shortLivedCommand()
	monitor, err := newBrowserProcessMonitor(cmd)
	if err != nil {
		t.Fatalf("初始化浏览器进程监控失败: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动短命测试命令失败: %v", err)
	}
	monitor.Start()

	serverReady := make(chan *devToolsTestServer, 1)
	go func() {
		time.Sleep(300 * time.Millisecond)
		serverReady <- startDevToolsServerOnPort(t, port, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/json/version":
				_, _ = w.Write([]byte(`{"Browser":"Chrome/142.0","webSocketDebuggerUrl":"ws://127.0.0.1/devtools/browser"}`))
			case "/json/list":
				_, _ = w.Write([]byte(`[{"id":"page-1"}]`))
			default:
				http.NotFound(w, r)
			}
		}))
	}()

	debugPort, err := waitBrowserDebugPortStable(port, "", 100*time.Millisecond, 250*time.Millisecond, monitor)
	server := <-serverReady
	defer server.Close()

	if err != nil {
		t.Fatalf("期望启动器退出后仍能等待到调试端口就绪，实际错误: %v", err)
	}
	if debugPort != port {
		t.Fatalf("期望发现调试端口 %d，实际=%d", port, debugPort)
	}
}

func TestWaitBrowserProcessKeepsRunningWhileDebugPortAlive(t *testing.T) {
	ln := mustListenLoopback(t)
	port := listenerPort(t, ln)

	app := NewApp("")
	app.browserMgr = browser.NewManager(config.DefaultConfig(), "")
	app.browserMgr.Profiles = map[string]*BrowserProfile{
		"profile-detached": {
			ProfileId:   "profile-detached",
			ProfileName: "Detached Browser",
			Running:     true,
			DebugPort:   port,
			DebugReady:  true,
			Pid:         12345,
		},
	}
	app.browserMgr.BrowserProcesses = make(map[string]*exec.Cmd)

	cmd := shortLivedCommand()
	monitor, err := newBrowserProcessMonitor(cmd)
	if err != nil {
		t.Fatalf("初始化测试进程监控失败: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动短命测试进程失败: %v", err)
	}
	monitor.Start()
	app.browserMgr.BrowserProcesses["profile-detached"] = cmd

	done := make(chan struct{})
	go func() {
		app.waitBrowserProcess("profile-detached", monitor)
		close(done)
	}()

	waitForCondition(t, 3*time.Second, func() bool {
		app.browserMgr.Mutex.Lock()
		defer app.browserMgr.Mutex.Unlock()

		profile := app.browserMgr.Profiles["profile-detached"]
		_, tracked := app.browserMgr.BrowserProcesses["profile-detached"]
		return profile != nil && profile.Running && !tracked
	})

	_ = ln.Close()

	waitForCondition(t, 4*time.Second, func() bool {
		app.browserMgr.Mutex.Lock()
		defer app.browserMgr.Mutex.Unlock()

		profile := app.browserMgr.Profiles["profile-detached"]
		return profile != nil && !profile.Running && profile.DebugPort == 0 && profile.Pid == 0
	})

	select {
	case <-done:
	case <-time.After(4 * time.Second):
		t.Fatal("waitBrowserProcess 未在调试端口关闭后结束")
	}
}

func TestWaitForBrowserDebugReadyMarksProfileReady(t *testing.T) {
	t.Parallel()

	port := freeLoopbackPort(t)
	app := NewApp("")
	app.browserMgr = browser.NewManager(config.DefaultConfig(), "")
	app.browserMgr.Profiles = map[string]*BrowserProfile{
		"profile-ready": {
			ProfileId:      "profile-ready",
			ProfileName:    "Ready Browser",
			Running:        true,
			DebugPort:      port,
			DebugReady:     false,
			RuntimeWarning: "pending",
			LastStartAt:    time.Now().Format(time.RFC3339),
		},
	}
	app.browserMgr.BrowserProcesses = make(map[string]*exec.Cmd)

	serverReady := make(chan *devToolsTestServer, 1)
	go func() {
		time.Sleep(200 * time.Millisecond)
		serverReady <- startDevToolsServerOnPort(t, port, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/json/version":
				_, _ = w.Write([]byte(`{"Browser":"Chrome/142.0","webSocketDebuggerUrl":"ws://127.0.0.1/devtools/browser"}`))
			case "/json/list":
				_, _ = w.Write([]byte(`[{"id":"page-1"}]`))
			default:
				http.NotFound(w, r)
			}
		}))
	}()

	snapshot, changed := app.waitForBrowserDebugReady("profile-ready", port, 2*time.Second)
	server := <-serverReady
	defer server.Close()

	if snapshot == nil {
		t.Fatal("期望等待到调试接口就绪")
	}
	if !changed {
		t.Fatal("期望调试接口就绪后标记实例状态变更")
	}
	if !snapshot.DebugReady {
		t.Fatal("期望实例被标记为调试接口已就绪")
	}
	if snapshot.RuntimeWarning != "" {
		t.Fatalf("期望调试接口就绪后清空警告，实际=%q", snapshot.RuntimeWarning)
	}
}

func TestSetProfileDebugReadyPreservesFingerprintRuntimeWarning(t *testing.T) {
	t.Parallel()

	app := NewApp("")
	app.browserMgr = browser.NewManager(config.DefaultConfig(), "")
	app.browserMgr.Profiles = map[string]*BrowserProfile{
		"profile-ready-warning": {
			ProfileId:      "profile-ready-warning",
			ProfileName:    "Ready Warning Browser",
			Running:        true,
			DebugPort:      9222,
			DebugReady:     false,
			RuntimeWarning: "[fingerprint] 字体风险\n[debug] 调试接口等待中",
		},
	}

	snapshot, changed := app.setProfileDebugReady("profile-ready-warning", 9222)
	if snapshot == nil || !changed {
		t.Fatalf("expected debug ready state change, got snapshot=%+v changed=%v", snapshot, changed)
	}
	if !snapshot.DebugReady {
		t.Fatalf("expected profile to be debug ready")
	}
	if snapshot.RuntimeWarning != "[fingerprint] 字体风险" {
		t.Fatalf("expected fingerprint warning to be preserved and debug warning removed, got %q", snapshot.RuntimeWarning)
	}

	_, changed = app.setProfileDebugReady("profile-ready-warning", 9222)
	if changed {
		t.Fatalf("expected second debug ready call not to report change when only fingerprint warning remains")
	}
}

func TestSanitizeManagedLaunchArgsRemovesSystemManagedFlags(t *testing.T) {
	t.Parallel()

	got, removed := sanitizeManagedLaunchArgs([]string{
		"--lang=en-US",
		"--remote-debugging-port=9222",
		"--user-data-dir", "D:\\profiles\\demo",
		"--proxy-server", "http://127.0.0.1:9000",
		"--remote-debugging-pipe",
		"--enable-automation",
		"--enable-unsafe-swiftshader",
		"https://example.com",
	})

	wantArgs := []string{"--lang=en-US", "https://example.com"}
	if !reflect.DeepEqual(got, wantArgs) {
		t.Fatalf("sanitizeManagedLaunchArgs args mismatch: got=%v want=%v", got, wantArgs)
	}

	wantRemoved := []string{
		"--remote-debugging-port",
		"--user-data-dir",
		"--proxy-server",
		"--remote-debugging-pipe",
		"--enable-automation",
		"--enable-unsafe-swiftshader",
	}
	if !reflect.DeepEqual(removed, wantRemoved) {
		t.Fatalf("sanitizeManagedLaunchArgs removed mismatch: got=%v want=%v", removed, wantRemoved)
	}
}

func TestSanitizeManagedLaunchArgsKeepsUnmanagedFlags(t *testing.T) {
	t.Parallel()

	input := []string{"--lang=en-US", "--disable-sync", "https://example.com"}
	got, removed := sanitizeManagedLaunchArgs(input)
	if !reflect.DeepEqual(got, input) {
		t.Fatalf("sanitizeManagedLaunchArgs should preserve unmanaged args: got=%v want=%v", got, input)
	}
	if len(removed) != 0 {
		t.Fatalf("sanitizeManagedLaunchArgs should not report managed args, got=%v", removed)
	}
}

func TestMergeLaunchArgsDeduplicatesSingleValueFingerprintFlags(t *testing.T) {
	t.Parallel()

	got := mergeLaunchArgs(
		[]string{"--fingerprint=111", "--fingerprint-platform=windows", "--lang=en-US", "--disable-sync"},
		[]string{"--fingerprint-platform=mac", "--window-size=1280,800", "--fingerprint-do-not-track=false"},
		[]string{"--lang=ja-JP", "--fingerprint-webrtc-ip=1.2.3.4", "--window-size=1440,900"},
		[]string{"--fingerprint-do-not-track=true"},
	)
	want := []string{
		"--fingerprint=111",
		"--fingerprint-platform=mac",
		"--lang=ja-JP",
		"--disable-sync",
		"--window-size=1440,900",
		"--fingerprint-do-not-track=true",
		"--fingerprint-webrtc-ip=1.2.3.4",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mergeLaunchArgs mismatch:\n got=%v\nwant=%v", got, want)
	}
}

func TestMergeLaunchArgsNormalizesSeparatedSingleValueFlags(t *testing.T) {
	t.Parallel()

	got := mergeLaunchArgs(
		[]string{"--lang", "en-US", "--fingerprint-platform", "windows", "--disable-sync"},
		[]string{"--lang=ja-JP", "--fingerprint-platform=mac"},
	)
	want := []string{
		"--lang=ja-JP",
		"--fingerprint-platform=mac",
		"--disable-sync",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mergeLaunchArgs separated value mismatch:\n got=%v\nwant=%v", got, want)
	}
}

func TestMergeLaunchArgsCanonicalizesAcceptLanguageAlias(t *testing.T) {
	t.Parallel()

	got := mergeLaunchArgs(
		[]string{"--accept-language", "zh-CN,zh;q=0.9", "--fingerprint-platform=windows"},
		[]string{"--fingerprint-accept-language=en-US,en;q=0.9"},
		[]string{"--accept-language=ja-JP,ja;q=0.9"},
	)
	want := []string{
		"--fingerprint-accept-language=ja-JP,ja;q=0.9",
		"--fingerprint-platform=windows",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mergeLaunchArgs accept-language alias mismatch:\n got=%v\nwant=%v", got, want)
	}
}

func TestBuildBrowserLaunchArgsOmitsDebugPortForManualStart(t *testing.T) {
	t.Parallel()

	profile := &BrowserProfile{ProfileId: "profile-manual", FingerprintArgs: []string{"--fingerprint=111"}}
	got := buildBrowserLaunchArgs(profile, "/tmp/ant-profile", 0, "direct://", []string{"--disable-sync"}, nil, nil, nil, nil, true, false)

	if containsLaunchArgPrefix(got, "--remote-debugging-port") {
		t.Fatalf("manual launch should not include remote debugging port: %v", got)
	}
	if containsLaunchArgPrefix(got, "--remote-debugging-address") {
		t.Fatalf("manual launch should not include remote debugging address: %v", got)
	}
}

func TestBuildBrowserLaunchArgsAddsLoopbackDebugAddressWhenDebugPortRequired(t *testing.T) {
	t.Parallel()

	profile := &BrowserProfile{ProfileId: "profile-debug", FingerprintArgs: []string{"--fingerprint=111"}}
	got := buildBrowserLaunchArgs(profile, "/tmp/ant-profile", 9333, "direct://", nil, nil, nil, nil, nil, true, false)

	if !containsLaunchArg(got, "--remote-debugging-port=9333") {
		t.Fatalf("debug launch should include remote debugging port: %v", got)
	}
	if !containsLaunchArg(got, "--remote-debugging-address=127.0.0.1") {
		t.Fatalf("debug launch should bind remote debugging to loopback: %v", got)
	}
}

func TestEnsureDefaultFingerprintNetworkArgsAddsWebRTCForProxy(t *testing.T) {
	t.Parallel()

	got := ensureDefaultFingerprintNetworkArgs([]string{"--fingerprint=111"}, "http://127.0.0.1:8080")
	want := []string{
		"--fingerprint=111",
		"--webrtc-ip-handling-policy=disable_non_proxied_udp",
		"--fingerprint-webrtc-ip=auto",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ensureDefaultFingerprintNetworkArgs mismatch:\n got=%v\nwant=%v", got, want)
	}
}

func TestEnsureDefaultFingerprintNetworkArgsDoesNotAddAutoForDirectProxy(t *testing.T) {
	t.Parallel()

	got := ensureDefaultFingerprintNetworkArgs([]string{"--fingerprint=111"}, "direct://")
	want := []string{
		"--fingerprint=111",
		"--webrtc-ip-handling-policy=disable_non_proxied_udp",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ensureDefaultFingerprintNetworkArgs direct mismatch:\n got=%v\nwant=%v", got, want)
	}
}

func TestEnsureDefaultFingerprintNetworkArgsKeepsExplicitWebRTCIP(t *testing.T) {
	t.Parallel()

	got := ensureDefaultFingerprintNetworkArgs([]string{"--fingerprint-webrtc-ip=1.2.3.4", "--webrtc-ip-handling-policy=default_public_interface_only"}, "http://127.0.0.1:8080")
	want := []string{"--fingerprint-webrtc-ip=1.2.3.4", "--webrtc-ip-handling-policy=default_public_interface_only"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ensureDefaultFingerprintNetworkArgs explicit mismatch:\n got=%v\nwant=%v", got, want)
	}
}

func TestRedactLaunchArgsForLogMasksProxyCredentials(t *testing.T) {
	t.Parallel()

	got := redactLaunchArgsForLog([]string{
		"--disable-sync",
		"--proxy-server=http://user:secret@example.com:8080",
		"--lang=zh-CN",
	})
	if strings.Contains(got, "secret") {
		t.Fatalf("redactLaunchArgsForLog leaked proxy credentials: %q", got)
	}
	if !strings.Contains(got, "--proxy-server=http://user:%2A%2A%2A@example.com:8080") {
		t.Fatalf("redactLaunchArgsForLog did not include redacted proxy: %q", got)
	}
}

func TestResolveAutoWebRTCIPLaunchArgRemovesAutoForDirectProxy(t *testing.T) {
	t.Parallel()

	app := NewApp("")
	args := []string{"--fingerprint=111", "--fingerprint-webrtc-ip=auto", "--lang=zh-CN"}
	got := app.resolveAutoWebRTCIPLaunchArg("profile-direct", args, "direct://")
	want := []string{"--fingerprint=111", "--lang=zh-CN"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("resolveAutoWebRTCIPLaunchArg direct mismatch:\n got=%v\nwant=%v", got, want)
	}
}

func TestResolveAutoWebRTCIPLaunchArgRemovesSplitAutoForDirectProxy(t *testing.T) {
	t.Parallel()

	app := &App{}
	args := []string{"--fingerprint=111", "--fingerprint-webrtc-ip", "auto", "--lang=en-US"}
	got := app.resolveAutoWebRTCIPLaunchArg("profile-direct", args, "direct://")
	want := []string{"--fingerprint=111", "--lang=en-US"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("resolveAutoWebRTCIPLaunchArg split direct mismatch:\n got=%v\nwant=%v", got, want)
	}
}

func TestResolveAutoWebRTCIPLaunchArgRemovesAutoOnResolutionFailure(t *testing.T) {
	t.Parallel()

	app := NewApp("")
	args := []string{"--fingerprint=111", "--fingerprint-webrtc-ip=auto", "--lang=zh-CN"}
	got := app.resolveAutoWebRTCIPLaunchArg("profile-proxy", args, "http://127.0.0.1:1")
	want := []string{"--fingerprint=111", "--lang=zh-CN"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("resolveAutoWebRTCIPLaunchArg failure mismatch:\n got=%v\nwant=%v", got, want)
	}
}

func TestNormalizeStealthLaunchRequestParamsMapsLocaleTimezoneToArgs(t *testing.T) {
	t.Parallel()

	noViewport := true
	params := normalizeStealthLaunchRequestParams(launchcode.LaunchRequestParams{
		LaunchArgs: []string{"--disable-sync"},
		StealthContext: launchcode.StealthContextOptions{
			Locale:     "en-US",
			TimezoneID: "America/New_York",
			UserAgent:  "Mozilla/5.0 test",
			Viewport:   map[string]int{"width": 1280, "height": 720},
			NoViewport: &noViewport,
		},
	})

	for _, want := range []string{
		"--disable-sync",
		"--lang=en-US",
		"--fingerprint-locale=en-US",
		"--timezone=America/New_York",
		"--fingerprint-timezone=America/New_York",
		"--user-agent=Mozilla/5.0 test",
	} {
		if !containsLaunchArg(params.LaunchArgs, want) {
			t.Fatalf("expected stealth context arg %q, got=%v", want, params.LaunchArgs)
		}
	}
	if containsLaunchArgPrefix(params.LaunchArgs, "--window-size") {
		t.Fatalf("viewport must not be translated to --window-size/CDP emulation args: %v", params.LaunchArgs)
	}
}

func TestCollectLaunchRuntimeWarningsDetectsWindowsFontMismatch(t *testing.T) {
	original := hostFontListingForRuntimeWarning
	originalGOOS := hostGOOSForRuntimeWarning
	hostGOOSForRuntimeWarning = func() string { return "linux" }
	hostFontListingForRuntimeWarning = func() (string, bool) {
		return "Noto Sans\nDejaVu Sans\n", true
	}
	defer func() {
		hostFontListingForRuntimeWarning = original
		hostGOOSForRuntimeWarning = originalGOOS
	}()

	warnings := collectLaunchRuntimeWarnings("profile-fonts", []string{
		"--fingerprint-platform=windows",
		"--fingerprint-fonts=Arial,Calibri,Segoe UI,Times New Roman",
	})

	if len(warnings) == 0 {
		t.Fatalf("expected Windows font runtime warning")
	}
	if !strings.Contains(strings.Join(warnings, "\n"), "Windows 字体") {
		t.Fatalf("unexpected font warning: %v", warnings)
	}
}

func TestCollectLaunchRuntimeWarningsSkipsWhenWindowsFontsPresent(t *testing.T) {
	original := hostFontListingForRuntimeWarning
	originalGOOS := hostGOOSForRuntimeWarning
	hostGOOSForRuntimeWarning = func() string { return "linux" }
	hostFontListingForRuntimeWarning = func() (string, bool) {
		return "Segoe UI\nCalibri\n", true
	}
	defer func() {
		hostFontListingForRuntimeWarning = original
		hostGOOSForRuntimeWarning = originalGOOS
	}()

	warnings := collectLaunchRuntimeWarnings("profile-fonts", []string{
		"--fingerprint-platform=windows",
		"--fingerprint-fonts=Arial,Calibri,Segoe UI,Times New Roman",
	})

	for _, warning := range warnings {
		if strings.Contains(warning, "Windows 字体") {
			t.Fatalf("did not expect font mismatch warning when fonts are present: %v", warnings)
		}
	}
}

func TestCollectLaunchRuntimeWarningsDetectsMacHostWindowsWebGLMismatch(t *testing.T) {
	originalGOOS := hostGOOSForRuntimeWarning
	hostGOOSForRuntimeWarning = func() string { return "darwin" }
	defer func() { hostGOOSForRuntimeWarning = originalGOOS }()

	warnings := collectLaunchRuntimeWarnings("profile-vm-risk", []string{
		"--fingerprint-platform=windows",
		"--fingerprint-webgl-vendor=Intel",
		"--fingerprint-webgl-renderer=Intel(R) UHD Graphics 630",
	})

	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, "Virtual Machine") {
		t.Fatalf("expected virtual machine risk warning for mac host + non-Apple WebGL, got=%v", warnings)
	}
	if !strings.Contains(joined, "宿主平台") {
		t.Fatalf("expected host platform mismatch warning, got=%v", warnings)
	}
}

func TestCollectLaunchRuntimeWarningsDetectsVirtualWebGLRenderer(t *testing.T) {
	t.Parallel()

	warnings := collectLaunchRuntimeWarnings("profile-virtual-renderer", []string{
		"--fingerprint-platform=linux",
		"--fingerprint-webgl-vendor=Google Inc.",
		"--fingerprint-webgl-renderer=Google SwiftShader",
	})

	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, "WebGL") || !strings.Contains(joined, "虚拟化") {
		t.Fatalf("expected virtual WebGL renderer warning, got=%v", warnings)
	}
}

func TestCollectLaunchRuntimeWarningsDetectsUserAgentWithoutUAChRuntime(t *testing.T) {
	t.Setenv("ANT_BROWSER_UACH_RUNTIME", "")

	warnings := collectLaunchRuntimeWarnings("profile-ua", []string{
		"--user-agent=Mozilla/5.0 test",
	})

	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, "UA-CH") {
		t.Fatalf("expected UA-CH warning, got=%v", warnings)
	}
}

func TestCollectLaunchRuntimeWarningsAllowsUAChRuntimeMarker(t *testing.T) {
	t.Setenv("ANT_BROWSER_UACH_RUNTIME", "native")

	warnings := collectLaunchRuntimeWarnings("profile-ua", []string{
		"--user-agent=Mozilla/5.0 test",
	})

	for _, warning := range warnings {
		if strings.Contains(warning, "未检测到 UA-CH runtime") {
			t.Fatalf("did not expect missing UA-CH runtime warning with marker: %v", warnings)
		}
	}
}

func TestCollectLaunchRuntimeWarningsDetectsBooleanFlags(t *testing.T) {
	t.Parallel()

	warnings := collectLaunchRuntimeWarnings("profile-boolean-flags", []string{
		"--headless",
		"--disable-gpu",
	})

	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, "headless") {
		t.Fatalf("expected headless warning for boolean flag, got=%v", warnings)
	}
	if !strings.Contains(joined, "禁用 GPU") {
		t.Fatalf("expected disable-gpu warning for boolean flag, got=%v", warnings)
	}
}

func TestSeedWidevineHintIfAvailableWritesHintOnLinux(t *testing.T) {
	if goruntime.GOOS != "linux" {
		t.Skip("Widevine hint seeding is Linux-only")
	}

	root := t.TempDir()
	binDir := filepath.Join(root, "core")
	cdmDir := filepath.Join(binDir, "WidevineCdm")
	if err := os.MkdirAll(cdmDir, 0o755); err != nil {
		t.Fatalf("create cdm dir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cdmDir, "manifest.json"), []byte(`{"version":"1.0.0.0"}`), 0o644); err != nil {
		t.Fatalf("write manifest failed: %v", err)
	}
	chromePath := filepath.Join(binDir, "chrome")
	if err := os.WriteFile(chromePath, []byte("stub"), 0o755); err != nil {
		t.Fatalf("write chrome stub failed: %v", err)
	}
	userDataDir := filepath.Join(root, "profile")

	resolved, ok := seedWidevineHintIfAvailable("profile-widevine", userDataDir, chromePath)
	if !ok {
		t.Fatalf("expected Widevine hint seeding to succeed")
	}
	if resolved == "" {
		t.Fatalf("expected resolved cdm dir")
	}
	hintPath := filepath.Join(userDataDir, "WidevineCdm", widevineHintFileName)
	data, err := os.ReadFile(hintPath)
	if err != nil {
		t.Fatalf("read hint failed: %v", err)
	}
	if !strings.Contains(string(data), resolved) {
		t.Fatalf("hint does not reference cdm dir: %s", string(data))
	}
}

func TestResolveAutoWebRTCIPLaunchArgRemovesSplitAutoOnResolutionFailure(t *testing.T) {
	t.Parallel()

	app := &App{}
	args := []string{"--fingerprint=111", "--fingerprint-webrtc-ip", "auto", "--lang=en-US"}
	got := app.resolveAutoWebRTCIPLaunchArg("profile-proxy", args, "http://127.0.0.1:1")
	want := []string{"--fingerprint=111", "--lang=en-US"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("resolveAutoWebRTCIPLaunchArg split failure mismatch:\n got=%v\nwant=%v", got, want)
	}
}

func TestResolveAutoWebRTCIPLaunchArgReplacesAutoOnSuccess(t *testing.T) {
	original := resolveFingerprintExitIP
	resolveFingerprintExitIP = func(proxyURL string, cacheKey string, timeout time.Duration) (string, error) {
		if proxyURL != "http://127.0.0.1:18080" {
			t.Fatalf("unexpected proxy URL: %q", proxyURL)
		}
		if cacheKey != "proxy-cache" {
			t.Fatalf("unexpected cache key: %q", cacheKey)
		}
		return "203.0.113.77", nil
	}
	defer func() { resolveFingerprintExitIP = original }()

	app := &App{}
	args := []string{"--fingerprint=111", "--fingerprint-webrtc-ip=auto", "--lang=zh-CN"}
	got := app.resolveAutoWebRTCIPLaunchArgWithCacheKey("profile-proxy", args, "http://127.0.0.1:18080", "proxy-cache")
	want := []string{"--fingerprint=111", "--fingerprint-webrtc-ip=203.0.113.77", "--lang=zh-CN"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("resolveAutoWebRTCIPLaunchArg success mismatch:\n got=%v\nwant=%v", got, want)
	}
}

func containsLaunchArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func containsLaunchArgPrefix(args []string, prefix string) bool {
	for _, arg := range args {
		if strings.HasPrefix(arg, prefix) {
			return true
		}
	}
	return false
}

func launchArgValueForTest(args []string, want string) string {
	want = strings.ToLower(strings.TrimSpace(want))
	if canonical, ok := singleValueLaunchArgKey(want); ok {
		want = canonical
	}
	for _, arg := range args {
		key, ok := singleValueLaunchArgKey(arg)
		if !ok || key != want {
			continue
		}
		value, _ := launchArgValueAt([]string{arg}, 0)
		return value
	}
	return ""
}

func newTemporaryProxyLaunchTestApp(t *testing.T, proxies []config.BrowserProxy, profile *BrowserProfile) *App {
	t.Helper()

	root := t.TempDir()
	coreDir := filepath.Join(root, "chrome-core")
	exePath := filepath.Join(coreDir, filepath.FromSlash(browser.CoreExecutableCandidates()[0]))
	if err := os.MkdirAll(filepath.Dir(exePath), 0o755); err != nil {
		t.Fatalf("创建测试内核目录失败: %v", err)
	}
	if err := os.WriteFile(exePath, []byte("stub"), 0o755); err != nil {
		t.Fatalf("写入测试内核失败: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Browser.Cores = []config.BrowserCore{
		{CoreId: "test-core", CoreName: "Test Core", CorePath: coreDir, IsDefault: true},
	}
	cfg.Browser.Proxies = proxies
	cfg.Browser.DefaultStartURLs = nil
	cfg.Browser.RestoreLastSession = true

	app := NewApp(root)
	app.config = cfg
	app.browserMgr = browser.NewManager(cfg, root)
	profile.CoreId = "test-core"
	if profile.LaunchArgs == nil {
		profile.LaunchArgs = []string{"--disable-sync"}
	}
	app.browserMgr.Profiles = map[string]*BrowserProfile{
		profile.ProfileId: profile,
	}
	return app
}

func TestPrepareBrowserStartPlanLinksTemporaryProxyIDRegionFingerprintArgs(t *testing.T) {
	original := resolveFingerprintExitIP
	resolveFingerprintExitIP = func(proxyURL string, cacheKey string, timeout time.Duration) (string, error) {
		return "203.0.113.88", nil
	}
	defer func() { resolveFingerprintExitIP = original }()

	profile := &BrowserProfile{
		ProfileId:       "profile-temp-proxy-region",
		ProfileName:     "Temporary Proxy Region",
		FingerprintArgs: []string{"--fingerprint=111", "--lang=zh-CN", "--fingerprint-locale=zh-CN", "--fingerprint-accept-language=zh-CN,zh;q=0.9", "--timezone=Asia/Shanghai", "--fingerprint-timezone=Asia/Shanghai", "--fingerprint-do-not-track=false"},
		ProxyId:         "stored-proxy",
		ProxyConfig:     "http://127.0.0.1:18080",
	}
	app := newTemporaryProxyLaunchTestApp(t, []config.BrowserProxy{
		{ProxyId: "stored-proxy", ProxyName: "Stored", ProxyConfig: "http://127.0.0.1:18080", Country: "CN", Locale: "zh-CN", Timezone: "Asia/Shanghai"},
		{ProxyId: "runtime-proxy", ProxyName: "Runtime", ProxyConfig: "http://127.0.0.1:28080", Country: "US", Locale: "en-US", Timezone: "America/Los_Angeles"},
	}, profile)
	input := newBrowserStartInput(profile.ProfileId, nil, nil, false, false, false, false, "runtime-proxy", "")

	plan, err := app.prepareBrowserStartPlan(input, profile)
	if err != nil {
		t.Fatalf("prepareBrowserStartPlan returned error: %v", err)
	}
	if plan.effectiveProxy != "http://127.0.0.1:28080" {
		t.Fatalf("expected runtime proxy, got %q", plan.effectiveProxy)
	}
	for key, want := range map[string]string{
		"--lang":                        "en-US",
		"--fingerprint-locale":          "en-US",
		"--fingerprint-accept-language": "en-US,en;q=0.9",
		"--timezone":                    "America/Los_Angeles",
		"--fingerprint-timezone":        "America/Los_Angeles",
	} {
		if got := launchArgValueForTest(plan.args, key); got != want {
			t.Fatalf("temporary proxy region arg %s mismatch: got=%q want=%q args=%v", key, got, want, plan.args)
		}
	}
}

func TestPrepareBrowserStartPlanTemporaryProxyDoesNotPersistFingerprintArgs(t *testing.T) {
	original := resolveFingerprintExitIP
	resolveFingerprintExitIP = func(proxyURL string, cacheKey string, timeout time.Duration) (string, error) {
		return "203.0.113.89", nil
	}
	defer func() { resolveFingerprintExitIP = original }()

	originalFingerprintArgs := []string{"--fingerprint=222", "--lang=zh-CN", "--fingerprint-locale=zh-CN", "--fingerprint-accept-language=zh-CN,zh;q=0.9", "--timezone=Asia/Shanghai", "--fingerprint-timezone=Asia/Shanghai", "--fingerprint-do-not-track=false"}
	profile := &BrowserProfile{
		ProfileId:       "profile-temp-proxy-no-persist",
		ProfileName:     "Temporary Proxy No Persist",
		FingerprintArgs: append([]string{}, originalFingerprintArgs...),
		ProxyId:         "stored-proxy",
		ProxyConfig:     "http://127.0.0.1:18080",
	}
	app := newTemporaryProxyLaunchTestApp(t, []config.BrowserProxy{
		{ProxyId: "stored-proxy", ProxyName: "Stored", ProxyConfig: "http://127.0.0.1:18080", Country: "CN"},
		{ProxyId: "runtime-proxy", ProxyName: "Runtime", ProxyConfig: "http://127.0.0.1:28080", Country: "US"},
	}, profile)
	input := newBrowserStartInput(profile.ProfileId, nil, nil, false, false, false, false, "runtime-proxy", "")

	plan, err := app.prepareBrowserStartPlan(input, profile)
	if err != nil {
		t.Fatalf("prepareBrowserStartPlan returned error: %v", err)
	}
	if got := launchArgValueForTest(plan.args, "--lang"); got != "en-US" {
		t.Fatalf("expected launch args to use temporary proxy region, got lang=%q args=%v", got, plan.args)
	}
	if !containsLaunchArg(profile.FingerprintArgs, "--fingerprint=222") {
		t.Fatalf("profile fingerprint seed should be preserved while applying defaults, got=%v", profile.FingerprintArgs)
	}
	for _, want := range originalFingerprintArgs[1:] {
		if !containsLaunchArg(profile.FingerprintArgs, want) {
			t.Fatalf("stored profile fingerprint semantic %q should be preserved, got=%v", want, profile.FingerprintArgs)
		}
	}
	for _, blocked := range []string{
		"--lang=en-US",
		"--fingerprint-locale=en-US",
		"--fingerprint-accept-language=en-US,en;q=0.9",
	} {
		if containsLaunchArg(profile.FingerprintArgs, blocked) {
			t.Fatalf("temporary proxy region should not persist %q into profile fingerprint args: %v", blocked, profile.FingerprintArgs)
		}
	}
}

func TestPrepareBrowserStartPlanExtraLaunchArgsOverrideTemporaryProxyRegion(t *testing.T) {
	original := resolveFingerprintExitIP
	resolveFingerprintExitIP = func(proxyURL string, cacheKey string, timeout time.Duration) (string, error) {
		return "203.0.113.90", nil
	}
	defer func() { resolveFingerprintExitIP = original }()

	profile := &BrowserProfile{
		ProfileId:       "profile-temp-proxy-extra-wins",
		ProfileName:     "Temporary Proxy Extra Wins",
		FingerprintArgs: []string{"--fingerprint=333", "--lang=zh-CN", "--fingerprint-locale=zh-CN", "--fingerprint-accept-language=zh-CN,zh;q=0.9", "--timezone=Asia/Shanghai", "--fingerprint-timezone=Asia/Shanghai", "--fingerprint-do-not-track=false"},
		ProxyId:         "stored-proxy",
		ProxyConfig:     "http://127.0.0.1:18080",
	}
	app := newTemporaryProxyLaunchTestApp(t, []config.BrowserProxy{
		{ProxyId: "stored-proxy", ProxyName: "Stored", ProxyConfig: "http://127.0.0.1:18080", Country: "CN"},
		{ProxyId: "runtime-proxy", ProxyName: "Runtime", ProxyConfig: "http://127.0.0.1:28080", Country: "US", Locale: "en-US", Timezone: "America/New_York"},
	}, profile)
	extraArgs := []string{
		"--lang=ja-JP",
		"--fingerprint-locale=ja-JP",
		"--accept-language=ja-JP,ja;q=0.9,en;q=0.8",
		"--timezone=Asia/Tokyo",
		"--fingerprint-timezone=Asia/Tokyo",
	}
	input := newBrowserStartInput(profile.ProfileId, extraArgs, nil, false, false, false, false, "runtime-proxy", "")

	plan, err := app.prepareBrowserStartPlan(input, profile)
	if err != nil {
		t.Fatalf("prepareBrowserStartPlan returned error: %v", err)
	}
	for key, want := range map[string]string{
		"--lang":                        "ja-JP",
		"--fingerprint-locale":          "ja-JP",
		"--fingerprint-accept-language": "ja-JP,ja;q=0.9,en;q=0.8",
		"--timezone":                    "Asia/Tokyo",
		"--fingerprint-timezone":        "Asia/Tokyo",
	} {
		if got := launchArgValueForTest(plan.args, key); got != want {
			t.Fatalf("extra launch arg %s should win: got=%q want=%q args=%v", key, got, want, plan.args)
		}
	}
}

func TestPrepareBrowserStartPlanCustomTemporaryProxyConfigDoesNotInferRegion(t *testing.T) {
	original := resolveFingerprintExitIP
	resolveFingerprintExitIP = func(proxyURL string, cacheKey string, timeout time.Duration) (string, error) {
		return "203.0.113.91", nil
	}
	defer func() { resolveFingerprintExitIP = original }()

	profile := &BrowserProfile{
		ProfileId:       "profile-temp-proxy-custom-no-region",
		ProfileName:     "Temporary Proxy Custom No Region",
		FingerprintArgs: []string{"--fingerprint=444", "--lang=zh-CN", "--fingerprint-locale=zh-CN", "--fingerprint-accept-language=zh-CN,zh;q=0.9", "--timezone=Asia/Shanghai", "--fingerprint-timezone=Asia/Shanghai", "--fingerprint-do-not-track=false"},
		ProxyId:         "stored-proxy",
		ProxyConfig:     "http://127.0.0.1:18080",
	}
	app := newTemporaryProxyLaunchTestApp(t, []config.BrowserProxy{
		{ProxyId: "stored-proxy", ProxyName: "Stored", ProxyConfig: "http://127.0.0.1:18080", Country: "CN"},
		{ProxyId: "runtime-proxy", ProxyName: "Runtime", ProxyConfig: "http://127.0.0.1:28080", Country: "US"},
	}, profile)
	input := newBrowserStartInput(profile.ProfileId, nil, nil, false, false, false, false, "", "http://us-runtime.example:38080")

	plan, err := app.prepareBrowserStartPlan(input, profile)
	if err != nil {
		t.Fatalf("prepareBrowserStartPlan returned error: %v", err)
	}
	if plan.effectiveProxy != "http://us-runtime.example:38080" {
		t.Fatalf("expected custom temporary proxy config, got %q", plan.effectiveProxy)
	}
	for key, want := range map[string]string{
		"--lang":                        "zh-CN",
		"--fingerprint-locale":          "zh-CN",
		"--fingerprint-accept-language": "zh-CN,zh;q=0.9",
		"--timezone":                    "Asia/Shanghai",
		"--fingerprint-timezone":        "Asia/Shanghai",
	} {
		if got := launchArgValueForTest(plan.args, key); got != want {
			t.Fatalf("custom temporary proxy should not infer %s: got=%q want=%q args=%v", key, got, want, plan.args)
		}
	}
}

func TestPrepareBrowserStartPlanTemporaryProxyUsesIPHealthRegionCache(t *testing.T) {
	original := resolveFingerprintExitIP
	resolveFingerprintExitIP = func(proxyURL string, cacheKey string, timeout time.Duration) (string, error) {
		return "203.0.113.92", nil
	}
	defer func() { resolveFingerprintExitIP = original }()

	profile := &BrowserProfile{
		ProfileId:       "profile-temp-proxy-ip-health",
		ProfileName:     "Temporary Proxy IP Health",
		FingerprintArgs: []string{"--fingerprint=555", "--lang=zh-CN", "--fingerprint-locale=zh-CN", "--fingerprint-accept-language=zh-CN,zh;q=0.9", "--timezone=Asia/Shanghai", "--fingerprint-timezone=Asia/Shanghai", "--fingerprint-do-not-track=false"},
		ProxyId:         "stored-proxy",
		ProxyConfig:     "http://127.0.0.1:18080",
	}
	app := newTemporaryProxyLaunchTestApp(t, []config.BrowserProxy{
		{ProxyId: "stored-proxy", ProxyName: "Stored", ProxyConfig: "http://127.0.0.1:18080", Country: "CN"},
		{ProxyId: "runtime-proxy", ProxyName: "Runtime", ProxyConfig: "http://127.0.0.1:28080", LastIPHealthJSON: `{"ok":true,"country":"JP","locale":"ja-JP","timezone":"Asia/Tokyo"}`},
	}, profile)
	input := newBrowserStartInput(profile.ProfileId, nil, nil, false, false, false, false, "runtime-proxy", "")

	plan, err := app.prepareBrowserStartPlan(input, profile)
	if err != nil {
		t.Fatalf("prepareBrowserStartPlan returned error: %v", err)
	}
	for key, want := range map[string]string{
		"--lang":                        "ja-JP",
		"--fingerprint-locale":          "ja-JP",
		"--fingerprint-accept-language": "ja-JP,ja;q=0.9,en;q=0.8",
		"--timezone":                    "Asia/Tokyo",
		"--fingerprint-timezone":        "Asia/Tokyo",
	} {
		if got := launchArgValueForTest(plan.args, key); got != want {
			t.Fatalf("ip health region arg %s mismatch: got=%q want=%q args=%v", key, got, want, plan.args)
		}
	}
}

func TestResolveBrowserStartProxyUsesTemporaryProxyWithoutMutatingProfile(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultConfig()
	cfg.Browser.Proxies = []config.BrowserProxy{
		{ProxyId: "stored-proxy", ProxyName: "Stored", ProxyConfig: "http://127.0.0.1:18080"},
		{ProxyId: "runtime-proxy", ProxyName: "Runtime", ProxyConfig: "http://127.0.0.1:28080"},
	}
	app := NewApp("")
	app.config = cfg
	app.browserMgr = browser.NewManager(cfg, t.TempDir())
	profile := &BrowserProfile{
		ProfileId:   "profile-temporary-proxy",
		ProfileName: "Temporary Proxy",
		ProxyId:     "stored-proxy",
		ProxyConfig: "http://127.0.0.1:18080",
	}
	input := newBrowserStartInput(profile.ProfileId, nil, nil, false, false, false, false, "runtime-proxy", "")

	effectiveProxy, exitIPCacheKey, bridgeKey, releaseBridge, temporaryProxyRegionArgs, err := app.resolveBrowserStartProxy(input, profile)
	if err != nil {
		t.Fatalf("resolveBrowserStartProxy returned error: %v", err)
	}
	if effectiveProxy != "http://127.0.0.1:28080" {
		t.Fatalf("expected temporary proxy, got %q", effectiveProxy)
	}
	if exitIPCacheKey != "runtime-proxy|http://127.0.0.1:28080" {
		t.Fatalf("expected cache key to include temporary proxy identity, got %q", exitIPCacheKey)
	}
	if bridgeKey != "" || releaseBridge {
		t.Fatalf("plain HTTP proxy should not acquire bridge: key=%q release=%v", bridgeKey, releaseBridge)
	}
	if len(temporaryProxyRegionArgs) != 0 {
		t.Fatalf("temporary proxy without region should not add region args: %v", temporaryProxyRegionArgs)
	}
	if profile.ProxyId != "stored-proxy" || profile.ProxyConfig != "http://127.0.0.1:18080" {
		t.Fatalf("temporary proxy should not mutate profile: %+v", profile)
	}

	fallbackInput := newBrowserStartInput(profile.ProfileId, nil, nil, false, false, false, false, "missing-proxy", "http://127.0.0.1:38080")
	effectiveProxy, exitIPCacheKey, bridgeKey, releaseBridge, temporaryProxyRegionArgs, err = app.resolveBrowserStartProxy(fallbackInput, profile)
	if err != nil {
		t.Fatalf("fallback temporary proxy returned error: %v", err)
	}
	if effectiveProxy != "http://127.0.0.1:38080" {
		t.Fatalf("expected fallback temporary proxy config, got %q", effectiveProxy)
	}
	if exitIPCacheKey != "http://127.0.0.1:38080" {
		t.Fatalf("expected fallback cache key to use explicit proxy config, got %q", exitIPCacheKey)
	}
	if bridgeKey != "" || releaseBridge {
		t.Fatalf("fallback HTTP proxy should not acquire bridge: key=%q release=%v", bridgeKey, releaseBridge)
	}
	if len(temporaryProxyRegionArgs) != 0 {
		t.Fatalf("custom fallback proxy should not infer region args: %v", temporaryProxyRegionArgs)
	}
	if profile.ProxyId != "stored-proxy" || profile.ProxyConfig != "http://127.0.0.1:18080" {
		t.Fatalf("fallback temporary proxy should not mutate profile: %+v", profile)
	}
}

func TestAppendLaunchTargetsUsesConfiguredDefaultStartURLs(t *testing.T) {
	t.Parallel()

	got := appendLaunchTargets([]string{"--disable-sync"}, nil, []string{"https://one.example/", "https://two.example/"}, false, false)
	want := []string{"--disable-sync", "https://one.example/", "https://two.example/"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("appendLaunchTargets mismatch: got=%v want=%v", got, want)
	}
}

func TestAppendLaunchTargetsUsesBlankPageWhenSessionRestoreDisabled(t *testing.T) {
	t.Parallel()

	got := appendLaunchTargets([]string{"--disable-sync"}, nil, []string{}, false, false)
	want := []string{"--disable-sync", "about:blank"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("appendLaunchTargets should fall back to about:blank: got=%v want=%v", got, want)
	}
}

func TestAppendLaunchTargetsPreservesSessionRestoreWhenEnabled(t *testing.T) {
	t.Parallel()

	got := appendLaunchTargets([]string{"--disable-sync"}, nil, []string{}, false, true)
	want := []string{"--disable-sync"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("appendLaunchTargets should preserve session restore behavior: got=%v want=%v", got, want)
	}
}

func mustListenLoopback(t *testing.T) net.Listener {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("监听测试端口失败: %v", err)
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	return ln
}

func listenerPort(t *testing.T, ln net.Listener) int {
	t.Helper()

	tcpAddr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("解析监听地址失败: %T", ln.Addr())
	}
	return tcpAddr.Port
}

func shortLivedCommand() *exec.Cmd {
	if goruntime.GOOS == "windows" {
		return exec.Command("cmd", "/c", "exit", "0")
	}
	return exec.Command("sh", "-c", "exit 0")
}

func longLivedCommand(duration time.Duration) *exec.Cmd {
	if goruntime.GOOS == "windows" {
		seconds := int(duration / time.Second)
		if seconds < 1 {
			seconds = 1
		}
		return exec.Command("cmd", "/c", fmt.Sprintf("ping -n %d 127.0.0.1 >nul", seconds+1))
	}
	return exec.Command("sh", "-c", fmt.Sprintf("sleep %.1f", duration.Seconds()))
}

func stderrFailingCommand(message string) *exec.Cmd {
	if goruntime.GOOS == "windows" {
		return exec.Command("cmd", "/c", fmt.Sprintf("echo %s 1>&2 & exit 5", message))
	}
	return exec.Command("sh", "-c", fmt.Sprintf("echo '%s' 1>&2; exit 5", message))
}

func stderrPortCommand(port int, holdFor time.Duration) *exec.Cmd {
	if goruntime.GOOS == "windows" {
		seconds := int(holdFor / time.Second)
		if seconds < 1 {
			seconds = 1
		}
		// ping -n N waits roughly N-1 seconds on Windows.
		return exec.Command("cmd", "/c", fmt.Sprintf("echo DevTools listening on ws://127.0.0.1:%d/devtools/browser/test 1>&2 & ping -n %d 127.0.0.1 >nul", port, seconds+1))
	}
	return exec.Command("sh", "-c", fmt.Sprintf("echo 'DevTools listening on ws://127.0.0.1:%d/devtools/browser/test' 1>&2; sleep %.1f", port, holdFor.Seconds()))
}

func waitForCondition(t *testing.T, timeout time.Duration, check func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("等待条件成立超时")
}

func freeLoopbackPort(t *testing.T) int {
	t.Helper()

	ln := mustListenLoopback(t)
	port := listenerPort(t, ln)
	_ = ln.Close()
	return port
}

type devToolsTestServer struct {
	port   int
	server *http.Server
	done   chan struct{}
}

func startDevToolsServer(t *testing.T, handler http.Handler) *devToolsTestServer {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("启动 DevTools 测试服务失败: %v", err)
	}

	srv := &http.Server{Handler: handler}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = srv.Serve(ln)
	}()

	return &devToolsTestServer{
		port:   listenerPort(t, ln),
		server: srv,
		done:   done,
	}
}

func startDevToolsServerOnPort(t *testing.T, port int, handler http.Handler) *devToolsTestServer {
	t.Helper()

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("在指定端口启动 DevTools 测试服务失败: %v", err)
	}

	srv := &http.Server{Handler: handler}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = srv.Serve(ln)
	}()

	return &devToolsTestServer{
		port:   port,
		server: srv,
		done:   done,
	}
}

func (s *devToolsTestServer) Close() error {
	if s == nil || s.server == nil {
		return nil
	}
	err := s.server.Close()
	<-s.done
	return err
}

func writeDevToolsActivePortFile(t *testing.T, userDataDir string, port int) {
	t.Helper()

	content := fmt.Sprintf("%d\n/devtools/browser/test\n", port)
	if err := os.WriteFile(filepath.Join(userDataDir, "DevToolsActivePort"), []byte(content), 0644); err != nil {
		t.Fatalf("写入 DevToolsActivePort 失败: %v", err)
	}
}
