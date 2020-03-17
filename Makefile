binaries=$(patsubst cmd/%,%,$(wildcard cmd/*))

all: lint test $(binaries)

$(binaries): %:
	go build ./cmd/$@/$@.go

lint:
	go vet ./...

fmt:
	go fmt ./...

test:
	go test ./...

.PHONY: lint fmt all $(binaries) test
