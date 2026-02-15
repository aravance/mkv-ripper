package drive

import (
	"database/sql"
	"testing"

	"github.com/aravance/go-makemkv"
	"github.com/google/go-cmp/cmp"
	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestSqliteDiscDatabase_SaveAndGet(t *testing.T) {
	db := openTestDB(t)
	discdb, err := NewSqliteDiscDatabase(db)
	if err != nil {
		t.Fatal(err)
	}

	info := &makemkv.DiscInfo{
		Name: "Test Disc",
		Titles: []makemkv.TitleInfo{
			{Id: 0, Name: "Title 1", FileName: "title1.mkv"},
		},
	}

	if err := discdb.SaveDiscInfo("uuid-1", info); err != nil {
		t.Fatal(err)
	}

	got, ok := discdb.GetDiscInfo("uuid-1")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !cmp.Equal(got, info) {
		t.Fatalf("mismatch: %s", cmp.Diff(info, got))
	}
}

func TestSqliteDiscDatabase_Upsert(t *testing.T) {
	db := openTestDB(t)
	discdb, err := NewSqliteDiscDatabase(db)
	if err != nil {
		t.Fatal(err)
	}

	info1 := &makemkv.DiscInfo{Name: "Original"}
	info2 := &makemkv.DiscInfo{Name: "Updated"}

	discdb.SaveDiscInfo("uuid-1", info1)
	discdb.SaveDiscInfo("uuid-1", info2)

	got, ok := discdb.GetDiscInfo("uuid-1")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if got.Name != "Updated" {
		t.Fatalf("expected Updated, got %s", got.Name)
	}
}

func TestSqliteDiscDatabase_GetNonExistent(t *testing.T) {
	db := openTestDB(t)
	discdb, err := NewSqliteDiscDatabase(db)
	if err != nil {
		t.Fatal(err)
	}

	_, ok := discdb.GetDiscInfo("nonexistent")
	if ok {
		t.Fatal("expected ok=false for nonexistent disc")
	}
}

func TestSqliteDiscDatabase_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	// First open: save data
	db1, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	discdb1, err := NewSqliteDiscDatabase(db1)
	if err != nil {
		t.Fatal(err)
	}
	info := &makemkv.DiscInfo{Name: "Persistent"}
	discdb1.SaveDiscInfo("uuid-p", info)
	db1.Close()

	// Second open: verify data
	db2, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	discdb2, err := NewSqliteDiscDatabase(db2)
	if err != nil {
		t.Fatal(err)
	}

	got, ok := discdb2.GetDiscInfo("uuid-p")
	if !ok {
		t.Fatal("expected data to persist across reopen")
	}
	if got.Name != "Persistent" {
		t.Fatalf("expected Persistent, got %s", got.Name)
	}
}
