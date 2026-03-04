package database

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"time"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

const defaultPostgresDSN = "postgres://postgres:postgres@localhost:5432/todos?sslmode=disable"

func Open(ctx context.Context, dsn string) (*bun.DB, error) {
	if dsn == "" {
		dsn = defaultPostgresDSN
	}

	dsn = normalizeDSN(dsn)

	sqlDB := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	if sqlDB == nil {
		return nil, fmt.Errorf("open database connection: nil connector")
	}

	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(25)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)

	return bun.NewDB(sqlDB, pgdialect.New()), nil
}

func normalizeDSN(dsn string) string {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return dsn
	}

	query := parsed.Query()
	if _, exists := query["channel_binding"]; exists {
		query.Del("channel_binding")
		parsed.RawQuery = query.Encode()
	}

	return parsed.String()
}
