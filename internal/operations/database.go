package operations

import (
	"context"

	"github.com/oh-my-cpa/oh-my-cpa/internal/capability"
	"github.com/oh-my-cpa/oh-my-cpa/internal/repository"
)

type DatabaseQueryInput struct {
	SQL     string `json:"sql" jsonschema:"One SQLite SELECT (or WITH ... SELECT) statement"`
	MaxRows int    `json:"max_rows,omitempty" jsonschema:"Rows to return, 1-200; defaults to 50"`
}

type DatabaseTables struct {
	Tables []repository.QueryTable `json:"tables"`
}

// registerDatabase exposes OMC's own database to read-only SQL. The policy - which tables and
// columns are readable, and how a statement is proven to be a read - lives with the repository
// (readonly_query.go), so the capability here only describes it to the model.
func (s *Service) registerDatabase(registry *capability.Registry) error {
	if err := read(registry, "database_schema", "List the OMC database tables and columns that database_query may read. Redacted columns and hidden tables (credentials, raw payloads, agent sessions, preferences) are not readable.", func(ctx context.Context, _ Empty) (DatabaseTables, error) {
		tables, err := s.Repo.QuerySchema(ctx)
		return DatabaseTables{Tables: tables}, err
	}); err != nil {
		return err
	}
	return read(registry, "database_query", "Run one read-only SQLite SELECT against the OMC database for questions the other capabilities cannot answer; call database_schema first. Times are epoch milliseconds (`*_ms`). Prefer aggregates and WHERE on time columns: at most 200 rows and 5 seconds, long text is cut, and credential-like text and email addresses are masked. Table-valued functions (json_each, pragma_*) are unavailable; use json_extract.", func(ctx context.Context, input DatabaseQueryInput) (repository.QueryResult, error) {
		return s.Repo.ReadOnlyQuery(ctx, input.SQL, input.MaxRows)
	})
}
