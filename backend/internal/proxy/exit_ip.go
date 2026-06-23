package proxy

import (
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	exitIPSuccessCacheTTL = 10 * time.Minute
	exitIPFailureCacheTTL = 45 * time.Second
)

var exitIPEchoURLs = []string{
	"https://api.ipify.org",
	"https://checkip.amazonaws.com",
	"https://ifconfig.me/ip",
}

var exitIPHTTPClientBuilder = buildProxyHTTPClient

type exitIPCacheEntry struct {
	ip        string
	err       error
	expiresAt time.Time
}

type exitIPInflightCall struct {
	wg  sync.WaitGroup
	ip  string
	err error
}

var exitIPCache = map[string]exitIPCacheEntry{}
var exitIPInflight = map[string]*exitIPInflightCall{}
var exitIPCacheMu sync.Mutex

func ResolveExitIP(proxyURL string, timeout time.Duration) (string, error) {
	return ResolveExitIPWithCacheKey(proxyURL, "", timeout)
}

func ResolveExitIPWithCacheKey(proxyURL string, cacheKey string, timeout time.Duration) (ip string, err error) {
	proxyURL = strings.TrimSpace(proxyURL)
	if proxyURL == "" || strings.EqualFold(proxyURL, "direct://") {
		return "", fmt.Errorf("proxy is direct")
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	cacheKey = strings.TrimSpace(cacheKey)
	if cacheKey == "" {
		cacheKey = proxyURL
	}
	exitIPCacheMu.Lock()
	if cached, ok := exitIPCache[cacheKey]; ok && time.Now().Before(cached.expiresAt) {
		exitIPCacheMu.Unlock()
		return cached.ip, cached.err
	}
	if inflight, ok := exitIPInflight[cacheKey]; ok {
		exitIPCacheMu.Unlock()
		inflight.wg.Wait()
		return inflight.ip, inflight.err
	}
	inflight := &exitIPInflightCall{}
	inflight.wg.Add(1)
	exitIPInflight[cacheKey] = inflight
	exitIPCacheMu.Unlock()
	defer func() {
		inflight.ip = ip
		inflight.err = err
		inflight.wg.Done()
		exitIPCacheMu.Lock()
		delete(exitIPInflight, cacheKey)
		exitIPCacheMu.Unlock()
	}()

	client, err := exitIPHTTPClientBuilder(proxyURL, "", nil, nil, nil, timeout)
	if err != nil {
		cacheExitIP(cacheKey, "", err, exitIPFailureCacheTTL)
		return "", err
	}

	ip, err = resolveExitIPWithClient(client, timeout)
	if err != nil {
		cacheExitIP(cacheKey, "", err, exitIPFailureCacheTTL)
		return "", err
	}
	cacheExitIP(cacheKey, ip, nil, exitIPSuccessCacheTTL)
	return ip, nil
}

func resolveExitIPWithClient(client *http.Client, timeout time.Duration) (string, error) {
	if client == nil {
		return "", fmt.Errorf("http client is nil")
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	deadline := time.Now().Add(timeout)
	var lastErr error
	for _, targetURL := range exitIPEchoURLs {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			lastErr = fmt.Errorf("exit IP resolution timed out after %s", timeout)
			break
		}
		client.Timeout = remaining
		req, err := http.NewRequest(http.MethodGet, targetURL, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Accept", "text/plain")
		req.Header.Set("User-Agent", "AntBrowser/1.0")

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 128))
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("%s returned HTTP %d", targetURL, resp.StatusCode)
			continue
		}

		ip := strings.TrimSpace(string(body))
		if _, err := netip.ParseAddr(ip); err != nil {
			lastErr = fmt.Errorf("%s returned invalid IP %q", targetURL, ip)
			continue
		}
		return ip, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no exit IP echo endpoint configured")
	}
	return "", lastErr
}

func cacheExitIP(key string, ip string, err error, ttl time.Duration) {
	if ttl <= 0 {
		return
	}
	exitIPCacheMu.Lock()
	defer exitIPCacheMu.Unlock()
	exitIPCache[key] = exitIPCacheEntry{ip: ip, err: err, expiresAt: time.Now().Add(ttl)}
}

func resetExitIPCacheForTest() {
	exitIPCacheMu.Lock()
	defer exitIPCacheMu.Unlock()
	exitIPCache = map[string]exitIPCacheEntry{}
	exitIPInflight = map[string]*exitIPInflightCall{}
}

func RedactProxyURL(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if strings.Contains(value, "\n") {
		return redactSensitiveProxyText(value)
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, chainSocks5Prefix) {
		payload := strings.TrimPrefix(value, chainSocks5Prefix)
		if decoded, err := url.QueryUnescape(payload); err == nil {
			return chainSocks5Prefix + url.QueryEscape(redactSensitiveProxyText(decoded))
		}
		return chainSocks5Prefix + url.QueryEscape(redactSensitiveProxyText(payload))
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return redactSensitiveProxyText(value)
	}
	if parsed.User != nil {
		scheme := strings.ToLower(parsed.Scheme)
		switch scheme {
		case "http", "https", "socks5", "socks4":
			username := parsed.User.Username()
			if username == "" {
				parsed.User = url.User("***")
			} else {
				parsed.User = url.UserPassword(username, "***")
			}
		default:
			parsed.User = url.User("***")
		}
	}
	query := parsed.Query()
	for key := range query {
		if isSensitiveProxyKey(key) {
			query.Set(key, "***")
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

var sensitiveProxyLineRE = regexp.MustCompile(`(?i)(["']?(?:password|passwd|uuid|token|secret|api[_-]?key|access[_-]?key|private[_-]?key|obfs-password)["']?\s*[:=]\s*["']?)([^"',\s}\]]+)`)

func redactSensitiveProxyText(value string) string {
	return sensitiveProxyLineRE.ReplaceAllString(value, "${1}***")
}

func isSensitiveProxyKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "password", "passwd", "pass", "pwd", "uuid", "token", "secret", "api_key", "api-key", "access_key", "access-key", "private_key", "private-key", "key", "obfs-password":
		return true
	default:
		return false
	}
}
