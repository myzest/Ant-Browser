package geoip

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ant-chrome/backend/internal/config"

	"github.com/oschwald/geoip2-golang"
)

const (
	DefaultProvider  = "maxmind"
	DefaultEditionID = "GeoLite2-City"
	defaultCacheDir  = "data/geoip"
)

type Result struct {
	IP        string
	Country   string
	Region    string
	City      string
	Timezone  string
	Latitude  float64
	Longitude float64
	Source    string
	UpdatedAt string
	Stale     bool
	Error     string
}

type Service struct {
	cfg      config.GeoIPConfig
	cacheDir string
	now      func() time.Time
	openDB   func(string) (*geoip2.Reader, error)
	download func(context.Context, config.GeoIPConfig, string) error
}

var databaseUpdateMu sync.Mutex

func NewService(cfg config.GeoIPConfig, cacheDir string) *Service {
	cfg = normalizeConfig(cfg)
	if strings.TrimSpace(cacheDir) == "" {
		cacheDir = cfg.CacheDir
	}
	return &Service{
		cfg:      cfg,
		cacheDir: cacheDir,
		now:      time.Now,
		openDB:   geoip2.Open,
		download: downloadMaxMindDatabase,
	}
}

func Enabled(cfg config.GeoIPConfig) bool {
	return normalizeConfig(cfg).Enabled
}

func (s *Service) Lookup(ctx context.Context, ip string) (Result, error) {
	if s == nil {
		return Result{}, fmt.Errorf("geoip service is nil")
	}
	cfg := normalizeConfig(s.cfg)
	if !cfg.Enabled {
		return Result{}, fmt.Errorf("geoip is disabled")
	}
	if cfg.Provider != DefaultProvider {
		return Result{}, fmt.Errorf("unsupported geoip provider %q", cfg.Provider)
	}
	ip = strings.TrimSpace(ip)
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return Result{}, fmt.Errorf("invalid ip %q", ip)
	}
	dbPath, stale, staleError, err := s.ensureDatabase(ctx)
	if err != nil {
		return Result{}, err
	}
	reader, err := s.openDB(dbPath)
	if err != nil {
		return Result{}, fmt.Errorf("open geoip database: %w", err)
	}
	defer reader.Close()
	record, err := reader.City(parsed)
	if err != nil {
		return Result{}, fmt.Errorf("lookup geoip city: %w", err)
	}
	country := strings.TrimSpace(record.Country.IsoCode)
	if country == "" {
		country = strings.TrimSpace(record.RegisteredCountry.IsoCode)
	}
	if country == "" {
		country = strings.TrimSpace(record.RepresentedCountry.IsoCode)
	}
	result := Result{
		IP:        ip,
		Country:   strings.ToUpper(country),
		Region:    firstEnglishName(record.Subdivisions),
		City:      name(record.City.Names),
		Timezone:  strings.TrimSpace(record.Location.TimeZone),
		Latitude:  record.Location.Latitude,
		Longitude: record.Location.Longitude,
		Source:    "geoip:" + cfg.Provider,
		UpdatedAt: s.now().Format(time.RFC3339),
		Stale:     stale,
		Error:     staleError,
	}
	if result.Country == "" && result.Timezone == "" && result.City == "" {
		return Result{}, fmt.Errorf("geoip record has no usable location fields for %s", ip)
	}
	return result, nil
}

func (s *Service) ensureDatabase(ctx context.Context) (string, bool, string, error) {
	cfg := normalizeConfig(s.cfg)
	if path := strings.TrimSpace(cfg.DatabasePath); path != "" {
		if _, err := os.Stat(path); err != nil {
			return "", false, "", fmt.Errorf("geoip database not found: %w", err)
		}
		return path, false, "", nil
	}
	cacheDir := strings.TrimSpace(s.cacheDir)
	if cacheDir == "" {
		cacheDir = defaultCacheDir
	}
	dbPath := filepath.Join(cacheDir, cfg.EditionID+".mmdb")
	if isFreshFile(dbPath, cfg.MaxAgeDays, s.now()) || !cfg.AutoUpdate {
		if _, err := os.Stat(dbPath); err == nil {
			return dbPath, false, "", nil
		}
	}
	if !cfg.AutoUpdate {
		return "", false, "", fmt.Errorf("geoip database is missing and auto_update is disabled")
	}
	if strings.TrimSpace(cfg.AccountID) == "" || strings.TrimSpace(cfg.LicenseKey) == "" {
		return "", false, "", fmt.Errorf("geoip maxmind account_id/license_key is required for auto update")
	}
	if s.download == nil {
		return "", false, "", fmt.Errorf("geoip downloader is not configured")
	}
	databaseUpdateMu.Lock()
	defer databaseUpdateMu.Unlock()
	if isFreshFile(dbPath, cfg.MaxAgeDays, s.now()) {
		return dbPath, false, "", nil
	}
	if err := s.download(ctx, cfg, dbPath); err != nil {
		if _, statErr := os.Stat(dbPath); statErr == nil {
			return dbPath, true, err.Error(), nil
		}
		return "", false, "", err
	}
	return dbPath, false, "", nil
}

func downloadMaxMindDatabase(ctx context.Context, cfg config.GeoIPConfig, dbPath string) error {
	cfg = normalizeConfig(cfg)
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return fmt.Errorf("create geoip cache dir: %w", err)
	}
	url := fmt.Sprintf("https://download.maxmind.com/geoip/databases/%s/download?suffix=tar.gz", cfg.EditionID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(cfg.AccountID, cfg.LicenseKey)
	req.Header.Set("User-Agent", "AntBrowser GeoIP Updater")

	client := &http.Client{Timeout: time.Duration(cfg.TimeoutMs) * time.Millisecond}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download geoip database: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download geoip database returned HTTP %d", resp.StatusCode)
	}
	tmp, err := os.CreateTemp(filepath.Dir(dbPath), cfg.EditionID+"-*.mmdb.tmp")
	if err != nil {
		return fmt.Errorf("create geoip temp database: %w", err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close geoip temp database: %w", err)
	}
	if err := extractMMDBFromTarGz(resp.Body, tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, dbPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("replace geoip database: %w", err)
	}
	_ = removeOldGeoIPDatabases(filepath.Dir(dbPath), cfg.EditionID, cfg.EditionID+".mmdb")
	return nil
}

func extractMMDBFromTarGz(src io.Reader, dstPath string) error {
	gz, err := gzip.NewReader(src)
	if err != nil {
		return fmt.Errorf("open geoip archive: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read geoip archive: %w", err)
		}
		if header == nil || header.Typeflag != tar.TypeReg || !strings.HasSuffix(header.Name, ".mmdb") {
			continue
		}
		if err := writeLimitedFile(dstPath, tr, 256<<20); err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("geoip archive does not contain .mmdb")
}

func writeLimitedFile(path string, src io.Reader, maxBytes int64) error {
	out, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("create geoip database: %w", err)
	}
	defer out.Close()
	limited := &io.LimitedReader{R: src, N: maxBytes + 1}
	written, err := io.Copy(out, limited)
	if err != nil {
		return fmt.Errorf("write geoip database: %w", err)
	}
	if written > maxBytes || limited.N == 0 {
		return fmt.Errorf("geoip database exceeds size limit")
	}
	return nil
}

func removeOldGeoIPDatabases(dir string, editionID string, keep string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	prefix := strings.TrimSpace(editionID) + "."
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || name == keep || !strings.HasSuffix(name, ".mmdb") {
			continue
		}
		if strings.HasPrefix(name, prefix) {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}
	return nil
}

func EnrichMap(data map[string]interface{}, result Result) map[string]interface{} {
	if data == nil {
		data = map[string]interface{}{}
	}
	setIfEmpty(data, "country", result.Country)
	setIfEmpty(data, "region", result.Region)
	setIfEmpty(data, "city", result.City)
	setIfEmpty(data, "timezone", result.Timezone)
	if result.Latitude != 0 || result.Longitude != 0 {
		if _, ok := data["latitude"]; !ok {
			data["latitude"] = result.Latitude
		}
		if _, ok := data["longitude"]; !ok {
			data["longitude"] = result.Longitude
		}
	}
	if strings.TrimSpace(result.Source) != "" {
		data["_geoip_source"] = result.Source
	}
	if strings.TrimSpace(result.UpdatedAt) != "" {
		data["_geoip_updated_at"] = result.UpdatedAt
	}
	if result.Stale {
		data["_geoip_stale"] = true
		if strings.TrimSpace(result.Error) != "" {
			data["_geoip_error"] = result.Error
		}
	}
	return data
}

func normalizeConfig(cfg config.GeoIPConfig) config.GeoIPConfig {
	if strings.TrimSpace(cfg.Provider) == "" {
		cfg.Provider = DefaultProvider
	} else {
		cfg.Provider = strings.ToLower(strings.TrimSpace(cfg.Provider))
	}
	if strings.TrimSpace(cfg.EditionID) == "" {
		cfg.EditionID = DefaultEditionID
	}
	cfg.AccountID = strings.TrimSpace(resolveEnv(cfg.AccountID))
	cfg.LicenseKey = strings.TrimSpace(resolveEnv(cfg.LicenseKey))
	cfg.DatabasePath = strings.TrimSpace(resolveEnv(cfg.DatabasePath))
	if strings.TrimSpace(cfg.CacheDir) == "" {
		cfg.CacheDir = defaultCacheDir
	} else {
		cfg.CacheDir = strings.TrimSpace(resolveEnv(cfg.CacheDir))
	}
	if cfg.MaxAgeDays <= 0 {
		cfg.MaxAgeDays = 30
	}
	if cfg.TimeoutMs <= 0 {
		cfg.TimeoutMs = 15000
	}
	return cfg
}

func isFreshFile(path string, maxAgeDays int, now time.Time) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if maxAgeDays <= 0 {
		maxAgeDays = 30
	}
	return now.Sub(info.ModTime()) <= time.Duration(maxAgeDays)*24*time.Hour
}

func resolveEnv(value string) string {
	value = strings.TrimSpace(value)
	return os.ExpandEnv(value)
}

func setIfEmpty(data map[string]interface{}, key string, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	current, ok := data[key]
	if !ok || current == nil || strings.TrimSpace(fmt.Sprint(current)) == "" {
		data[key] = value
	}
}

func name(names map[string]string) string {
	for _, lang := range []string{"en", "zh-CN", "zh", "ja", "de", "fr"} {
		if value := strings.TrimSpace(names[lang]); value != "" {
			return value
		}
	}
	for _, value := range names {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstEnglishName(items []struct {
	Names     map[string]string `maxminddb:"names"`
	IsoCode   string            `maxminddb:"iso_code"`
	GeoNameID uint              `maxminddb:"geoname_id"`
}) string {
	if len(items) == 0 {
		return ""
	}
	return name(items[0].Names)
}

func RedactConfig(cfg config.GeoIPConfig) config.GeoIPConfig {
	cfg = normalizeConfig(cfg)
	if strings.TrimSpace(cfg.LicenseKey) != "" {
		cfg.LicenseKey = "***"
	}
	return cfg
}
