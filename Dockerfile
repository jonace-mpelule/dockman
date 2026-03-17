# syntax=docker/dockerfile:1.7

FROM --platform=$BUILDPLATFORM golang:1.25.5-alpine AS build

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG GOPRIVATE

WORKDIR /src

RUN apk add --no-cache ca-certificates git

COPY go.mod ./

RUN --mount=type=secret,id=github_token \
    set -eu; \
    if [ -n "${GOPRIVATE:-}" ]; then \
      go env -w GOPRIVATE="${GOPRIVATE}"; \
    fi; \
    if [ -s /run/secrets/github_token ]; then \
      token="$(cat /run/secrets/github_token)"; \
      git config --global url."https://${token}:x-oauth-basic@github.com/".insteadOf "https://github.com/"; \
    fi; \
    go mod download

COPY . .

RUN --mount=type=secret,id=github_token \
    set -eu; \
    if [ -n "${GOPRIVATE:-}" ]; then \
      go env -w GOPRIVATE="${GOPRIVATE}"; \
    fi; \
    if [ -s /run/secrets/github_token ]; then \
      token="$(cat /run/secrets/github_token)"; \
      git config --global url."https://${token}:x-oauth-basic@github.com/".insteadOf "https://github.com/"; \
    fi; \
    CGO_ENABLED=0 GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" \
      go build -ldflags="-s -w -X main.version=${VERSION}" -o /out/dockman .

FROM alpine:3.21

RUN apk add --no-cache ca-certificates docker-cli

COPY --from=build /out/dockman /usr/local/bin/dockman

ENTRYPOINT ["dockman"]
CMD ["help"]
