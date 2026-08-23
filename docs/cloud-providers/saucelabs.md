# Sauce Labs

**Maestro-runner** runs your Maestro flows on the **Sauce Labs** platform. You can target Sauce Labs **Android and iOS real devices**, as well as **Android emulators** and **iOS simulators** hosted by Sauce Labs.

Maestro-runner acts as a bridge between Maestro and **Appium**: **Maestro flow YAML** is executed through an Appium session against the **Sauce Labs’ Appium endpoint**, so execution follows the same model as standard Appium tests on Sauce Labs.

## Run command

```bash
maestro-runner \
  --driver appium \
  --appium-url "https://$SAUCE_USERNAME:$SAUCE_ACCESS_KEY@ondemand.us-west-1.saucelabs.com:443/wd/hub" \
  --caps provider-caps.json \
  test flows/
```

- Default example uses `us-west-1`. Replace the Sauce Labs endpoints with your region as needed (for example `eu-central-1`, `us-east-4`).
- The Appium URL should include Sauce credentials (`$SAUCE_USERNAME` and `$SAUCE_ACCESS_KEY`).

After the `test` subcommand you pass **which Maestro flows to run**. That can be:

- **A directory** — e.g. `test flows/` discovers Maestro flow files (`*.yaml` / `*.yml`) **one level deep**: only files directly inside the folder you pass.
- **One flow file** — e.g. `test flows/login.yaml`.
- **Several flow files** — e.g. `test flow_a.yaml flow_b.yaml flow_c.yaml` (order is preserved).

## Example capabilities

Example `provider-caps.json` for Android real device:

```json
{
  "platformName": "Android",
  "appium:automationName": "UiAutomator2",
  "appium:deviceName": "Samsung.*",
  "appium:platformVersion": "^1[5-6].*",
  "appium:app": "storage:filename=mda-2.2.0-25.apk",
  "sauce:options": {
    "build": "Maestro Android Run",
    "appiumVersion": "latest"
  }
}
```

Example `provider-caps.json` for iOS real device:

```json
{
  "platformName": "iOS",
  "appium:automationName": "XCUITest",
  "appium:deviceName": "iPhone.*",
  "appium:platformVersion": "^(18|26).*",
  "appium:app": "storage:filename=SauceLabs-Demo-App.ipa",
  "sauce:options": {
    "build": "Maestro iOS Run",
    "appiumVersion": "latest",
    "resigningEnabled": true
  }
}
```

Example `provider-caps.json` for Android emulator:

```json
{
  "platformName": "Android",
  "appium:automationName": "UiAutomator2",
  "appium:deviceName": "Google Pixel 9 Emulator",
  "appium:platformVersion": "16.0",
  "appium:app": "storage:filename=mda-2.2.0-25.apk",
  "sauce:options": {
    "build": "Maestro Android Emulator Run",
    "appiumVersion": "2.11.0"
  }
}
```

Example `provider-caps.json` for iOS simulator:

```json
{
  "platformName": "iOS",
  "appium:automationName": "XCUITest",
  "appium:deviceName": "iPhone Simulator",
  "appium:platformVersion": "17.0",
  "appium:app": "storage:filename=SauceLabs-Demo-App.Simulator.zip",
  "sauce:options": {
    "build": "Maestro iOS Simulator Run",
    "appiumVersion": "2.1.3"
  }
}
```

## Include and exclude tags

Tags are defined **per flow** in the Maestro YAML **config** section (above the `---` that separates config from steps). Use a `tags` list of strings:

```yaml
appId: com.example.app
name: Login Test
tags:
  - smoke
  - regression
---
- launchApp
- tapOn: "Login"
```

**CLI filtering**:

- **`--include-tags`** — only flows that list **at least one** of the given tags are run. You can repeat the flag for multiple tags (a flow needs to match **any** of them). Example:

  ```bash
  maestro-runner --driver appium --appium-url "..." --caps provider-caps.json \
    test --include-tags smoke --include-tags quick flows/
  ```

- **`--exclude-tags`** — flows that list **any** of these tags are skipped. Example:

  ```bash
  maestro-runner --driver appium --appium-url "..." --caps provider-caps.json \
    test --exclude-tags slow flows/
  ```

Matching is **exact** (case-sensitive) on the tag strings in the YAML.

## Job / test name (capabilities vs YAML)

How the **test / job name** is chosen for a Sauce Labs job:

1. **`name` in `sauce:options`**  
   Set the test name from your capabilities by adding `name` under `sauce:options`. Sauce uses it when the session/job is created. See Sauce’s **`name`** option: [Test configuration options — `name`](https://docs.saucelabs.com/dev/test-configuration-options/#name).

   Example fragment inside your caps JSON:

   ```json
   "sauce:options": {
     "name": "My regression suite",
     "build": "Maestro Android Run",
     "appiumVersion": "latest"
   }
   ```

2. **No `name` in capabilities**  
   If you do **not** set a name that way, the name comes from the **YAML flow file**: the file’s **basename without** the `.yaml` or `.yml` extension. If **several YAML flows run in the same Appium session** (for example one worker runs multiple flows one after another), the name is taken from the **first** of those flows for that session’s Sauce job.

## Parallel execution (`--parallel` with Appium)

For Sauce Labs, parallel mode uses **multiple Appium sessions** on the same `--appium-url`. **Each parallel worker has exactly one Appium session** (on Sauce, usually **one device and one job** until that worker is done). Workers share a **queue** of flows: the **same session** often runs **several YAML files in sequence** when there are **more flows than workers**. When **flows and workers are equal** and all start together, **each YAML runs in its own session** at the same time.

Sauce Labs assigns devices to each session according to your capabilities and account limits.

Enable it with `--parallel N` (where `N` is the desired number of concurrent sessions):

```bash
maestro-runner \
  --driver appium \
  --parallel 3 \
  --appium-url "https://$SAUCE_USERNAME:$SAUCE_ACCESS_KEY@ondemand.us-west-1.saucelabs.com:443/wd/hub" \
  --caps provider-caps.json \
  test flow_a.yaml flow_b.yaml flow_c.yaml
```

### Behaviour for three common cases

1. **Number of YAML flows = number of parallel workers (sessions)**  
   With `--parallel N` and `N` flows, the runner starts **N** Appium sessions (one per worker). **Each flow runs in its own session** because each worker takes one YAML and they all run **at the same time**, **if** your Sauce account has enough **concurrency** for `N` simultaneous sessions.

2. **More YAML flows than sessions**  
   With `--parallel N` and more than `N` flows, at most `N` flows run concurrently. The rest wait in the queue. When a worker finishes a flow, it **takes the next YAML** from the queue until all flows are done.

3. **Fewer YAML flows than `--parallel N`**  
   Example: **3** flow files and `--parallel 5`. The runner starts **3** workers ( **3** Appium sessions), not 5 — **one session per flow**, no empty sessions. Sauce still needs concurrency for **3** simultaneous jobs, not 5.

Sauce Labs enforces **how many tests can run at once** via your plan. If sessions fail to start or queue on the Sauce side, check parallel test limits and regional capacity in the Sauce UI or docs.

## References

- [Run Maestro Flows on Any Cloud Provider](https://devicelab.dev/blog/run-maestro-flows-any-cloud)
- [Sauce Labs: `name` (test configuration)](https://docs.saucelabs.com/dev/test-configuration-options/#name)
- [Sauce Labs: Mobile Appium capabilities](https://docs.saucelabs.com/dev/test-configuration-options/#mobile-appium-capabilities)
