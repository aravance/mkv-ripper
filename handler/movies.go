package handler

import (
	"slices"
	"strconv"

	"github.com/aravance/mkv-ripper/model"
)

// resolutionRank orders known resolutions from highest to lowest quality so
// badges show up in a predictable order; unknown resolutions sort last,
// alphabetically among themselves.
var resolutionRank = map[string]int{
	"4k":    0,
	"1080p": 1,
	"720p":  2,
	"480p":  3,
}

func movieKey(wf *model.Workflow) string {
	if wf.ImdbId != nil && *wf.ImdbId != "" {
		return "imdb:" + *wf.ImdbId
	}
	if wf.Name != nil && *wf.Name != "" {
		year := ""
		if wf.Year != nil {
			year = *wf.Year
		}
		return "name:" + normalizeTitle(*wf.Name) + ":" + year
	}
	return "wf:" + wf.DiscId + ":" + strconv.Itoa(wf.TitleId)
}

// groupMovies groups done workflows for the same movie (e.g. separate 4k and
// 1080p disc rips) into a single MovieGroup, sorted most-recent-first.
func groupMovies(done []*model.Workflow) []*model.MovieGroup {
	groups := make(map[string]*model.MovieGroup)
	order := make([]string, 0)

	for _, wf := range done {
		key := movieKey(wf)
		g, ok := groups[key]
		if !ok {
			name := wf.Label
			if wf.Name != nil && *wf.Name != "" {
				name = *wf.Name
			}
			year := ""
			if wf.Year != nil {
				year = *wf.Year
			}
			g = &model.MovieGroup{Name: name, Year: year, ImdbId: wf.ImdbId}
			groups[key] = g
			order = append(order, key)
		}
		g.Workflows = append(g.Workflows, wf)
		if wf.UpdatedAt.After(g.UpdatedAt) {
			g.UpdatedAt = wf.UpdatedAt
		}
	}

	result := make([]*model.MovieGroup, 0, len(order))
	for _, key := range order {
		g := groups[key]
		slices.SortFunc(g.Workflows, func(a, b *model.Workflow) int {
			return resolutionRankOf(a) - resolutionRankOf(b)
		})
		result = append(result, g)
	}

	slices.SortFunc(result, func(a, b *model.MovieGroup) int {
		return b.UpdatedAt.Compare(a.UpdatedAt)
	})

	return result
}

func resolutionRankOf(wf *model.Workflow) int {
	if wf.Resolution == nil {
		return len(resolutionRank)
	}
	if rank, ok := resolutionRank[*wf.Resolution]; ok {
		return rank
	}
	return len(resolutionRank)
}
