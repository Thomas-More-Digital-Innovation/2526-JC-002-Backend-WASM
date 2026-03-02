package model

import "github.com/uptrace/bun"

type Status struct {
	bun.BaseModel `bun:"table:statuses,alias:s"`

	ID   int64  `bun:",pk,autoincrement" json:"id"`
	Name string `bun:",notnull,unique" json:"name"`
}

type Todo struct {
	bun.BaseModel `bun:"table:todos,alias:t"`

	ID          int64   `bun:",pk,autoincrement" json:"id"`
	Title       string  `bun:",notnull" json:"title"`
	Description string  `bun:",notnull,unique" json:"description"`
	StatusID    int64   `bun:"status_id,notnull" json:"statusId"`
	Status      *Status `bun:"rel:belongs-to,join:status_id=id" json:"status,omitempty"`
}

type CreateTodoRequest struct {
	Title       string `json:"title" binding:"required"`
	Description string `json:"description" binding:"required"`
	Status      struct {
		Name string `json:"name" binding:"required"`
	} `json:"status" binding:"required"`
}
