# Kubernetes

Deploy the exporter as a Deployment with the config in a ConfigMap and the cluster
password in a Secret. The example uses the `prometheus.io/*` scrape annotations; if you
run the Prometheus Operator, use the `ServiceMonitor` at the bottom instead.

## Secret + ConfigMap

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: pscale-exporter-secrets
  namespace: monitoring
type: Opaque
stringData:
  PSCALE1_PASSWORD: "your-monitor-password"
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: pscale-exporter-config
  namespace: monitoring
data:
  config.yaml: |
    server:
      host: "0.0.0.0"
      port: "9444"
      uri: "/metrics"
      logName: "/var/log/pscale_exporter/pscale-exporter.log"
    collection:
      interval: "30s"
      timeout: "20s"
    clusters:
      - name: pscale-cluster1
        endpoint: pscale-clu1.example.com
        port: 8080
        username: pscale-monitor
        password: "${PSCALE1_PASSWORD}"
        insecureSkipVerify: false
```

## Deployment + Service

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: pscale-exporter
  namespace: monitoring
spec:
  replicas: 1
  selector:
    matchLabels: { app: pscale-exporter }
  template:
    metadata:
      labels: { app: pscale-exporter }
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 10001
      containers:
        - name: pscale-exporter
          image: ghcr.io/fjacquet/pscale_exporter:latest
          args: ["--config", "/etc/pscale_exporter/config.yaml"]
          ports:
            - { name: metrics, containerPort: 9444 }
          envFrom:
            - secretRef: { name: pscale-exporter-secrets }
          livenessProbe:
            httpGet: { path: /health, port: metrics }
            initialDelaySeconds: 10
          readinessProbe:
            httpGet: { path: /health, port: metrics }
          volumeMounts:
            - { name: config, mountPath: /etc/pscale_exporter, readOnly: true }
            - { name: logs, mountPath: /var/log/pscale_exporter }
      volumes:
        - name: config
          configMap: { name: pscale-exporter-config }
        - name: logs
          emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: pscale-exporter
  namespace: monitoring
  labels: { app: pscale-exporter }
  annotations:
    prometheus.io/scrape: "true"
    prometheus.io/port: "9444"
    prometheus.io/path: "/metrics"
spec:
  selector: { app: pscale-exporter }
  ports:
    - { name: metrics, port: 9444, targetPort: metrics }
```

!!! note "Single replica"
    Run **one replica**. Each instance polls every configured cluster on its own
    interval; multiple replicas would duplicate OneFS API load and produce duplicate
    series. The exporter is a poller, not a request-serving app that benefits from
    horizontal scaling.

## Prometheus Operator (ServiceMonitor)

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: pscale-exporter
  namespace: monitoring
spec:
  selector:
    matchLabels: { app: pscale-exporter }
  endpoints:
    - port: metrics
      interval: 30s
      path: /metrics
```
