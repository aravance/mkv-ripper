package workflow

import (
	"database/sql"
	"encoding/json"
	"log"
	"time"

	"github.com/aravance/mkv-ripper/drive"
	"github.com/aravance/mkv-ripper/ingest"
	"github.com/aravance/mkv-ripper/model"
)

func NewSqliteWorkflowManager(
	db *sql.DB,
	driveman drive.DriveManager,
	discdb drive.DiscDatabase,
	targets []ingest.Target,
	outdir string,
) (WorkflowManager, error) {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS workflows (
		disc_id TEXT NOT NULL,
		title_id INTEGER NOT NULL,
		label TEXT,
		original_name TEXT,
		status TEXT,
		imdb_id TEXT,
		name TEXT,
		year TEXT,
		file_json TEXT,
		resolution TEXT,
		updated_at TEXT,
		collections_json TEXT,
		PRIMARY KEY(disc_id, title_id)
	)`)
	if err != nil {
		return nil, err
	}

	if err := addColumnIfMissing(db, "workflows", "resolution", "TEXT"); err != nil {
		return nil, err
	}
	if err := addColumnIfMissing(db, "workflows", "updated_at", "TEXT"); err != nil {
		return nil, err
	}
	if err := addColumnIfMissing(db, "workflows", "collections_json", "TEXT"); err != nil {
		return nil, err
	}

	workflows := make(map[string]map[int]*model.Workflow)

	rows, err := db.Query("SELECT disc_id, title_id, label, original_name, status, imdb_id, name, year, file_json, resolution, updated_at, collections_json FROM workflows")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var discId, label, originalName, status string
		var titleId int
		var imdbId, name, year, fileJson, resolution, updatedAt, collectionsJson sql.NullString

		if err := rows.Scan(&discId, &titleId, &label, &originalName, &status, &imdbId, &name, &year, &fileJson, &resolution, &updatedAt, &collectionsJson); err != nil {
			log.Println("error scanning workflow row:", err)
			continue
		}

		wf := &model.Workflow{
			DiscId:       discId,
			TitleId:      titleId,
			Label:        label,
			OriginalName: originalName,
			Status:       model.WorkflowStatus(status),
		}
		if imdbId.Valid {
			wf.ImdbId = &imdbId.String
		}
		if name.Valid {
			wf.Name = &name.String
		}
		if year.Valid {
			wf.Year = &year.String
		}
		if fileJson.Valid {
			var f model.MkvFile
			if err := json.Unmarshal([]byte(fileJson.String), &f); err != nil {
				log.Println("error unmarshaling file_json:", err)
			} else {
				wf.File = &f
			}
		}
		if resolution.Valid {
			wf.Resolution = &resolution.String
		}
		if updatedAt.Valid {
			if t, err := time.Parse(time.RFC3339Nano, updatedAt.String); err == nil {
				wf.UpdatedAt = t
			}
		}
		if collectionsJson.Valid {
			if err := json.Unmarshal([]byte(collectionsJson.String), &wf.Collections); err != nil {
				log.Println("error unmarshaling collections_json:", err)
			}
		}

		titleWfs := getOrCreate(workflows, discId)
		titleWfs[titleId] = wf
	}

	persistFn := func(m *workflowManager, w *model.Workflow) error {
		return sqlitePersist(db, w)
	}

	return &workflowManager{
		workflows: workflows,
		driveman:  driveman,
		discdb:    discdb,
		targets:   targets,
		outdir:    outdir,
		persistFn: persistFn,
	}, nil
}

func sqlitePersist(db *sql.DB, w *model.Workflow) error {
	var fileJson *string
	if w.File != nil {
		b, err := json.Marshal(w.File)
		if err != nil {
			return err
		}
		s := string(b)
		fileJson = &s
	}

	// keep nil (never chosen) distinct from empty (chosen none)
	var collectionsJson *string
	if w.Collections != nil {
		b, err := json.Marshal(w.Collections)
		if err != nil {
			return err
		}
		s := string(b)
		collectionsJson = &s
	}

	updatedAt := w.UpdatedAt.Format(time.RFC3339Nano)

	_, err := db.Exec(
		`INSERT INTO workflows (disc_id, title_id, label, original_name, status, imdb_id, name, year, file_json, resolution, updated_at, collections_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(disc_id, title_id) DO UPDATE SET
			label = excluded.label,
			original_name = excluded.original_name,
			status = excluded.status,
			imdb_id = excluded.imdb_id,
			name = excluded.name,
			year = excluded.year,
			file_json = excluded.file_json,
			resolution = excluded.resolution,
			updated_at = excluded.updated_at,
			collections_json = excluded.collections_json`,
		w.DiscId, w.TitleId, w.Label, w.OriginalName, string(w.Status),
		w.ImdbId, w.Name, w.Year, fileJson, w.Resolution, updatedAt, collectionsJson,
	)
	return err
}

// addColumnIfMissing adds a column to an existing sqlite table if it isn't
// already present, so databases created before the column was introduced
// get migrated in place on startup.
func addColumnIfMissing(db *sql.DB, table, column, coltype string) error {
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	_, err = db.Exec("ALTER TABLE " + table + " ADD COLUMN " + column + " " + coltype)
	return err
}
