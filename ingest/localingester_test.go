package ingest

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/aravance/mkv-ripper/model"
)

const testdir = "localingester_test"
const shasum = "c3ab8ff13720e8ad9047dd39466b3c8974e592c2fa383d4a3960714caef0c4f2"
const mkvfileContent = "foobar"
const movieDirShafileContent = `
5ecf8d2cc410094e8b82dd0bc178a57f3aa1e80916689beb00fe56148b1b1256  foo (1990)/foo (1990) - 480p.mkv
97df3588b5a3f24babc3851b372f0ba71a9dcdded43b14b9d06961bfc1707d9d  bar (1989)/bar (1989) - 4k.mkv
1b8e84ccf80aae39e1ca16393920c801a8fb78c5ae8ce5e6a5d636baa3d9386d  baz (2000)/baz (2000) - 4k.mkv
`
const shafileContent = `
5ecf8d2cc410094e8b82dd0bc178a57f3aa1e80916689beb00fe56148b1b1256  foo (1990) - 480p.mkv
97df3588b5a3f24babc3851b372f0ba71a9dcdded43b14b9d06961bfc1707d9d  bar (1989) - 4k.mkv
1b8e84ccf80aae39e1ca16393920c801a8fb78c5ae8ce5e6a5d636baa3d9386d  baz (2000) - 4k.mkv
`

const name = "bar"
const year = "1989"
const res = "1080p"

var mkvfile = model.MkvFile{
	Filename:   fmt.Sprintf("%s/bar.mkv", testdir),
	Shasum:     shasum,
	Resolution: res,
}

func TestIngest_movieDir(t *testing.T) {
	createTestDir(t)
	defer os.RemoveAll(testdir)

	useMovieDir := true
	createMkvFile(t)
	createShaFile(t, useMovieDir)

	ingester := LocalIngester{&url.URL{Path: testdir}, useMovieDir}
	if err := ingester.Ingest(mkvfile, name, year); err != nil {
		t.Fatalf("ingester.Ingest(m, %s, %s) error: %v", name, year, err)
	}

	statMkvFile(t, useMovieDir)
	compareShaFile(t, `
c3ab8ff13720e8ad9047dd39466b3c8974e592c2fa383d4a3960714caef0c4f2  bar (1989)/bar (1989) - 1080p.mkv
97df3588b5a3f24babc3851b372f0ba71a9dcdded43b14b9d06961bfc1707d9d  bar (1989)/bar (1989) - 4k.mkv
1b8e84ccf80aae39e1ca16393920c801a8fb78c5ae8ce5e6a5d636baa3d9386d  baz (2000)/baz (2000) - 4k.mkv
5ecf8d2cc410094e8b82dd0bc178a57f3aa1e80916689beb00fe56148b1b1256  foo (1990)/foo (1990) - 480p.mkv
`)
}

func TestIngest_noMovieDir(t *testing.T) {
	createTestDir(t)
	defer os.RemoveAll(testdir)

	useMovieDir := false
	createMkvFile(t)
	createShaFile(t, useMovieDir)

	ingester := LocalIngester{&url.URL{Path: testdir}, useMovieDir}
	if err := ingester.Ingest(mkvfile, name, year); err != nil {
		t.Fatalf("ingester.Ingest(m, %s, %s) error: %v", name, year, err)
	}

	statMkvFile(t, useMovieDir)
	compareShaFile(t, `
c3ab8ff13720e8ad9047dd39466b3c8974e592c2fa383d4a3960714caef0c4f2  bar (1989) - 1080p.mkv
97df3588b5a3f24babc3851b372f0ba71a9dcdded43b14b9d06961bfc1707d9d  bar (1989) - 4k.mkv
1b8e84ccf80aae39e1ca16393920c801a8fb78c5ae8ce5e6a5d636baa3d9386d  baz (2000) - 4k.mkv
5ecf8d2cc410094e8b82dd0bc178a57f3aa1e80916689beb00fe56148b1b1256  foo (1990) - 480p.mkv
`)
}

func TestIngestNoShafile(t *testing.T) {
	createTestDir(t)
	defer os.RemoveAll(testdir)

	useMovieDir := false
	createMkvFile(t)

	ingester := LocalIngester{&url.URL{Path: testdir}, useMovieDir}
	if err := ingester.Ingest(mkvfile, name, year); err != nil {
		t.Fatalf("ingester.Ingest(m, %s, %s) error: %v", name, year, err)
	}

	statMkvFile(t, useMovieDir)
	compareShaFile(t, `c3ab8ff13720e8ad9047dd39466b3c8974e592c2fa383d4a3960714caef0c4f2  bar (1989) - 1080p.mkv`)
}

func createTestDir(t *testing.T) {
	if err := os.Mkdir(testdir, 0755); err != nil && !errors.Is(err, os.ErrExist) {
		t.Fatalf("errror making dir '%s': %v", testdir, err)
	}
}

func createMkvFile(t *testing.T) {
	if err := os.WriteFile(mkvfile.Filename, []byte(mkvfileContent), 0644); err != nil {
		t.Fatalf("errror writing file '%s': %v", mkvfile.Filename, err)
	}
}

func createShaFile(t *testing.T, useMovieDir bool) string {
	shafile := fmt.Sprintf("%s/Movies.sha256", testdir)
	var content string
	if useMovieDir {
		content = movieDirShafileContent
	} else {
		content = shafileContent
	}
	if err := os.WriteFile(shafile, []byte(content), 0644); err != nil {
		t.Fatalf("errror writing file '%s': %v", shafile, err)
	}
	return shafile
}

func statMkvFile(t *testing.T, useMovieDir bool) {
	var outfile string
	if useMovieDir {
		outfile = fmt.Sprintf("%s/Movies/%s (%s)/%s (%s) - %s.mkv", testdir, name, year, name, year, res)
	} else {
		outfile = fmt.Sprintf("%s/Movies/%s (%s) - %s.mkv", testdir, name, year, res)
	}
	if _, err := os.Stat(outfile); err != nil {
		t.Fatalf("os.Stat(%s) failed: %v", outfile, err)
	}
}

func compareShaFile(t *testing.T, expected string) {
	shafile := fmt.Sprintf("%s/Movies.sha256", testdir)
	bytes, err := os.ReadFile(shafile)
	if err != nil {
		t.Fatalf("os.Readfile(%s) failed: %v", shafile, err)
	}

	content := string(bytes)
	if strings.TrimSpace(content) != strings.TrimSpace(expected) {
		t.Fatalf("shafile mismatch\n  --- actual ---\n%s\n\n  --- expected ---\n%s", strings.TrimSpace(content), strings.TrimSpace(expected))
	}
}
