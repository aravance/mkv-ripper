package workflow

import (
	"database/sql"
	"os"
	"testing"

	"github.com/aravance/go-makemkv"
	"github.com/aravance/mkv-ripper/drive"
	"github.com/aravance/mkv-ripper/model"
	"github.com/google/go-cmp/cmp"
	_ "modernc.org/sqlite"
)

// Mock DriveManager
type mockDriveManager struct{}

func (m *mockDriveManager) Eject() error                            { return nil }
func (m *mockDriveManager) GetDiscInfo() (*makemkv.DiscInfo, error) { return nil, nil }
func (m *mockDriveManager) GetDisc() *drive.Disc                    { return nil }
func (m *mockDriveManager) HasDisc() bool                           { return false }
func (m *mockDriveManager) Start() error                            { return nil }
func (m *mockDriveManager) Stop() error                             { return nil }
func (m *mockDriveManager) Status() drive.DriveStatus               { return drive.StatusEmpty }
func (m *mockDriveManager) RipFile(_ *makemkv.TitleInfo, _ string, _ chan makemkv.Status) (*model.MkvFile, error) {
	return nil, nil
}

// Mock DiscDatabase
type mockDiscDB struct {
	data map[string]*makemkv.DiscInfo
}

func (d *mockDiscDB) GetDiscInfo(id string) (*makemkv.DiscInfo, bool) {
	info, ok := d.data[id]
	return info, ok
}
func (d *mockDiscDB) SaveDiscInfo(id string, info *makemkv.DiscInfo) error {
	d.data[id] = info
	return nil
}

func newTestManager(t *testing.T) (WorkflowManager, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	wfm, err := NewSqliteWorkflowManager(db, &mockDriveManager{}, &mockDiscDB{data: map[string]*makemkv.DiscInfo{}}, nil, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return wfm, db
}

func strPtr(s string) *string { return &s }

func TestSqliteWorkflowManager_SaveAndGet(t *testing.T) {
	wfm, _ := newTestManager(t)

	wf := &model.Workflow{
		DiscId: "disc-1", TitleId: 0, Label: "MOVIE", OriginalName: "movie.mkv", Status: model.StatusStart,
	}
	if err := wfm.Save(wf); err != nil {
		t.Fatal(err)
	}

	got := wfm.GetWorkflow("disc-1", 0)
	if got == nil {
		t.Fatal("expected workflow")
	}
	if !cmp.Equal(got, wf, cmp.FilterPath(func(p cmp.Path) bool { return p.String() == "MkvStatus" }, cmp.Ignore())) {
		t.Fatalf("mismatch: %s", cmp.Diff(wf, got))
	}
}

func TestSqliteWorkflowManager_GetWorkflows(t *testing.T) {
	wfm, _ := newTestManager(t)

	wfm.Save(&model.Workflow{DiscId: "d1", TitleId: 0, Label: "L", OriginalName: "a", Status: model.StatusStart})
	wfm.Save(&model.Workflow{DiscId: "d1", TitleId: 1, Label: "L", OriginalName: "b", Status: model.StatusStart})
	wfm.Save(&model.Workflow{DiscId: "d2", TitleId: 0, Label: "L", OriginalName: "c", Status: model.StatusStart})

	wfs := wfm.GetWorkflows("d1")
	if len(wfs) != 2 {
		t.Fatalf("expected 2 workflows for d1, got %d", len(wfs))
	}
}

func TestSqliteWorkflowManager_GetAllWorkflows(t *testing.T) {
	wfm, _ := newTestManager(t)

	wfm.Save(&model.Workflow{DiscId: "d1", TitleId: 0, Label: "L", OriginalName: "a", Status: model.StatusStart})
	wfm.Save(&model.Workflow{DiscId: "d2", TitleId: 0, Label: "L", OriginalName: "b", Status: model.StatusStart})

	all := wfm.GetAllWorkflows()
	if len(all) != 2 {
		t.Fatalf("expected 2 total workflows, got %d", len(all))
	}
}

func TestSqliteWorkflowManager_NewWorkflow(t *testing.T) {
	wfm, _ := newTestManager(t)

	wf, isNew := wfm.NewWorkflow("d1", 0, "LABEL", "movie")
	if !isNew {
		t.Fatal("expected isNew=true")
	}
	if wf.DiscId != "d1" || wf.Status != model.StatusStart {
		t.Fatalf("unexpected workflow: %+v", wf)
	}

	// Save it, then NewWorkflow should return existing
	wfm.Save(wf)
	wf2, isNew2 := wfm.NewWorkflow("d1", 0, "NEWLABEL", "movie")
	if isNew2 {
		t.Fatal("expected isNew=false for existing workflow")
	}
	if wf2.Label != "NEWLABEL" {
		t.Fatalf("expected label update, got %s", wf2.Label)
	}
}

func TestSqliteWorkflowManager_NilOptionalFields(t *testing.T) {
	wfm, _ := newTestManager(t)

	wf := &model.Workflow{DiscId: "d1", TitleId: 0, Label: "L", OriginalName: "a", Status: model.StatusStart}
	wfm.Save(wf)

	got := wfm.GetWorkflow("d1", 0)
	if got.ImdbId != nil || got.Name != nil || got.Year != nil || got.File != nil {
		t.Fatal("expected nil optional fields")
	}
}

func TestSqliteWorkflowManager_FileRoundTrip(t *testing.T) {
	wfm, _ := newTestManager(t)

	file := &model.MkvFile{Filename: "/tmp/movie.mkv", Shasum: "abc123", Resolution: "1080p"}
	wf := &model.Workflow{
		DiscId: "d1", TitleId: 0, Label: "L", OriginalName: "a", Status: model.StatusPending,
		ImdbId: strPtr("tt1234567"), Name: strPtr("Test Movie"), Year: strPtr("2024"), File: file,
	}
	wfm.Save(wf)

	// Reopen from same DB to verify SQLite persistence
	got := wfm.GetWorkflow("d1", 0)
	if got.File == nil {
		t.Fatal("expected file")
	}
	if !cmp.Equal(got.File, file) {
		t.Fatalf("file mismatch: %s", cmp.Diff(file, got.File))
	}
}

func TestSqliteWorkflowManager_FileRoundTripPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	// Save with file
	db1, _ := sql.Open("sqlite", dbPath)
	wfm1, _ := NewSqliteWorkflowManager(db1, &mockDriveManager{}, &mockDiscDB{data: map[string]*makemkv.DiscInfo{}}, nil, tmpDir)
	file := &model.MkvFile{Filename: "/tmp/m.mkv", Shasum: "sha", Resolution: "4k"}
	wfm1.Save(&model.Workflow{DiscId: "d1", TitleId: 0, Label: "L", OriginalName: "a", Status: model.StatusDone, File: file})
	db1.Close()

	// Reopen
	db2, _ := sql.Open("sqlite", dbPath)
	defer db2.Close()
	wfm2, _ := NewSqliteWorkflowManager(db2, &mockDriveManager{}, &mockDiscDB{data: map[string]*makemkv.DiscInfo{}}, nil, tmpDir)
	got := wfm2.GetWorkflow("d1", 0)
	if got == nil || got.File == nil {
		t.Fatal("expected file after reopen")
	}
	if !cmp.Equal(got.File, file) {
		t.Fatalf("file mismatch: %s", cmp.Diff(file, got.File))
	}
}

func TestSqliteWorkflowManager_ResolutionAndUpdatedAtRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	db1, _ := sql.Open("sqlite", dbPath)
	wfm1, _ := NewSqliteWorkflowManager(db1, &mockDriveManager{}, &mockDiscDB{data: map[string]*makemkv.DiscInfo{}}, nil, tmpDir)
	wf := &model.Workflow{DiscId: "d1", TitleId: 0, Label: "L", OriginalName: "a", Status: model.StatusDone, Resolution: strPtr("4k")}
	if err := wfm1.Save(wf); err != nil {
		t.Fatal(err)
	}
	if wf.UpdatedAt.IsZero() {
		t.Fatal("expected Save to set UpdatedAt")
	}
	wantUpdatedAt := wf.UpdatedAt
	db1.Close()

	// Reopen from a fresh manager instance to verify SQLite persistence
	db2, _ := sql.Open("sqlite", dbPath)
	defer db2.Close()
	wfm2, _ := NewSqliteWorkflowManager(db2, &mockDriveManager{}, &mockDiscDB{data: map[string]*makemkv.DiscInfo{}}, nil, tmpDir)
	got := wfm2.GetWorkflow("d1", 0)
	if got == nil {
		t.Fatal("expected workflow after reopen")
	}
	if got.Resolution == nil || *got.Resolution != "4k" {
		t.Fatalf("expected resolution 4k, got %v", got.Resolution)
	}
	if !got.UpdatedAt.Equal(wantUpdatedAt) {
		t.Fatalf("expected UpdatedAt %v, got %v", wantUpdatedAt, got.UpdatedAt)
	}
}

func TestSqliteWorkflowManager_MigratesLegacySchema(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/legacy.db"

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Simulate a database created before the resolution/updated_at columns existed.
	_, err = db.Exec(`CREATE TABLE workflows (
		disc_id TEXT NOT NULL,
		title_id INTEGER NOT NULL,
		label TEXT,
		original_name TEXT,
		status TEXT,
		imdb_id TEXT,
		name TEXT,
		year TEXT,
		file_json TEXT,
		PRIMARY KEY(disc_id, title_id)
	)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO workflows (disc_id, title_id, label, original_name, status, imdb_id, name, year)
		VALUES ('d1', 0, 'L', 'a', 'Done', 'tt1', 'Movie', '2020')`)
	if err != nil {
		t.Fatal(err)
	}

	wfm, err := NewSqliteWorkflowManager(db, &mockDriveManager{}, &mockDiscDB{data: map[string]*makemkv.DiscInfo{}}, nil, tmpDir)
	if err != nil {
		t.Fatal("expected legacy schema to migrate cleanly:", err)
	}

	got := wfm.GetWorkflow("d1", 0)
	if got == nil {
		t.Fatal("expected pre-existing row to load")
	}
	if got.Resolution != nil {
		t.Fatal("expected nil resolution for migrated row")
	}

	// New saves should populate the migrated columns.
	got.Resolution = strPtr("1080p")
	if err := wfm.Save(got); err != nil {
		t.Fatal(err)
	}
	if got.UpdatedAt.IsZero() {
		t.Fatal("expected UpdatedAt to be set after save on migrated row")
	}
}

func TestSqliteWorkflowManager_Clean(t *testing.T) {
	wfm, _ := newTestManager(t)
	tmpDir := t.TempDir()

	// Create a temp file to clean
	tmpFile := tmpDir + "/movie.mkv"
	if err := writeFile(tmpFile); err != nil {
		t.Fatal(err)
	}

	wf := &model.Workflow{
		DiscId: "d1", TitleId: 0, Label: "L", OriginalName: "a", Status: model.StatusDone,
		File:       &model.MkvFile{Filename: tmpFile, Shasum: "abc", Resolution: "1080p"},
		Resolution: strPtr("1080p"),
	}
	wfm.Save(wf)

	if err := wfm.Clean(wf); err != nil {
		t.Fatal(err)
	}
	if wf.File != nil {
		t.Fatal("expected File to be nil after Clean")
	}
	if wf.Resolution == nil || *wf.Resolution != "1080p" {
		t.Fatal("expected Resolution to survive Clean")
	}

	got := wfm.GetWorkflow("d1", 0)
	if got.File != nil {
		t.Fatal("expected persisted File to be nil after Clean")
	}
	if got.Resolution == nil || *got.Resolution != "1080p" {
		t.Fatal("expected persisted Resolution to survive Clean")
	}
}

func writeFile(path string) error {
	return os.WriteFile(path, []byte("data"), 0644)
}
