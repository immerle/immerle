---
sidebar_position: 2.5
title: Sonic analysis
---

# Sonic analysis

An optional feature (**Admin → Settings → Audio analysis**) that listens to
your library itself: Immerle decodes each track with ffmpeg and measures its
tempo, key, energy and timbre, locally, without any external service. Those
measures then power a track radio, endless queues, mood playlists, Auto-DJ,
new smart playlist rules and duplicate detection.

Everything is **off by default**. The analysis is the master switch; each
feature built on it has its own toggle on top, so you turn on only what you
want.

| Toggle | What it does |
| ------ | ------------ |
| **Analyze the library** | Runs the analysis in the background. Required by every toggle below |
| **Track radio** | "Start radio" on a track, album or artist; also answers `getSimilarSongs` for Subsonic clients |
| **Autoplay** | Extends a queue about to run out with similar tracks |
| **Mood playlists** | Publishes shared Calm, Focus, Melancholy, Energy, Party and Workout playlists |
| **Auto-DJ** | Plays a playlist, or reorders the queue, for smooth transitions |
| **Sonic duplicates** | Lists tracks that sound identical in **Admin → Library** |

Smart playlist rules on BPM, energy, danceability and key are available as
soon as the analysis runs; they need no toggle of their own.

## The analysis

- **What it needs**: ffmpeg, the same binary used for transcoding (it ships in
  the Docker image). Without it the section shows a warning and nothing runs.
- **What it decodes**: three 30-second excerpts per track (at 20 %, 50 % and
  80 % of its length), downmixed to mono. The whole file is never read and no
  audio is stored.
- **What it measures**:
  - **tempo** (BPM), from the rhythm of the onsets;
  - **key** and **mode** (major/minor), also given in
    [Camelot](https://mixedinkey.com/camelot-wheel/) notation (`8A`, `9B`…);
  - **energy** (0 to 1, from loudness and brightness), **danceability**
    (how clear the beat is) and **brightness**;
  - a **sonic fingerprint** of 34 values (the measures above plus the
    timbre) used to compare tracks.
- **Cost**: well under a second per track on a desktop CPU, a few seconds on a
  Raspberry Pi. The first pass over a large library takes hours there; it
  runs in the background, one track at a time by default (raise **Tracks
  analyzed in parallel** on a bigger machine), pauses during library scans
  and resumes after a restart. Progress shows in the same settings section.
- **Storage**: about 200 bytes per track (around 20 MB for 100,000 tracks).
- **When it re-runs**: a track is analyzed again when its file changes, or
  when an Immerle update improves the analysis. A file that can't be analyzed
  is counted as failed and not retried until one of those happens.

Tracks not downloaded yet (on-demand results) have nothing to decode: they
join the analysis once they're in your library.

## Track radio

"Start radio" from a track's menu, or the radio button on an album or artist
page, plays tracks that sound like it: the closest matches in the library, at
most two per artist, with a little randomness so two radios from the same
track differ. From a track, that track plays first.

Subsonic clients get it too: `getSimilarSongs` / `getSimilarSongs2` (the
"instant mix" of Symfonium, DSub and others) use the sonic radio once the
seed is analyzed, and fall back to tracks of the same genre otherwise.

## Autoplay

When fewer than three tracks are left in the queue, the player adds 20 that
sound like the last five played, so the music never stops. It stays out of
the way on internet radio, while following a Jam, with repeat on, and on a
device that only mirrors another one. Each listener can turn it off under
**Settings** in the app once the admin has enabled it.

## Mood playlists

Six shared, public playlists, refreshed daily with the other
[auto-generated playlists](./discovery.md#auto-generated-playlists) and shown
in the Home screen's "made for you" row:

| Playlist | Picks |
| -------- | ----- |
| **Calm** | the least energetic tracks, not too danceable |
| **Focus** | mid energy, little pulse |
| **Melancholy** | minor key, low energy |
| **Energy** | the most energetic tracks, 115 BPM and up |
| **Party** | very danceable, energetic, 100 to 135 BPM |
| **Workout** | energetic, 120 to 160 BPM |

Energy and danceability thresholds are relative to your own library (Calm is
the calmest part of *your* music), each playlist holds up to 100 tracks with
at most three per artist, and a mood with fewer than 15 matching tracks gets
no playlist. Turning the toggle off removes them.

## Auto-DJ

Orders tracks so each one flows into the next: close tempos (a half or
double tempo counts as close), compatible keys on the Camelot wheel, and no
sudden energy jumps. Two ways to use it:

- on a playlist, the **Auto-DJ** button next to Play plays it in that order
  (the playlist itself is unchanged);
- in the queue, **Auto-DJ** reorders everything after the current track,
  which keeps playing.

Tracks not analyzed yet are kept at the end, in their original order. It
pairs well with the [crossfade](../clients/index.md#crossfade).

## Smart playlist rules

Smart playlists gain **Energy** and **Danceability** (0 to 100), **Key
(Camelot)** and **Mode** rules, plus energy and danceability sorts. **BPM**
now falls back on the analyzed tempo when a file has no BPM tag (the tag
wins when present). A track not analyzed yet matches none of these rules.

## Sonic duplicates

**Admin → Library → Probable duplicates** lists groups of tracks that sound
identical and last about as long: the same recording under other tags, or in
another format. In each group the best copy (highest bitrate, then biggest
file) is marked to keep; the others can be deleted from there. Nothing is
deleted automatically. A different version of a song (radio edit, live) is
not a duplicate.

## Limits

- **Tempo** can read as half or double the real one (85 instead of 170).
  Auto-DJ treats those as matching, but a BPM smart rule may miss the track.
- **Energy** is an estimate from loudness and brightness, not a model trained
  on listening tests: a quiet but intense track can read as calm.
- Voice and instruments aren't detected, so there's no "instrumental" mood;
  Focus is about steadiness, not the absence of vocals.
