.PHONY: build clean install vet release snapshot

build:
	go build -o quark .

strip: build
	strip quark

clean:
	rm -f quark
	rm -rf dist/

install: build
	mv quark /usr/local/bin/quark

vet:
	go vet ./...

release:
	goreleaser release --clean

snapshot:
	goreleaser release --snapshot --clean
