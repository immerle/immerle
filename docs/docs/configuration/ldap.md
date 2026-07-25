---
sidebar_position: 4
title: LDAP authentication
---

# LDAP authentication

LDAP is an optional, runtime-configured login path managed from the admin UI
(**Settings → LDAP**). It uses a direct **simple bind**, no service account, no
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
  local password: it can only ever authenticate through LDAP.
- A successful bind is **cached in memory for 5 minutes**, so chatty clients
  (notably Subsonic) don't hit the directory on every request. Disabling LDAP in
  the UI takes effect immediately; a password change is honored within 5 minutes.

:::warning Subsonic clients must use password auth with LDAP

LDAP only works with credential logins that carry the **password**: the
Immerle REST login (which then issues a device JWT) and Subsonic's **password
mode** (`p=` / `p=enc:`).

It **cannot** work with Subsonic **token auth** (`t=md5(password+salt)` + `s=`),
which is the default for most clients. Token auth requires the server to recompute
the hash from the stored plaintext password, but LDAP never exposes a password:
the directory validates it during the bind. This is an inherent LDAP limitation,
not specific to Immerle.

**If you use LDAP, configure your Subsonic clients to send the password**
(often labeled "plain password", "legacy auth", or "disable token auth"). Only
do this over **HTTPS**, since the password is sent on every request.

:::
