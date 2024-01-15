package main

import (
	"fmt"
	"log"
	"net/url"
	"os/exec"
	"path"
)

type SshIngester struct {
	uri *url.URL
}

func (t*SshIngester) runCommand(cmd string) error {
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

func (t *SshIngester) Ingest(w *Workflow) error {
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

	from := path.Join(w.Dir, f.Filename)
	to := fmt.Sprintf("%s:%s", t.uri.Hostname(), path.Join(t.uri.Path, ".input"))
	log.Println("starting scp", from, to)
	scp := exec.Command("scp", from, to)
	if err := scp.Start(); err != nil {
		log.Println("error starting scp", from, to, err)
		return err
	}
	if err := scp.Wait(); err != nil {
		log.Println("error running scp", from, to, err)
		return err
	}

	var cmd string

	// check sha256sum
	cmd = fmt.Sprintf("echo '%s  %s' | sha256sum -c", f.Shasum, oldfile)
	if err := t.runCommand(cmd); err != nil {
		log.Println("failed to verify checksum")
		return err
	}

	// create directory
	cmd = fmt.Sprintf("mkdir -p '%s'", newdir)
	if err := t.runCommand(cmd); err != nil {
		log.Println("failed to mkdir", newdir)
		return err
	}

	// fix permissions
	cmd = fmt.Sprintf("chmod 775 '%s'", newdir)
	if err := t.runCommand(cmd); err != nil {
		log.Println("failed to chmod dir", newdir)
		return err
	}
	cmd = fmt.Sprintf("chmod 664 '%s'", oldfile)
	if err := t.runCommand(cmd); err != nil {
		log.Println("failed to chmod file", newdir)
		return err
	}

	// add sha256sum to Movies.sha256
	cmd = fmt.Sprintf("echo '%s  %s/%s' | sort -k2 -o %s -m - %s", f.Shasum, moviedir, mkvfile, shafile, shafile)
	if err := t.runCommand(cmd); err != nil {
		log.Println("failed to add shasum", newdir)
		return err
	}

	// move Files
	cmd = fmt.Sprintf("mv '%s' '%s'", oldfile, newfile)
	if err := t.runCommand(cmd); err != nil {
		log.Println("failed to move files", newdir)
		return err
	}

	return nil
}

