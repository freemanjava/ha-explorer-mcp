# Supervisor pulling an App image from a private registry

### 2026-08-24 · Can Supervisor pull an App's `image:` from an authenticated (private) registry, and if so, where does the credential live?

**Kind:** world-discoverable

**Method:** Read the Supervisor source directly, at `main`
(`home-assistant/supervisor`), the same branch `P0-08` read for the "App" rename
— matches this project's own "App" terminology. Fetched and grepped:
`supervisor/docker/manager.py` (`DockerConfig`, `DockerAPI.pull_image`),
`supervisor/docker/interface.py` (`DockerInterface.install`,
`_get_credentials`), `supervisor/const.py` (`FILE_HASSIO_DOCKER`),
`supervisor/validate.py` (`SCHEMA_DOCKER_CONFIG`), `supervisor/api/docker.py`
(`APIDocker.registries`/`create_registry`/`remove_registry`), and, on the
`home-assistant/frontend` repo at `dev`,
`src/panels/config/apps/ha-config-apps.ts` and
`src/panels/config/apps/ha-config-apps-registries.ts`. All via raw GitHub
content fetches (`gh` CLI not available in this environment); GitHub's
authenticated code-search API was not available either, so the frontend UI path
was located by browsing `src/panels/config/apps/` directly rather than by
searching, which is slower but does not depend on documentation.

**Found:**

1. **Supervisor supports it.** `DockerAPI.pull_image()`
   (`supervisor/docker/manager.py`) takes an optional `auth: dict[str, str]`
   passed straight through to `aiodocker`'s pull call. `DockerInterface.install()`
   (`supervisor/docker/interface.py:274-402`) resolves credentials before every
   pull via `_get_credentials(image)`, which looks up
   `self.sys_docker.config.get_registry_for_image(image)` and, if a registry
   entry matches, hands back `{username, password, registry}` plus an image name
   qualified with the registry host (needed so aiodocker's `ServerAddress` in
   `X-Registry-Auth` matches what containerd expects).

2. **The credential lives in `/data/docker.json`** (`FILE_HASSIO_DOCKER =
   Path(SUPERVISOR_DATA, "docker.json")`, `supervisor/const.py`), loaded/saved
   by `DockerConfig(FileConfiguration)` (`supervisor/docker/manager.py:206-236`)
   against `SCHEMA_DOCKER_CONFIG` (`supervisor/validate.py:239-254`):
   ```python
   {"registries": {"<hostname>": {"username": str, "password": str}}}
   ```
   `<hostname>` must match `RE_REGISTRY = r"^([a-z0-9]+(-[a-z0-9]+)*\.)+[a-z]{2,}$"`
   — a bare domain like `ghcr.io`, not a URL. `get_registry_for_image()`
   (`manager.py:238-260`) matches an image's registry prefix against this map
   (falling back to `docker.io`/`hub.docker.com` for unprefixed/Hub images), so
   a `ghcr.io/<org>/<image>` App reference resolves against a `"ghcr.io"` entry.
   `SUPERVISOR_DATA` is Supervisor's persistent data directory (the same
   directory holding `addons.json`, `homeassistant.json`, etc.) — every other
   file in it is documented and relied on elsewhere in this project's own
   research (`docs/research/2026-08-23-supervisor-permissions.md`) to survive
   Supervisor restarts, so `docker.json` following the same `FileConfiguration`
   mechanism is not distinguished from them; not independently re-verified
   against an OS update in this session (see **Not established**).

3. **The credential is entered through a dedicated UI page, not the repository
   URL field.** `supervisor/api/docker.py` exposes `GET/POST /docker/registries`
   and `DELETE /docker/registries/{hostname}` (`APIDocker.registries` /
   `create_registry` / `remove_registry`, lines 104-131), which only ever
   read/write `self.sys_docker.config.registries` — the same `docker.json`.
   The frontend consumes this at `ha-config-apps-registries.ts`
   (`fetchHassioDockerRegistries` / `removeHassioDockerRegistry`, imported from
   `src/data/hassio/docker`), registered as the `registries` route of
   `HaConfigApps` (`src/panels/config/apps/ha-config-apps.ts:26-42`) alongside
   `installed` / `available` / `repositories` / `info`. So: **Settings → Add-ons
   (the "Apps" panel) → Registries**, a sibling page to the repositories list,
   not a field on the App/repository entry itself — a private package cannot be
   installed by pasting a repository URL alone, confirming the premise `P0-11`'s
   box already states.

4. **Failure mode is not uniformly a clear auth error — it depends on whether a
   credential is configured at all.** In `DockerInterface.install()`'s
   `except aiodocker.DockerError` handler (`interface.py:379-402`):
   ```python
   if err.status == HTTPStatus.UNAUTHORIZED and credentials:
       raise DockerRegistryAuthError(_LOGGER.error, registry=credentials[ATTR_REGISTRY]) from err
   ...
   raise DockerError(f"Can't install {image}:{version!s}: {err}", _LOGGER.error) from err
   ```
   The `and credentials` guard means a typed, registry-named
   `DockerRegistryAuthError` is only raised when a credential *was* looked up
   and used and the registry rejected it (wrong/expired password, or missing
   the app in an org token's scope) — i.e. an **expired or wrong credential
   surfaces as a clear, typed auth error naming the registry.** If **no**
   `docker.json` entry exists for the registry at all — the credential is
   simply **absent**, as it would be on first install before the owner visits
   Registries — `credentials` is `{}` (falsy), the guard fails, and any 401/403
   from the registry falls through to the generic `DockerError` branch. Its
   message embeds the raw `aiodocker`/registry error text (which for GHCR is
   typically a JSON body containing the word "denied" or "unauthorized"), but
   it is **not** a distinct exception type — at the type level it is
   indistinguishable from any other pull failure, including a genuinely
   nonexistent image or tag, and nothing here special-cases image-not-found for
   a private repository (a private repo with a valid image path returns the
   same 401/403/404-shaped denial as a typo'd path, by design of registries
   that don't leak the existence of private repos to unauthenticated callers).

**Not established:** did not run a live Supervisor pull (private or absent
credential) to confirm the exact wire-level status code GHCR returns for "app
private, no credential configured" versus "repository does not exist" — the
source shows Supervisor does not distinguish them either way, so a live
confirmation would only pin down GHCR's status code choice, not Supervisor's
behavior, which is what `P0-11` needs. Did not independently verify that
`docker.json` survives a Home Assistant OS update (only that it lives beside
other `SUPERVISOR_DATA` files this project already treats as durable); no
Supervisor source path documents data-partition persistence across an OS
image update specifically. Did not confirm GitHub org/user access-token scoping
requirements for a GHCR private package (that is a GHCR-side, not a
Supervisor-side, fact and outside this task's DoD).

**Means:** the private half of the App-distribution decision
(`phases/00-spike-foundations.md`, App distribution) is **implementable as
written** — Supervisor does support pulling an App's `image:` from an
authenticated private registry, the credential is entered once at
**Settings → Add-ons → Registries** (`ghcr.io` + a PAT/token as the password)
and stored in `/data/docker.json`, independent of the App's own repository
entry. `P0-11`'s install documentation must include this step explicitly, since
it is a separate action from adding the App's repository. `P0-11`'s DoD should
also not assume a clean, typed auth error is always what the owner sees on a
misconfigured pull: only a *present-but-wrong* credential gets the clear,
registry-named `DockerRegistryAuthError`; a *missing* one degrades to a generic
`DockerError` whose message is the raw registry response text, which is worth
one line in the install doc's troubleshooting section ("no registries
configured" looks the same as any other failed pull, not like an auth prompt).
