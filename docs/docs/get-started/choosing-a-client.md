---
sidebar_position: 2
title: Choosing a client
---

import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

# Choosing a client

The server from [Quick Start](./quick-start.md) is running, now pick how you
want to listen. See [Compare clients](../clients/compare.md) for the full
feature/platform breakdown.

## Immerle app (recommended)

**Web** needs nothing, it's already at `http://<host>:4533`. For everything
else, grab the installer for your platform from the
[latest release](https://github.com/immerle/immerle/releases/latest):

<Tabs groupId="os">
<TabItem value="mac" label="macOS">

```bash
# Apple Silicon
curl -L -o Immerle.dmg https://github.com/immerle/immerle/releases/latest/download/Immerle-mac-arm64.dmg && open Immerle.dmg
# Intel
curl -L -o Immerle.dmg https://github.com/immerle/immerle/releases/latest/download/Immerle-mac-x64.dmg && open Immerle.dmg
```

</TabItem>
<TabItem value="windows" label="Windows">

Download
[Immerle-win-x64.exe](https://github.com/immerle/immerle/releases/latest/download/Immerle-win-x64.exe)
and run it.

</TabItem>
<TabItem value="linux" label="Linux">

```bash
# Debian/Ubuntu
curl -L -o immerle.deb https://github.com/immerle/immerle/releases/latest/download/Immerle-linux-amd64.deb && sudo apt install ./immerle.deb
# Any distro (AppImage)
curl -L -o Immerle.AppImage https://github.com/immerle/immerle/releases/latest/download/Immerle-linux-x86_64.AppImage && chmod +x Immerle.AppImage && ./Immerle.AppImage
```

</TabItem>
<TabItem value="ios" label="iOS">

Not published to an app store yet, build it yourself, see
[Connecting clients](../clients/index.md#the-immerle-app-recommended).

</TabItem>
<TabItem value="android" label="Android">

Not published to an app store yet, build it yourself, see
[Connecting clients](../clients/index.md#the-immerle-app-recommended).

</TabItem>
</Tabs>

It's capability-aware and syncs playback across devices, see
[Connecting clients](../clients/index.md#the-immerle-app-recommended) for the
details.

## `iml` (terminal client)

A single small binary, no GUI, barely touches memory or CPU, good for having
music running in the background without competing with a game.

<Tabs groupId="os">
<TabItem value="mac" label="macOS">

```bash
# Apple Silicon
curl -L https://github.com/immerle/immerle/releases/latest/download/iml-darwin-arm64.tar.gz | tar xz && sudo install -m 0755 iml* /usr/local/bin/iml
# Intel
curl -L https://github.com/immerle/immerle/releases/latest/download/iml-darwin-amd64.tar.gz | tar xz && sudo install -m 0755 iml* /usr/local/bin/iml
```

</TabItem>
<TabItem value="windows" label="Windows">

Download
[iml-windows-amd64.zip](https://github.com/immerle/immerle/releases/latest/download/iml-windows-amd64.zip)
(or the [arm64 build](https://github.com/immerle/immerle/releases/latest/download/iml-windows-arm64.zip)),
extract it, and run `iml.exe` from that folder.

</TabItem>
<TabItem value="linux" label="Linux">

```bash
# amd64
curl -L https://github.com/immerle/immerle/releases/latest/download/iml-linux-amd64.tar.gz | tar xz && sudo install -m 0755 iml* /usr/local/bin/iml
# arm64
curl -L https://github.com/immerle/immerle/releases/latest/download/iml-linux-arm64.tar.gz | tar xz && sudo install -m 0755 iml* /usr/local/bin/iml
```

</TabItem>
</Tabs>

Run `iml`, enter your server URL and credentials once, they're saved for next
time. See [Connecting clients](../clients/index.md#iml-terminal-client) for
the keybindings.

## A Subsonic/OpenSubsonic client you already use

Point it at `http://<host>:4533` with your Immerle credentials. Works as a
fallback: browsing, search, streaming, transcoding, playlists, scrobbling,
but none of the Immerle-only features. See
[Connecting clients](../clients/index.md#subsonic--opensubsonic-clients) for
tested clients (Supersonic, Symfonium, DSub).

## Next

- [The 5 commandments of security](./security.md): read before you connect
  from outside your home network.
