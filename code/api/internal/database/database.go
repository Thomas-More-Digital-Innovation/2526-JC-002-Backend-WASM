package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

const defaultSQLiteDSN = "file:data/todos.db?cache=shared&_pragma=foreign_keys(1)"

func Open(ctx context.Context, dsn string) (*bun.DB, error) {
	if dsn == "" {
		if err := os.MkdirAll("data", 0o755); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
		dsn = defaultSQLiteDSN
	}

	sqlDB, err := sql.Open(sqliteshim.ShimName, dsn)
	if err != nil {
		return nil, fmt.Errorf("open database connection: %w", err)
	}

	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(25)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)

	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return bun.NewDB(sqlDB, sqlitedialect.New()), nil
}
