package ingest

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/aravance/mkv-ripper/model"
	"github.com/aravance/mkv-ripper/util"
)

type LocalIngester struct {
	uri         *url.URL
	useMovieDir bool
}

func readShasums(shafile string) (map[string]string, error) {
	_, err := os.Stat(shafile)
	var lines []string
	if err == nil {
		b, err := os.ReadFile(shafile)
		if err != nil {
			return nil, err
		}
		lines = strings.Split(string(b), "\n")
	} else if errors.Is(err, fs.ErrNotExist) {
		err = nil
		lines = []string{}
	} else {
		return nil, err
	}
	shasums := make(map[string]string, len(lines)+1)
	for _, line := range lines {
		if len(line) > 66 {
			shasums[line[66:]] = line[0:64]
		}
	}
	return shasums, nil
}

func writeShasums(shafile string, shasums map[string]string) error {
	keys := make([]string, 0, len(shasums))
	for k := range shasums {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var buffer bytes.Buffer
	for _, key := range keys {
		buffer.WriteString(shasums[key])
		buffer.WriteString("  ")
		buffer.WriteString(key)
		buffer.WriteString("\n")
	}
	os.WriteFile(shafile, buffer.Bytes(), 0644)
	return nil
}

func (t *LocalIngester) Ingest(mkv model.MkvFile, name string, year string) error {
	moviedir := fmt.Sprintf("%s (%s)", name, year)
	mkvfile := fmt.Sprintf("%s (%s) - %s.mkv", name, year, mkv.Resolution)

	var newdir string
	if t.useMovieDir {
		newdir = path.Join(t.uri.Path, "Movies", moviedir)
	} else {
		newdir = path.Join(t.uri.Path, "Movies")
	}
	newfile := path.Join(newdir, mkvfile)
	ingestfile := path.Join(t.uri.Path, ".input", path.Base(mkv.Filename))
	shafile := path.Join(t.uri.Path, "Movies.sha256")

	err := os.MkdirAll(path.Join(t.uri.Path, ".input"), 0775)
	if err != nil {
		log.Println("error making input dir", err)
		return err
	}
	i, err := os.Open(mkv.Filename)
	if err != nil {
		log.Println("error opening existing file", i, err)
		return err
	}
	o, err := os.Create(ingestfile)
	if err != nil {
		log.Println("error opening write file", i, err)
		return err
	}
	_, err = io.Copy(o, i)
	if err != nil {
		log.Println("error copying file", i, err)
		return err
	}

	// check sha256sum
	log.Println("Checking shasum", ingestfile)
	shasum, err := util.Sha256sum(ingestfile)
	if err != nil {
		return err
	}
	if shasum != mkv.Shasum {
		return fmt.Errorf("shasum does not match expected: " + mkv.Shasum + ", actual: " + shasum)
	}

	// create directory
	err = os.MkdirAll(newdir, 0775)
	if err != nil {
		return err
	}

	// fix permissions
	err = os.Chmod(ingestfile, 0664)
	if err != nil {
		return err
	}

	// add sha256sum to Movies.sha256
	log.Println("Adding shasum to shasums file")
	shasums, err := readShasums(shafile)
	if err != nil {
		return err
	}

	var shakey string
	if t.useMovieDir {
		shakey = path.Join(moviedir, mkvfile)
	} else {
		shakey = path.Join(mkvfile)
	}
	shasums[shakey] = shasum

	err = writeShasums(shafile, shasums)
	if err != nil {
		return err
	}

	// move Files
	log.Println("Moving files")
	err = os.Rename(ingestfile, newfile)
	if err != nil {
		return err
	}

	log.Println("Done.")
	return nil
}
