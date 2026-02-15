TEMPL_SRC := $(shell find view -name '*.templ')
TEMPL_GEN := $(TEMPL_SRC:.templ=_templ.go)

.PHONY: build clean test

build: $(TEMPL_GEN)
	go build -o build/mkv-ripper ./cmd/server

%_templ.go: %.templ
	go tool github.com/a-h/templ/cmd/templ generate -f $<

test: $(TEMPL_GEN)
	go test ./...

clean:
	rm -f build/mkv-ripper
	rm -f $(TEMPL_GEN)
