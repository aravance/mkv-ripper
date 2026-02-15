package main

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/aravance/go-makemkv"
	"github.com/aravance/mkv-ripper/drive"
	"github.com/aravance/mkv-ripper/model"
	"github.com/aravance/mkv-ripper/workflow"
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

// Mock DriveManager for workflow manager
type testDriveManager struct{}

func (m *testDriveManager) Eject() error                             { return nil }
func (m *testDriveManager) GetDiscInfo() (*makemkv.DiscInfo, error)  { return nil, nil }
func (m *testDriveManager) GetDisc() *drive.Disc                    { return nil }
func (m *testDriveManager) HasDisc() bool                           { return false }
func (m *testDriveManager) Start() error                            { return nil }
func (m *testDriveManager) Stop() error                             { return nil }
func (m *testDriveManager) Status() drive.DriveStatus                { return drive.StatusEmpty }
func (m *testDriveManager) RipFile(_ *makemkv.TitleInfo, _ string, _ chan makemkv.Status) (*model.MkvFile, error) {
	return nil, nil
}

func newTestDiscDB(t *testing.T, db *sql.DB) drive.DiscDatabase {
	t.Helper()
	discdb, err := drive.NewSqliteDiscDatabase(db)
	if err != nil {
		t.Fatal(err)
	}
	return discdb
}

func newTestWorkflowManager(t *testing.T, db *sql.DB, discdb drive.DiscDatabase) workflow.WorkflowManager {
	t.Helper()
	wfm, err := workflow.NewSqliteWorkflowManager(db, &testDriveManager{}, discdb, nil, t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	return wfm
}

func TestMigrateJsonDiscs_ImportsAll(t *testing.T) {
	db := openTestDB(t)
	discdb := newTestDiscDB(t, db)
	tmpDir := t.TempDir()

	discs := map[string]*makemkv.DiscInfo{
		"uuid-1": {Name: "Disc 1"},
		"uuid-2": {Name: "Disc 2"},
	}
	data, _ := json.Marshal(discs)
	file := filepath.Join(tmpDir, "discs.json")
	os.WriteFile(file, data, 0644)

	migrateJsonDiscs(file, discdb)

	if _, ok := discdb.GetDiscInfo("uuid-1"); !ok {
		t.Fatal("uuid-1 not migrated")
	}
	if _, ok := discdb.GetDiscInfo("uuid-2"); !ok {
		t.Fatal("uuid-2 not migrated")
	}

	// Verify renamed to .bak
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("expected json file to be renamed")
	}
	if _, err := os.Stat(file + ".bak"); err != nil {
		t.Fatal("expected .bak file to exist")
	}
}

func TestMigrateJsonDiscs_SkipsExisting(t *testing.T) {
	db := openTestDB(t)
	discdb := newTestDiscDB(t, db)
	tmpDir := t.TempDir()

	// Pre-populate
	discdb.SaveDiscInfo("uuid-1", &makemkv.DiscInfo{Name: "Original"})

	discs := map[string]*makemkv.DiscInfo{
		"uuid-1": {Name: "FromJSON"},
		"uuid-2": {Name: "New"},
	}
	data, _ := json.Marshal(discs)
	file := filepath.Join(tmpDir, "discs.json")
	os.WriteFile(file, data, 0644)

	migrateJsonDiscs(file, discdb)

	info, _ := discdb.GetDiscInfo("uuid-1")
	if info.Name != "Original" {
		t.Fatalf("expected existing to be preserved, got %s", info.Name)
	}
	if _, ok := discdb.GetDiscInfo("uuid-2"); !ok {
		t.Fatal("uuid-2 not migrated")
	}
}

func TestMigrateJsonDiscs_NoFile(t *testing.T) {
	db := openTestDB(t)
	discdb := newTestDiscDB(t, db)
	// Should not panic
	migrateJsonDiscs("/nonexistent/discs.json", discdb)
}

func TestMigrateJsonDiscs_MalformedJSON(t *testing.T) {
	db := openTestDB(t)
	discdb := newTestDiscDB(t, db)
	tmpDir := t.TempDir()

	file := filepath.Join(tmpDir, "discs.json")
	os.WriteFile(file, []byte("{bad json"), 0644)

	// Should not panic
	migrateJsonDiscs(file, discdb)

	// File should still exist (not renamed since migration failed)
	if _, err := os.Stat(file); os.IsNotExist(err) {
		t.Fatal("malformed json file should not be renamed")
	}
}

func TestMigrateJsonWorkflows_ImportsAll(t *testing.T) {
	db := openTestDB(t)
	discdb := newTestDiscDB(t, db)
	wfm := newTestWorkflowManager(t, db, discdb)
	tmpDir := t.TempDir()

	wfs := map[string]map[int]*model.Workflow{
		"d1": {
			0: {Label: "MOVIE", OriginalName: "movie.mkv", Status: model.StatusDone},
		},
		"d2": {
			1: {Label: "TV", OriginalName: "ep.mkv", Status: model.StatusStart},
		},
	}
	data, _ := json.Marshal(wfs)
	file := filepath.Join(tmpDir, "workflows.json")
	os.WriteFile(file, data, 0644)

	migrateJsonWorkflows(file, wfm)

	if wfm.GetWorkflow("d1", 0) == nil {
		t.Fatal("d1/0 not migrated")
	}
	if wfm.GetWorkflow("d2", 1) == nil {
		t.Fatal("d2/1 not migrated")
	}

	// Renamed
	if _, err := os.Stat(file + ".bak"); err != nil {
		t.Fatal("expected .bak")
	}
}

func TestMigrateJsonWorkflows_SkipsExisting(t *testing.T) {
	db := openTestDB(t)
	discdb := newTestDiscDB(t, db)
	wfm := newTestWorkflowManager(t, db, discdb)
	tmpDir := t.TempDir()

	// Pre-populate
	wfm.Save(&model.Workflow{DiscId: "d1", TitleId: 0, Label: "EXISTING", OriginalName: "x", Status: model.StatusDone})

	wfs := map[string]map[int]*model.Workflow{
		"d1": {0: {Label: "FROM_JSON", OriginalName: "y", Status: model.StatusStart}},
	}
	data, _ := json.Marshal(wfs)
	file := filepath.Join(tmpDir, "workflows.json")
	os.WriteFile(file, data, 0644)

	migrateJsonWorkflows(file, wfm)

	got := wfm.GetWorkflow("d1", 0)
	if got.Label != "EXISTING" {
		t.Fatalf("expected existing preserved, got %s", got.Label)
	}
}

func TestMigrateJsonWorkflows_NoFile(t *testing.T) {
	db := openTestDB(t)
	discdb := newTestDiscDB(t, db)
	wfm := newTestWorkflowManager(t, db, discdb)
	migrateJsonWorkflows("/nonexistent/workflows.json", wfm)
}

func TestMigrateJsonWorkflows_MalformedJSON(t *testing.T) {
	db := openTestDB(t)
	discdb := newTestDiscDB(t, db)
	wfm := newTestWorkflowManager(t, db, discdb)
	tmpDir := t.TempDir()

	file := filepath.Join(tmpDir, "workflows.json")
	os.WriteFile(file, []byte("not json"), 0644)

	migrateJsonWorkflows(file, wfm)

	if _, err := os.Stat(file); os.IsNotExist(err) {
		t.Fatal("malformed json file should not be renamed")
	}
}
