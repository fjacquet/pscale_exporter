# Stage 1: Build
FROM golang:1.26 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Static build; statisticsKeys.json is embedded via go:embed.
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o pscale_exporter .

# Stage 2: Runtime
FROM alpine:latest

RUN apk --no-cache add ca-certificates && \
    adduser -D -u 10001 pscale && \
    mkdir -p /var/log/pscale_exporter && \
    chown pscale:pscale /var/log/pscale_exporter

COPY --from=builder /app/pscale_exporter /usr/bin/pscale_exporter
COPY config.yaml /etc/pscale_exporter/config.yaml

EXPOSE 2112

USER pscale

ENTRYPOINT ["/usr/bin/pscale_exporter"]
CMD ["--config", "/etc/pscale_exporter/config.yaml"]
