PACKAGES = $(wildcard src/*/*.go)

kilo: kilo.go $(PACKAGES)
	go build kilo.go

clean:
	-rm -rf kilo
