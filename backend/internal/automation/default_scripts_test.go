package automation

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDefaultScriptsIncludesFingerprintAudit(t *testing.T) {
	var script ScriptRecord
	found := false
	for _, item := range DefaultScripts() {
		if item.ID == FingerprintAuditScriptID {
			script = item
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected builtin script %q", FingerprintAuditScriptID)
	}

	if script.Type != "playwright-cdp" {
		t.Fatalf("expected playwright-cdp script type, got %q", script.Type)
	}
	if script.Status != "ready" {
		t.Fatalf("expected ready status, got %q", script.Status)
	}
	if script.EntryFile != defaultScriptEntryFile {
		t.Fatalf("expected default entry file, got %q", script.EntryFile)
	}
	if script.Source.Path != FingerprintAuditScriptID {
		t.Fatalf("expected source path %q, got %q", FingerprintAuditScriptID, script.Source.Path)
	}

	var params map[string]any
	if err := json.Unmarshal([]byte(script.ParamsText), &params); err != nil {
		t.Fatalf("params text must be valid JSON: %v", err)
	}
	for _, key := range []string{
		"detectors",
		"localProbeOnly",
		"captureScreenshot",
		"saveHtml",
		"enableHeaderEcho",
		"headerEchoUrl",
		"probePersistStorage",
		"webrtcProbeTimeoutMs",
		"waitAfterLoadMs",
		"timeoutMs",
	} {
		if _, ok := params[key]; !ok {
			t.Fatalf("expected params key %q in fingerprint audit defaults", key)
		}
	}

	for _, snippet := range []string{
		"schemaVersion: 2",
		"collectLocalProbe",
		"collectHeaderEcho",
		"buildFingerprintMatrix",
		"uaChHeaders",
		"viewportGeometry",
		"gpuWebgpu",
		"pluginsWidevine",
		"mediaCapabilities",
		"MediaSource.isTypeSupported",
		"canPlayType",
		"outerWidth 小于 innerWidth",
		"ua_ch_major_mismatch",
		"widevine_without_mp4_codecs",
		"geolocationPermissions",
		"navigator.webdriver",
		"navigator.userAgentData",
		"getHighEntropyValues",
		"navigator.languages",
		"Intl.DateTimeFormat().resolvedOptions()",
		"WEBGL_debug_renderer_info",
		"navigator.gpu",
		"RTCPeerConnection",
		"stun:stun.l.google.com:19302",
		"navigator.plugins",
		"navigator.mimeTypes",
		"navigator.storage.estimate",
		"indexedDB.open",
		"navigator.requestMediaKeySystemAccess",
		"navigator.permissions.query",
		"client_hints.json 是 data-only skeleton",
		"local-probe.json",
		"report.json",
		"page.screenshot",
		"page.content",
	} {
		if !strings.Contains(script.ScriptText, snippet) {
			t.Fatalf("expected script text to contain %q", snippet)
		}
	}
}
