TARGETDIR=.\deploy
sha1ver := $(shell git rev-parse HEAD)
test := $(shell date /t)


all: vet test  build

vet:
	go vet .\cmd\crontab
	go vet .\app

test: 
	go.exe test -timeout 30s .\app

# The sha1 stuff isn't working as of now
build:
	go build -o "$(TARGETDIR)\crontab.exe" -a -ldflags "-X main.sha1ver=$(sha1ver)" .\cmd\crontab
