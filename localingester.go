package main

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
)

type LocalIngester struct {
	uri *url.URL
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

func (t *LocalIngester) Ingest(w *Workflow) error {
	if len(w.Files) == 0 {
		return fmt.Errorf("no available files to ingest")
	}
	if len(w.Files) > 1 {
		return fmt.Errorf("too many available files to ingest")
	}
	f := w.Files[0]

	moviedir := fmt.Sprintf("%s (%s)", *w.Name, *w.Year)
	mkvfile := fmt.Sprintf("%s (%s) - %s.mkv", *w.Name, *w.Year, f.Resolution)

	newdir := path.Join(t.uri.Path, "Movies", moviedir)
	newfile := path.Join(newdir, mkvfile)
	oldfile := path.Join(t.uri.Path, ".input", f.Filename)
	shafile := path.Join(t.uri.Path, "Movies.sha256")

	err := os.MkdirAll(path.Join(t.uri.Path, ".input"), 0775)
	if err != nil {
		log.Println("error making input dir", err)
		return err
	}
	from := path.Join(w.Dir, f.Filename)
	i, err := os.Open(from)
	if err != nil {
		log.Println("error opening existing file", i, err)
		return err
	}
	o, err := os.Create(oldfile)
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
	log.Println("Checking shasum", oldfile)
	shasum, err := sha256sum(oldfile)
	if err != nil {
		return err
	}
	if shasum != f.Shasum {
		return fmt.Errorf("shasum does not match expected: " + f.Shasum + ", actual: " + shasum)
	}

	// create directory
	err = os.MkdirAll(newdir, 0775)
	if err != nil {
		return err
	}

	// fix permissions
	err = os.Chmod(oldfile, 0664)
	if err != nil {
		return err
	}

	// add sha256sum to Movies.sha256
	log.Println("Adding shasum to shasums file")
	shasums, err := readShasums(shafile)
	if err != nil {
		return err
	}
	shakey := path.Join(moviedir, mkvfile)
	shasums[shakey] = shasum
	err = writeShasums(shafile, shasums)
	if err != nil {
		return err
	}

	// move Files
	log.Println("Moving files")
	err = os.Rename(oldfile, newfile)
	if err != nil {
		return err
	}

	log.Println("Done.")
	return nil
}
