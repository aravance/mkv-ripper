package main

type FakeDevice struct {
	label string
	dev string
	devType string
	available bool
}

func (t *FakeDevice) Label() string {
	return t.label
}

func (t *FakeDevice) Dev() string {
	return t.dev
}

func (t *FakeDevice) Type() string {
	return t.devType
}

func (t *FakeDevice) Available() bool {
	return t.available
}

