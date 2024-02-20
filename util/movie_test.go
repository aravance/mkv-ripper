package util

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/aravance/go-makemkv"
	"github.com/eefret/gomdb"
	"github.com/google/go-cmp/cmp"
)

func TestGetMovie(t *testing.T) {
	getMovieTest := func(name string) (*gomdb.MovieResult, error) {
		if strings.ToLower(name) == "toy story 3" {
			return &gomdb.MovieResult{Title: "Toy Story 3"}, nil
		} else {
			return nil, fmt.Errorf("no result found")
		}
	}

	result, err := getMovie("Toy Story 3  (Disc 1)", getMovieTest)
	if err != nil {
		t.Fatalf(`getMovie("Toy Story 3  (Disc 1)", getMovieTest) error : %v`, err)
	}

	expected := &gomdb.MovieResult{Title: "Toy Story 3"}
	if !cmp.Equal(result, expected) {
		t.Fatalf(`getMovie("Toy Story 3  (Disc 1)", getMovieTest) = %v, expected %v`, result, expected)
	}
}

func TestGetMainTitleToyStory3(t *testing.T) {
	data := readTestData(t)
	toyStory3 := data["toyStory3"]
	result := GuessMainTitle(toyStory3)
	if result.Id != 0 {
		t.Fatalf(`GuessMainTitle(toyStory3) = %v, expected %v`, result.Id, 0)
	}
}

func TestGetMainTitleToyStory4(t *testing.T) {
	data := readTestData(t)
	toyStory4 := data["toyStory4"]
	result := GuessMainTitle(toyStory4)
	if result.Id != 2 {
		t.Fatalf(`GuessMainTitle(toyStory4) = %v, expected %v`, result.Id, 2)
	}
}

func TestGetMainTitleLaLaLand(t *testing.T) {
	data := readTestData(t)
	laLaLand := data["laLaLand"]
	result := GuessMainTitle(laLaLand)
	if result.Id != 1 {
		t.Fatalf(`GuessMainTitle(laLaLand) = %v, expected %v`, result.Id, 1)
	}
}

func readTestData(t *testing.T) map[string]*makemkv.DiscInfo {
	b, err := os.ReadFile("movie_testdata")
	if err != nil {
		t.Fatalf("error reading test data: %v", err)
	}
	var m map[string]*makemkv.DiscInfo
	json.Unmarshal(b, &m)
	return m
}
