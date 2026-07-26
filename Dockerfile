FROM oven/bun:1 AS frontend
WORKDIR /build
COPY backend/ui/package.json backend/ui/bun.lock ./
RUN bun install --frozen-lockfile
COPY backend/ui/ .
RUN bun run build

FROM golang:1.26-alpine AS backend
WORKDIR /build
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ .
COPY --from=frontend /build/dist ./ui/dist
RUN CGO_ENABLED=0 go build -o /usr/local/bin/pgmanager .

FROM alpine:3.21

ARG WALG_VERSION=3.0.8
ARG TARGETARCH=amd64
RUN apk add --no-cache ca-certificates tzdata postgresql17 postgresql17-client su-exec wget gcompat && \
    wget -q "https://github.com/wal-g/wal-g/releases/download/v${WALG_VERSION}/wal-g-pg-22.04-${TARGETARCH}" \
         -O /usr/local/bin/wal-g && \
    chmod +x /usr/local/bin/wal-g && \
    apk del wget

COPY --from=backend /usr/local/bin/pgmanager /usr/local/bin/
EXPOSE 8080
CMD ["pgmanager"]
