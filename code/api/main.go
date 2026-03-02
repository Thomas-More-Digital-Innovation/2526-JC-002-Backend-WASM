package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"api/internal/database"
	"api/internal/model"

	"github.com/gin-gonic/gin"
	"github.com/uptrace/bun"
)

func main() {
	ctx := context.Background()
	db, err := database.Open(ctx, os.Getenv("DB_DSN"))
	if err != nil {
		log.Fatalf("database setup failed: %v", err)
	}
	defer db.Close()

	if err := database.RunMigrations(ctx, db); err != nil {
		log.Fatalf("migration failed: %v", err)
	}

	router := gin.Default()
	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "pong",
		})
	})

	router.POST("/new-todo", func(c *gin.Context) {
		var req model.CreateTodoRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		requestCtx := c.Request.Context()
		statusName := strings.TrimSpace(req.Status.Name)
		if statusName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "status.name is required"})
			return
		}

		statusID, err := resolveStatusID(requestCtx, db, statusName)
		if err != nil {
			log.Printf("resolve status failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to upsert status"})
			return
		}

		todo := &model.Todo{
			Title:       req.Title,
			Description: req.Description,
			StatusID:    statusID,
		}

		if _, err := db.NewInsert().Model(todo).Exec(requestCtx); err != nil {
			log.Printf("create todo failed: %v", err)
			if isUniqueConstraintError(err) {
				c.JSON(http.StatusConflict, gin.H{"error": "todo description already exists"})
				return
			}

			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create todo"})
			return
		}

		if err := db.NewSelect().
			Model(todo).
			Relation("Status").
			WherePK().
			Scan(requestCtx); err != nil {
			log.Printf("fetch created todo failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch created todo"})
			return
		}

		c.JSON(http.StatusCreated, gin.H{"message": "Todo created", "todo": todo})
	})

	router.GET("/todos", func(c *gin.Context) {
		todos := make([]model.Todo, 0)
		if err := db.NewSelect().
			Model(&todos).
			Relation("Status").
			OrderExpr("t.id ASC").
			Scan(c.Request.Context()); err != nil {
			log.Printf("list todos failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list todos"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"todos": todos})
	})

	router.PUT("/todos/:id", func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid todo id"})
			return
		}

		var req model.CreateTodoRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		requestCtx := c.Request.Context()
		statusName := strings.TrimSpace(req.Status.Name)
		if statusName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "status.name is required"})
			return
		}

		statusID, err := resolveStatusID(requestCtx, db, statusName)
		if err != nil {
			log.Printf("resolve status failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to upsert status"})
			return
		}

		todo := &model.Todo{ID: id}
		if err := db.NewSelect().Model(todo).WherePK().Scan(requestCtx); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"error": "todo not found"})
				return
			}

			log.Printf("load todo failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load todo"})
			return
		}

		todo.Title = req.Title
		todo.Description = req.Description
		todo.StatusID = statusID

		if _, err := db.NewUpdate().
			Model(todo).
			Column("title", "description", "status_id").
			WherePK().
			Exec(requestCtx); err != nil {
			log.Printf("update todo failed: %v", err)
			if isUniqueConstraintError(err) {
				c.JSON(http.StatusConflict, gin.H{"error": "todo description already exists"})
				return
			}

			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update todo"})
			return
		}

		if err := db.NewSelect().
			Model(todo).
			Relation("Status").
			WherePK().
			Scan(requestCtx); err != nil {
			log.Printf("fetch updated todo failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch updated todo"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Todo updated", "todo": todo})
	})

	router.DELETE("/todos/:id", func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid todo id"})
			return
		}

		result, err := db.NewDelete().
			Model(&model.Todo{ID: id}).
			WherePK().
			Exec(c.Request.Context())
		if err != nil {
			log.Printf("delete todo failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete todo"})
			return
		}

		affected, err := result.RowsAffected()
		if err != nil {
			log.Printf("rows affected failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to confirm deletion"})
			return
		}

		if affected == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "todo not found"})
			return
		}

		c.Status(http.StatusNoContent)
	})

	if err := router.Run(); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, os.ErrExist) {
		return true
	}

	errText := strings.ToLower(err.Error())
	return strings.Contains(errText, "unique") || strings.Contains(errText, "duplicate")
}

func resolveStatusID(ctx context.Context, db *bun.DB, statusName string) (int64, error) {
	status := &model.Status{Name: statusName}
	if _, err := db.NewInsert().
		Model(status).
		On("CONFLICT (name) DO NOTHING").
		Exec(ctx); err != nil {
		return 0, fmt.Errorf("insert status: %w", err)
	}

	if err := db.NewSelect().
		Model(status).
		Where("name = ?", statusName).
		Limit(1).
		Scan(ctx); err != nil {
		return 0, fmt.Errorf("select status: %w", err)
	}

	return status.ID, nil
}
