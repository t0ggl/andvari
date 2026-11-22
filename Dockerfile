# Stage 1: Build Frontend Assets
FROM node:26.13.1-alpine@sha256:6bd9ab43c0ec0190ca7b2d193906cb8b7edf6e7f4afa43ddeccd6452710bffb5 AS frontend-builder
RUN apk add --no-cache make
WORKDIR /app

COPY Makefile ./
COPY web/ ./web/
COPY assets/icon.svg ./web/public/icon.svg
RUN make build-frontend

# Stage 2: Build Go Static Binary
FROM golang:1.27.4-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS backend-builder
WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata make

COPY go.mod go.sum ./
COPY . .
# Copy compiled frontend into web/dist
COPY --from=frontend-builder /app/web/dist ./web/dist

RUN make build-backend BIN_DIR=/app/bin

# Stage 3: Minimal Non-Root Runtime
FROM alpine:3.24.4@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf

RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -g 1000 andvari && \
    adduser -u 1000 -G andvari -s /bin/sh -D andvari && \
    mkdir -p /config /data && \
    chown -R andvari:andvari /config /data

COPY --from=backend-builder /app/bin/andvari /usr/local/bin/andvari

USER 1000:1000
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://127.0.0.1:8080/healthz || exit 1

VOLUME ["/config", "/data"]

ENTRYPOINT ["/usr/local/bin/andvari"]
