# mila

Desktop client built with Go, Wails v2, and Svelte 5.

## Requirements

- Go 1.26+
- Node.js 20+ and npm
- [Wails CLI v2](https://wails.io/docs/gettingstarted/installation)
- WebView2 Runtime (Windows)

## Setup

Install frontend dependencies:

```sh
cd frontend
npm install
```

## Development

Run in live-reload mode from the repository root:

```sh
wails dev
```

Frontend-only typecheck and build:

```sh
cd frontend
npm run check
npm run build
```

## Build

Build the production binary into `build/bin`:

```sh
wails build
```

## Structure

```
.
├── main.go         Wails entry point and window configuration
├── internal/       Go packages (app bindings, logger, screen helpers)
├── frontend/       Svelte 5 + TypeScript + Vite UI
└── build/          Platform packaging assets and build output
```
