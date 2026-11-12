# Stage 1: Build Frontend Assets
FROM node:22-alpine AS frontend-builder
WORKDIR /app/web

COPY web/package.json web/package-lock.json ./
RUN npm ci

COPY web/ ./
COPY assets/icon.svg ./public/icon.svg
RUN npm run build

# Stage 2: Build Go Static Binary
FROM golang:alpine AS backend-builder
WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY go.mod go.sum ./
COPY . .
# Copy compiled frontend into web/dist
COPY --from=frontend-builder /app/web/dist ./web/dist

RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/bin/andvari ./cmd/andvari

# Stage 3: Minimal Non-Root Runtime
FROM alpine:3.24

RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -g 1000 andvari && \
    adduser -u 1000 -G andvari -s /bin/sh -D andvari && \
    mkdir -p /config /data && \
    chown -R andvari:andvari /config /data

COPY --from=backend-builder /app/bin/andvari /usr/local/bin/andvari

USER 1000:1000
EXPOSE 8080

VOLUME ["/config", "/data"]

ENTRYPOINT ["/usr/local/bin/andvari"]
