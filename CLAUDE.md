# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A Go web server that automates ripping movies from optical discs. It watches for a disc being
inserted (via udev), shells out to `makemkv` to identify and rip the main title, looks up the
movie's metadata on OMDb, and ingests the finished `.mkv` file to one or more remote/local
targets (e.g. a Plex library over SSH). A small Echo + templ web UI shows drive status, in-progress
rips, and rip history.

## Commands

```sh
make build   # generates templ files, then builds ./cmd/server -> build/mkv-ripper
make test    # generates templ files, then runs go test ./...
make clean   # removes the binary and generated *_templ.go files
```

- Run a single package's tests: `go test ./handler/...`
- Run a single test: `go test ./handler/ -run TestGetIndex`
- `*_templ.go` files are gitignored and generated from `*.templ` via `templ generate`
  (invoked through the `go tool github.com/a-h/templ/cmd/templ` tool declared in `go.mod`).
  They must exist to compile — `make build`/`make test` regenerate them automatically; if
  running `go build`/`go test` directly instead of through `make`, generate them first with
  `go tool github.com/a-h/templ/cmd/templ generate`.
- The server needs a `mkv-ripper.toml` config file in the working directory (see below) and an
  OMDb API key to start — `main()` calls `log.Fatalln` immediately without one.

## Configuration

`mkv-ripper.toml` (gitignored, real deployments keep their own) configures:
- `port` — HTTP listen port
- `data`/`log`/`rip` — directories for the sqlite DB, log file, and working rip output
- `[omdb] apikey` — required OMDb API key
- `[[targets]]` — one or more ingest destinations, each with `scheme` (`file`/empty or `ssh`),
  `host`, `path`, `shafile` (path to a sha256sums-style file to append to), and `useMovieDir`
  (whether to nest the file under a `Title (Year)/` directory)
- `[[libraries]]` — optional media servers to manage collections on after ingest, each with
  `type` (only `jellyfin` so far), `url`, `token` (API key), and optional `user` (defaults to
  the first admin). Without any, the collections UI renders nothing.
- `[tmdb] apikey` — optional TMDb v3 API key or v4 read access token (`eyJ...`, sent as a
  Bearer header). Lets the collections picker identify a movie's release series before it's
  ripped; without it, series are only known once Jellyfin has the movie.

State lives in a single sqlite database (`<data>/mkv-ripper.db`, driver: `modernc.org/sqlite`,
pure Go, no cgo). Older deployments may still have legacy `discs.json`/`workflows.json` files;
`cmd/server/migrate.go` imports them into sqlite once on startup and renames them to `.bak`.

## Architecture

The system is a small pipeline of packages, each owning one stage, wired together in
`cmd/server/main.go`:

1. **`drive`** — `DriveManager` listens for udev disc-insert/eject events (`udevlistener.go`)
   and wraps `makemkv` CLI calls to read disc info and rip a title to a file
   (`drivemanager.go`). `DiscDatabase` (`drivedb.go` interface, `sqlitedb.go` impl) caches
   `makemkv.DiscInfo` per disc UUID so a previously-seen disc doesn't need to be rescanned.

2. **`workflow`** — `WorkflowManager` (`manager.go`) is the state machine driving one
   `model.Workflow` per (disc, title) pair through `Start` -> `Ingest`. It orchestrates calls
   into `drive` (rip) and `ingest` (deliver), and persists workflow state after every
   transition via a pluggable `persistFn` (`sqlite_manager.go` is the current backend; a JSON
   backend still exists in `manager.go` for reference/migration but is no longer wired up in
   `main`).

3. **`ingest`** — `Ingester` delivers a ripped `MkvFile` to a configured `Target`. Resolved by
   URL scheme in `NewIngester` (`ingester.go`): `file`/empty -> `LocalIngester`, `ssh` ->
   `SshIngester`. Both copy into a `.input` staging dir, verify the file's sha256 against the
   one recorded when it was ripped, append to a shasums file, then move it into place.

4. **`handler`** — Echo HTTP handlers, one per view: `index.go` (dashboard + history),
   `movies.go`, `drive.go`, `workflow.go`, `omdb.go`. `poster_cache.go` memoizes OMDb poster
   lookups (by IMDB id, with a title/year fallback for movies ripped before their IMDB id was
   recorded) since the history page can render 100+ movies.

5. **`library`** — Media-server integration for collections. `Library` (`library.go`) is a
   per-server interface working on collection *names*; `Jellyfin` (`jellyfin.go`) implements
   it over the REST API. `Syncer` (`syncer.go`) fans operations out across all configured
   libraries; `main` registers `Syncer.AfterIngest` via `WorkflowManager.OnIngested` to scan,
   poll until the new movie appears, and add it to `Workflow.Collections`. Jellyfin quirks
   handled there: `AnyProviderIdEquals` is ignored (IMDb ids are matched client side, with a
   full-library fallback since OMDb and TMDb titles differ), `ParentId`+`Ids` ignores
   `ParentId`, and collection members only list with a `userId`.
   `handler/collections.go` serves the picker on the workflow and movie pages.

6. **`view`** — `templ` templates (`.templ` source, compiled to `_templ.go`), one subpackage
   per page/section (`index`, `movie`, `drive`, `workflow`, `omdb`, plus shared `layout`).

7. **`model`** — Shared domain types: `Workflow` (one disc/title rip+ingest attempt, with
   `WorkflowStatus` moving through `Start -> Ripping -> Pending -> Importing -> Done`, or
   `Error`), `MkvFile`, and `MovieGroup` (groups workflows for the same movie, e.g. separate 4K
   and 1080p rips of the same title, into one history entry).

8. **`util`** — Cross-cutting helpers: `movie.go` (OMDb title lookup with punctuation-stripping
   retry, and heuristics for guessing a disc's main feature title from `makemkv` disc info —
   e.g. recognizing Disney/Ghibli menu-file conventions), `sha.go` (sha256 of a file), `url.go`.

### Control flow on disc insert

`main.go`'s `handleDisc` is the entry point triggered by `DriveManager`'s udev callback: look up
cached disc info (or fetch it fresh via `makemkv`), guess the main title and a name from it
(`util.GuessMainTitle`/`GuessName`), create a `Workflow`, kick off an async OMDb lookup to fill
in `Name`/`Year`/`ImdbId` in parallel with `wfman.Start` (which runs the actual rip), then once
ripping finishes `Start` hands off to `Ingest` (also async) to deliver the file and clean up.

On process startup, `main` also sweeps existing workflows: any left `Ripping` becomes `Error`
(rip was interrupted), any left `Importing` reverts to `Pending`, and any `Pending` workflow
with a resolved name/year is re-ingested.

### Notes for making changes

- Workflow status transitions and sqlite persistence are tightly coupled in
  `workflow/manager.go`/`sqlite_manager.go` — always go through `WorkflowManager.Save` (via
  `Start`/`Ingest`/`Clean`) rather than mutating a `Workflow` directly, since `Save` also stamps
  `UpdatedAt` and persists.
- Schema changes to the `workflows` table should follow the existing
  `addColumnIfMissing`-on-startup pattern in `sqlite_manager.go` rather than a separate
  migration mechanism.
- `handler` tests (`*_test.go`) exercise the pure grouping/sorting/filtering logic
  (`splitWorkflows`, `groupMovies`, comparators) directly rather than through HTTP, and construct
  `model.Workflow`/`model.MovieGroup` fixtures by hand — follow that pattern for new handler
  logic.
