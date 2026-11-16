<div align="center">
  <br />
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset=".github/assets/logo-dark.png" />
    <source media="(prefers-color-scheme: light)" srcset=".github/assets/logo-light.png" />
    <img alt="Mila" src=".github/assets/logo-light.png" width="210" />
  </picture>
  <br /><br />
  <p><strong>Minimalist desktop anime client built with Go, Wails v2, and Svelte 5.</strong></p>
  <p>
    <img src="https://img.shields.io/badge/Go-1.26-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go 1.26" />
    <img src="https://img.shields.io/badge/Wails-v2-DF0000?style=flat-square" alt="Wails v2" />
    <img src="https://img.shields.io/badge/Svelte-5-FF3E00?style=flat-square&logo=svelte&logoColor=white" alt="Svelte 5" />
    <img src="https://img.shields.io/badge/Platform-Windows-0078D4?style=flat-square&logo=windows&logoColor=white" alt="Windows" />
  </p>
</div>

---

## Overview

**Mila** is a native Windows desktop anime streaming application powered by a Go host backend (`Wails v2` + WebView2) and a custom `Svelte 5` interface (`marlakit`). It combines multi-provider adaptive HLS/MP4 playback, built-in torrent streaming, local caching, and optional cloud profile sync.

## Features

- **Multi-Source Playback** — Unified resolution across AniLibria, Dream Cast, AnimeGO, AnimeVost, Kodik, HentaiMama, and HentaiGasm with automatic fallback and voice/source switching.
- **Built-in Torrent Streaming** — Concurrent search across AniLibria, Nyaa, and Sukebei with direct streaming and on-the-fly remuxing via `anacrolix/torrent`.
- **Custom Player (`hls.js`)** — Adaptive bitrate switching, OP/ED skip markers, native OS Picture-in-Picture, and local Go stream/M3U8 proxy (`/ext/stream`).
- **Local & Cloud Sync** — SQLite persistence paired with an optional Go cloud service (`cmd/cloud`) supporting Google OAuth and PostgreSQL storage.
- **Responsive Native Shell** — Custom frameless window controls, L1/L2 media & metadata caching, and keyboard/mouse navigation bindings.

## Tech Stack

| Layer | Technology |
| --- | --- |
| Desktop Shell | [Wails v2](https://wails.io/) (`v2.17.1`), WebView2 |
| Frontend | [Svelte 5](https://svelte.dev/), TypeScript, Vite |
| Video & Streaming | `hls.js`, HTML5 MP4 Range, Go `/ext/stream` proxy |
| Backend & Cloud | Go `1.26`, `net/http`, SQLite (`modernc.org/sqlite`), PostgreSQL (`pgx/v5`) |
| Torrents | [`anacrolix/torrent`](https://github.com/anacrolix/torrent) (`v1.62.0`) |
| Tooling | `go test`, `svelte-check`, `vitest` |

## Getting Started

### Prerequisites

- **Go** `1.26+`
- **Node.js** `20+` and `npm`
- **Wails CLI** `v2` (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`)
- **WebView2 Runtime** (Windows)

### Install & Run

```sh
# Install frontend dependencies
npm --prefix frontend install

# Run in development mode (hot reload)
wails dev
```

### Production Build

```sh
wails build
```

The compiled executable is written to `build/bin/mila.exe`.

### Tests & Checks

```sh
# Backend unit tests
go test -count=1 ./internal/torrent ./internal/mila ./internal/cloud ./cmd/cloud

# Frontend typecheck & unit tests
npm --prefix frontend run check
npm --prefix frontend test
```

## Project Structure

```text
.
├── main.go                 # Wails entry point and window lifecycle
├── wails.json              # Wails project configuration
├── cmd/
│   ├── cloud/              # Cloud auth & PostgreSQL sync server
│   └── server/             # Standalone HTTP host entry point
├── internal/
│   ├── mila/               # App bindings, stream proxy, provider extractors
│   ├── torrent/            # Nyaa/Sukebei/AniLibria search & torrent streaming
│   ├── cloud/              # OAuth & cloud API client
│   ├── shikimori/          # Catalog & metadata integration
│   └── cache/              # L1/L2 caching layer
├── frontend/
│   ├── src/
│   │   ├── api/            # Providers, unified bridge & cloud API
│   │   ├── app/            # Auth, router, bindings & state controllers
│   │   ├── marlakit/       # Reusable UI kit & HLS player
│   │   └── pages/          # Home, Anime, Library, News, Watch Together
│   └── tests/              # Vitest test suite
└── build/                  # Platform icons, manifests & installer assets
```
