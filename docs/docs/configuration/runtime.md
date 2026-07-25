---
sidebar_position: 3
title: Runtime (admin API)
---

# Runtime (admin API)

Everything beyond [bootstrap](./bootstrap.md): providers, scan cadence,
transcoding, CORS, device-token TTL, federation… Managed by an admin via the
API and persisted in `data/configuration.yaml`. No restart needed.

| Area      | Endpoint                |
| --------- | ----------------------- |
| Settings  | `GET/POST /admin/settings`  |
| Providers | `GET/POST /admin/providers` |
| Cleanup   | `GET/POST /admin/cleanup`   |

Providers (including built-ins like Jamendo and their credentials) are **not**
set in `.env`. Jamendo, for instance, is seeded disabled with a
`{"params":{"client_id":"<token>"}}` config to fill in and enable from the admin
UI. See [On-demand catalog](../features/on-demand-providers.md) for how
providers work, or
[Building a custom content provider](../developers/custom-provider.md) for
the config schema and the `/capabilities` contract used to add an HTTP
provider.
