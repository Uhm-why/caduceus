.PHONY: all build test vet fmt fmt-check clean

all: fmt-check vet test build
build:
	CGO_ENABLED=0 go build -o bin/caduceus ./cmd/caduceus
test:
	go test -v -race -count=1 ./...
vet:
	go vet ./...
fmt:
	gofmt -s -w .
fmt-check:
	test -z "$$(gofmt -s -l .)"
clean:
	rm -rf bin/
