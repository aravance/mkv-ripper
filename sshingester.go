package main

import (
	"fmt"
	"log"
	"net/url"
	"os/exec"
	"path"
	"strings"
)

type SshIngester struct {
	uri *url.URL
}

func runCommand(host string, cmd string) error {
	ssh := exec.Command("ssh", host, cmd)
	log.Println(ssh)

	err := ssh.Start()
	if err != nil {
		log.Println("error starting ssh", cmd, err)
		return err
	}

	err = ssh.Wait()
	if err != nil {
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

	parts := strings.Split(t.uri.Opaque, ":")
	if len(parts) != 2 {
		return fmt.Errorf("unable to parse host and path")
	}
	host := parts[0]
	remotepath := parts[1]
	newdir := path.Join(remotepath, "Movies", moviedir)
	newfile := path.Join(newdir, mkvfile)
	oldfile := path.Join(remotepath, ".input", f.Filename)
	shafile := path.Join(remotepath, "Movies.sha256")

	from := path.Join(w.Dir, f.Filename)
	to := path.Join(t.uri.Opaque, ".input")
	log.Println("starting scp", from, to)
	scp := exec.Command("scp", from, to)
	err := scp.Start()
	if err != nil {
		log.Println("error starting scp", from, to, err)
		return err
	}
	err = scp.Wait()
	if err != nil {
		log.Println("error running scp", from, to, err)
		return err
	}

	// check sha256sum
	cmd := fmt.Sprintf("echo '%s  %s' | sha256sum -c", f.Shasum, oldfile)
	err = runCommand(host, cmd)
	if err != nil {
		log.Println("failed to verify checksum")
		return err
	}

	// create directory
	cmd = fmt.Sprintf("mkdir -p '%s'", newdir)
	err = runCommand(host, cmd)
	if err != nil {
		log.Println("failed to mkdir", newdir)
		return err
	}

	// fix permissions
	cmd = fmt.Sprintf("chmod 775 '%s'", newdir)
	err = runCommand(host, cmd)
	if err != nil {
		log.Println("failed to chmod dir", newdir)
		return err
	}
	cmd = fmt.Sprintf("chmod 664 '%s'", oldfile)
	err = runCommand(host, cmd)
	if err != nil {
		log.Println("failed to chmod file", newdir)
		return err
	}

	// add sha256sum to Movies.sha256
	cmd = fmt.Sprintf("echo '%s  %s/%s' | sort -k2 -o %s -m - %s", f.Shasum, moviedir, mkvfile, shafile, shafile)
	err = runCommand(host, cmd)
	if err != nil {
		log.Println("failed to add shasum", newdir)
		return err
	}

	// move Files
	cmd = fmt.Sprintf("mv '%s' '%s'", oldfile, newfile)
	err = runCommand(host, cmd)
	if err != nil {
		log.Println("failed to move files", newdir)
		return err
	}

	return nil
}

