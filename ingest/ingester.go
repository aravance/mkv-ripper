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
