# Skill: device-integration-testing

# Device Integration Testing — shared harness guidance

Common pitfalls and error-handling for **any** maestro-runner integration
test that drives a real device/emulator (Python `conftest.py`, TypeScript
`setup.ts`, or the `maestro-runner test` YAML runner). Both the
`python-test-runner` and `typescript-test-runner` skills reuse this; keep the
duplicated knowledge HERE, not in each runner.

## The single-session device lock (uiautomator2)

The uiautomator2 driver locks the device **per server process**. On session
creation it writes a host-side guard:

- `/tmp/uia2-<serial>.sock` — Unix socket forward
- `/tmp/uia2-<serial>.pid` — owning process PID

`IsOwnerAlive()` returns true (→ "device … already in use") when that PID
file exists **and** the process is still alive. Because of this, **only one
session may be active against a device at a time** within a single server.

Consequences / rules:
- Don't open a *second* session in a test that already has one. The shared
  harness fixtures already open one per test process:
  - Python: the autouse `_track_client` → `client` fixture (session-scoped).
  - TypeScript: `getClient()` in `setup.ts` (shared `MaestroClient`).
- If a test needs raw HTTP access, **reuse the existing session id**
  (`client.session_id` / `client.sessionId`) instead of `POST /session`.
- Teardown of the session belongs to the shared fixture — tests must not
  delete the session themselves.

## Clean slate between runs

Before starting a fresh server/run, clear any stale guard left by a crashed
or killed previous run:

```sh
# Kill whatever holds the server port (do NOT use `pkill -f maestro-runner`, see below)
lsof -ti tcp:9999 2>/dev/null | xargs -r kill -9

# Remove BOTH guard files — a leftover .pid pointing at a reused/alive PID
# trips IsOwnerAlive and fails session creation even with no server running.
rm -f /tmp/uia2-<serial>.sock /tmp/uia2-<serial>.pid

# Clear any adb forward leaked by the previous run
adb -s <serial> forward --remove-all
```

> **`pkill -f maestro-runner` is a trap.** The pattern matches the *shell*
> running the command (its argv contains "maestro-runner"), so it can kill its
> own shell and orphan the backgrounded server. Always kill **by port**
> (`lsof -ti tcp:<port> | xargs kill`) instead.

## Server auto-start

Both harnesses auto-start the maestro-runner server when none is reachable
(`MAESTRO_SERVER_URL`), so you usually **don't** start one by hand:

- Python: `maestro_server` session fixture.
- TypeScript: `ensureServer()` in `setup.ts`.

If you do start a server manually, give it a unique port and don't also let
the harness start a second one — two servers on the same port conflict.

## Emulator must be attached

```sh
adb devices   # expect "<serial>  device" (e.g. emulator-5554)
```

## Teardown stream race (child subprocess logs)

When the harness pipes a child server's stdout/stderr into a `WriteStream`
log file, **unpipe the child streams before ending the log stream**.
Killing the child and then immediately `stream.end()` lets the dying process
flush final output into an already-closed stream → `write after end` crash
*after tests passed*. Fix:

```ts
child.stdout?.unpipe(logStream);
child.stderr?.unpipe(logStream);
child.kill();
// …then later…
logStream.end();
```

## Quick diagnostic checklist

| Symptom | Cause | Fix |
|---------|-------|-----|
| `device … already in use` | Second session, or stale `.pid` guard | Reuse shared session; clear `/tmp/uia2-*` guard files |
| `Connection refused` | No server up | Let harness auto-start, or start one on a free port |
| Server crashes/orphaned | `pkill -f maestro-runner` killed its own shell | Kill by port instead |
| Hang at session create | Stale `.pid` with alive (reused) PID | `rm -f /tmp/uia2-<serial>.sock /tmp/uia2-<serial>.pid` |
| `write after end` at exit | Child log stream closed before child flushed | Unpipe child stdout/stderr before `stream.end()` |
