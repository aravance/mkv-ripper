package drive

import (
	"database/sql"
	"encoding/json"
	"log"

	"github.com/aravance/go-makemkv"
)

func NewSqliteDiscDatabase(db *sql.DB) (DiscDatabase, error) {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS disc_info (
		uuid TEXT PRIMARY KEY,
		info_json TEXT NOT NULL
	)`)
	if err != nil {
		return nil, err
	}

	discInfoMap := make(map[string]*makemkv.DiscInfo)

	rows, err := db.Query("SELECT uuid, info_json FROM disc_info")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var uuid, infoJson string
		if err := rows.Scan(&uuid, &infoJson); err != nil {
			log.Println("error scanning disc_info row:", err)
			continue
		}
		var info makemkv.DiscInfo
		if err := json.Unmarshal([]byte(infoJson), &info); err != nil {
			log.Println("error unmarshaling disc info for", uuid, err)
			continue
		}
		discInfoMap[uuid] = &info
	}

	return &sqliteDiscDatabase{db: db, discInfoMap: discInfoMap}, nil
}

type sqliteDiscDatabase struct {
	db          *sql.DB
	discInfoMap map[string]*makemkv.DiscInfo
}

func (d *sqliteDiscDatabase) GetDiscInfo(id string) (info *makemkv.DiscInfo, ok bool) {
	info, ok = d.discInfoMap[id]
	return info, ok
}

func (d *sqliteDiscDatabase) SaveDiscInfo(id string, info *makemkv.DiscInfo) error {
	bytes, err := json.Marshal(info)
	if err != nil {
		return err
	}

	_, err = d.db.Exec(
		"INSERT INTO disc_info (uuid, info_json) VALUES (?, ?) ON CONFLICT(uuid) DO UPDATE SET info_json = excluded.info_json",
		id, string(bytes),
	)
	if err != nil {
		return err
	}

	d.discInfoMap[id] = info
	return nil
}
