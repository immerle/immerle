---
slug: /configuration
sidebar_position: 1
title: Configuration
---

# Configuration

Immerle splits configuration in two:

- **[Bootstrap settings](./bootstrap.md)**: a handful of values read from the
  environment (or a `.env` file) at startup. Changing them needs a
  **restart**.
- **[Runtime settings](./runtime.md)**: everything else (providers, scan
  cadence, transcoding, CORS, device-token TTL, federation…), managed by an
  admin via the API and stored in `data/configuration.yaml`. No restart
  needed.

LDAP login is a third, optional piece, configured entirely at runtime: see
[LDAP authentication](./ldap.md).
