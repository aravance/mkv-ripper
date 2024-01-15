package ingest

import (
	"fmt"
	"log"
	"net/url"

	"github.com/aravance/mkv-ripper/internal/model"
)

type Ingester interface {
	Ingest(w *model.Workflow) error
}

func NewIngester(uri string) (Ingester, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}
	switch u.Scheme {
	case "", "file":
		log.Println("file ingester", u)
		return &LocalIngester{u}, nil
	case "ssh":
		log.Println("ssh ingester", u)
		return &SshIngester{u}, nil
	default:
		return nil, fmt.Errorf("unknown scheme: %s", u.Scheme)
	}
}
