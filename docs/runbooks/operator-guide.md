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

## 5. Replica Promotion & Failover

BeastDB V1 does not employ automatic distributed consensus. In the event of leader loss:
1. Fence the decommissioned leader immediately to prevent split-brain writes.
2. Verify follower synchronization: check `/readyz` or call `CurrentLSN()` on each follower.
3. Select the candidate follower with the highest LSN.
4. Stop the follower process and restart it with `-role=leader`.
5. Point client traffic and remaining followers to the newly promoted leader.
