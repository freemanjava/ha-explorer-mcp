# Supervisor App builder's build context

### 2026-08-24 · Does Supervisor's own local-App builder pass the repo root or the App's own folder as Docker build context?

**Kind:** world-discoverable

**Method:** Read the Supervisor source directly (no live Supervisor probe needed
— the builder's own published behavior settles this, per the task's DoD).
Fetched `supervisor/apps/build.py` and the relevant slice of `supervisor/apps/app.py`
from `home-assistant/supervisor` at `main`
(https://raw.githubusercontent.com/home-assistant/supervisor/main/supervisor/apps/build.py,
https://raw.githubusercontent.com/home-assistant/supervisor/main/supervisor/apps/app.py).
Local install note: current pinned Supervisor/Core tags used elsewhere in this
project's research (`docs/research/2026-08-23-supervisor-permissions.md`) are
2026.08.0 / 2026.8.3; `main` was read because Supervisor add-ons were renamed to
"Apps" in this codebase and the `main` branch is where that rename lives —
matches this project's own "App" terminology in `CLAUDE.md`.

**Found:** `AppBuild.get_docker_args()` (`supervisor/apps/build.py`) constructs:

```python
build_cmd = ["docker", "buildx", "build", ".", "--tag", image_tag,
             "--file", str(dockerfile_path), "--platform", ..., "--pull"]
...
app_extern_path = self.sys_config.local_to_extern_path(self.app.path_location)
mounts = [
    DockerMount(type=BIND, source=<docker.sock>, target="/var/run/docker.sock", read_only=False),
    DockerMount(type=BIND, source=app_extern_path.as_posix(), target="/addon", read_only=True),
]
return {"command": build_cmd, "mounts": mounts, "working_dir": PurePath("/addon")}
```

The build runs `docker buildx build .` with `working_dir=/addon`, and `/addon`
is a **read-only bind mount of `self.app.path_location` only** — nothing else
from the host is mounted in. `App.path_location` (`supervisor/apps/app.py`)
resolves to `Path(self.app_store.data[ATTR_LOCATION])`: the directory the store
scanner found containing that App's `config.yaml`/`config.json`, i.e. the App's
own folder — not the repository root, and not any parent of it. Dockerfile
selection (`get_dockerfile()`) also resolves relative to `self.app.path_location`
(`Dockerfile.<arch>` if present, else `Dockerfile`), and it is passed with
`--file` relative to that same context, not an absolute path outside it.

**Not established:** not confirmed against a live Supervisor build (owner did
not run one for this task — the source is unambiguous enough that a live
confirmation would be redundant per the DoD's "or from the builder's own
published behavior" clause). Did not check whether a `.dockerignore` or
multi-app-per-repo store layout changes which folder is scanned as
`path_location` — irrelevant here since this repo has exactly one App folder
(`addon/`).

**Means:** Supervisor's own builder passes **the App's own directory as build
context**, bind-mounted read-only and nothing else — the conventional layout,
not the repo root. `addon/Dockerfile`'s `COPY go.mod go.sum ./`, `COPY cmd/
cmd/`, `COPY internal/ internal/` (`addon/Dockerfile:9-11`) reference paths that
exist at the **repository root**, one level above `addon/`. Under a real
Supervisor local-App build, none of those paths exist inside the `/addon`
build context, so the build stage fails at the first `COPY` (`go.mod`) with a
"not found" error — this passes today only because `P0-02`'s manual check runs
`docker buildx build -f addon/Dockerfile .` from the repo root, which is not
what Supervisor does.

This confirms **F-8**'s suspicion and voids the manual-check DoD's implicit
assumption that context == repo root. It does not void the App otherwise — only
its build path.

**Layout change required (named per DoD, not applied here):** three options,
each viable, none applied in this task:

1. **Relocate the Dockerfile to the repo root** and make the repo root itself
   the App's folder (`config.yaml`, `build.yaml`, `apparmor.txt`, `rootfs/` all
   move to the repository root, alongside `go.mod`/`cmd/`/`internal/`). Cheapest
   change to the build, but conflicts with `CLAUDE.md`'s documented Module
   Layout (`addon/` as the dedicated App-packaging directory) and mixes App
   manifest files into the repo root.
2. **Vendor the Go source into `addon/` before Supervisor builds it** — a
   pre-build step (`make`/CI) copies or symlinks `go.mod`, `go.sum`, `cmd/`,
   `internal/` into `addon/` so they are inside the App's own folder when
   Supervisor scans and builds it. Keeps `CLAUDE.md`'s layout but adds a
   generated/synced tree that must never be hand-edited and must stay
   `.gitignore`d or regenerated in CI.
3. **Ship a prebuilt image and reference it from `config.yaml`** (`image:`
   field) instead of a local build — CI (`make release`) cross-builds
   `linux/arm64`/`linux/amd64`, pushes to a registry (e.g. GHCR), and
   Supervisor pulls rather than builds. Avoids the build-context problem
   entirely; trades local-build simplicity for a registry/publish step and
   version-tag discipline between `config.yaml`'s `image`/`version` and the
   published tag.

No recommendation is made here between the three — that choice is deferred to
whichever task implements the fix, planned separately per this task's DoD.
