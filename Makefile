binaries=$(patsubst cmd/%,%,$(wildcard cmd/*))

all: lint test $(binaries)

$(binaries): %:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath ./cmd/$@/$@.go

lint:
	go vet ./...

fmt:
	go fmt ./...

test:
	go test ./...

clean:
	rm -f ch-k8s-lbaas-agent ch-k8s-lbaas-controller

SHA256SUMS: $(binaries)
	sha256sum $(binaries) > $@

SHA256SUMS.asc: SHA256SUMS
	gpg2 --clearsign SHA256SUMS

signed: SHA256SUMS.asc

.PHONY: lint fmt all $(binaries) test signed
