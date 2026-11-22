# devicelab-ios-agent (prebuilt)

> The agent builds in `simulator/` and `device/` are proprietary binaries, free to use with
> maestro-runner but not open source: see [../../LICENSE-BINARIES.md](../../LICENSE-BINARIES.md).
> `signing-stub/` is source under the Apache License 2.0.

The XCUITest agent behind `--driver devicelab` on iOS simulators and real iPhones, built from
the private `devicelab-ios-agent` repository with `scripts/build.sh`
(universal arm64 + x86_64, Swift Testing interop libraries bundled).

`simulator/manifest.json` records the agent commit (`version`), the wire
protocol and a sha256 per file. The host re-installs the agent on a
simulator when `version` changes.

To update: build in the agent repository, then replace `simulator/` with its
`dist/simulator/`.

## Real iPhones

`device/` is the same agent built with `scripts/build.sh --device --release`:
arm64, unsigned, no XCTest frameworks. Replace it with the agent repository's
`dist/device/`.

`signing-stub/` is a tiny public Xcode project (a host app and a UI-test
bundle, no agent code) with the agent's bundle ids. On the first run on a
device, maestro-runner builds it with `-allowProvisioningUpdates
DEVELOPMENT_TEAM=<--team-id>` so Xcode provisions the device, then re-signs a
copy of `device/` with the stub's certificate, profiles, entitlements and
XCTest frameworks. The signed agent is cached under
`~/.maestro-runner/cache/devicelab-ios-agent/device-signed/<version>-<team>-<udid>/`
until the agent changes or its profile nears expiry.

```bash
maestro-runner --platform ios --driver devicelab --team-id <TEAM_ID> \
  --device <UDID> --app-file <signed .app or .ipa> test <flows>
```

Regenerate `SigningStub.xcodeproj` from `signing-stub/project.yml` with
`xcodegen generate`.

## Access

Each run starts the agent with a random token (`DL_AGENT_TOKEN`, through
`SIMCTL_CHILD_` or the xctestrun's environment) and sends it on every request
as `X-DeviceLab-Token`; an agent built with token support answers a request
without it 401 and runs nothing. The token is saved, readable by the user only,
in `~/.maestro-runner/cache/devicelab-ios-agent/<udid>/agent.json` so a later
run can re-attach to a kept agent. The agent listens on 127.0.0.1 only: on a
simulator that is the Mac, on a device the end of the usbmux forward.

Overrides:

- `DEVICELAB_IOS_AGENT_DIR` — use an agent build from another directory
- `DEVICELAB_IOS_AGENT_LAUNCH` — `auto` (default), `simctl` or `xcodebuild`
- `DEVICELAB_IOS_AGENT_KEEP=0` — stop the agent when the run ends
  (by default an agent started with simctl stays up for the next run)
- `DEVICELAB_IOS_AGENT_DEVICE_DIR` — use a device agent build from another
  directory
- `DEVICELAB_IOS_AGENT_BUNDLE_ID` — sign the agent on a device as another
  bundle id (a personal team cannot use the team wildcard App ID, and
  `dev.devicelab.agent` may already be registered elsewhere)
- `DEVICELAB_IOS_AGENT_BIND=any` — on a device, have the agent listen on every
  interface instead of loopback (the token is still required)
