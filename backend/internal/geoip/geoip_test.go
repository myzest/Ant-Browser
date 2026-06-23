package geoip

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"ant-chrome/backend/internal/config"
)

func TestExtractMMDBFromTarGz(t *testing.T) {
	t.Parallel()

	payload := []byte("mmdb")
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "GeoLite2-City/GeoLite2-City.mmdb", Mode: 0o644, Size: int64(len(payload)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(t.TempDir(), "GeoLite2-City.mmdb")
	if err := extractMMDBFromTarGz(bytes.NewReader(archive.Bytes()), dst); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("extracted payload = %q, want %q", got, payload)
	}
}

func TestEnrichMapPreservesExistingFields(t *testing.T) {
	t.Parallel()

	data := map[string]interface{}{
		"ip":      "203.0.113.9",
		"country": "US",
	}
	got := EnrichMap(data, Result{
		Country:   "JP",
		Region:    "Tokyo",
		City:      "Tokyo",
		Timezone:  "Asia/Tokyo",
		Latitude:  35.68,
		Longitude: 139.76,
		Source:    "geoip:maxmind",
		UpdatedAt: "2026-06-24T00:00:00Z",
	})
	if got["country"] != "US" {
		t.Fatalf("country should be preserved, got %v", got["country"])
	}
	if got["timezone"] != "Asia/Tokyo" || got["_geoip_source"] != "geoip:maxmind" {
		t.Fatalf("geoip fields not merged: %#v", got)
	}
}

func TestEnrichMapMarksStaleDatabaseFallback(t *testing.T) {
	t.Parallel()

	got := EnrichMap(map[string]interface{}{}, Result{
		Country:   "US",
		Timezone:  "America/Los_Angeles",
		Source:    "geoip:maxmind",
		UpdatedAt: "2026-06-24T00:00:00Z",
		Stale:     true,
		Error:     "download failed",
	})
	if got["_geoip_stale"] != true || got["_geoip_error"] != "download failed" {
		t.Fatalf("stale metadata not merged: %#v", got)
	}
}

func TestNormalizeConfigExpandsEnvPathSegments(t *testing.T) {
	t.Setenv("ANT_GEOIP_TEST_DIR", "/tmp/ant-geoip-test")

	cfg := normalizeConfig(config.GeoIPConfig{
		Enabled:      true,
		DatabasePath: "$ANT_GEOIP_TEST_DIR/GeoLite2-City.mmdb",
		CacheDir:     "${ANT_GEOIP_TEST_DIR}/cache",
	})

	if cfg.DatabasePath != "/tmp/ant-geoip-test/GeoLite2-City.mmdb" {
		t.Fatalf("database path = %q", cfg.DatabasePath)
	}
	if cfg.CacheDir != "/tmp/ant-geoip-test/cache" {
		t.Fatalf("cache dir = %q", cfg.CacheDir)
	}
}

func TestRemoveOldGeoIPDatabasesKeepsOtherEditions(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	files := []string{
		"GeoLite2-City.mmdb",
		"GeoLite2-City.old.mmdb",
		"GeoLite2-Country.mmdb",
		"GeoIP2-City.mmdb",
	}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("db"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := removeOldGeoIPDatabases(dir, "GeoLite2-City", "GeoLite2-City.mmdb"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "GeoLite2-City.old.mmdb")); !os.IsNotExist(err) {
		t.Fatalf("old city database should be removed, err=%v", err)
	}
	for _, name := range []string{"GeoLite2-City.mmdb", "GeoLite2-Country.mmdb", "GeoIP2-City.mmdb"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("%s should be kept: %v", name, err)
		}
	}
}
