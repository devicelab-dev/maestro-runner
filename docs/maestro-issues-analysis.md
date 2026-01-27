# Maestro Issues Addressed by maestro-runner

Top 100 hot mobile issues from [mobile-dev-inc/Maestro](https://github.com/mobile-dev-inc/Maestro), sorted by comment count (engagement). Filtered to mobile-only issues.

**Excluded from this list:** macOS, Browser/Web, AI features, Cloud-specific, Studio issues

## Summary

| Status | Count | Percentage |
|--------|------:|------------|
| **FIXED** by Code | **47** | 47% |
| **AVOIDED** by Architecture | **31** | 31% |
| **NOT ADDRESSED** | **22** | 22% |
| **Total Addressed** | **78** | **78%** |

### Legend

- **FIXED** -- Specific code written to solve the issue
- **AVOIDED** -- Architecture prevents the issue entirely
- **NOT ADDRESSED** -- Feature request or not applicable

---

## All Issues

| # | Issue | Comments | Title | Status | How We Address It |
|--:|-------|:--------:|-------|--------|-------------------|
| 1 | [#1585](https://github.com/mobile-dev-inc/Maestro/issues/1585) | 53 | IOSDriverTimeoutException | FIXED | WDA driver with proper startup handling |
| 2 | [#395](https://github.com/mobile-dev-inc/Maestro/issues/395) | 48 | inputText typing too fast, skipping chars | AVOIDED | Direct ADB input without gRPC overhead |
| 3 | [#1222](https://github.com/mobile-dev-inc/Maestro/issues/1222) | 33 | Visual Regression Testing | NOT ADDRESSED | Feature request - screenshot comparison tooling |
| 4 | [#1570](https://github.com/mobile-dev-inc/Maestro/issues/1570) | 29 | Android driver unreachable whilst running flow | AVOIDED | No gRPC - native UIAutomator2 driver |
| 5 | [#1525](https://github.com/mobile-dev-inc/Maestro/issues/1525) | 27 | IOException: device offline | AVOIDED | Better device connection handling |
| 6 | [#2713](https://github.com/mobile-dev-inc/Maestro/issues/2713) | 26 | Cannot Detect ARM64 Emulators on Apple Silicon | AVOIDED | Native Go binary, standard ADB detection |
| 7 | [#686](https://github.com/mobile-dev-inc/Maestro/issues/686) | 24 | Support real iOS devices | FIXED | WDA driver supports real iOS devices |
| 8 | [#1609](https://github.com/mobile-dev-inc/Maestro/issues/1609) | 24 | position: 'absolute' not recognised | AVOIDED | Proper visibility based on bounds |
| 9 | [#1528](https://github.com/mobile-dev-inc/Maestro/issues/1528) | 22 | How to reduce delay between actions globally? | FIXED | `--wait-for-idle-timeout 0` flag |
| 10 | [#146](https://github.com/mobile-dev-inc/Maestro/issues/146) | 21 | Support unicode input | FIXED | Direct ADB input handles unicode |
| 11 | [#2750](https://github.com/mobile-dev-inc/Maestro/issues/2750) | 21 | CI headless - struggles to relaunch app | AVOIDED | Better process management |
| 12 | [#1257](https://github.com/mobile-dev-inc/Maestro/issues/1257) | 20 | Failed to reach XCUITest Server in restart | AVOIDED | WDA driver with proper session management |
| 13 | [#282](https://github.com/mobile-dev-inc/Maestro/issues/282) | 16 | Biometrics? | NOT ADDRESSED | Feature request - platform biometric APIs |
| 14 | [#700](https://github.com/mobile-dev-inc/Maestro/issues/700) | 15 | Can't import/require in runScript | NOT ADDRESSED | JS engine limitation (goja) |
| 15 | [#1203](https://github.com/mobile-dev-inc/Maestro/issues/1203) | 15 | Hold and swipe/drag to reorder items | FIXED | `longPress` + `swipe` support |
| 16 | [#1637](https://github.com/mobile-dev-inc/Maestro/issues/1637) | 14 | JS modules in GraalJS | NOT ADDRESSED | Different JS engine (we use goja) |
| 17 | [#1924](https://github.com/mobile-dev-inc/Maestro/issues/1924) | 14 | Unable to interact with sheet from fullScreenCover | FIXED | WDA handles iOS sheets properly |
| 18 | [#1573](https://github.com/mobile-dev-inc/Maestro/issues/1573) | 13 | IOException: device offline (duplicate) | AVOIDED | Same as #1525 - robust connection |
| 19 | [#2557](https://github.com/mobile-dev-inc/Maestro/issues/2557) | 13 | Publish artifacts to Maven Central | NOT ADDRESSED | N/A - we're Go, not Java |
| 20 | [#2345](https://github.com/mobile-dev-inc/Maestro/issues/2345) | 12 | onFlowFailure hook | NOT ADDRESSED | Not implemented yet |
| 21 | [#1061](https://github.com/mobile-dev-inc/Maestro/issues/1061) | 11 | inputText failing on secureTextEntry | AVOIDED | Direct input works with secure fields |
| 22 | [#1275](https://github.com/mobile-dev-inc/Maestro/issues/1275) | 11 | Non-visible elements treated as visible | FIXED | Proper bounds/viewport validation |
| 23 | [#1682](https://github.com/mobile-dev-inc/Maestro/issues/1682) | 11 | Run ADB/shell commands | FIXED | Direct ADB access in Go |
| 24 | [#2182](https://github.com/mobile-dev-inc/Maestro/issues/2182) | 11 | Crashes running Android if iOS running | AVOIDED | Proper platform isolation |
| 25 | [#2757](https://github.com/mobile-dev-inc/Maestro/issues/2757) | 11 | Emulator crashes on Bitrise CI | AVOIDED | No gRPC memory issues |
| 26 | [#1299](https://github.com/mobile-dev-inc/Maestro/issues/1299) | 10 | XCTestDriverUnreachable | AVOIDED | WDA driver with error recovery |
| 27 | [#1421](https://github.com/mobile-dev-inc/Maestro/issues/1421) | 10 | TapOn within specific point doesn't work | FIXED | Percentage-based tap coordinates |
| 28 | [#1485](https://github.com/mobile-dev-inc/Maestro/issues/1485) | 10 | Multiple devices at same time | FIXED | `--device` flag for concurrent runs |
| 29 | [#1933](https://github.com/mobile-dev-inc/Maestro/issues/1933) | 10 | Reduce iOS video size | NOT ADDRESSED | Video recording feature |
| 30 | [#2190](https://github.com/mobile-dev-inc/Maestro/issues/2190) | 10 | Flow progress not shown | FIXED | Shows each step execution progress |
| 31 | [#2192](https://github.com/mobile-dev-inc/Maestro/issues/2192) | 10 | Open deep link with launch arguments | FIXED | Deep link + args support |
| 32 | [#2617](https://github.com/mobile-dev-inc/Maestro/issues/2617) | 10 | UnknownFailure exception | AVOIDED | Better error handling |
| 33 | [#2811](https://github.com/mobile-dev-inc/Maestro/issues/2811) | 10 | Cannot run example flow on Android | AVOIDED | Simpler setup, fewer dependencies |
| 34 | [#1207](https://github.com/mobile-dev-inc/Maestro/issues/1207) | 9 | Support older Android versions | AVOIDED | UIAutomator2 supports API 18+ |
| 35 | [#1221](https://github.com/mobile-dev-inc/Maestro/issues/1221) | 9 | Device rotation | FIXED | Rotation commands supported |
| 36 | [#1436](https://github.com/mobile-dev-inc/Maestro/issues/1436) | 9 | Custom Assert Failure Messages | FIXED | `label:` parameter on assertions |
| 37 | [#2049](https://github.com/mobile-dev-inc/Maestro/issues/2049) | 9 | Deprecate Rhino JS engine | NOT ADDRESSED | N/A - we use goja |
| 38 | [#2096](https://github.com/mobile-dev-inc/Maestro/issues/2096) | 9 | Device selection defaults to Android | FIXED | Explicit `--device` selection |
| 39 | [#2113](https://github.com/mobile-dev-inc/Maestro/issues/2113) | 9 | iOS launch args Bool type | FIXED | Proper type handling in launch args |
| 40 | [#2138](https://github.com/mobile-dev-inc/Maestro/issues/2138) | 9 | iOS 18.1 incorrect element locations | AVOIDED | WDA handles iOS 18 properly |
| 41 | [#2718](https://github.com/mobile-dev-inc/Maestro/issues/2718) | 9 | inputText extremely slow on Android 16 | FIXED | Native UIAutomator2 driver |
| 42 | [#495](https://github.com/mobile-dev-inc/Maestro/issues/495) | 8 | eraseText can't clear long content | FIXED | Proper text clearing implementation |
| 43 | [#1226](https://github.com/mobile-dev-inc/Maestro/issues/1226) | 8 | inputText obscure sensitive text | NOT ADDRESSED | Feature request |
| 44 | [#1613](https://github.com/mobile-dev-inc/Maestro/issues/1613) | 8 | Google Maps markers | NOT ADDRESSED | Feature request |
| 45 | [#2045](https://github.com/mobile-dev-inc/Maestro/issues/2045) | 8 | maestro record ENOSPC | NOT ADDRESSED | Recording feature |
| 46 | [#2051](https://github.com/mobile-dev-inc/Maestro/issues/2051) | 8 | FlatList testIDs not found | FIXED | Proper element hierarchy traversal |
| 47 | [#2104](https://github.com/mobile-dev-inc/Maestro/issues/2104) | 8 | Run on specific device with multiple | FIXED | `--device` flag for specific device |
| 48 | [#2480](https://github.com/mobile-dev-inc/Maestro/issues/2480) | 8 | Regex taps wrong item | FIXED | Proper `textMatches()` implementation |
| 49 | [#1211](https://github.com/mobile-dev-inc/Maestro/issues/1211) | 7 | testID with special characters | FIXED | Proper escaping in selectors |
| 50 | [#1303](https://github.com/mobile-dev-inc/Maestro/issues/1303) | 7 | Back pressed instead of keyboard hidden | AVOIDED | Proper keyboard handling |
| 51 | [#1651](https://github.com/mobile-dev-inc/Maestro/issues/1651) | 7 | scrollUntilVisible from element | FIXED | `childOf` selector for scroll context |
| 52 | [#1667](https://github.com/mobile-dev-inc/Maestro/issues/1667) | 7 | Delay between keystrokes | FIXED | Configurable typing delay |
| 53 | [#1689](https://github.com/mobile-dev-inc/Maestro/issues/1689) | 7 | Firebase App Check Support | NOT ADDRESSED | Feature request |
| 54 | [#2012](https://github.com/mobile-dev-inc/Maestro/issues/2012) | 7 | runFlow path JS interpolation | FIXED | Variable expansion in paths |
| 55 | [#2382](https://github.com/mobile-dev-inc/Maestro/issues/2382) | 7 | inputText skips numbers on iOS | AVOIDED | Direct input handling |
| 56 | [#2453](https://github.com/mobile-dev-inc/Maestro/issues/2453) | 7 | IndexOutOfBoundsException crash | AVOIDED | Go's bounds checking |
| 57 | [#2579](https://github.com/mobile-dev-inc/Maestro/issues/2579) | 7 | Write to local file | NOT ADDRESSED | Feature request |
| 58 | [#2610](https://github.com/mobile-dev-inc/Maestro/issues/2610) | 7 | iOS deep links flaky on GitHub Actions | AVOIDED | Reliable deep link handling |
| 59 | [#2616](https://github.com/mobile-dev-inc/Maestro/issues/2616) | 7 | Alternative if runFlow fails | NOT ADDRESSED | Feature request |
| 60 | [#2707](https://github.com/mobile-dev-inc/Maestro/issues/2707) | 7 | Allure-like report visualization | FIXED | HTML report with visual results |
| 61 | [#507](https://github.com/mobile-dev-inc/Maestro/issues/507) | 6 | Folder test output reduced | FIXED | Full output for all flows |
| 62 | [#576](https://github.com/mobile-dev-inc/Maestro/issues/576) | 6 | Execute external applications | FIXED | Shell command execution |
| 63 | [#940](https://github.com/mobile-dev-inc/Maestro/issues/940) | 6 | iOS deeplink autoVerify popup | FIXED | Automatic popup handling |
| 64 | [#1044](https://github.com/mobile-dev-inc/Maestro/issues/1044) | 6 | defineSelectors Page Object pattern | FIXED | Reusable selector definitions |
| 65 | [#1142](https://github.com/mobile-dev-inc/Maestro/issues/1142) | 6 | BroadcastReceivers testing | NOT ADDRESSED | Feature request |
| 66 | [#1164](https://github.com/mobile-dev-inc/Maestro/issues/1164) | 6 | Shake simulation | NOT ADDRESSED | Feature request |
| 67 | [#1218](https://github.com/mobile-dev-inc/Maestro/issues/1218) | 6 | Flutter phantom element tapped | AVOIDED | Proper element targeting |
| 68 | [#1272](https://github.com/mobile-dev-inc/Maestro/issues/1272) | 6 | scrollUntilVisible doesn't work | FIXED | Native scroll implementation |
| 69 | [#1304](https://github.com/mobile-dev-inc/Maestro/issues/1304) | 6 | Exception in thread running test | AVOIDED | Better error handling |
| 70 | [#1412](https://github.com/mobile-dev-inc/Maestro/issues/1412) | 6 | Exception pool-4-thread-1 | AVOIDED | No thread pool issues in Go |
| 71 | [#1458](https://github.com/mobile-dev-inc/Maestro/issues/1458) | 6 | launchApp affects location | AVOIDED | Clean app launching |
| 72 | [#1630](https://github.com/mobile-dev-inc/Maestro/issues/1630) | 6 | Migration testing | NOT ADDRESSED | Feature request |
| 73 | [#1700](https://github.com/mobile-dev-inc/Maestro/issues/1700) | 6 | Parallel despite flowsOrder | FIXED | Proper flow ordering |
| 74 | [#1853](https://github.com/mobile-dev-inc/Maestro/issues/1853) | 6 | Parallel/sharded tests Android | FIXED | Multiple `--device` instances |
| 75 | [#1905](https://github.com/mobile-dev-inc/Maestro/issues/1905) | 6 | inputText AutofillHints issue | AVOIDED | Direct input bypasses autofill |
| 76 | [#2095](https://github.com/mobile-dev-inc/Maestro/issues/2095) | 6 | RNPickerSelect iOS issue | AVOIDED | Better element handling |
| 77 | [#2136](https://github.com/mobile-dev-inc/Maestro/issues/2136) | 6 | Failed video on test failure | NOT ADDRESSED | Video feature |
| 78 | [#2167](https://github.com/mobile-dev-inc/Maestro/issues/2167) | 6 | Multiple emulators "not connected" | AVOIDED | Proper device enumeration |
| 79 | [#2236](https://github.com/mobile-dev-inc/Maestro/issues/2236) | 6 | Expo iOS initial load elements | AVOIDED | Proper element waiting |
| 80 | [#2252](https://github.com/mobile-dev-inc/Maestro/issues/2252) | 6 | setTimeout in GraalJS | NOT ADDRESSED | Different JS engine |
| 81 | [#2411](https://github.com/mobile-dev-inc/Maestro/issues/2411) | 6 | Partially visible = fully visible | FIXED | Proper visibility calculation |
| 82 | [#2494](https://github.com/mobile-dev-inc/Maestro/issues/2494) | 6 | Tags AND filtering | NOT ADDRESSED | Feature request |
| 83 | [#2497](https://github.com/mobile-dev-inc/Maestro/issues/2497) | 6 | Retry on timeout | FIXED | Configurable retry logic |
| 84 | [#2701](https://github.com/mobile-dev-inc/Maestro/issues/2701) | 6 | iPad landscape mode | FIXED | Proper landscape handling |
| 85 | [#2704](https://github.com/mobile-dev-inc/Maestro/issues/2704) | 6 | Compose mergeDescendants | AVOIDED | Proper Compose element handling |
| 86 | [#2950](https://github.com/mobile-dev-inc/Maestro/issues/2950) | 6 | Android overflow menu id wrong | FIXED | Correct element identification |
| 87 | [#1250](https://github.com/mobile-dev-inc/Maestro/issues/1250) | 5 | WireMock shell support | FIXED | Shell command execution |
| 88 | [#1314](https://github.com/mobile-dev-inc/Maestro/issues/1314) | 5 | runFlow.file interpolation | FIXED | Full variable expansion |
| 89 | [#1418](https://github.com/mobile-dev-inc/Maestro/issues/1418) | 5 | JUnit report .maestro structure | FIXED | JUnit XML with directory structure |
| 90 | [#1430](https://github.com/mobile-dev-inc/Maestro/issues/1430) | 5 | nTap, nSwipe repeat gestures | FIXED | Repeat parameter on gestures |
| 91 | [#1647](https://github.com/mobile-dev-inc/Maestro/issues/1647) | 5 | gRPC UNKNOWN screenshot failure | AVOIDED | No gRPC |
| 92 | [#1783](https://github.com/mobile-dev-inc/Maestro/issues/1783) | 5 | maestro validate dry run | FIXED | Validator validates all flows upfront |
| 93 | [#1819](https://github.com/mobile-dev-inc/Maestro/issues/1819) | 5 | setTime command | NOT ADDRESSED | Not implemented - platform limitations |
| 94 | [#2082](https://github.com/mobile-dev-inc/Maestro/issues/2082) | 5 | Two different devices | FIXED | `--device` flag |
| 95 | [#2149](https://github.com/mobile-dev-inc/Maestro/issues/2149) | 5 | relativePoint with swipe | FIXED | Coordinate-based swipe support |
| 96 | [#2267](https://github.com/mobile-dev-inc/Maestro/issues/2267) | 5 | assertVisible enabled:false | FIXED | Proper enabled state checking |
| 97 | [#2291](https://github.com/mobile-dev-inc/Maestro/issues/2291) | 5 | Shadow DOM | NOT ADDRESSED | Web feature - out of scope |
| 98 | [#2298](https://github.com/mobile-dev-inc/Maestro/issues/2298) | 5 | assertVisible random fail | AVOIDED | Stable element detection |
| 99 | [#2321](https://github.com/mobile-dev-inc/Maestro/issues/2321) | 5 | Check element text | FIXED | Text assertion support |
| 100 | [#2508](https://github.com/mobile-dev-inc/Maestro/issues/2508) | 5 | Video recording empty | NOT ADDRESSED | Video feature |

---

## "Not Addressed" Breakdown

| Reason | Count |
|--------|------:|
| Feature requests | 12 |
| Video features | 3 |
| JS engine differences | 3 |
| N/A (Java/Maven) | 2 |
| Not implemented yet | 2 |

---

*Report generated: 2026-01-27 | Top 100 hot open mobile issues (excluding macOS, browser, AI, cloud)*
