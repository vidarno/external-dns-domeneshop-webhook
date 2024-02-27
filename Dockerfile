# syntax=docker/dockerfile:1

# We use a multi-stage build setup.
# (https://docs.docker.com/build/building/multi-stage/)

# Stage 1 (to create a "build" image, ~850MB)
FROM golang:1.22.0 AS builder
# smoke test to verify if golang is available
RUN go version

WORKDIR /app/
COPY go.mod go.sum ./

RUN go mod download

COPY ./ ./

RUN GOOS=linux GOARCH=amd64 \
    go build \
    -trimpath \
    -ldflags="-w -s" \
    -o app
#Commented-out until tests are ready..
#RUN go test -cover -v ./...

# Stage 2 (to create a downsized "container executable", ~5MB)

# If you need SSL certificates for HTTPS, replace `FROM SCRATCH` with:
#
#   FROM alpine:3.17.1
#   RUN apk --no-cache add ca-certificates
#
FROM scratch
WORKDIR /root/
COPY --from=builder /app .

EXPOSE 8888
ENTRYPOINT ["./app"]
