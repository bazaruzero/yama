package postgres

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestBuildPoolConfigSetsMaxConns(t *testing.T) {
	connStr := "host=localhost port=5432 user=admin dbname=postgres sslmode=prefer"
	for _, n := range []int{1, 5} {
		cfg, err := buildPoolConfig(connStr, n)
		if err != nil {
			t.Fatalf("maxConns=%d: unexpected error: %v", n, err)
		}
		if cfg.MaxConns != int32(n) {
			t.Errorf("maxConns=%d: MaxConns got %d", n, cfg.MaxConns)
		}
		if cfg.MinConns != 0 {
			t.Errorf("maxConns=%d: MinConns should stay 0, got %d", n, cfg.MinConns)
		}
	}
}

func TestBuildPoolConfigInvalidConnStr(t *testing.T) {
	if _, err := buildPoolConfig("not a valid connection string ==", 1); err == nil {
		t.Fatal("expected error for invalid connection string")
	}
}

func TestPoolConfigSatisfiesPgxParse(t *testing.T) {
	// The keyword-style string we build must remain parseable by pgxpool.
	cfg, err := pgxpool.ParseConfig("host=localhost port=5415 user=admin dbname=postgres sslmode=prefer")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !strings.EqualFold(cfg.ConnConfig.Host, "localhost") || cfg.ConnConfig.Port != 5415 {
		t.Errorf("parsed config wrong: %+v", cfg.ConnConfig)
	}
}
