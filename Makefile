binaries=$(patsubst cmd/%,%,$(wildcard cmd/*))

all: lint test $(binaries)

$(binaries): %:
	go build -trimpath ./cmd/$@/$@.go

lint:
	go vet ./...

fmt:
	go fmt ./...

test:
	go test ./...

clean:
	rm -f ch-k8s-lbaas-agent ch-k8s-lbaas-controller

SHA256SUMS: $(binaries)
	sha256sums $(binaries) > $@

SHA256SUMS.asc:
	gpg2 --clearsign SHA256SUMS

signed: SHA256SUMS.asc

.PHONY: lint fmt all $(binaries) test signed
