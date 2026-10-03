# HTTP transport on the Pi (P8-09)

### 2026-10-03 · Does the App stay up and serve a real MCP client over the LAN, with the secret as the gate?

**Kind:** world-discoverable
**Method:** owner installed image `0.9.1`, then `0.9.2`, on the Pi (HAOS, aarch64), set `http_secret`, and ran `curl`
and Claude Code from a Mac on the LAN. Owner pasted the App log and `curl` output (secret redacted).
**Found:**
- `0.9.1` failed to start: log `cannot read the App options file`, status Error. Cause: `apparmor.txt` allowed no read
  of `/data/options.json`, so the open failed (`EACCES`). Unit tests cannot see this; only a live container does.
  Fixed in `0.9.2` with exactly `/data/options.json r,`.
- `0.9.2` log: `starting version=0.9.2 transport=http privacy_profile=mask`, then `listening addr=[::]:8790`.
- Host port empty on the Network tab: `curl` to `:8790` from the LAN → `Failed to connect` (000).
- Host port set, no `Authorization`: `POST /mcp` → **401**.
- With the bearer secret: `initialize` → result, `serverInfo` `ha-inspector-mcp 0.9.2`, protocol `2025-06-18`.
- Host port closed, `POST /mcp` from the Terminal & SSH App by container hostname → **401**: another App on the
  `hassio` network reaches the listener; only the secret stands in the way (D-08-6 as predicted).
- Claude Code (`claude mcp add --transport http … --header "Authorization: Bearer …"`) connected; owner reports the
  `tools/list` and one tool call completed.
**Not established:**
- The exact tool called and the tool count (owner reported "passed" only).
- `mcp-proxy` flags for stdio-only clients — not run.
**Means:** the F-37 defect is closed in practice: the App stays up and a real client works over the LAN. The AppArmor
miss is the one surprise.
