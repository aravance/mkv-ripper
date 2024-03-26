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

func NewIngester(u *url.URL, useMovieDir bool) (Ingester, error) {
	switch u.Scheme {
	case "", "file":
		log.Println("file ingester", u)
		return &LocalIngester{u, useMovieDir}, nil
	case "ssh":
		log.Println("ssh ingester", u)
		return &SshIngester{u, useMovieDir}, nil
	default:
		return nil, fmt.Errorf("unsupported scheme: %s", u.Scheme)
	}
}
