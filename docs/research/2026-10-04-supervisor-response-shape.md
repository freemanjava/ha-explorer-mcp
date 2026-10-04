# Supervisor response shape vs. what the mappers assume (F-45)

### 2026-10-04 · Why does `get_system_health` answer all-empty on the Pi, with no unsupported marker?

**Kind:** world-discoverable — what the real Supervisor returns, and what this code does with it, are both observable; the owner cannot answer it.
**Method:** read `internal/ha/supervisor.go` (`get`, the `Supervisor*` readers) and `internal/ha/mapping.go` (`MapCoreInfo`, `MapOSInfo`, `MapHostDisk`, `MapResolutionInfo`, `MapSupervisorInfo`); read the Supervisor tests' bodies (`supervisor_test.go`, `supervisor_health_test.go`); listed `test/fixtures/`; searched `docs/research/2026-08-23-supervisor-permissions.md` for a recorded raw response; live calls to `get_system_health` and `list_apps` on the Pi (App `0.9.2`, Core `2026.9.4`), read through the App's HTTP port.
**Found:**
- `get` returns the response body as-is; each mapper `json.Unmarshal`s it into a flat struct (`coreInfoWire{homeassistant, supervisor, hostname, …}`). Nothing unwraps an envelope, and `encoding/json` ignores unknown keys, so a body with none of the expected top-level keys decodes to the zero value with a nil error. `systemHealth` therefore never reaches `markUnsupported`.
- Every Supervisor test body is hand-written and flat (`{"supervisor":"2026.08.0","homeassistant":"2026.8.3",…}`). `test/fixtures/` has no Supervisor payload, and the research doc records no raw Supervisor response. The flat shape was never observed.
- Live `get_system_health`: 1044 ms, audited `success`, all fields empty/zero, `Unsupported:false`. Live `list_apps`: `Items:[]`, `Unsupported:false`, on an installation that runs several Apps. Two routes (`/info`, `/supervisor/info`), two mappers, same blank result.
- Both blanks are what a `{"result":"ok","data":{…}}` envelope would produce under these mappers. The Supervisor API documents that envelope; this repo never checked it.
**Not established:** the raw body the Pi's Supervisor returns — no token reaches the agent, so it was not read. The envelope is the best-supported cause, not an observation. Also not established: whether any other mapper (`MapAddonStats`, `MapResolutionInfo`) would also blank; by the same reading they would.
**Means:** F-45 is a defect, not a configuration gap, and it is wider than `get_system_health`: every Supervisor-backed field is blank on the Pi, and `list_apps` reports "none installed". The tests cannot catch it because their fixtures share the mapper's assumption. A fix should start from a captured real response committed as a fixture, and the mappers should fail loudly on a body with none of the expected keys (the code's own P1-08 rationale says "fails loudly", and the zero-value path defeats it). F-47 (`get_automation_traces`) goes through the Core WebSocket, not Supervisor, so this does **not** explain it.

### 2026-10-04 · What does the Pi's Supervisor actually return for `/info`? (owner, SSH App)

**Kind:** world-discoverable
**Method:** owner ran `ha info --raw-json | jq 'keys, (.data | keys)'` in the Terminal & SSH App and pasted a screenshot of the key names (no values).
**Found:** top-level keys are exactly `data` and `result`. Under `data`: `arch`, `channel`, `docker`, `features`, `hassos`, `homeassistant`, `hostname`, `logging`, `machine`, `machine_id`, `operating_system`, `state`, `supervisor`, `supported`, `supported_arch`, `timezone`. Every field `coreInfoWire` reads is under `data`, none at the top level.
**Not established:** this is the `ha` CLI's raw output of `/info`, not the App's own HTTP request, though the CLI passes the Supervisor's body through. The other routes (`/supervisor/info`, `/os/info`, `/host/info`, `/resolution/info`, the App's own stats) were not read; by the same API they carry the same envelope, and the blank live `list_apps` agrees.
**Means:** the F-45 cause is **answered**: the mappers decode a flat shape the Supervisor does not send, so they return zero values with no error. The unknown is closed; the defect stands and is wider than `get_system_health` (every Supervisor-backed field, `list_apps` included).
