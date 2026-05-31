.PHONY: build clean install

build:
	go build -o quark .

strip: build
	strip quark

clean:
	rm -f quark

install: build
	mv quark /usr/local/bin/quark
