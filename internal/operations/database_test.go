package operations

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/oh-my-cpa/oh-my-cpa/internal/capability"
	"github.com/oh-my-cpa/oh-my-cpa/internal/repository"
)

func TestDatabaseQueryCapabilityReportsWhyItRefused(t *testing.T) {
	ctx := context.Background()
	database, err := repository.Open(ctx, filepath.Join(t.TempDir(), "operations.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	repo := repository.New(database)
	service := &Service{Repo: repo}
	registry := capability.NewRegistry()
	if err := service.registerDatabase(registry); err != nil {
		t.Fatal(err)
	}
	executor := &capability.Executor{Registry: registry, Store: repository.AgentStore{Repo: repo}}
	principal := capability.Principal{ID: "administrator", Adapter: "agent", IsAdmin: true}

	result, err := executor.Invoke(ctx, principal, "database_query", json.RawMessage(`{"sql":"SELECT count(*) AS requests FROM usage_events","max_rows":5}`), "")
	if err != nil || result.Status != "success" {
		t.Fatalf("query: %+v %v", result, err)
	}
	var rows repository.QueryResult
	if err := json.Unmarshal(result.Data, &rows); err != nil || rows.Columns[0] != "requests" || len(rows.Rows) != 1 {
		t.Fatalf("rows: %s %v", result.Data, err)
	}
	result, err = executor.Invoke(ctx, principal, "database_query", json.RawMessage(`{"sql":"SELECT pref_value FROM ui_preferences"}`), "")
	if err != nil || result.Status != "error" || result.Code != "query_forbidden" || result.Detail != "table ui_preferences is not readable" {
		t.Fatalf("refusal: %+v %v", result, err)
	}
	result, err = executor.Invoke(ctx, principal, "database_schema", json.RawMessage(`{}`), "")
	if err != nil || result.Status != "success" {
		t.Fatalf("schema: %+v %v", result, err)
	}
}
