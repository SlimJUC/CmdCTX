package history

import (
	"path/filepath"
	"testing"
	"time"
)

// openTestStore opens a Store backed by an isolated temp-dir database.
// openDB is defined in history.go and handles schema migration.
func openTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test-history.db")

	store, err := openDB(dbPath)
	if err != nil {
		t.Fatalf("opening test store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestRecord_Basic(t *testing.T) {
	store := openTestStore(t)

	entry := &Entry{
		Prompt:      "find all php files",
		IntentType:  "search_text",
		IntentJSON:  `{"intent":"search_text","pattern":"test"}`,
		RenderedCmd: "grep -r test .",
		ParsedBy:    "rule_based",
		Risk:        "low",
	}

	id, err := store.Record(entry)
	if err != nil {
		t.Fatalf("Record failed: %v", err)
	}
	if id == 0 {
		t.Error("expected non-zero ID")
	}
}

func TestRecord_Redaction(t *testing.T) {
	store := openTestStore(t)

	entry := &Entry{
		Prompt:      "find files with api_key=supersecret123456789012345678901234",
		IntentType:  "search_text",
		RenderedCmd: "grep -r test .",
		ParsedBy:    "ai",
		Risk:        "low",
	}

	id, err := store.Record(entry)
	if err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	retrieved, err := store.GetByID(id)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	// The prompt should have been redacted.
	if containsStr(retrieved.Prompt, "supersecret") {
		t.Error("expected secret to be redacted from stored prompt")
	}
}

func TestList_Order(t *testing.T) {
	store := openTestStore(t)

	for i := 0; i < 3; i++ {
		_, err := store.Record(&Entry{
			Prompt:     "request " + string(rune('A'+i)),
			IntentType: "search_text",
			ParsedBy:   "rule_based",
			Risk:       "low",
		})
		if err != nil {
			t.Fatalf("Record failed: %v", err)
		}
	}

	entries, err := store.List(10)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}
	// Newest first.
	if entries[0].ID < entries[1].ID {
		t.Error("expected entries to be ordered newest-first")
	}
}

func TestSearch_Matching(t *testing.T) {
	store := openTestStore(t)

	store.Record(&Entry{Prompt: "find php files", IntentType: "search_text", ParsedBy: "rule_based", Risk: "low"})
	store.Record(&Entry{Prompt: "search nginx logs", IntentType: "search_logs", ParsedBy: "rule_based", Risk: "low"})
	store.Record(&Entry{Prompt: "count timeouts", IntentType: "count_occurrences", ParsedBy: "rule_based", Risk: "low"})

	entries, err := store.Search("nginx", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 matching entry, got %d", len(entries))
	}
	if entries[0].Prompt != "search nginx logs" {
		t.Errorf("unexpected entry: %q", entries[0].Prompt)
	}
}

func TestUpdateExecution(t *testing.T) {
	store := openTestStore(t)

	id, err := store.Record(&Entry{
		Prompt:     "find go files",
		IntentType: "find_files",
		ParsedBy:   "rule_based",
		Risk:       "low",
	})
	if err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	if err := store.UpdateExecution(id, 0, 1234, "output here", ""); err != nil {
		t.Fatalf("UpdateExecution failed: %v", err)
	}

	entry, err := store.GetByID(id)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if !entry.Executed {
		t.Error("expected Executed to be true")
	}
	if entry.ExitCode == nil || *entry.ExitCode != 0 {
		t.Error("expected exit code 0")
	}
	if entry.DurationMS != 1234 {
		t.Errorf("expected duration 1234ms, got %d", entry.DurationMS)
	}
}

func TestPurge(t *testing.T) {
	store := openTestStore(t)

	// Record an old entry by manipulating created_at.
	_, err := store.db.Exec(`
		INSERT INTO history (prompt, intent_type, rendered_cmd, parsed_by, risk, cwd, created_at)
		VALUES ('old prompt', 'search_text', 'grep .', 'rule_based', 'low', '/tmp', ?)
	`, time.Now().AddDate(0, 0, -100).UTC().Format("2006-01-02T15:04:05Z"))
	if err != nil {
		t.Fatalf("inserting old entry: %v", err)
	}

	// Record a recent entry.
	store.Record(&Entry{Prompt: "recent prompt", IntentType: "search_text", ParsedBy: "rule_based", Risk: "low"})

	deleted, err := store.Purge(30)
	if err != nil {
		t.Fatalf("Purge failed: %v", err)
	}
	if deleted == 0 {
		t.Error("expected at least 1 entry to be purged")
	}

	entries, _ := store.List(10)
	for _, e := range entries {
		if e.Prompt == "old prompt" {
			t.Error("expected old entry to be purged")
		}
	}
}

func TestContextChunks(t *testing.T) {
	store := openTestStore(t)

	if err := store.StoreContextChunk("machine", "tools", "grep, find, jq", []string{"tools", "search"}); err != nil {
		t.Fatalf("StoreContextChunk failed: %v", err)
	}
	if err := store.StoreContextChunk("machine", "logs", "/var/log/nginx", []string{"logs", "nginx"}); err != nil {
		t.Fatalf("StoreContextChunk failed: %v", err)
	}

	// Query by tags.
	chunks, err := store.QueryContextChunks([]string{"nginx"}, 10)
	if err != nil {
		t.Fatalf("QueryContextChunks failed: %v", err)
	}
	if len(chunks) == 0 {
		t.Error("expected chunks matching 'nginx' tag")
	}
	if chunks[0].Content != "/var/log/nginx" {
		t.Errorf("unexpected content: %q", chunks[0].Content)
	}
}

func TestContextChunks_Upsert(t *testing.T) {
	store := openTestStore(t)

	// First insert.
	store.StoreContextChunk("machine", "tools", "grep", []string{"tools"})
	// Update (same source+section key).
	store.StoreContextChunk("machine", "tools", "grep, rg, fd", []string{"tools"})

	chunks, _ := store.QueryContextChunks([]string{"tools"}, 10)
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk after upsert, got %d", len(chunks))
	}
	if chunks[0].Content != "grep, rg, fd" {
		t.Errorf("expected updated content, got %q", chunks[0].Content)
	}
}

func containsStr(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && func() bool {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}()
}
