package handler

import (
	"log"

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
	q := c.Param("query")
	qd := &gomdb.QueryData{
		Title:      q,
		SearchType: "movie",
	}
	res, err := h.omdbapi.Search(qd)
	if err != nil {
		log.Println("error searching omdb q:", q, "err:", err)
		return err
	}
	movies := make([]*gomdb.MovieResult, 0, len(res.Search))
	for _, r := range res.Search {
		m, err := h.omdbapi.MovieByImdbID(r.ImdbID)
		if err == nil {
			movies = append(movies, m)
		}
	}
	return render(c, omdbview.Show(movies))
}
