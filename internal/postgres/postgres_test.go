package postgres

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
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

func TestBuildRowMap(t *testing.T) {
	tests := []struct {
		name    string
		names   []string
		raws    []any
		want    map[string]float64
		wantErr string
	}{
		{
			name:  "numeric types map by column name",
			names: []string{"int32c", "int64c", "float32c", "float64c"},
			raws:  []any{int32(1), int64(2), float32(3.5), 4.25},
			want:  map[string]float64{"int32c": 1, "int64c": 2, "float32c": 3.5, "float64c": 4.25},
		},
		{
			name:  "pgtype.Numeric converts",
			names: []string{"num"},
			raws:  []any{mustNumeric(t, "123.45")},
			want:  map[string]float64{"num": 123.45},
		},
		{
			name:  "numeric []byte converts",
			names: []string{"textnum"},
			raws:  []any{[]byte("67.89")},
			want:  map[string]float64{"textnum": 67.89},
		},
		{
			name:    "non-numeric []byte errors naming the column",
			names:   []string{"bad"},
			raws:    []any{[]byte("not-a-number")},
			wantErr: `column "bad"`,
		},
		{
			name:    "string value errors naming the column",
			names:   []string{"s"},
			raws:    []any{"oops"},
			wantErr: `column "s"`,
		},
		{
			name:    "length mismatch errors",
			names:   []string{"a", "b"},
			raws:    []any{1.0},
			wantErr: "mismatch",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildRowMap(tt.names, tt.raws)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(got) != len(tt.want) {
					t.Fatalf("got %v, want %v", got, tt.want)
				}
				for k, v := range tt.want {
					if got[k] != v {
						t.Errorf("column %q = %v, want %v", k, got[k], v)
					}
				}
				return
			}
			if err == nil {
				t.Fatalf("buildRowMap succeeded, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

// mustNumeric builds a pgtype.Numeric from a decimal string for tests.
func mustNumeric(t *testing.T, s string) pgtype.Numeric {
	t.Helper()
	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		t.Fatalf("scan numeric %q: %v", s, err)
	}
	return n
}
