package ingest

import (
	"fmt"
	"log"
	"net/url"

	"github.com/aravance/mkv-ripper/model"
)

type Ingester interface {
	Ingest(mkv model.MkvFile, name string, year string) error
}

type Target struct {
	URL         *url.URL
	Shafile     string
	UseMovieDir bool
}

func NewIngester(t Target) (Ingester, error) {
	switch t.URL.Scheme {
	case "", "file":
		log.Println("file ingester", t)
		return &LocalIngester{t.URL, t.UseMovieDir, t.Shafile}, nil
	case "ssh":
		log.Println("ssh ingester", t)
		return &SshIngester{t.URL, t.UseMovieDir, t.Shafile}, nil
	default:
		return nil, fmt.Errorf("unsupported scheme: %s", t.URL.Scheme)
	}
}
