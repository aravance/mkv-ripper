package main

import (
	"encoding/json"
	"log"
	"os"

	"github.com/aravance/go-makemkv"
	"github.com/aravance/mkv-ripper/drive"
	"github.com/aravance/mkv-ripper/model"
	"github.com/aravance/mkv-ripper/workflow"
)

func migrateJsonDiscs(file string, discdb drive.DiscDatabase) {
	bytes, err := os.ReadFile(file)
	if err != nil {
		return // no file to migrate
	}

	var discInfoMap map[string]*makemkv.DiscInfo
	if err := json.Unmarshal(bytes, &discInfoMap); err != nil {
		log.Println("failed to parse discs.json for migration:", err)
		return
	}

	migrated := 0
	for uuid, info := range discInfoMap {
		if _, ok := discdb.GetDiscInfo(uuid); ok {
			continue // already in sqlite
		}
		if err := discdb.SaveDiscInfo(uuid, info); err != nil {
			log.Println("failed to migrate disc", uuid, err)
		} else {
			migrated++
		}
	}

	if migrated > 0 {
		log.Printf("migrated %d disc(s) from %s", migrated, file)
	}
	if err := os.Rename(file, file+".bak"); err != nil {
		log.Println("failed to rename", file, "to .bak:", err)
	}
}

func migrateJsonWorkflows(file string, wfman workflow.WorkflowManager) {
	bytes, err := os.ReadFile(file)
	if err != nil {
		return // no file to migrate
	}

	var wfMap map[string]map[int]*model.Workflow
	if err := json.Unmarshal(bytes, &wfMap); err != nil {
		log.Println("failed to parse workflows.json for migration:", err)
		return
	}

	migrated := 0
	for discId, titles := range wfMap {
		for titleId, wf := range titles {
			if existing := wfman.GetWorkflow(discId, titleId); existing != nil {
				continue // already in sqlite
			}
			wf.DiscId = discId
			wf.TitleId = titleId
			if err := wfman.Save(wf); err != nil {
				log.Println("failed to migrate workflow", discId, titleId, err)
			} else {
				migrated++
			}
		}
	}

	if migrated > 0 {
		log.Printf("migrated %d workflow(s) from %s", migrated, file)
	}
	if err := os.Rename(file, file+".bak"); err != nil {
		log.Println("failed to rename", file, "to .bak:", err)
	}
}
