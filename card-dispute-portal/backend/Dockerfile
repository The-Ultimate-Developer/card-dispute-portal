# syntax=docker/dockerfile:1

ARG GO_VERSION=1.26.2

# ---------- Stage 1: build ----------
FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src

# Dependencies first: this layer is cached until go.mod/go.sum change
COPY go.mod go.sum* ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

# Then source code
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

# ---------- Stage 2: runtime ----------
FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/api /api
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/api"]
