# syntax=docker/dockerfile:1
# Multi-stage build (docs/10-INFRA-DEVOPS.md §1). One build stage compiles both
# binaries; two final targets (api/worker) ship each on distroless static as
# nonroot. Static (CGO_ENABLED=0) => the tiny distroless/static base works.
FROM golang:1.25 AS build
WORKDIR /src

# Cache module downloads independently of source changes.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG GIT_SHA=dev
ENV CGO_ENABLED=0 GOOS=linux
RUN go build -ldflags "-s -w -X main.version=${GIT_SHA}" -o /out/api ./cmd/api
RUN go build -ldflags "-s -w -X main.version=${GIT_SHA}" -o /out/worker ./cmd/worker

# --- api target ---
FROM gcr.io/distroless/static:nonroot AS api
COPY --from=build /out/api /api
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/api"]

# --- worker target ---
FROM gcr.io/distroless/static:nonroot AS worker
COPY --from=build /out/worker /worker
EXPOSE 8081
USER nonroot:nonroot
ENTRYPOINT ["/worker"]
