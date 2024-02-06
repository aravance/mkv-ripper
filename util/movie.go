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

func GuessMainTitle(info *makemkv.DiscInfo) *makemkv.TitleInfo {
	if info == nil || len(info.Titles) == 0 {
		return nil
	}

	for _, t := range info.Titles {
		if guessIsMainTitle(t) {
			return &t
		}
	}
	return &info.Titles[0]
}

func GuessName(disc *makemkv.DiscInfo, title *makemkv.TitleInfo) string {
	if title != nil && title.Name != "" {
		return title.Name
	} else if disc != nil {
		if disc.Name != "" {
			return disc.Name
		} else {
			return disc.VolumeName
		}
	}
	return ""
}
