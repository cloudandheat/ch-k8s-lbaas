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

clean:
	rm -f ch-k8s-lbaas-agent ch-k8s-lbaas-controller

.PHONY: lint fmt all $(binaries) test
