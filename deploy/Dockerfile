# Akena Watch — imagen para Cloudflare Containers (y Docker en general).
# Multi-stage: compilación Go estática + runtime Alpine mínimo.

# ---- etapa de compilación ----
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/akena-watch .

# ---- etapa de runtime ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates fuse curl bash tzdata

# Adaptador FUSE para montar buckets R2 (persistencia en Cloudflare Containers).
# Sin credenciales R2 el montaje se omite y /data es un directorio normal.
RUN ARCH=$(uname -m) && \
    if [ "$ARCH" = "x86_64" ]; then ARCH="amd64"; fi && \
    if [ "$ARCH" = "aarch64" ]; then ARCH="arm64"; fi && \
    VERSION=$(curl -s https://api.github.com/repos/tigrisdata/tigrisfs/releases/latest | grep -o '"tag_name": "[^"]*' | cut -d'"' -f4) && \
    curl -L "https://github.com/tigrisdata/tigrisfs/releases/download/${VERSION}/tigrisfs_${VERSION#v}_linux_${ARCH}.tar.gz" -o /tmp/tigrisfs.tar.gz && \
    tar -xzf /tmp/tigrisfs.tar.gz -C /usr/local/bin/ && \
    rm /tmp/tigrisfs.tar.gz && \
    chmod +x /usr/local/bin/tigrisfs

COPY --from=build /out/akena-watch /usr/local/bin/akena-watch
COPY deploy/startup.sh /startup.sh
RUN chmod +x /startup.sh

ENV AKENA_DATA_DIR=/data
ENV AKENA_BIND=0.0.0.0
ENV AKENA_PORT=8080

EXPOSE 8080
CMD ["/startup.sh"]
