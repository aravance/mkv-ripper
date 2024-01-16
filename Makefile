.MAIN: mkv-ripper

all:   mkv-ripper

bin/templ:
	GOBIN="${PWD}/bin" go install github.com/a-h/templ/cmd/templ@latest

generate: bin/templ
	bin/templ generate

mkv-ripper: generate
	go build -o mkv-ripper github.com/aravance/mkv-ripper/cmd/server

clean:
	go clean github.com/aravance/mkv-ripper/cmd/server
	rm -f mkv-ripper
