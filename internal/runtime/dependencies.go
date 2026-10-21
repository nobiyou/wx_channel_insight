package runtime

import (
	"github.com/nobiyou/wx_channel_insight/internal/postgres"
	"github.com/nobiyou/wx_channel_insight/internal/rediscache"
	"github.com/nobiyou/wx_channel_insight/internal/sqlite"
)

func BuildSQLiteClient(cfg Config) (*sqlite.Client, error) {
	if usesStoreMode(cfg, StoreModeSQLite) {
		return sqlite.Open(cfg.SQLite.Path)
	}
	return nil, nil
}

func usesStoreMode(cfg Config, mode StoreMode) bool {
	for _, store := range configuredStores(cfg) {
		if store.Mode == mode {
			return true
		}
	}
	return false
}

func BuildPostgresClient(cfg Config) (*postgres.Client, error) {
	if !cfg.Postgres.Enabled() {
		return nil, nil
	}
	return postgres.NewClient(cfg.Postgres.DSN)
}

func BuildRedisClient(cfg Config) (*rediscache.Client, error) {
	if !cfg.Redis.Enabled() {
		return nil, nil
	}
	return rediscache.NewClient(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
}
