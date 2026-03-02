package database

import (
	"context"
	"fmt"

	"api/internal/model"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		if _, err := db.NewCreateTable().
			Model((*model.Status)(nil)).
			IfNotExists().
			Exec(ctx); err != nil {
			return fmt.Errorf("create statuses table: %w", err)
		}

		if _, err := db.NewCreateTable().
			Model((*model.Todo)(nil)).
			IfNotExists().
			ForeignKey(`("status_id") REFERENCES "statuses" ("id") ON DELETE RESTRICT`).
			Exec(ctx); err != nil {
			return fmt.Errorf("create todos table: %w", err)
		}

		statuses := []model.Status{
			{Name: "todo"},
			{Name: "in_progress"},
			{Name: "done"},
		}

		for _, status := range statuses {
			if _, err := db.NewInsert().
				Model(&status).
				On("CONFLICT (name) DO NOTHING").
				Exec(ctx); err != nil {
				return fmt.Errorf("seed statuses: %w", err)
			}
		}

		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		if _, err := db.NewDropTable().
			Model((*model.Todo)(nil)).
			IfExists().
			Cascade().
			Exec(ctx); err != nil {
			return fmt.Errorf("drop todos table: %w", err)
		}

		if _, err := db.NewDropTable().
			Model((*model.Status)(nil)).
			IfExists().
			Cascade().
			Exec(ctx); err != nil {
			return fmt.Errorf("drop statuses table: %w", err)
		}

		return nil
	})
}
