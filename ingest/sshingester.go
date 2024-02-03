package ingest

import (
	"fmt"
	"log"
	"net/url"
	"os/exec"
	"path"
	"strings"

	"github.com/aravance/mkv-ripper/model"
)

type SshIngester struct {
	uri *url.URL
}

func (t *SshIngester) runCommand(cmd string) error {
	ssh := exec.Command("ssh", t.uri.Host, cmd)
	log.Println(ssh)

	if err := ssh.Start(); err != nil {
		log.Println("error starting ssh", cmd, err)
		return err
	}

	if err := ssh.Wait(); err != nil {
		log.Println("error running ssh", cmd, err)
		return err
	}

	return nil
}

func escapeSsh(s string) string {
	return strings.ReplaceAll(s, `'`, `'\''`)
}

func (t *SshIngester) Ingest(mkv model.MkvFile, name string, year string) error {
	moviedir := fmt.Sprintf("%s (%s)", name, year)
	mkvfile := fmt.Sprintf("%s (%s) - %s.mkv", name, year, mkv.Resolution)

	newdir := path.Join(t.uri.Path, "Movies", moviedir)
	newfile := path.Join(newdir, mkvfile)
	ingestfile := path.Join(t.uri.Path, ".input", path.Base(mkv.Filename))
	shafile := path.Join(t.uri.Path, "Movies.sha256")

	out := fmt.Sprintf("%s:%s", t.uri.Hostname(), path.Join(t.uri.Path, ".input"))
	log.Println("starting scp", mkv.Filename, out)
	scp := exec.Command("scp", mkv.Filename, out)
	if err := scp.Start(); err != nil {
		log.Println("error starting scp", mkv.Filename, out, err)
		return err
	}
	if err := scp.Wait(); err != nil {
		log.Println("error running scp", mkv.Filename, out, err)
		return err
	}

	var cmd string

	// check sha256sum
	cmd = fmt.Sprintf("echo '%s  %s' | sha256sum -c", mkv.Shasum, escapeSsh(ingestfile))
	if err := t.runCommand(cmd); err != nil {
		log.Println("failed to verify checksum")
		return err
	}

	// create directory
	cmd = fmt.Sprintf("mkdir -p '%s'", escapeSsh(newdir))
	if err := t.runCommand(cmd); err != nil {
		log.Println("failed to mkdir", newdir)
		return err
	}

	// fix permissions
	cmd = fmt.Sprintf("chmod 775 '%s'", escapeSsh(newdir))
	if err := t.runCommand(cmd); err != nil {
		log.Println("failed to chmod dir", newdir)
		return err
	}
	cmd = fmt.Sprintf("chmod 664 '%s'", escapeSsh(ingestfile))
	if err := t.runCommand(cmd); err != nil {
		log.Println("failed to chmod file", newdir)
		return err
	}

	// add sha256sum to Movies.sha256
	cmd = fmt.Sprintf("echo '%s  %s/%s' | sort -k2 -o %s -m - %s", mkv.Shasum, escapeSsh(moviedir), escapeSsh(mkvfile), shafile, shafile)
	if err := t.runCommand(cmd); err != nil {
		log.Println("failed to add shasum", newdir)
		return err
	}

	// move Files
	cmd = fmt.Sprintf("mv '%s' '%s'", escapeSsh(ingestfile), escapeSsh(newfile))
	if err := t.runCommand(cmd); err != nil {
		log.Println("failed to move files", newdir)
		return err
	}

	return nil
}
