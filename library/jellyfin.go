package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Jellyfin is a Library backed by the Jellyfin REST API, authenticated with
// an API key (Dashboard -> API Keys).
type Jellyfin struct {
	baseURL string
	token   string
	// user optionally names the Jellyfin user used for queries that need a
	// user context (listing collection members). Defaults to the first
	// administrator.
	user   string
	client *http.Client

	mu     sync.Mutex
	userId string
}

func NewJellyfin(baseURL, token, user string) *Jellyfin {
	return &Jellyfin{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		user:    user,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (j *Jellyfin) Name() string { return "Jellyfin" }

type jfItem struct {
	Id             string
	Name           string
	ProductionYear int
	ProviderIds    map[string]string
}

type jfItems struct {
	Items            []jfItem
	TotalRecordCount int
}

func (j *Jellyfin) do(ctx context.Context, method, path string, query url.Values, out any) error {
	u := j.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", fmt.Sprintf(`MediaBrowser Token="%s"`, j.token))
	req.Header.Set("Accept", "application/json")

	resp, err := j.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("rejected the API key (%s); check the [[libraries]] token in mkv-ripper.toml", resp.Status)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		if msg := strings.TrimSpace(string(body)); msg != "" {
			return fmt.Errorf("jellyfin %s %s: %s: %s", method, path, resp.Status, msg)
		}
		return fmt.Errorf("jellyfin %s %s: %s", method, path, resp.Status)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (j *Jellyfin) items(ctx context.Context, query url.Values) ([]jfItem, error) {
	var res jfItems
	if err := j.do(ctx, http.MethodGet, "/Items", query, &res); err != nil {
		return nil, err
	}
	return res.Items, nil
}

// userID resolves (and caches) the user id used for user-scoped queries.
// Collection members are only listed when a user context is supplied.
func (j *Jellyfin) userID(ctx context.Context) (string, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.userId != "" {
		return j.userId, nil
	}

	var users []struct {
		Id     string
		Name   string
		Policy struct{ IsAdministrator bool }
	}
	if err := j.do(ctx, http.MethodGet, "/Users", nil, &users); err != nil {
		return "", err
	}
	for _, u := range users {
		if (j.user != "" && strings.EqualFold(u.Name, j.user)) || (j.user == "" && u.Policy.IsAdministrator) {
			j.userId = u.Id
			return u.Id, nil
		}
	}
	if j.user != "" {
		return "", fmt.Errorf("jellyfin user %q not found", j.user)
	}
	return "", errors.New("no jellyfin administrator found")
}

func (j *Jellyfin) Scan(ctx context.Context) error {
	return j.do(ctx, http.MethodPost, "/Library/Refresh", nil, nil)
}

func movieQuery() url.Values {
	return url.Values{
		"Recursive":        {"true"},
		"IncludeItemTypes": {"Movie"},
		"Fields":           {"ProviderIds"},
		"EnableImages":     {"false"},
		"EnableUserData":   {"false"},
	}
}

func (j *Jellyfin) FindMovie(ctx context.Context, q MovieQuery) (string, error) {
	if q.ImdbId == "" && q.Name == "" {
		return "", ErrNotFound
	}

	if q.Name != "" {
		query := movieQuery()
		query.Set("searchTerm", q.Name)
		if _, err := strconv.Atoi(q.Year); err == nil {
			query.Set("Years", q.Year)
		}
		items, err := j.items(ctx, query)
		if err != nil {
			return "", err
		}
		if id := matchMovie(items, q); id != "" {
			return id, nil
		}
	}

	// OMDb and Jellyfin (TMDb) titles don't always agree, e.g. "Sorcerer's"
	// vs "Philosopher's Stone", so fall back to matching the IMDb id across
	// the whole library. Jellyfin ignores the AnyProviderIdEquals filter, so
	// this has to be done client side.
	if q.ImdbId != "" {
		items, err := j.items(ctx, movieQuery())
		if err != nil {
			return "", err
		}
		if id := matchMovie(items, q); id != "" {
			return id, nil
		}
	}
	return "", ErrNotFound
}

// matchMovie picks the item matching q: by IMDb id when q has one, otherwise
// by exact (case-insensitive) title and year.
func matchMovie(items []jfItem, q MovieQuery) string {
	for _, it := range items {
		if q.ImdbId != "" {
			if strings.EqualFold(providerId(it, "imdb"), q.ImdbId) {
				return it.Id
			}
			continue
		}
		if strings.EqualFold(it.Name, q.Name) && (q.Year == "" || strconv.Itoa(it.ProductionYear) == q.Year) {
			return it.Id
		}
	}
	return ""
}

func (j *Jellyfin) boxSets(ctx context.Context) ([]jfItem, error) {
	return j.items(ctx, url.Values{
		"Recursive":        {"true"},
		"IncludeItemTypes": {"BoxSet"},
		"Fields":           {"ProviderIds"},
		"EnableImages":     {"false"},
		"EnableUserData":   {"false"},
	})
}

func (j *Jellyfin) findBoxSet(ctx context.Context, name string) (*jfItem, error) {
	sets, err := j.boxSets(ctx)
	if err != nil {
		return nil, err
	}
	for _, s := range sets {
		if strings.EqualFold(s.Name, name) {
			return &s, nil
		}
	}
	return nil, nil
}

func (j *Jellyfin) Collections(ctx context.Context) ([]Collection, error) {
	sets, err := j.boxSets(ctx)
	if err != nil {
		return nil, err
	}
	cols := make([]Collection, len(sets))
	for i, s := range sets {
		seriesId := providerId(s, "tmdb")
		cols[i] = Collection{Name: s.Name, Series: seriesId != "", SeriesId: seriesId}
	}
	return cols, nil
}

func providerId(it jfItem, provider string) string {
	for k, v := range it.ProviderIds {
		if strings.EqualFold(k, provider) {
			return v
		}
	}
	return ""
}

func (j *Jellyfin) MovieSeriesId(ctx context.Context, movieId string) (string, error) {
	items, err := j.items(ctx, url.Values{
		"Ids":            {movieId},
		"Fields":         {"ProviderIds"},
		"EnableImages":   {"false"},
		"EnableUserData": {"false"},
	})
	if err != nil {
		return "", err
	}
	for _, it := range items {
		if it.Id == movieId {
			return providerId(it, "tmdbcollection"), nil
		}
	}
	return "", ErrNotFound
}

func (j *Jellyfin) MovieCollections(ctx context.Context, movieId string) ([]string, error) {
	userId, err := j.userID(ctx)
	if err != nil {
		return nil, err
	}
	sets, err := j.boxSets(ctx)
	if err != nil {
		return nil, err
	}

	// There's no "collections containing item X" query, and combining
	// ParentId with Ids ignores the ParentId, so list each set's members.
	names := make([]string, 0)
	for _, s := range sets {
		members, err := j.items(ctx, url.Values{
			"userId":         {userId},
			"ParentId":       {s.Id},
			"EnableImages":   {"false"},
			"EnableUserData": {"false"},
		})
		if err != nil {
			return nil, err
		}
		for _, m := range members {
			if m.Id == movieId {
				names = append(names, s.Name)
				break
			}
		}
	}
	return names, nil
}

func (j *Jellyfin) AddToCollection(ctx context.Context, movieId, collection string) error {
	set, err := j.findBoxSet(ctx, collection)
	if err != nil {
		return err
	}
	if set == nil {
		return j.do(ctx, http.MethodPost, "/Collections", url.Values{
			"Name": {collection},
			"Ids":  {movieId},
		}, nil)
	}
	return j.do(ctx, http.MethodPost, "/Collections/"+url.PathEscape(set.Id)+"/Items", url.Values{"Ids": {movieId}}, nil)
}

func (j *Jellyfin) RemoveFromCollection(ctx context.Context, movieId, collection string) error {
	set, err := j.findBoxSet(ctx, collection)
	if err != nil || set == nil {
		return err
	}
	return j.do(ctx, http.MethodDelete, "/Collections/"+url.PathEscape(set.Id)+"/Items", url.Values{"Ids": {movieId}}, nil)
}
