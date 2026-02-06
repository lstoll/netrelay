# syntax=docker/dockerfile:1.3

FROM golang:1-trixie AS build

RUN mkdir -p /src/netrelay
WORKDIR /src/netrelay

COPY go.mod go.sum ./
COPY cmd/go.mod cmd/go.sum ./cmd/
RUN cd cmd && go mod download

COPY . .

RUN --mount=type=cache,target=/root/.cache/go-build cd cmd && CGO_ENABLED=0 go install ./...

FROM debian:trixie

COPY --from=build /go/bin/local-relay /usr/bin/local-relay
COPY --from=build /go/bin/ts-relay /usr/bin/ts-relay
