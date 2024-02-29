# syntax=docker/dockerfile:1

# We use a multi-stage build setup.
# (https://docs.docker.com/build/building/multi-stage/)

# Stage 1 (to create a "build" image, ~850MB)
FROM golang:1.22.0 AS builder
LABEL org.opencontainers.image.source="https://github.com/vidarno/external-dns-domeneshop-webhook>"

WORKDIR /src/
COPY go.mod go.sum ./

RUN go mod download

COPY ./ ./

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
    -trimpath \
    -ldflags="-w -s" \
    -o /app

#Commented-out until tests are ready..
#RUN go test -cover -v ./...

# Stage 2 (to create a downsized "container executable", ~5MB)
FROM gcr.io/distroless/base-debian11 AS build-release-stage

WORKDIR /
COPY --from=builder /app /app

EXPOSE 8888

USER nonroot:nonroot

ENTRYPOINT ["./app"]
