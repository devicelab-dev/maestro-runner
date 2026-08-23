# devicelab-ios-runner

XCUITest-based iOS automation runner for the `devicelab` driver in [maestro-runner](https://github.com/devicelab-dev/maestro-runner). On-device counterpart to [`devicelab-android-driver`](../devicelab-android-driver).

## Goal

Replace `pkg/driver/wda/` in maestro-runner with a self-contained, owned XCUITest runner. Fix the two iOS blockers from react-navigation PR #13027:

1. **No runtime third-party download** — runner is bundled in `maestro-runner` release artifacts; no fetch over the network at test time.
2. **Local parity with upstream Maestro** — flows that pass under `maestro` must also pass under `maestro-runner` locally, with the same retry profile.

## Status

**Phase 0 — Reference validation.** Building `callstackincubator/agent-device`'s XCUITest runner as a black-box sanity check that the architectural design (XCUITest + `Network.framework` HTTP + JSON command protocol) actually works for our flows before committing to a fresh Swift rewrite.

See [PLAN.md](./PLAN.md).

## Layout (planned)

```
devicelab-ios-runner/
├── README.md
├── PLAN.md                       # phased plan and acceptance criteria
├── PROTOCOL.md                   # wire protocol spec (written after Phase 0)
├── .gitignore
├── DevicelabIOSRunner.xcodeproj/ # Xcode project (Phase 2+)
├── DevicelabIOSRunner/           # UI test target sources (fresh Swift)
└── scripts/
    └── build.sh                  # standalone build pipeline
```

Built artifact will be vendored into `maestro-runner` at `drivers/ios/devicelab-ios-runner/` and bundled by `build-release.sh`.

## References

- Architecture inspiration: [agent-device findings doc](../../temp/test/agent-device/findings-ios-android.md)
- Cloned agent-device repo (reference only): `/Users/omnarayan/work/temp/test/agent-device/`
- Android counterpart pattern: `/Users/omnarayan/work/support-tools/devicelab-android-driver`
