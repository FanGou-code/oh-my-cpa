package repository

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func queryTestRepository(t *testing.T) *Repository {
	t.Helper()
	database, err := Open(context.Background(), filepath.Join(t.TempDir(), "query.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	return New(database)
}

func queryCode(err error) string {
	var failure *QueryError
	if errors.As(err, &failure) {
		return failure.Code
	}
	if err == nil {
		return ""
	}
	return err.Error()
}

// Every table a migration creates is a decision: readable (with its hidden columns named) or
// hidden with a reason. A new table fails here until someone makes that decision.
func TestEveryTableIsClassifiedForOperatorQueries(t *testing.T) {
	repo := queryTestRepository(t)
	rows, err := repo.SQL().Query(`SELECT name FROM sqlite_schema WHERE type = 'table'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		_, isReadable := QUERY_READABLE_TABLES[name]
		_, isHidden := QUERY_HIDDEN_TABLES[name]
		if isReadable == isHidden {
			t.Errorf("table %s must be in exactly one of QUERY_READABLE_TABLES and QUERY_HIDDEN_TABLES", name)
		}
	}
	schema, err := repo.QuerySchema(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range schema {
		if _, isHidden := QUERY_HIDDEN_TABLES[table.Name]; isHidden {
			t.Errorf("hidden table %s is listed", table.Name)
		}
		if table.Name == "error_events" && (len(table.Redacted) != 1 || table.Redacted[0] != "body") {
			t.Errorf("error_events redaction: %+v", table.Redacted)
		}
		for _, column := range table.Columns {
			if table.Name == "error_events" && column.Name == "body" {
				t.Error("redacted column listed")
			}
		}
	}
}

func TestReadOnlyQueryRefusesWhatItMustNotRead(t *testing.T) {
	repo := queryTestRepository(t)
	ctx := context.Background()
	for _, statement := range []string{
		`SELECT * FROM cpa_instances`,
		`SELECT management_key_nonce FROM cpa_instances`,
		`SELECT pref_value FROM ui_preferences`,
		`SELECT count(*) FROM agent_documents`,
		`SELECT raw_message FROM usage_inboxes`,
		`WITH hidden AS (SELECT * FROM ui_preferences) SELECT * FROM hidden`,
		`SELECT (SELECT pref_value FROM ui_preferences LIMIT 1)`,
		`SELECT id FROM usage_events WHERE instance_id IN (SELECT id FROM cpa_instances)`,
		`SELECT body FROM error_events`,
		`SELECT e.body AS harmless FROM error_events e`,
		`SELECT * FROM error_events`,
		`SELECT length(body) FROM error_events`,
		`SELECT id FROM error_events WHERE body LIKE '%key%'`,
		`SELECT details_json FROM discovered_resources`,
		`SELECT * FROM main.cpa_instances`,
		`SELECT * FROM temp.sqlite_schema`,
		`SELECT * FROM json_each('[1]')`,
		`SELECT * FROM pragma_table_info('usage_events')`,
		`DELETE FROM usage_events`,
		`UPDATE model_prices SET model = 'x'`,
		`WITH x AS (SELECT 1) DELETE FROM usage_events`,
		`INSERT INTO pricing_channels VALUES ('x', 1, '', 0)`,
		`ATTACH DATABASE 'other.db' AS other`,
		`PRAGMA table_info(usage_events)`,
		`SELECT 1; DELETE FROM usage_events`,
		`SELECT 1; SELECT 2`,
		`VACUUM`,
		`CREATE TABLE x (id INTEGER)`,
	} {
		result, err := repo.ReadOnlyQuery(ctx, statement, 10)
		code := queryCode(err)
		if code != "query_forbidden" && code != "query_invalid" {
			t.Errorf("%s: accepted (%v, %+v)", statement, err, result)
		}
	}
}

func TestReadOnlyQueryReadsBoundedAndMasked(t *testing.T) {
	repo := queryTestRepository(t)
	ctx := context.Background()
	if _, err := repo.SQL().Exec(`INSERT INTO pricing_channels (channel, multiplier, note, updated_at_ms) VALUES
		('alpha', 1.5, 'owner alice@example.com', 1),
		('beta', 2, 'key `+"sk-"+`abcdefghijklmnopqrstu', 2),
		('gamma', 1, ?, 3)`, strings.Repeat("x", 512)); err != nil {
		t.Fatal(err)
	}
	result, err := repo.ReadOnlyQuery(ctx, " -- channels\n SELECT channel, multiplier, note FROM pricing_channels ORDER BY channel; ", 2)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(result.Columns, ",") != "channel,multiplier,note" || len(result.Rows) != 2 || !result.IsTruncated {
		t.Fatalf("bounded result: %+v", result)
	}
	if result.Rows[0][2] != "owner a***@example.com" || result.Rows[1][2] != "key [redacted]" {
		t.Fatalf("masking: %+v", result.Rows)
	}
	result, err = repo.ReadOnlyQuery(ctx, `SELECT note FROM pricing_channels WHERE channel = 'gamma'`, 0)
	if err != nil || len([]rune(result.Rows[0][0].(string))) != MAX_QUERY_CELL_CHARS+1 {
		t.Fatalf("cell bound: %+v %v", result, err)
	}
	// Reading a redacted table's other columns, including through its indexes, stays allowed.
	if _, err := repo.ReadOnlyQuery(ctx, `SELECT count(*), max(status_code) FROM error_events WHERE timestamp_ms > 0`, 0); err != nil {
		t.Fatalf("readable columns of a redacted table: %v", err)
	}
	if _, err := repo.ReadOnlyQuery(ctx, `SELECT name FROM sqlite_schema WHERE type = 'table'`, 0); err != nil {
		t.Fatalf("schema read: %v", err)
	}
	if _, err := repo.ReadOnlyQuery(ctx, `SELECT length(randomblob(2000000))`, 0); queryCode(err) != "query_invalid" {
		t.Fatalf("value size bound: %v", err)
	}
	if _, err := repo.ReadOnlyQuery(ctx, `SELECT missing_column FROM usage_events`, 0); queryCode(err) != "query_invalid" || !strings.Contains(err.(*QueryError).Detail, "missing_column") {
		t.Fatalf("invalid query detail: %v", err)
	}
}

func TestReadOnlyQueryStopsAtItsTimeLimit(t *testing.T) {
	repo := queryTestRepository(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := repo.ReadOnlyQuery(ctx, `WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n) SELECT count(*) FROM n`, 0)
	if err == nil {
		t.Fatal("cancelled query ran")
	}
}

func TestSingleSelectFindsTheRealStatementBoundary(t *testing.T) {
	for statement, isAccepted := range map[string]bool{
		`SELECT ';' AS semicolon`:                              true,
		`SELECT "a;b" FROM usage_events;`:                      true,
		"SELECT 1 -- trailing; comment":                        true,
		"SELECT 1; /* done */ -- really\n":                     true,
		`(SELECT 1)`:                                           true,
		"/* lead */ WITH x AS (SELECT 1) SELECT * FROM x":      true,
		`SELECT 1;;`:                                           false,
		`SELECT 'unterminated`:                                 false,
		`SELECT 1; DROP TABLE usage_events`:                    false,
		"SELECT 1 /* ; */; SELECT 2":                           false,
		`EXPLAIN SELECT 1`:                                     false,
		`REPLACE INTO pricing_channels VALUES ('x', 1, '', 0)`: false,
	} {
		_, err := singleSelect(statement)
		if (err == nil) != isAccepted {
			t.Errorf("%q: %v", statement, err)
		}
	}
}
