# Go port of Sherlock.
#   docker build -t sherlock .
#   docker run --rm sherlock --help
#   docker run --rm sherlock user123

FROM golang:1.22-bookworm AS build
WORKDIR /src

ARG VERSION=0.16.2

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X github.com/Nurdich/sherlock/internal/version.Version=${VERSION}" \
    -o /out/sherlock ./cmd/sherlock

FROM debian:bookworm-slim

ARG VERSION=0.16.2
ARG VCS_REF=
ARG VCS_URL="https://github.com/Nurdich/sherlock"

ENV SHERLOCK_ENV=docker

LABEL org.label-schema.vcs-ref=$VCS_REF \
      org.label-schema.vcs-url=$VCS_URL \
      org.label-schema.name="Sherlock" \
      org.label-schema.version=$VERSION \
      website="https://sherlockproject.xyz"

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=build /out/sherlock /usr/local/bin/sherlock

WORKDIR /sherlock
ENTRYPOINT ["sherlock"]
