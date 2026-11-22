# Stage 1: Build Frontend Assets
FROM node:26.13.1-alpine@sha256:6bd9ab43c0ec0190ca7b2d193906cb8b7edf6e7f4afa43ddeccd6452710bffb5 AS frontend-builder
WORKDIR /app/web

COPY web/package.json web/package-lock.json ./
RUN npm ci

COPY web/ ./
COPY assets/icon.svg ./public/icon.svg
RUN npm run build

# Stage 2: Build Go Static Binary
FROM golang:1.27.4-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS backend-builder
WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY go.mod go.sum ./
COPY . .
# Copy compiled frontend into web/dist
COPY --from=frontend-builder /app/web/dist ./web/dist

RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/bin/andvari ./cmd/andvari

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
