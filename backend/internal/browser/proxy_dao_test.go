package browser

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestSQLiteProxyDAOUpsertBackfillsMissingOptionalColumns(t *testing.T) {
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE browser_proxies (
		proxy_id     TEXT PRIMARY KEY,
		proxy_name   TEXT NOT NULL,
		proxy_config TEXT NOT NULL,
		dns_servers  TEXT NOT NULL DEFAULT '',
		sort_order   INTEGER NOT NULL DEFAULT 0,
		created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		t.Fatalf("create legacy table failed: %v", err)
	}

	dao := NewSQLiteProxyDAO(db)
	if err := dao.Upsert(Proxy{ProxyId: "__direct__", ProxyName: "直连（不走代理）", ProxyConfig: "direct://"}); err != nil {
		t.Fatalf("Upsert returned error: %v", err)
	}

	list, err := dao.List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("len(list) = %d, want 1", len(list))
	}
	if list[0].ProxyId != "__direct__" || list[0].ProxyConfig != "direct://" {
		t.Fatalf("unexpected proxy: %+v", list[0])
	}
}
