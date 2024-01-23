package drive

import (
	"encoding/json"
	"log"
	"os"

	"github.com/aravance/go-makemkv"
)

type DiscDatabase interface {
	GetDiscInfo(string) (info makemkv.DiscInfo, ok bool)
	SaveDiscInfo(string, makemkv.DiscInfo) error
}

func NewJsonDiscDatabase(file string) DiscDatabase {
	discInfoMap, err := loadDiscInfoJson(file)
	if err != nil {
		log.Printf("failed to load previous disc info")
		discInfoMap = make(map[string]makemkv.DiscInfo)
	}
	return &jsonDiscDatabase{
		file,
		discInfoMap,
	}
}

type jsonDiscDatabase struct {
	file        string
	discInfoMap map[string]makemkv.DiscInfo
}

func (d *jsonDiscDatabase) GetDiscInfo(id string) (info makemkv.DiscInfo, ok bool) {
	info, ok = d.discInfoMap[id]
	return info, ok
}

func (d *jsonDiscDatabase) SaveDiscInfo(id string, info makemkv.DiscInfo) error {
	d.discInfoMap[id] = info

	if bytes, err := json.Marshal(d.discInfoMap); err != nil {
		return err
	} else if err := os.WriteFile(d.file, bytes, 0644); err != nil {
		return err
	} else {
		return nil
	}
}

func loadDiscInfoJson(file string) (infomap map[string]makemkv.DiscInfo, err error) {
	bytes, err := os.ReadFile(file)
	if err != nil {
		log.Println("Failed to read file:", file, err)
		return nil, err
	}

	err = json.Unmarshal(bytes, &infomap)
	if err != nil {
		log.Println("failed to unmarshal json:", file, err)
		return nil, err
	}

	return infomap, nil
}
