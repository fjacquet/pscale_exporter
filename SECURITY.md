# Security Policy

## Reporting a Vulnerability

**Please do not open a public GitHub issue for security vulnerabilities.**

Report privately through a GitHub security advisory:
<https://github.com/fjacquet/pscale_exporter/security/advisories/new>

Include:

- A description of the vulnerability and its potential impact.
- Steps to reproduce, or a proof of concept if you have one.
- The version(s) affected (`pscale_exporter --version`).

You will get an acknowledgement within 72 hours and a resolution timeline once the issue
is assessed. Please allow time for a fix before public disclosure.

## Supported Versions

Only the latest release receives security fixes.

## Security Notes

### Credentials

Cluster credentials are supplied in one of two ways:

- **Environment-variable references** in `config.yaml`: `password: "${PSCALE1_PASSWORD}"`.
  A reference that resolves to an empty value is rejected at startup for the `endpoint`,
  `username` and `password` fields, and only the field name is logged, never the value.
- **`passwordFile`**: a path to a file containing the password, readable only by the
  exporter's user.

Never commit credentials. A `.env` next to the config file is loaded automatically; keep it
out of version control.

### TLS verification

TLS certificate verification of the OneFS API is **on by default**. `insecureSkipVerify`
is an operator opt-in for clusters with a self-signed certificate: the exporter logs a
warning at startup when it is set. Use it only on a trusted network, and prefer installing
a valid certificate.

### Exposed endpoints

The exporter serves, on port 9444 by default:

- `/metrics`: Prometheus metrics.
- `/health`: JSON status of every configured cluster (always answers 200).
- `/livez` and `/readyz`: static probes (always 200).

**There is no authentication or TLS on these endpoints, and the default bind address is
`0.0.0.0`.** The metrics expose cluster, node, quota path and share names. Run the exporter
on a trusted network, bind it to a specific interface through `server.host`, or put it
behind a reverse proxy that adds TLS and authentication.

### Least privilege

The exporter needs a **read-only** OneFS account, with read access to at least
`ISI_PRIV_STATISTICS`, `ISI_PRIV_QUOTA` and `ISI_PRIV_DEVICES`. It makes no configuration
changes; apart from the session login it only issues read requests.

### Debug output

`--trace` logs OneFS response bodies (method, URL, status and body; never headers, so
session cookies are not logged) and `--dump-dir` writes the raw responses to disk. These
contain cluster, node, quota path and share names. Review them before sharing, for
example in a bug report. Never enable the SDK's own verbose logging
(`GOISILON_DEBUG` / `verboseLogging`): it prints response headers, including the session
cookie.
