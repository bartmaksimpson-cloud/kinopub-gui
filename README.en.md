<p align="right"><a href="README.md">Русский</a> · <b>English</b></p>

# kino.watch downloader · GUI

**An app for downloading from [kino.watch](https://kino.watch), with a real interface.** Run it and it opens as a browser tab. Browse the catalog, preview a title right in the player, and download — movies and whole series, with every audio track and subtitle. While it downloads you see per-episode progress: speed and how much is left.

You sign in once, with a short device code. Nothing heavy under it — a single file, no Electron, no Node (a Go server with the React UI built in). Run it and you're set.

<p align="center">
  <img src="docs/screenshots/catalog.png" alt="kino.watch downloader" width="900">
</p>

<p align="center">
  <img alt="Go" src="https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white">
  <img alt="React" src="https://img.shields.io/badge/React-18-61DAFB?logo=react&logoColor=black">
  <img alt="TypeScript" src="https://img.shields.io/badge/TypeScript-5-3178C6?logo=typescript&logoColor=white">
  <img alt="Tailwind CSS" src="https://img.shields.io/badge/Tailwind-3-38BDF8?logo=tailwindcss&logoColor=white">
  <img alt="License" src="https://img.shields.io/badge/License-MIT-f59e0b">
</p>

---

## Highlights

- 🎬 **Browse the catalog** — search, tops, collections, genre and country filters, year and IMDb/Kinopoisk rating ranges, your watch history and "continue watching", plus a title page with plot, cast, ratings and the full season/episode tree.
- ▶️ **Built-in player** — preview any title inside the app before downloading. The stream goes through the app itself, so nothing needs setting up in the browser.
- 🗂️ **Pick a file, not a "quality"** — the list shows what the service actually offers: "2160p · HEVC", "2160p · H.264", "1080p · H.264", with per-episode coverage ("· 200/338"), because seasons are not always encoded alike.
- 📺 **Automatic fit for the player** — a frame taller than 2160 is scaled down, and 4K H.264 above 30 fps is halved (48 → 24). TV decoders refuse such files and fall back to stuttering software playback; anything that already plays is copied untouched.
- 🔊 **Audio picked properly** — the track list comes from the playlist rather than a summary, and each choice matches one exact track, so picking "MTV" no longer drags in the Ukrainian dub as well.
- 💬 **Subtitles are a choice** — the section is collapsed and nothing is selected by default: the service offers dozens of languages per episode. Forced tracks are labelled.
- ⚡ **Live state** — the card shows not just percentages but what is happening: downloading, muxing, re-encoding (with the resulting frame, codec, encoder and CPU threads in use), or moving to the output folder.
- ⏯️ **Live control** — pause, cancel and retry per episode; a pause actually stops downloading instead of swapping in the next episode. The queue survives a restart and resumes where it stopped.
- 📁 **Names and folders that make sense** — a film is saved as one file named after it, a serial as a folder with seasons and episode titles. Work files can live on another disk: on a hard drive that is most of the speed.
- 📚 **Library** — active downloads and everything already on disk on one page: sizes, resolutions, missing-file detection; open a finished file or reveal its folder.
- 🔐 **Sign in once** — device-code login; tokens are encrypted and bound to the machine. Local features (Library, Settings) work signed out.
- 🌍 **Two languages** — English and Russian, one click to switch (remembered between sessions).
- 📦 **One binary** — the UI is embedded; self-updates from GitHub releases.

## Screenshots

| Catalog browser | Title card |
| --- | --- |
| ![Catalog](docs/screenshots/catalog.png) | ![Title](docs/screenshots/title.png) |

| Live downloads | Settings |
| --- | --- |
| ![Downloads](docs/screenshots/downloads.png) | ![Settings](docs/screenshots/settings.png) |

---

## Requirements

- **ffmpeg** — the app uses it to merge video, audio and subtitles into one file. If it's installed (on your `PATH`), the app picks it up on its own; the Settings page shows a green or red check for `ffmpeg` and `ffprobe`.
  ```bash
  brew install ffmpeg          # macOS
  sudo apt install ffmpeg      # Debian/Ubuntu
  ```
  ```powershell
  winget install Gyan.FFmpeg   # Windows (or: choco install ffmpeg / scoop install ffmpeg)
  ```
  On Windows, make sure `ffmpeg.exe` and `ffprobe.exe` are on your `PATH` (the package managers above do this) — the Settings page confirms both are found.

  **Don't want to install it by hand?** If ffmpeg is missing, hit **Settings → System → Install ffmpeg** (the same button is on the Download page) — the app downloads a ready-made build for your system and uses it from then on. Nothing is written into the system, no admin rights needed.
- A browser (the app opens in whatever is your default).
- A kino.watch account with an active subscription — without it there's no catalog, no playback, no downloads.

## Install & run

**Prebuilt clients for every major platform** — grab one from the [releases page](https://github.com/bartmaksimpson-cloud/kinopub-gui/releases):

- 🍎 **macOS** — `.dmg` menu-bar app + standalone binaries, Apple Silicon (`arm64`) and Intel (`amd64`)
- 🪟 **Windows** — `x64` (`amd64`) executable, no console window and an embedded icon
- 🐧 **Linux** — `x64` (`amd64`, with a system-tray icon; also an `AppImage`) and `ARM64`
- 🤖 **Android** — `ARM64` (no native tray, web UI as usual; runs under Termux)

Same single binary everywhere — the React UI is embedded, so there is nothing else to install.

### Option A — download a release binary

Grab `kinopub-gui-*` for your platform from the [releases page](https://github.com/bartmaksimpson-cloud/kinopub-gui/releases), then run it:

```bash
chmod +x kinopub-gui-darwin-arm64
./kinopub-gui-darwin-arm64
# → opens http://127.0.0.1:8765 in your browser
```

On **macOS** you can instead grab the `.dmg` and drag **KinoPub** to Applications — it runs as a menu-bar app (no Dock icon; the status-bar item has *Open* and *Quit*).

The app isn't signed with an Apple certificate, so macOS blocks the first launch. Unblocking it is a one-time step:

1. Drag **KinoPub** from the disk image into **Applications** and launch it from there.
2. macOS will warn that it can't verify the app — dismiss the dialog (**Done**).
3. Open **System Settings → Privacy & Security**, scroll down to the message about KinoPub and click **Open Anyway**, then confirm with your password or Touch ID.

After that it opens normally. On older macOS (Sonoma and earlier) a right-click on the app → **Open** → **Open** is enough.

On **Windows**, download `kinopub-gui-windows-amd64.exe` and run it (double-click or from a terminal):

```powershell
.\kinopub-gui-windows-amd64.exe
# → opens http://127.0.0.1:8765 in your browser
```

> The binary is unsigned, so SmartScreen / Gatekeeper may warn on first run — on Windows choose **More info → Run anyway**; on macOS follow the steps above (**Privacy & Security → Open Anyway**). Windows Firewall may also prompt; the server only listens locally, so allowing private-network access is enough. Credentials are stored encrypted at `~/.config/kinopub/credentials.enc` (`%USERPROFILE%\.config\kinopub\credentials.enc` on Windows).

### Option B — build from source

You need Go 1.26+ and Node 20+ (only to build the UI; not at runtime).

```bash
git clone https://github.com/bartmaksimpson-cloud/kinopub-gui
cd kinopub-gui
make run          # builds the web UI, builds the GUI binary, and launches it
```

Or step by step:

```bash
make web          # build the React frontend into web/dist (embedded via go:embed)
make gui          # build the ./kinopub-gui binary
./kinopub-gui
```

> **Distribution:** grab the prebuilt binaries from the [releases page](https://github.com/bartmaksimpson-cloud/kinopub-gui/releases), or build with `make`. `web/dist` is committed, so a plain `go build ./cmd/kinopub-gui` also produces a runnable binary, and `make web` regenerates the UI.
>
> `go install` is not the way here: the Go module path is inherited from the original project (`github.com/ZioSHik/kinopub-gui`), so installing by it would fetch that code, not this.

### Flags

```
kinopub-gui [flags]
  -addr      address to listen on (default 127.0.0.1:8765;
             falls back to an ephemeral port if taken)
  -no-open   do not open the browser automatically
  -version   print version and exit
```

The server listens on your computer only (`127.0.0.1`) — it's not a public service, nothing outside can reach it. It also rejects requests that don't come from its own page, so a random site in your browser can't quietly poke at it.

### Updating

Prebuilt releases update themselves. **Settings → Software update** shows the
current version, and an **Update & restart** button when a newer GitHub release is
out. Hit it and the app downloads the new build for your system, checks its
checksum, replaces itself in place and restarts; your open browser tab reconnects
on its own. (Builds from source are tagged `dev` and don't self-update — rebuild
with `make`.)

---

## Using it

### 1. Sign in

Local features — **Library, Settings, the folder picker** — work without signing in. The catalog, search, the in-app player and downloads need an account.

Click **Sign in** (top-right or in the sidebar) and:

1. The app shows a short **device code** and a link (`kino.watch/device`).
2. Open that link in any browser where you're logged into kino.watch and enter the code.
3. Confirm — the app detects it within a couple of seconds and you're in.

The device shows up in your kino.watch account's device list as `kinopub-gui (your-hostname)`. Tokens are stored encrypted, tied to your computer, and kept at `~/.config/kinopub/credentials.enc`. Sign out any time from Settings.

> **kino.watch is often unavailable without a VPN.** If sign-in, the catalog or downloads hang or time out, enable a VPN or set a proxy (Settings → Proxy, or per-download in Advanced options). The UI shows a reminder and detects timeouts.

### 2. Find something

Open **Catalog** to search and browse. Filter by type, genre, country, year range and IMDb/Kinopoisk rating; browse tops and collections; or jump back into your **history** and **continue-watching** rows. Open a title to see its details, ratings, available voiceovers and the full season/episode tree — and hit ▶ to **preview it in the built-in player** before downloading.

You can also paste a kino.watch link directly on the **Download** page if you already have one.

### 3. Download

From a title's detail view (or the Download page), tick the seasons/episodes you want, pick a quality, and hit **Start download**. Progress shows up in the **Offline library**, in a "Downloading" section above what's on disk — overall, per-episode, and per track, with speed and ETA. Any episode can be paused, canceled or bumped ahead without touching its siblings; the queue survives an app restart and resumes where it stopped.

An **Advanced options** panel covers the fine print: container (MKV / MP4), proxy (HTTP/HTTPS/SOCKS5), a *Force re-download* toggle, verbose logs, and an extra-ffmpeg-args field. It's pre-filled from your Settings, so most of the time you can leave it alone.

### 4. Audio tracks

You pick dubs/voiceovers right where you start the download:

- **From a title's page** in the catalog — under **Voiceover**, tick the tracks you want to keep (with *Select all* / *Deselect all*). Your choice is remembered and pre-applied on the next titles; if your last voiceover isn't available here, the app prompts you to pick another.
- **When downloading from a direct link** (the Download page), the picker pops up as a timed modal the moment the download starts: tick the tracks, *Only this* to keep one, or *Keep all* to take everything (also what the timer does on expiry).

The track list comes from the playlist itself rather than the title summary, and each tick matches exactly one track. The choice is matched by name and language rather than position, because track numbering differs between episodes. When the picked dub is missing from an episode, the engine takes another one in the same language and flags that episode as "voiceover substituted".

**Subtitles** sit next to it in their own section. It is collapsed and nothing is selected by default: the service offers dozens of languages per episode. Forced tracks — signs and foreign lines only — are labelled.

### 5. Library

The **Library** scans your output folders and shows everything you've downloaded — posters, sizes, and flags for files that have gone missing on disk. Live downloads sit on the same page, above what's on disk. Open or reveal any file straight from the list; an episode or a whole title you no longer need can be deleted from disk right there.

### 6. Settings

Defaults for new downloads, kino.watch sign-in, the ffmpeg installer and app updates. Stored in `~/.config/kinopub/gui.json`.

**Two folders, and they are for different things:**

- **Where finished files go** — the finished film or episode lands here; this is the folder your player or NAS points at.
- **Where work files are kept while downloading** — segments, joined tracks and the file ffmpeg is writing. All of it is deleted once the episode is done. Empty means they sit next to the finished file. On a hard drive, putting them on **another** disk is most of the speed: otherwise one head reads and writes at the same time. Needs roughly three times the size of an episode.

A path can be typed in — that is how a network folder is reached (`\\192.168.1.174\Video`, `/Volumes/NAS`) — and the folder is checked for write access before it is saved, so a read-only share shows up immediately rather than after gigabytes.

**Player limits** (on by default): maximum frame height 2160 and maximum 30 fps for 4K. A file that fits is copied without re-encoding — real 4K stays untouched. Re-encoding takes every core between 00:00 and 09:00 and leaves one free the rest of the day, so the machine stays usable.

There is no speed to tune — the app works it out itself. The structure is fixed: one title at a time (the rest wait in a queue you can reorder) and two episodes in parallel inside it. More doesn't go faster — the link is already busy — it just splits it and multiplies connections to the CDN.

How many segments are fetched at once, though, is tuned to your link at runtime: the downloader measures real throughput in 4-second windows and raises concurrency while it keeps improving; on a plateau it settles, and on a drop it steps back. Every half minute it probes again, because links change under you (Wi-Fi to Ethernet, a video call ending). The ceiling is 16, the floor is one slot per track so audio always downloads alongside video. If the CDN answers 429 or 503, concurrency is halved immediately and the pause comes from the `Retry-After` header — exactly what the server asked for, not a number we invented. On top of that, network failures are retried with growing backoff (1→2→4→8→16s).

---

## How it works

```
┌──────────────────────────────┐        SSE (live progress)        ┌───────────────────────┐
│  React + TS + Tailwind UI     │ ◀───────────────────────────────── │  Go HTTP server       │
│  (embedded via go:embed)      │ ──── REST (commands) ────────────▶ │  internal/gui         │
└──────────────────────────────┘                                    └─────┬───────────┬─────┘
                                                                          │ drives    │ API
                                                          ┌───────────────▼──┐   ┌────▼──────────────┐
                                                          │ kinopub engine    │   │ kino.watch API      │
                                                          │ internal/app +    │   │ services/kinopubapi│
                                                          │ services (HLS,    │   │ (device login,    │
                                                          │ downloader, …)    │   │ discovery, stream)│
                                                          └───────────────────┘   └───────────────────┘
```

The server doesn't run the engine as a separate process — it works with it directly, in one program: download progress streams to the browser live, the audio picker pops up and holds the download until you answer, and the engine's log shows up in each job's log view.

Catalog and playback go through `internal/services/kinopubapi`, a small client for the kino.watch API: it keeps you signed in and refreshes the tokens on its own. The player gets video through `/api/hls`, a proxy inside the app itself; every link is signed, so it can't be reused as someone's open proxy.

### Project layout

```
cmd/
  kinopub-gui/      GUI server entrypoint (embeds the UI, opens the browser, macOS/Windows tray)
internal/
  app/kinopub/      engine composition root (App.Run)
  domain/           ports & models
  services/
    kinopubapi/     kino.watch API client (device login, discovery, stream resolution)
    downloader/     HLS + file download, ffmpeg muxing
    hlsdownloader/  HLS manifest parsing & segment download
    statestore/     per-series .kinopub-state.json
    …               outputlayout, scheduler, progress, proxyprovider
  gui/              REST + SSE server, job manager, discovery, HLS player proxy, reporter/chooser
  lib/              credstore (encrypted creds), httpx (uTLS), logx, audiomenu, …
web/                React + Vite + Tailwind frontend
  dist/             built UI, embedded into the binary (go:embed)
```

## Development

```bash
# Terminal 1 — run the Go server (serves the embedded UI + API)
make gui && ./kinopub-gui

# Terminal 2 — hot-reloading frontend with API proxy to :8765
make dev            # → http://localhost:5173
```

`make vet` runs `go vet`, `make test` runs the test suite. CI builds the UI, vets, and runs the suite (including the race detector) on Linux, Windows and macOS.

## Credits

This repository is a fork of **[ZioSHik/kinopub-gui](https://github.com/ZioSHik/kinopub-gui)**; releases and self-updates come from here, [bartmaksimpson-cloud/kinopub-gui](https://github.com/bartmaksimpson-cloud/kinopub-gui).

- The download engine and the hard parts it grew from (HLS, retries, encrypted creds): **[niazlv/kinopub-downloader](https://github.com/niazlv/kinopub-downloader)**.
- The web interface, the catalog/player integration, and the packaging (`cmd/kinopub-gui`, `internal/gui`, `internal/services/kinopubapi`, `web/`): the upstream project.
- Codec choice and player fitting, subtitle selection, the work folder, file naming and the live download state: this fork.

## License

MIT — see [LICENSE](LICENSE). The upstream engine is MIT-licensed; this repository preserves that license and adds the GUI under the same terms.
