# syntax=docker/dockerfile:1
# Build from apps/server: docker buildx build --platform linux/amd64,linux/arm64 .
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server && \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate
# The distroless image has no shell or curl, so the Compose health check runs
# this probe: it exits 0 when /readyz answers 200 on the port the server uses.
RUN mkdir /probe && cat > /probe/healthcheck.go <<'EOF'
package main

import (
	"net/http"
	"os"
)

func main() {
	resp, err := http.Get("http://127.0.0.1:8080/readyz")
	if err != nil {
		os.Exit(1)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		os.Exit(1)
	}
}
EOF
RUN cd /probe && CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/healthcheck healthcheck.go

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/server /out/migrate /out/healthcheck /
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/server"]
