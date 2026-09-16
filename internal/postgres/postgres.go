// Package postgres manages the connection to the monitored PostgreSQL
// instance and executes metric queries with strict result-shape validation.
package postgres

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bazaruzero/yama/internal/config"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool wraps a pgx connection pool for the monitored database.
type Pool struct {
	pool *pgxpool.Pool
}

// Open builds a connection pool from the structured config fields and
// verifies connectivity with a ping bounded by timeout.
func Open(ctx context.Context, cfg config.PostgresConfig, timeout time.Duration) (*Pool, error) {
	connStr := fmt.Sprintf("host=%s port=%d user=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Database, cfg.SSLMode)
	if cfg.Password != "" {
		connStr += " password=" + quoteConnValue(cfg.Password)
	}
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		return nil, fmt.Errorf("postgres: invalid connection config: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: cannot connect to %s:%d/%s: %w", cfg.Host, cfg.Port, cfg.Database, err)
	}
	return &Pool{pool: pool}, nil
}

// Close releases the connection pool.
func (p *Pool) Close() {
	p.pool.Close()
}

// QueryValue executes a metric query and returns its single numeric value.
// The query MUST return exactly one row with exactly one numeric column;
// anything else is an error.
func (p *Pool) QueryValue(ctx context.Context, query string) (float64, error) {
	rows, err := p.pool.Query(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	fields := rows.FieldDescriptions()
	if len(fields) != 1 {
		return 0, fmt.Errorf("query must return exactly 1 column, got %d", len(fields))
	}
	var values []float64
	for rows.Next() {
		var raw any
		if err := rows.Scan(&raw); err != nil {
			return 0, fmt.Errorf("scan result: %w", err)
		}
		v, err := toFloat64(raw)
		if err != nil {
			return 0, err
		}
		values = append(values, v)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("query failed: %w", err)
	}
	if len(values) != 1 {
		return 0, fmt.Errorf("query must return exactly 1 row, got %d", len(values))
	}
	return values[0], nil
}

// toFloat64 converts common PostgreSQL numeric representations to float64.
func toFloat64(raw any) (float64, error) {
	switch v := raw.(type) {
	case int32:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case float32:
		return float64(v), nil
	case float64:
		return v, nil
	case pgtype.Numeric:
		f, err := v.Float64Value()
		if err != nil || !f.Valid {
			return 0, fmt.Errorf("cannot convert numeric to float64")
		}
		return f.Float64, nil
	case []byte:
		f, err := strconv.ParseFloat(string(v), 64)
		if err != nil {
			return 0, fmt.Errorf("result is not numeric: %q", string(v))
		}
		return f, nil
	default:
		return 0, fmt.Errorf("result type %T is not numeric", raw)
	}
}

// quoteConnValue quotes a value for the keyword-style connection string.
func quoteConnValue(s string) string {
	r := strings.NewReplacer("\\", "\\\\", "'", "\\'")
	return "'" + r.Replace(s) + "'"
}
