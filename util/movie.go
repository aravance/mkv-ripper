package util

import (
	"log"
	"strings"

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
