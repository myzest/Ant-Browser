package launchcode

import (
	"testing"

	"ant-chrome/backend/internal/browser"
)

func TestProfileRuntimePayloadKeepsCDPCompatibility(t *testing.T) {
	srv := NewLaunchServer(nil, nil, nil, 0)
	profile := &browser.Profile{
		ProfileId:       "p-cdp",
		ProfileName:     "CDP",
		Running:         true,
		DebugReady:      true,
		DebugPort:       9333,
		RuntimeProtocol: browser.RuntimeProtocolCDP,
	}
	srv.SetActiveProfile(profile)

	payload := srv.profileRuntimePayload(profile)
	if payload["runtimeProtocol"] != browser.RuntimeProtocolCDP {
		t.Fatalf("runtimeProtocol = %v", payload["runtimeProtocol"])
	}
	if payload["cdpUrl"] == "" {
		t.Fatalf("cdpUrl should remain populated for CDP payload: %+v", payload)
	}
	if payload["runtimeEndpoint"] == "" {
		t.Fatalf("runtimeEndpoint should be populated for CDP payload: %+v", payload)
	}
}

func TestProfileRuntimePayloadSupportsCamoufoxBridgeEndpoint(t *testing.T) {
	srv := NewLaunchServer(nil, nil, nil, 0)
	bridgeEndpoint := "http://127.0.0.1:41000/bridge/token"
	profile := &browser.Profile{
		ProfileId:          "p-camoufox",
		ProfileName:        "Camoufox",
		Running:            true,
		DebugReady:         true,
		RuntimeProtocol:    browser.RuntimeProtocolPlaywright,
		RuntimeEndpoint:    bridgeEndpoint,
		PlaywrightEndpoint: bridgeEndpoint,
	}
	srv.SetActiveProfile(profile)

	payload := srv.profileRuntimePayload(profile)
	if payload["runtimeProtocol"] != browser.RuntimeProtocolPlaywright {
		t.Fatalf("runtimeProtocol = %v", payload["runtimeProtocol"])
	}
	if payload["runtimeEndpoint"] != bridgeEndpoint {
		t.Fatalf("runtimeEndpoint = %v", payload["runtimeEndpoint"])
	}
	if payload["playwrightEndpoint"] != bridgeEndpoint {
		t.Fatalf("playwrightEndpoint = %v", payload["playwrightEndpoint"])
	}
	// 字段名保留是为了兼容旧客户端；Camoufox persistent context 下值可以是 HTTP bridge endpoint。
	if payload["playwrightWsEndpoint"] != bridgeEndpoint {
		t.Fatalf("playwrightWsEndpoint = %v", payload["playwrightWsEndpoint"])
	}
	if payload["cdpUrl"] != "" {
		t.Fatalf("camoufox bridge payload must not expose cdpUrl, got %+v", payload)
	}
}

func TestProfileRuntimePayloadSupportsPlaywrightWithoutCDPURL(t *testing.T) {
	srv := NewLaunchServer(nil, nil, nil, 0)
	profile := &browser.Profile{
		ProfileId:          "p-pw",
		ProfileName:        "PW",
		Running:            true,
		DebugReady:         true,
		RuntimeProtocol:    browser.RuntimeProtocolPlaywright,
		RuntimeEndpoint:    "ws://127.0.0.1:41000/ws",
		PlaywrightEndpoint: "ws://127.0.0.1:41000/ws",
	}
	srv.SetActiveProfile(profile)

	payload := srv.profileRuntimePayload(profile)
	if payload["runtimeProtocol"] != browser.RuntimeProtocolPlaywright {
		t.Fatalf("runtimeProtocol = %v", payload["runtimeProtocol"])
	}
	if payload["runtimeEndpoint"] != "ws://127.0.0.1:41000/ws" {
		t.Fatalf("runtimeEndpoint = %v", payload["runtimeEndpoint"])
	}
	if payload["playwrightWsEndpoint"] != "ws://127.0.0.1:41000/ws" {
		t.Fatalf("playwrightWsEndpoint = %v", payload["playwrightWsEndpoint"])
	}
	if payload["cdpUrl"] != "" {
		t.Fatalf("playwright payload must not expose cdpUrl, got %+v", payload)
	}
}
