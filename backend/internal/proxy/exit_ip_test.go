package proxy

import (
	"ant-chrome/backend/internal/config"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRedactProxyURLMasksCredentials(t *testing.T) {
	t.Parallel()

	got := RedactProxyURL("http://user:secret@example.com:8080")
	want := "http://user:%2A%2A%2A@example.com:8080"
	if got != want {
		t.Fatalf("RedactProxyURL mismatch: got=%q want=%q", got, want)
	}
	if got == "http://user:secret@example.com:8080" {
		t.Fatal("RedactProxyURL leaked original credentials")
	}
}

func TestRedactProxyURLMasksNonStandardProxySecrets(t *testing.T) {
	t.Parallel()

	if got := RedactProxyURL("trojan://super-secret@example.com:443?sni=example.com&password=query-secret"); strings.Contains(got, "super-secret") || strings.Contains(got, "query-secret") {
		t.Fatalf("RedactProxyURL leaked trojan secrets: %q", got)
	}
	chain := "chain+socks5://%7B%22first%22%3A%7B%22password%22%3A%22p1%22%7D%2C%22uuid%22%3A%22id1%22%7D"
	if got := RedactProxyURL(chain); strings.Contains(got, "p1") || strings.Contains(got, "id1") {
		t.Fatalf("RedactProxyURL leaked chain secrets: %q", got)
	}
	yaml := "type: hysteria2\nserver: example.com\npassword: node-secret\nuuid: node-id\n"
	if got := RedactProxyURL(yaml); strings.Contains(got, "node-secret") || strings.Contains(got, "node-id") {
		t.Fatalf("RedactProxyURL leaked YAML secrets: %q", got)
	}
}

func TestRedactProxyURLLeavesPlainProxyURL(t *testing.T) {
	t.Parallel()

	raw := "socks5://127.0.0.1:1080"
	if got := RedactProxyURL(raw); got != raw {
		t.Fatalf("RedactProxyURL should keep plain proxy URL: got=%q want=%q", got, raw)
	}
}

func TestResolveExitIPWithClientFallsBackAndValidatesIP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bad":
			_, _ = w.Write([]byte("not-an-ip"))
		case "/ok":
			_, _ = w.Write([]byte("203.0.113.10\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	originalURLs := exitIPEchoURLs
	exitIPEchoURLs = []string{server.URL + "/bad", server.URL + "/ok"}
	defer func() { exitIPEchoURLs = originalURLs }()

	got, err := resolveExitIPWithClient(server.Client(), time.Second)
	if err != nil {
		t.Fatalf("resolveExitIPWithClient returned error: %v", err)
	}
	if got != "203.0.113.10" {
		t.Fatalf("resolveExitIPWithClient IP mismatch: got=%q", got)
	}
}

func TestResolveExitIPUsesCache(t *testing.T) {
	resetExitIPCacheForTest()
	defer resetExitIPCacheForTest()

	serverHits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverHits++
		_, _ = w.Write([]byte("203.0.113.11"))
	}))
	defer server.Close()

	originalURLs := exitIPEchoURLs
	originalBuilder := exitIPHTTPClientBuilder
	exitIPEchoURLs = []string{server.URL}
	exitIPHTTPClientBuilder = func(src string, proxyId string, proxies []config.BrowserProxy, xrayMgr *XrayManager, singboxMgr *SingBoxManager, timeout time.Duration) (*http.Client, error) {
		return server.Client(), nil
	}
	defer func() {
		exitIPEchoURLs = originalURLs
		exitIPHTTPClientBuilder = originalBuilder
	}()

	first, err := ResolveExitIP("http://127.0.0.1:8080", time.Second)
	if err != nil {
		t.Fatalf("first ResolveExitIP returned error: %v", err)
	}
	second, err := ResolveExitIP("http://127.0.0.1:8080", time.Second)
	if err != nil {
		t.Fatalf("second ResolveExitIP returned error: %v", err)
	}
	if first != "203.0.113.11" || second != first {
		t.Fatalf("cached IP mismatch: first=%q second=%q", first, second)
	}
	if serverHits != 1 {
		t.Fatalf("expected one server hit due to cache, got %d", serverHits)
	}
}

func TestResolveExitIPUsesExplicitCacheKey(t *testing.T) {
	resetExitIPCacheForTest()
	defer resetExitIPCacheForTest()

	serverHits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverHits++
		if serverHits == 1 {
			_, _ = w.Write([]byte("203.0.113.11"))
			return
		}
		_, _ = w.Write([]byte("203.0.113.12"))
	}))
	defer server.Close()

	originalURLs := exitIPEchoURLs
	originalBuilder := exitIPHTTPClientBuilder
	exitIPEchoURLs = []string{server.URL}
	exitIPHTTPClientBuilder = func(src string, proxyId string, proxies []config.BrowserProxy, xrayMgr *XrayManager, singboxMgr *SingBoxManager, timeout time.Duration) (*http.Client, error) {
		return server.Client(), nil
	}
	defer func() {
		exitIPEchoURLs = originalURLs
		exitIPHTTPClientBuilder = originalBuilder
	}()

	first, err := ResolveExitIPWithCacheKey("socks5://127.0.0.1:18080", "node-a", time.Second)
	if err != nil {
		t.Fatalf("first ResolveExitIPWithCacheKey returned error: %v", err)
	}
	second, err := ResolveExitIPWithCacheKey("socks5://127.0.0.1:18080", "node-b", time.Second)
	if err != nil {
		t.Fatalf("second ResolveExitIPWithCacheKey returned error: %v", err)
	}
	third, err := ResolveExitIPWithCacheKey("socks5://127.0.0.1:18080", "node-a", time.Second)
	if err != nil {
		t.Fatalf("third ResolveExitIPWithCacheKey returned error: %v", err)
	}
	if first != "203.0.113.11" || second != "203.0.113.12" || third != first {
		t.Fatalf("cache key isolation mismatch: first=%q second=%q third=%q", first, second, third)
	}
	if serverHits != 2 {
		t.Fatalf("expected two server hits for two cache keys, got %d", serverHits)
	}
}
