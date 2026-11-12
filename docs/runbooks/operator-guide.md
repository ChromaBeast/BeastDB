# BeastDB V1 Operator Guide

This document describes operations, cluster roles, transport security, and observability for production BeastDB deployments.

---

## 1. Architecture & Cluster Roles

A BeastDB cluster consists of:
- **Leader (Writable):** Exactly one writable node running with `-role=leader`. Receives client writes, updates WAL and pages, and broadcasts committed WAL records to connected replicas.
- **Followers (Read Replicas):** Zero or more read replicas running with `-role=follower`. Write paths are strictly locked (`api.ErrReadOnlyReplica`). Followers continuously stream and replay WAL records from the leader.

---

## 2. Security & Credentials Policy

### Admin Bootstrap Protection
When bound to non-loopback interfaces or when `BEASTDB_ENV=production`, the server will **refuse to start** if the default password `admin` is configured.
- Set `BEASTDB_ADMIN_PASSWORD` to a cryptographically strong secret.
- For local ephemeral testing, pass `--dev` (binds to loopback, auto-cleans on exit).
- The flag `--insecure-auth` is strictly for headless non-production testing and must never be used in production environments.

### Session Secret & API Tokens
- Web sessions are authenticated via HMAC-SHA256 signatures backed by `BEASTDB_SESSION_SECRET` or persisted to `<data-dir>/session.key` (0600 file permissions).
- gRPC requests require Bearer token authorization when `BEASTDB_API_TOKEN` is configured or API tokens exist in the token store.

---

## 3. Transport Security (TLS / mTLS)

All client-to-leader, client-to-follower, and leader-to-replica streaming links support TLS:

```bash
# Leader with TLS
beastdb \
  -role=leader \
  -bind-addr=0.0.0.0 \
  -port=50051 \
  -tls-cert=/etc/beastdb/tls/server.crt \
  -tls-key=/etc/beastdb/tls/server.key \
  -tls-ca=/etc/beastdb/tls/ca.crt

# Follower with mTLS
beastdb \
  -role=follower \
  -leader-addr=leader.prod:50051 \
  -tls-cert=/etc/beastdb/tls/client.crt \
  -tls-key=/etc/beastdb/tls/client.key \
  -tls-ca=/etc/beastdb/tls/ca.crt
```

- When `-tls-ca` is provided on the leader, mutual TLS (mTLS) is enforced: connecting followers and gRPC clients must present valid certificates signed by the configured CA.

---

## 4. Health & Readiness Probes

The embedded HTTP server exposes unauthenticated health endpoints for Kubernetes, container health checks, and load balancers:

- **Liveness (`GET /healthz`):**
  Returns HTTP 200 with `{ "status": "ok", "version": "...", "uptime_seconds": 1234 }`. Indicates the HTTP listener is responsive.
- **Readiness (`GET /readyz`):**
  Returns HTTP 200 with `{ "status": "ready", "role": "leader", "lsn": 4821, "total_pages": 64 }` when the engine is initialized and storage is operational. Returns HTTP 503 if the database engine is uninitialized or degraded.

---

### Prometheus Metrics (`GET /metrics`)

The web server exposes Prometheus text exposition (format 0.0.4) at `/metrics` on `--web-addr`. It is **never anonymous**: there is no loopback exemption, because a reverse proxy or sidecar on the same host makes every remote peer look like `127.0.0.1`. A request is accepted only with one of:

- `Authorization: Bearer <token>` equal to the gRPC `--api-token`, or to `BEASTDB_METRICS_TOKEN` (preferred: a dedicated scrape-only secret), or
- an admin web session cookie.

With neither token configured, only admin sessions work (Prometheus cannot scrape). Set `BEASTDB_METRICS_TOKEN` and keep the web listener on loopback or a private network/TLS-terminating proxy, since the bearer token travels in cleartext over plain HTTP.

| Metric | Meaning |
|---|---|
| `beastdb_requests_total{protocol,method,code}` | gRPC (code name) and HTTP (status, route pattern) request counts; errors are non-`OK` / 4xx-5xx |
| `beastdb_request_duration_seconds` (histogram) | latency per protocol/method (streams: total stream duration) |
| `beastdb_grpc_requests_in_flight` | executing gRPC calls |
| `beastdb_wal_size_bytes`, `beastdb_wal_current_lsn` | WAL file size and highest LSN |
| `beastdb_checkpoint_last_lsn`, `beastdb_checkpoint_age_seconds` | last checkpoint by this process (series absent until the first checkpoint after start) |
| `beastdb_buffer_pool_{size,cached,pinned,dirty}_pages`, `beastdb_buffer_pool_hits_total` / `_misses_total` / `_hit_ratio` | buffer pool state; ratio is lifetime, use `rate()` of the counters for recent behavior |
| `beastdb_data_file_size_bytes`, `beastdb_disk_free_bytes` | data file size, free space on the data dir filesystem (unix only) |
| `beastdb_read_only` | 1 when writes are rejected (follower or fail-stop after a failed apply) |
| `beastdb_replica_applied_lsn` | followers only: last applied LSN |
| `beastdb_build_info{version,role}` | always 1 |

Replica lag is computed in PromQL: `max(beastdb_wal_current_lsn{role="leader"}) - beastdb_replica_applied_lsn` (add a `role` label via relabeling, or key on `instance`).

Sample scrape config (`prometheus.yml`):

```yaml
scrape_configs:
  - job_name: beastdb
    metrics_path: /metrics
    scrape_interval: 15s
    authorization:
      type: Bearer
      credentials_file: /etc/prometheus/beastdb.token   # contains BEASTDB_METRICS_TOKEN
    static_configs:
      - targets: ["leader.internal:8080"]
        labels: { role: leader }
      - targets: ["replica1.internal:8080", "replica2.internal:8080"]
        labels: { role: follower }
```

Example alert rules:

```yaml
groups:
  - name: beastdb
    rules:
      - alert: BeastDBDown
        expr: up{job="beastdb"} == 0
        for: 2m
      - alert: BeastDBHighErrorRate
        expr: |
          sum by (instance) (rate(beastdb_requests_total{protocol="grpc",code!~"OK|NotFound|AlreadyExists|FailedPrecondition"}[5m]))
            / sum by (instance) (rate(beastdb_requests_total{protocol="grpc"}[5m])) > 0.05
        for: 5m
      - alert: BeastDBHighP99Latency
        expr: histogram_quantile(0.99, sum by (instance, le) (rate(beastdb_request_duration_seconds_bucket{protocol="grpc",method!~".*Sync.*|.*Scan.*"}[5m]))) > 0.25
        for: 10m
      - alert: BeastDBWALGrowing
        expr: beastdb_wal_size_bytes > 1073741824 or beastdb_checkpoint_age_seconds > 600
        for: 10m
      - alert: BeastDBDiskLow
        expr: beastdb_disk_free_bytes < 5 * 1024^3
        for: 5m
      - alert: BeastDBReplicaLagging
        expr: max(beastdb_wal_current_lsn{role="leader"}) - on() group_right() beastdb_replica_applied_lsn > 10000
        for: 5m
      - alert: BeastDBFailStop
        expr: beastdb_read_only{role="leader"} == 1
        for: 1m
```

---

## 5. Replica Promotion & Failover

BeastDB V1 does not employ automatic distributed consensus. In the event of leader loss:
1. Fence the decommissioned leader immediately to prevent split-brain writes.
2. Verify follower synchronization: check `/readyz` or call `CurrentLSN()` on each follower.
3. Select the candidate follower with the highest LSN.
4. Stop the follower process and restart it with `-role=leader`.
5. Point client traffic and remaining followers to the newly promoted leader.
