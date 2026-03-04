package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// Request types (matching the original API contract).
type statusRef struct {
	Name string `json:"name"`
}

type createTodoRequest struct {
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      statusRef `json:"status"`
}

// dbAction is emitted via the X-Db-Action CGI header so the JS host can
// execute the operation against Cloudflare D1.
type dbAction struct {
	Action string `json:"action"`
	Params any    `json:"params,omitempty"`
}

// ---------------------------------------------------------------------------
// Entrypoint – pure WAGI: reads CGI env‑vars + stdin, writes CGI response.
// ---------------------------------------------------------------------------

func main() {
	method := envOr("REQUEST_METHOD", "GET")
	path := parsePath()

	body, _ := io.ReadAll(os.Stdin)

	switch {
	case path == "/ping" && method == http.MethodGet:
		writeJSON(http.StatusOK, map[string]string{"message": "pong"})

	case path == "/todos" && method == http.MethodGet:
		writeDBAction(http.StatusOK, "list-todos", nil)

	case path == "/new-todo" && method == http.MethodPost:
		handleCreateTodo(body)

	case strings.HasPrefix(path, "/todos/") && method == http.MethodPut:
		id, err := extractID(path)
		if err != nil {
			writeJSON(http.StatusBadRequest, map[string]string{"error": "invalid todo id"})
			return
		}
		handleUpdateTodo(body, id)

	case strings.HasPrefix(path, "/todos/") && method == http.MethodDelete:
		id, err := extractID(path)
		if err != nil {
			writeJSON(http.StatusBadRequest, map[string]string{"error": "invalid todo id"})
			return
		}
		writeDBAction(http.StatusNoContent, "delete-todo", map[string]int64{"id": id})

	default:
		writeJSON(http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

// ---------------------------------------------------------------------------
// Route handlers
// ---------------------------------------------------------------------------

func handleCreateTodo(body []byte) {
	req, ok := parseAndValidateTodoRequest(body)
	if !ok {
		return
	}

	writeDBAction(http.StatusCreated, "create-todo", map[string]string{
		"title":       req.Title,
		"description": req.Description,
		"statusName":  strings.TrimSpace(req.Status.Name),
	})
}

func handleUpdateTodo(body []byte, id int64) {
	req, ok := parseAndValidateTodoRequest(body)
	if !ok {
		return
	}

	writeDBAction(http.StatusOK, "update-todo", map[string]any{
		"id":          id,
		"title":       req.Title,
		"description": req.Description,
		"statusName":  strings.TrimSpace(req.Status.Name),
	})
}

func parseAndValidateTodoRequest(body []byte) (createTodoRequest, bool) {
	var req createTodoRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		return req, false
	}

	if strings.TrimSpace(req.Title) == "" {
		writeJSON(http.StatusBadRequest, map[string]string{"error": "title is required"})
		return req, false
	}

	if strings.TrimSpace(req.Description) == "" {
		writeJSON(http.StatusBadRequest, map[string]string{"error": "description is required"})
		return req, false
	}

	if strings.TrimSpace(req.Status.Name) == "" {
		writeJSON(http.StatusBadRequest, map[string]string{"error": "status.name is required"})
		return req, false
	}

	return req, true
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func extractID(path string) (int64, error) {
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(segments) < 2 {
		return 0, fmt.Errorf("missing id")
	}

	id, err := strconv.ParseInt(segments[1], 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid id")
	}

	return id, nil
}

func parsePath() string {
	path := strings.TrimSpace(os.Getenv("PATH_INFO"))
	if path == "" {
		path = strings.TrimSpace(os.Getenv("REQUEST_URI"))
	}

	if path == "" {
		return "/"
	}

	if idx := strings.Index(path, "?"); idx >= 0 {
		path = path[:idx]
	}

	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	return path
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}

	return fallback
}

// writeJSON emits a complete WAGI/CGI response (no DB action).
func writeJSON(status int, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		status = http.StatusInternalServerError
		data = []byte(`{"error":"failed to encode response"}`)
	}

	fmt.Printf("Status: %d %s\r\n", status, http.StatusText(status))
	fmt.Printf("Content-Type: application/json\r\n")
	fmt.Printf("\r\n")
	_, _ = os.Stdout.Write(data)
}

// writeDBAction emits a WAGI/CGI response whose X-Db-Action header tells
// the JS host which D1 operation to run.  The response body is left empty;
// the JS host fills it in after executing the query.
func writeDBAction(status int, action string, params any) {
	actionJSON, _ := json.Marshal(dbAction{Action: action, Params: params})

	fmt.Printf("Status: %d %s\r\n", status, http.StatusText(status))
	fmt.Printf("Content-Type: application/json\r\n")
	fmt.Printf("X-Db-Action: %s\r\n", string(actionJSON))
	fmt.Printf("\r\n")
}
