---
sidebar_position: 3
title: Configuration
---

# Configuration

Immerle splits configuration in two:

- **Bootstrap settings** — a handful of values read from the environment (or a
  `.env` file) at startup. Changing them needs a **restart**.
- **Runtime settings** — everything else (providers, scan cadence, transcoding,
  CORS, device-token TTL, federation…), managed by an admin via the API and
  stored in `data/configuration.yaml`. No restart needed.

## Bootstrap (`.env`)

Copy `.env.example` to `.env`; real environment variables take precedence.

```bash
# --- HTTP server ---
PORT=4533

# --- Auth ---
# If unset, a random secret is generated at startup and persisted.
# AUTH_SECRET=
AUTH_REQUIRE_SETUP_TOKEN=false   # gate first-run admin behind a startup token (see note below)
# Optional: create the first admin from these instead of the setup UI (see note
# below). Both must be set together, or neither.
# ADMIN_USERNAME=
# ADMIN_PASSWORD=

# --- Database ---
DATABASE_DRIVER=sqlite
DATABASE_DSN=immerle.db
# For Postgres:
#   DATABASE_DRIVER=postgres
#   DATABASE_DSN=postgres://immerle:immerle@localhost:5432/immerle?sslmode=disable

# --- Logging ---
LOG_LEVEL=info     # debug | info | warn | error
LOG_FORMAT=text    # text | json

# --- Library ---
LIBRARY_PATHS=/music
LIBRARY_DATA_DIR=data
```

:::info[First-run admin setup]

`AUTH_REQUIRE_SETUP_TOKEN` defaults to `false` on purpose. The first time the
server starts with no users, `POST /api/v1/setup` lets you create the admin
account straight from the web UI — no token to copy out of the logs. This keeps
onboarding simple for non-technical, self-hosting users.

The setup endpoint **self-locks the moment any user exists**, so it can only be
used once. The only exposure window is between the instance first becoming
reachable on the network and you finishing setup: if someone reaches it before
you do, they could claim the admin account.

If your instance is exposed to the public internet before you've initialized it,
either set `AUTH_REQUIRE_SETUP_TOKEN=true` (the server then prints a one-time
token you must supply to create the admin) or keep the instance off the public
network until setup is complete.

For fully automated deployments (Docker, IaC) with no interactive setup step,
set `ADMIN_USERNAME`/`ADMIN_PASSWORD` instead: the server creates that admin
account at startup, before serving traffic, and skips the setup UI/token
entirely. Like the setup endpoint, this only ever applies while the server has
no users — safe to leave set permanently, it's a no-op on every later restart.

:::

## Runtime (admin API)

Runtime settings are managed via the admin API and persisted in
`data/configuration.yaml`:

| Area      | Endpoint                |
| --------- | ----------------------- |
| Settings  | `GET/POST /admin/settings`  |
| Providers | `GET/POST /admin/providers` |
| Cleanup   | `GET/POST /admin/cleanup`   |

Providers (including built-ins like Jamendo and their credentials) are **not**
set in `.env`. Jamendo, for instance, is seeded disabled with a
`{"params":{"client_id":"<token>"}}` config to fill in and enable from the admin
UI. See [On-demand catalog](./on-demand-providers.md) for how providers work,
or [Building a custom content provider](./developers/custom-provider.md) for
the config schema and the `/capabilities` contract used to add an HTTP
provider.

## LDAP authentication

LDAP is an optional, runtime-configured login path managed from the admin UI
(**Settings → LDAP**). It uses a direct **simple bind** — no service account, no
search:

| Field | Meaning |
| ----- | ------- |
| Enable LDAP | Master switch (off = local accounts only) |
| Server URL | Directory endpoint, e.g. `ldaps://ldap.example.com:636` |
| Bind DN template | DN built from the username via a single `%s`, e.g. `uid=%s,ou=people,dc=example,dc=com` |

How it works:

- **Local accounts are checked first**, then LDAP. So a local admin always
  works even if the directory is down.
- On the **first successful bind**, the LDAP user is provisioned a local account
  automatically (needed for playlists, scrobbles and devices). It has no usable
  local password — it can only ever authenticate through LDAP.
- A successful bind is **cached in memory for 5 minutes**, so chatty clients
  (notably Subsonic) don't hit the directory on every request. Disabling LDAP in
  the UI takes effect immediately; a password change is honored within 5 minutes.

:::warning Subsonic clients must use password auth with LDAP

LDAP only works with credential logins that carry the **password** — the
Immerle REST login (which then issues a device JWT) and Subsonic's **password
mode** (`p=` / `p=enc:`).

It **cannot** work with Subsonic **token auth** (`t=md5(password+salt)` + `s=`),
which is the default for most clients. Token auth requires the server to recompute
the hash from the stored plaintext password, but LDAP never exposes a password —
the directory validates it during the bind. This is an inherent LDAP limitation,
not specific to Immerle.

**If you use LDAP, configure your Subsonic clients to send the password**
(often labeled "plain password", "legacy auth", or "disable token auth"). Only
do this over **HTTPS**, since the password is sent on every request.

:::
