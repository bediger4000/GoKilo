kilo: kilo.go
	GOPATH=$$PWD go build kilo.go

clean:
	-rm -rf kilo
