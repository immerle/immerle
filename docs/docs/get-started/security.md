---
sidebar_position: 3
title: The 5 commandments of security
---

# The 5 commandments of security

You just got Immerle running in [Quick Start](./quick-start.md). Before you
connect a client from outside your home network, five things worth getting
right.

## 1. Prefer a VPN over a public port

Putting Immerle straight on the public internet **is possible**, it's just an
HTTP server, but it's not free: it means keeping up with security patches,
watching logs, hardening the box it runs on, and reacting fast if a
vulnerability shows up anywhere in the stack (Immerle, Postgres, Docker, the
host OS). Skip that upkeep and an open port is a real risk, not a theoretical
one, and it matters most if the machine is sitting in your home, on the same
network as everything else you own.

If you don't already do that kind of maintenance for other services, put
Immerle behind a VPN instead and don't open a port for it at all:

- **[Tailscale](https://tailscale.com/)** (recommended, easiest): install it
  on the machine running Immerle and on your other devices, join the same
  account (tailnet), and every device on it can reach
  `http://<machine-name>:4533` as if it were on your home network. No port
  forwarding, ever.
- **[Pangolin](https://github.com/fosrl/pangolin)**, when you want a real
  domain or need to share access with people outside your tailnet: a
  self-hosted reverse-proxy/tunnel that runs on a small VPS you control, with
  a lightweight agent on your Immerle machine connecting out to it. Still no
  inbound port on your home router, but more setup than Tailscale (you need
  that VPS), so reach for it only once "just give me a Tailscale link" isn't
  enough.

## 2. If you do expose it publicly, put a proxy in front of it

Never point the internet straight at Immerle's port. Put a TLS-terminating
reverse proxy (Traefik, Caddy, nginx…) in front instead, it gives you HTTPS
and HTTP/2, which several live features (Jam, cross-device sync) need to stay
responsive under a browser's per-origin connection limit. See
[Troubleshooting](../troubleshooting.md#pages-feel-slow--stall-while-a-jam-is-running)
for why that limit matters and how the proxy fixes it.

## 3. Change the default admin password, and close the setup window

The `docker-compose.yml` from Quick Start creates the admin account from
`ADMIN_USERNAME`/`ADMIN_PASSWORD`, change `change-me` before you start it. If
you'd rather use the interactive setup screen instead (leave those two
variables unset), be aware it **self-locks the moment any user exists**, but
until then anyone who reaches the server first can claim the admin account.
Keep the instance off the network until setup is done, or set
`AUTH_REQUIRE_SETUP_TOKEN=true`, see
[First-run admin setup](../configuration/bootstrap.md) for the details.

## 4. Keep it updated

`docker compose pull && docker compose up -d` picks up the latest Immerle
image; do the same for the `postgres` image periodically. Watch the
[releases page](https://github.com/immerle/immerle/releases) for anything
security-relevant, this is the single biggest factor in whether exposing
Immerle publicly (commandment 1) is actually safe for you.

Doing that by hand forever is how updates stop happening. A few self-hosted
options that automate it:

- **[Renovate](https://docs.renovatebot.com/)** watches your
  `docker-compose.yml` and opens a PR (or commits directly) whenever a newer
  image tag is out, self-hostable as a Docker image or GitHub Action, no
  external service required.
- **[Komodo](https://komo.do/)** and **[Portainer](https://www.portainer.io/)**
  manage and deploy your containers from a web UI, including pulling and
  redeploying updated images on a schedule.

Any of these still needs you to actually look at what changed once in a
while, don't turn on auto-deploy and never check the logs again.

## 5. Back up more than your music

Your library files matter, but so does `LIBRARY_DATA_DIR` (metadata, images,
on-demand cache), that's where your users, playlists, scrobbles and settings
actually live, not just in the database. The Quick Start compose file already
runs a `backup` container that `pg_dump`s Postgres daily and keeps the last 7
in the `immerle_backup` volume, don't also rely on copying database files
while the container is running. Copy `immerle_backup`, `immerle-data` and
`immerle-music` off the host on a schedule (a cron job running `docker run
--rm -v <volume>:/v -v $PWD/backups:/out alpine tar czf /out/<volume>.tgz -C
/v .` per volume is enough for a home instance).

## Next

- [Installation](../installation.md) and [Configuration](../configuration/index.md)
  cover every option in more depth once you're past the defaults above.
