package util

import (
	"log"
	"strings"

	"github.com/aravance/go-makemkv"
	"github.com/eefret/gomdb"
)

func GetMovie(api *gomdb.OmdbApi, name string) (movie *gomdb.MovieResult, err error) {
	movie, err = api.MovieByTitle(&gomdb.QueryData{Title: name})
	if err != nil {
		log.Println("error fetching movie:", name, "err:", err)
		index := strings.IndexAny(name, "[{(:")
		if index > 0 {
			name = strings.TrimSpace(name[0:index])
			movie, err = api.MovieByTitle(&gomdb.QueryData{Title: name})
			if err != nil {
				log.Println("error fetching movie with stripped name:", name, "err:", err)
			}
		}
	}

	return movie, err
}

func GuessMainTitleAndName(info *makemkv.DiscInfo) (title *makemkv.TitleInfo, name string) {
	if info == nil || len(info.Titles) == 0 {
		return
	}

	for _, t := range info.Titles {
		if t.SourceFileName == "00800.mpls" {
			title = &t
			break
		}
	}
	if title == nil {
		title = &info.Titles[0]
	}
	if title != nil && title.Name != "" {
		name = title.Name
	}
	if info.Name != info.VolumeName {
		name = info.Name
	}
	return
}
