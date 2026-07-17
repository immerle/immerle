---
sidebar_position: 11
title: Troubleshooting
---

# Troubleshooting

## Pages feel slow / stall while a Jam is running

**Symptom:** the app is noticeably laggy — pages take a while to load,
requests seem to queue up — specifically while you're hosting a
[Jam](./social.md) session (or otherwise have several live features active at
once: cross-device play-queue sync, a Jam session, Jam invite notifications).

**Cause:** each of those features keeps a **Server-Sent Events (SSE)**
connection open for as long as it's relevant. Browsers cap concurrent
connections to **~6 per origin** under **HTTP/1.1** — a limit enforced by the
browser, not by Immerle. If Immerle is served over plain HTTP (the default —
see below), a few always-open SSE streams plus normal page-load requests can
hit that ceiling, and everything else queues behind the SSE connections until
one frees up.

**Fix: put the instance behind a TLS-terminating reverse proxy** (Traefik,
Caddy, nginx…). Browsers negotiate **HTTP/2** automatically over TLS — a
single multiplexed connection per origin, with a per-connection concurrent
stream limit typically around 100 (server-configured), instead of 6 separate
connections. This removes the practical ceiling entirely; no Immerle-side
change is required.

A few things worth knowing:

- **The Immerle binary itself doesn't need TLS.** It keeps serving plain HTTP
  internally — the proxy terminates TLS with the browser and forwards to
  Immerle over a normal, un-encrypted connection on your network. You don't
  need `PORT`, `.env`, or any server code changed; just put the proxy in front
  and point it at Immerle's existing port.
- **The certificate is the proxy's job**, not Immerle's — Traefik and Caddy can
  both provision and renew Let's Encrypt certificates automatically with a
  handful of lines of config; nginx needs them provided (e.g. via `certbot`).
- **HTTP/2 is automatic once TLS is on** — Traefik and Caddy enable it by
  default on their HTTPS entrypoints, no extra flag needed.
- **HTTP/3 is not automatic** the same way. It runs over QUIC/UDP, which needs
  explicit proxy configuration (a dedicated UDP entrypoint) even though the
  proxy supports it — it doesn't just fall out of adding a certificate.
- If you serve the web UI and the API from **different origins** (e.g. the
  proxy on one domain, Immerle directly reachable on another), you'll also
  need to allow that origin in `corsAllowedOrigins` — see
  [Configuration](./configuration.md#runtime-admin-api).

If you can't add a reverse proxy right now, the lag is cosmetic — playback and
sync still work, requests just wait longer than they should behind the
connection limit.
