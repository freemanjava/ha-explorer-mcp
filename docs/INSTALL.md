# Installing the App on Home Assistant OS

The App ships as a **prebuilt multi-arch image pulled from a private GHCR
package** — Supervisor never builds it locally (see the App-distribution
decision in
[`development/phases/00-spike-foundations.md`](development/phases/00-spike-foundations.md)).
Because the package is private, installing it is **two separate steps**, not
one — pasting the repository URL alone is not enough
(`docs/research/2026-08-24-supervisor-private-registry-pull.md`).

## 1. Give Supervisor a GHCR credential

Before Supervisor can pull `ghcr.io/freemanjava/ha-explorer-mcp/...`, it needs
a registry credential — this is unrelated to adding the App's repository below
and must be done first.

1. In Home Assistant, go to **Settings → Add-ons → Apps → Registries** (a
   sibling page to the repositories list, not a field on the repository
   entry).
2. Add a registry entry for hostname `ghcr.io`, using a GitHub Personal Access
   Token with `read:packages` scope for the `ha-explorer-mcp` package as the
   password. The username can be any GitHub username with access to the
   package.
3. Save. The credential is stored in Supervisor's own `docker.json` and
   persists across Supervisor restarts.

## 2. Add the App repository and install

1. **Settings → Add-ons → Apps → Repositories**, add
   `https://github.com/freemanjava/ha-explorer-mcp`.
2. Find **HA Inspector MCP** in the store and click **Install**. Supervisor
   reads `addon/config.yaml`'s `image:` field, substitutes your device's
   architecture (`aarch64` on Raspberry Pi), and pulls that tag from GHCR using
   the credential from step 1 — it does not build anything.

## Troubleshooting

- **The install fails with a generic pull error, not a clear "auth failed"
  message.** This is expected if step 1 was skipped: Supervisor only raises a
  distinct, registry-named auth error when a credential was configured and the
  registry *rejected* it. A **missing** credential looks the same as any other
  failed pull (a typo'd image name, a nonexistent tag) — check that the
  Registries page actually has a `ghcr.io` entry before assuming the image
  itself is broken.
- **A version bump doesn't show up.** The image tag matches
  `addon/config.yaml`'s `version:` exactly (enforced by
  `TestAddonManifestImageIsPinnedToVersion` in `addon/config_test.go`) — the
  release workflow must have pushed a matching tag for the new version before
  Supervisor's store will offer it.
