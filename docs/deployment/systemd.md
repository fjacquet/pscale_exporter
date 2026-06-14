# systemd

Run the exporter as a hardened systemd service on a Linux host.

## 1. Install the binary

Place the binary at `/usr/local/bin/pscale_exporter` (from a
[release build](../getting-started/installation.md) or `make cli`) and create a dedicated
unprivileged user:

```bash
sudo useradd --system --no-create-home --shell /usr/sbin/nologin pscale
sudo install -m 0755 bin/pscale_exporter /usr/local/bin/pscale_exporter
sudo install -d -o pscale -g pscale /etc/pscale_exporter /var/log/pscale_exporter
sudo install -m 0640 -o pscale -g pscale config.yaml /etc/pscale_exporter/config.yaml
```

## 2. Cluster secret

Keep the password out of the unit. Use an `EnvironmentFile` referenced from
`config.yaml` as `${PSCALE1_PASSWORD}`:

```bash
# /etc/pscale_exporter/pscale.env  (root:pscale, mode 0640)
PSCALE1_PASSWORD=your-monitor-password
```

```bash
sudo install -m 0640 -o root -g pscale /dev/stdin /etc/pscale_exporter/pscale.env <<'EOF'
PSCALE1_PASSWORD=your-monitor-password
EOF
```

## 3. Unit file

```ini
# /etc/systemd/system/pscale_exporter.service
[Unit]
Description=Dell PowerScale (OneFS) Prometheus exporter
After=network-online.target
Wants=network-online.target

[Service]
User=pscale
Group=pscale
EnvironmentFile=/etc/pscale_exporter/pscale.env
ExecStart=/usr/local/bin/pscale_exporter --config /etc/pscale_exporter/config.yaml
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
RestartSec=5

# Hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=/var/log/pscale_exporter

[Install]
WantedBy=multi-user.target
```

## 4. Enable & verify

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now pscale_exporter
sudo systemctl status pscale_exporter
curl -s http://localhost:9444/health
```

`ExecReload` wires `systemctl reload pscale_exporter` to the exporter's `SIGHUP`
[hot reload](../getting-started/configuration.md#hot-reload) — edit the config and reload
without dropping the process.
