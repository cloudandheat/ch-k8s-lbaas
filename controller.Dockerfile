FROM golang:1.21.3 AS builder

COPY cmd/ /src/cmd/
COPY internal/ /src/internal/
COPY go.mod go.sum /src/

WORKDIR /src/
RUN go mod vendor
RUN go vet ./... && go test ./...
RUN go build -trimpath ./cmd/ch-k8s-lbaas-controller/ch-k8s-lbaas-controller.go

FROM debian:bookworm-slim
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-suggests --no-install-recommends ca-certificates && apt-get clean && rm -rf /var/cache/apt/lists

COPY --from=builder /src/ch-k8s-lbaas-controller /

USER 10000:10000

EXPOSE 15203

ENTRYPOINT ["/ch-k8s-lbaas-controller"]
