package main

import "os"

type FakeDevice struct {
	label     string
	device    string
	devType   string
	available bool
}

func (t *FakeDevice) Label() string {
	return t.label
}

func (t *FakeDevice) Device() string {
	return t.device
}

func (t *FakeDevice) Type() string {
	return t.devType
}

func (t *FakeDevice) Available() bool {
	return t.available
}

type IsoDevice struct {
	label string
	path  string
}

func (d *IsoDevice) Label() string {
	return d.label
}

func (d *IsoDevice) Device() string {
	return d.path
}

func (d *IsoDevice) Type() string {
	return "iso"
}

func (d *IsoDevice) Available() bool {
	info, err := os.Stat(d.path)
	return err != nil && !info.IsDir()
}

type FileDevice struct {
	label string
	path  string
}

func (d *FileDevice) Label() string {
	return d.label
}

func (d *FileDevice) Device() string {
	return d.path
}

func (d *FileDevice) Type() string {
	return "file"
}

func (d *FileDevice) Available() bool {
	info, err := os.Stat(d.path)
	return err != nil && info.IsDir()
}
