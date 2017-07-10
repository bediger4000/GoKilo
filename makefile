PACKAGES = $(wildcard src/*/*.go)

kilo: kilo.go $(PACKAGES)
	GOPATH=$$PWD go build kilo.go

clean:
	-rm -rf kilo
