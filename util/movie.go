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

func guessIsMainTitle(title makemkv.TitleInfo) bool {
	return strings.Contains(title.FileName, "MainFeature") ||
		title.SourceFileName == "00800.mpls"
}

func GuessMainTitleAndName(info *makemkv.DiscInfo) (title *makemkv.TitleInfo, name string) {
	if info == nil || len(info.Titles) == 0 {
		return
	}

	title = &info.Titles[0]
	for _, t := range info.Titles {
		if guessIsMainTitle(t) {
			title = &t
			break
		}
	}

	if title.Name != "" {
		name = title.Name
	} else if info.Name != info.VolumeName {
		name = info.Name
	}

	return
}
