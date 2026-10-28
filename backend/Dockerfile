# syntax=docker/dockerfile:1

ARG GO_VERSION=1.26.10

# ---------- Stage 1: build ----------
FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src

# Dependencies first: this layer is cached until go.mod/go.sum change
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

# Then source code
COPY . .

# Stamped into the binary so /version shows exactly which commit is running
ARG VERSION=dev
ARG BUILD_TIME=unknown
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath \
      -ldflags="-s -w \
        -X github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/buildinfo.Version=${VERSION} \
        -X github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/buildinfo.BuildTime=${BUILD_TIME}" \
      -o /out/api ./cmd/api

# ---------- Stage 2: runtime ----------
# No shell, no package manager, runs as UID 65532
FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/api /api
USER nonroot:nonroot
EXPOSE 8080
# Exec form: the binary is PID 1 and receives SIGTERM directly (graceful shutdown)
ENTRYPOINT ["/api"]
