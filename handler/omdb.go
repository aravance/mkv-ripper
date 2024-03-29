package handler

import (
	"log"
	"slices"
	"strconv"
	"strings"
	"sync"

	omdbview "github.com/aravance/mkv-ripper/view/omdb"
	"github.com/eefret/gomdb"
	"github.com/labstack/echo/v4"
)

type OmdbHandler struct {
	omdbapi *gomdb.OmdbApi
}

func NewOmdbHandler(omdbapi *gomdb.OmdbApi) OmdbHandler {
	return OmdbHandler{
		omdbapi: omdbapi,
	}
}

func (h OmdbHandler) Search(c echo.Context) error {
	q := strings.TrimSpace(c.QueryParam("q"))
	if q == "" {
		return render(c, omdbview.Search(make([]*gomdb.MovieResult, 0)))
	}
	qd := &gomdb.QueryData{
		Title:      q,
		SearchType: "movie",
	}
	res, err := h.omdbapi.Search(qd)
	if res == nil && err != nil {
		log.Println("error searching omdb q:", q, "err:", err)
		return err
	}

	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	limit = min(limit, len(res.Search))
	if limit <= 0 {
		limit = len(res.Search)
	}

	var wg sync.WaitGroup
	movies := make([]*gomdb.MovieResult, limit, limit)
	for i, r := range res.Search {
		if i >= limit {
			break
		}
		wg.Add(1)
		go func(i int, imdbid string) {
			defer wg.Done()
			m, err := h.omdbapi.MovieByImdbID(imdbid)
			if err == nil {
				movies[i] = m
			}
		}(i, r.ImdbID)
	}
	wg.Wait()
	movies = slices.DeleteFunc(movies, isNil)
	return render(c, omdbview.Search(movies))
}

func isNil(m *gomdb.MovieResult) bool {
	return m == nil
}
