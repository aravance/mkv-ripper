package workflow

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/url"

	"github.com/aravance/mkv-ripper/drive"
	"github.com/aravance/mkv-ripper/model"
)

func NewSqliteWorkflowManager(
	db *sql.DB,
	driveman drive.DriveManager,
	discdb drive.DiscDatabase,
	targets []*url.URL,
	outdir string,
	useMovieDir bool,
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
		PRIMARY KEY(disc_id, title_id)
	)`)
	if err != nil {
		return nil, err
	}

	workflows := make(map[string]map[int]*model.Workflow)

	rows, err := db.Query("SELECT disc_id, title_id, label, original_name, status, imdb_id, name, year, file_json FROM workflows")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var discId, label, originalName, status string
		var titleId int
		var imdbId, name, year, fileJson sql.NullString

		if err := rows.Scan(&discId, &titleId, &label, &originalName, &status, &imdbId, &name, &year, &fileJson); err != nil {
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

		titleWfs := getOrCreate(workflows, discId)
		titleWfs[titleId] = wf
	}

	persistFn := func(m *workflowManager, w *model.Workflow) error {
		return sqlitePersist(db, w)
	}

	return &workflowManager{
		workflows:   workflows,
		driveman:    driveman,
		discdb:      discdb,
		targets:     targets,
		outdir:      outdir,
		useMovieDir: useMovieDir,
		persistFn:   persistFn,
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

	_, err := db.Exec(
		`INSERT INTO workflows (disc_id, title_id, label, original_name, status, imdb_id, name, year, file_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(disc_id, title_id) DO UPDATE SET
			label = excluded.label,
			original_name = excluded.original_name,
			status = excluded.status,
			imdb_id = excluded.imdb_id,
			name = excluded.name,
			year = excluded.year,
			file_json = excluded.file_json`,
		w.DiscId, w.TitleId, w.Label, w.OriginalName, string(w.Status),
		w.ImdbId, w.Name, w.Year, fileJson,
	)
	return err
}
