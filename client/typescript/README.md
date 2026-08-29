# maestro-runner — TypeScript Client

TypeScript/JavaScript client for the [maestro-runner](../../README.md) REST API.

## Installation

```bash
cd client/typescript
npm install
```

## Quick Start

```ts
import { MaestroClient } from "maestro-runner";

const client = new MaestroClient("http://localhost:9999");
await client.createSession({ platformName: "android" });

try {
  await client.tap({ text: "Login" });
  await client.inputText("user@example.com");
  await client.assertVisible({ text: "Welcome" });
} finally {
  await client.close();
}
```

## Page Object Model

Tests use the Page Object Model pattern for maintainable E2E tests:

```ts
import { getClient, teardown } from "./setup";
import { ContactListPage } from "./pages/ContactListPage";

afterAll(() => teardown());

it("adds a contact", async () => {
  const client = await getClient();
  const contactList = new ContactListPage(client);

  await contactList.launch(true);
  const editPage = await contactList.openCreateContact();
  await editPage.setFirstName("Alice");
  await editPage.setLastName("Tester");
  await editPage.setPhone("5550100");
  await editPage.save();
  await contactList.assertContactVisible("Alice Tester");
});
```

## Running Tests

```bash
# Prerequisites for device tests: emulator/simulator running, maestro-runner server started
./maestro-runner --platform android server --port 9999

# Run unit tests only
npm run test:unit

# Run device tests
npm run test:device:android
npm run test:device:ios
```

## Environment Variables

| Variable              | Default                    | Description                     |
| --------------------- | -------------------------- | ------------------------------- |
| `MAESTRO_SERVER_URL`  | `http://localhost:9999`    | Server URL                      |
| `MAESTRO_PLATFORM`    | `android`                  | Target platform                 |
| `MAESTRO_RUNNER_BIN`  | `../../maestro-runner`     | Path to maestro-runner binary   |

## API Reference

### MaestroClient

| Method                | Description                            |
| --------------------- | -------------------------------------- |
| `createSession()`     | Initialize a session                   |
| `close()`             | Delete the session                     |
| `launchApp()`         | Launch an app                          |
| `stopApp()`           | Stop an app                            |
| `clearState()`        | Clear app state                        |
| `tap()`               | Tap on an element                      |
| `longPress()`         | Long-press on an element               |
| `tapOnPoint()`        | Tap on a coordinate                    |
| `inputText()`         | Type text                              |
| `eraseText()`         | Erase text                             |
| `pressKey()`          | Press a key                            |
| `back()`              | Press back button                      |
| `hideKeyboard()`      | Hide the keyboard                      |
| `scroll()`            | Scroll                                 |
| `swipe()`             | Swipe in a direction                   |
| `assertVisible()`     | Assert element is visible              |
| `assertNotVisible()`  | Assert element is not visible          |
| `elementExists()`     | Check if element exists (no throw)     |
| `tapFirstMatch()`     | Tap first matching selector            |
| `deviceInfo()`        | Get device information                 |
| `screenshot()`        | Get screenshot as ArrayBuffer          |
| `viewHierarchy()`     | Get view hierarchy XML                 |
| `setPermissions()`    | Grant/deny app permissions (`setPermissions`) |
| `resetPermissions()`  | Reset browser permissions (`resetPermissions`) |
| `evalWebViewScript()` | Run JS in a mobile WebView via CDP (`evalWebViewScript`) |
| `runWebViewScript()`  | Load & run a JS file in a mobile WebView via CDP (`runWebViewScript`) |
| `doubleTapOn()`       | Double-tap on an element (`doubleTapOn`) |
| `longPressOn()`       | Long-press on an element (`longPressOn`) |
| `dragAndDrop()`       | Drag one element onto another (`dragAndDrop`) |
| `scrollUntilVisible()` | Scroll until an element appears (`scrollUntilVisible`) |
| `assertScreenshot()` | Visual regression assert (`assertScreenshot`) |
| `takeScreenshot()`    | Save a screenshot to disk (`takeScreenshot`) |
| `copyTextFrom()`      | Copy text from an element to clipboard (`copyTextFrom`) |
| `pasteText()`         | Paste clipboard text (`pasteText`) |
| `setClipboard()`      | Set clipboard contents (`setClipboard`) |
| `assertWithAI()`      | Natural-language assertion via AI (`assertWithAI`) |
| `evalScript()`        | Run an inline JS snippet (`evalScript`) |
| `runScript()`         | Run a JS file with env vars (`runScript`) |
| `evalBrowserScript()` | Run JS in a desktop browser (`evalBrowserScript`) |
| `setLocation()`       | Set device GPS location (`setLocation`) |
| `setAirplaneMode()`   | Enable/disable airplane mode (`setAirplaneMode`) |
| `toggleAirplaneMode()` | Toggle airplane mode (`toggleAirplaneMode`) |
| `setNetworkConditions()` | Throttle/simulate network (`setNetworkConditions`) |
| `openNotifications()` | Open the notification shade (`openNotifications`) |
| `setDarkMode()`       | Enable/disable dark mode (`setDarkMode`) |
| `setOrientation()`    | Set screen orientation (`setOrientation`) |
| `openBrowser()`       | Open a URL in the desktop browser (`openBrowser`) |
| `switchTab()`         | Switch browser tab (`switchTab`) |
| `closeTab()`          | Close current browser tab (`closeTab`) |
| `getConsoleLogs()`    | Read browser console logs (`getConsoleLogs`) |
| `clearConsoleLogs()`  | Clear browser console logs (`clearConsoleLogs`) |
| `assertNoJSErrors()`  | Assert no console JS errors (`assertNoJSErrors`) |
| `mockNetwork()`       | Mock a network request (`mockNetwork`) |

Any step type not listed here can still be sent via `executeStep({ type: "...", ... })` —
the client forwards the raw step dict straight to the server, which supports ~90 step
types in total.
