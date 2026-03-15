BIN := clux
GOBIN ?= $(shell go env GOPATH)/bin

.PHONY: build install clean vet

build:
	go build -o $(BIN) .

install:
	go install .

clean:
	rm -f $(BIN)

vet:
	go vet ./...
