# Stage 1: Build
FROM golang:1.26.4 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Static build; statisticsKeys.json is embedded via go:embed.
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o pscale_exporter .

# Stage 2: Runtime
FROM alpine:latest

# Create the runtime user and log dir. These are busybox builtins (no network).
RUN adduser -D -u 10001 pscale && \
    mkdir -p /var/log/pscale_exporter && \
    chown pscale:pscale /var/log/pscale_exporter

# Copy the CA bundle from the builder stage instead of `apk add ca-certificates`.
# The latter fetches from the Alpine CDN over TLS, which fails behind a corporate
# MITM proxy: the bare alpine image has no CA bundle yet to validate the proxy
# cert (chicken-and-egg). The Debian-based golang builder already ships the bundle.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

COPY --from=builder /app/pscale_exporter /usr/bin/pscale_exporter
COPY config.yaml /etc/pscale_exporter/config.yaml

EXPOSE 2112

USER pscale

ENTRYPOINT ["/usr/bin/pscale_exporter"]
CMD ["--config", "/etc/pscale_exporter/config.yaml"]
