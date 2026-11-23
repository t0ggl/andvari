# syntax=docker/dockerfile:1@sha256:572cebbc2b15e6f84c077742952674889cb63cd5e9b80abdffcbe587b6f19183

# Stage 1: Build Frontend Assets
FROM --platform=$BUILDPLATFORM node:26.13.1-alpine@sha256:6bd9ab43c0ec0190ca7b2d193906cb8b7edf6e7f4afa43ddeccd6452710bffb5 AS frontend-builder
RUN apk add --no-cache make
WORKDIR /app

COPY Makefile ./
COPY web/package.json web/package-lock.json ./web/
RUN make install-frontend

COPY web/index.html web/tsconfig.json web/vite.config.ts ./web/
COPY web/public ./web/public
COPY web/src ./web/src
COPY assets/icon.svg ./web/public/icon.svg
RUN npm --prefix web run build

# Stage 2: Build Go Static Binary
FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS backend-builder
WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata make && \
    mkdir -p /out/root/config/users /out/root/data/users

COPY go.mod go.sum ./
RUN go mod download

COPY Makefile ./
COPY cmd ./cmd
COPY internal ./internal
COPY web/web.go ./web/web.go
COPY --from=frontend-builder /app/web/dist ./web/dist

ARG TARGETOS=linux
ARG TARGETARCH
ARG VERSION=dev

RUN GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" make build-backend BIN_DIR=/app/bin VERSION="${VERSION}"

# Stage 3: Minimal Non-Root Runtime
FROM alpine:3.24.4@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf

COPY --from=backend-builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=backend-builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=backend-builder --chown=1000:1000 /out/root/ /
COPY --from=backend-builder /app/bin/andvari /usr/local/bin/andvari

USER 1000:1000
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://127.0.0.1:8080/healthz || exit 1

VOLUME ["/config", "/data"]

ENTRYPOINT ["/usr/local/bin/andvari"]
