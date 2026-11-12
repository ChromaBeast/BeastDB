# Security Policy

## Reporting a vulnerability

**Please do not open a public issue, pull request or discussion for a security problem.**

Report it privately through GitHub:

1. Go to the repository's **Security** tab.
2. Choose **Report a vulnerability** (GitHub private vulnerability reporting).

Include what you can of:

- the affected version or commit and how you run BeastDB (flags, TLS, roles, replication);
- steps to reproduce, or a proof of concept;
- the impact you observed (for example unauthorised read or write, data corruption, denial of service);
- any suggested fix.

If the **Report a vulnerability** button is missing, the maintainers have not enabled it yet. Open a public issue that says only "security contact needed" and contains no technical details, and a maintainer will arrange a private channel.

### What to expect

BeastDB is maintained by a small team, so these are goals, not guarantees:

- acknowledgement of your report within about 5 working days;
- an assessment and a plan within about 14 days;
- a fix or documented mitigation as quickly as the severity allows, with coordinated public disclosure after a fix is available. We will credit you if you want to be credited.

## Supported versions

BeastDB has not reached a tagged V1 release (see [ROADMAP.md](ROADMAP.md)). Until then, security fixes are made on the `main` branch only. Once V1 is released, this section will list the supported release lines.

## Scope

In scope:

- authentication and authorization (gRPC bearer tokens and roles, Studio sessions and cookies, the `/metrics` endpoint);
- transport security (TLS and mTLS for clients and replicas);
- the storage engine, WAL, recovery, backup and restore paths, where a flaw could lose or corrupt acknowledged data or expose data to the wrong caller;
- the replication stream;
- the container image and default configuration shipped in this repository.

Out of scope:

- findings that require the attacker to already have admin credentials, local root access, or write access to the data directory;
- running with `--dev` or `--insecure-auth` on a reachable network (these flags are documented as for local, non-production use);
- denial of service that needs a very large volume of traffic;
- missing hardening that is already listed as planned in [docs/upgrade-spec.md](docs/upgrade-spec.md) (for example encryption at rest). Reports on how a planned item could be exploited today are still welcome.

## Deploying safely

The [Operator Guide](docs/runbooks/operator-guide.md) is the reference. In short:

- set a strong `BEASTDB_ADMIN_PASSWORD`; the server refuses the default password on non-loopback listeners and when `BEASTDB_ENV=production`;
- enable TLS (and mTLS for replicas) and keep gRPC and Studio off public interfaces;
- give each application its own API token with the smallest role it needs (`viewer`, `writer`, `admin`, `replication`) and set an expiry where you can;
- protect `/metrics` with a dedicated `BEASTDB_METRICS_TOKEN` and scrape it over TLS;
- keep backups off the host and test restores.

## Safe harbour

We will not pursue action against researchers who make a good-faith effort to follow this policy: test only against systems you own or have permission to test, avoid accessing or modifying other people's data, do not degrade service for others, and give us reasonable time to fix an issue before disclosing it.
