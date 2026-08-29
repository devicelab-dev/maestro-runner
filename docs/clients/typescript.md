# TypeScript Client Tutorial

The TypeScript client wraps the maestro-runner [REST API Server](../README.md#rest-api-server)
so you can drive Android, iOS, and Web devices from Node.js test runners (Jest, Vitest,
Playwright) using the same selectors and assertions as YAML flows — with full type safety.

Source: [`client/typescript`](../../client/typescript).

## 1. Prerequisites

- A built `maestro-runner` binary on your `PATH` (or set `MAESTRO_RUNNER_BIN` to its path).
- An emulator/simulator/device available, **or** let the tests auto-start a server.
- Node.js 18+ (uses the built-in `fetch`).

## 2. Install

```bash
cd client/typescript
npm install
```

This installs the client and its dev dependencies (Jest, etc.). The package name is
`maestro-runner`, so you import it as:

```ts
import { MaestroClient } from "maestro-runner";
```

## 3. Start the server (or let the client tests do it)

Start the REST server once, in a separate terminal:

```bash
maestro-runner server --port 9999
# or pre-select a platform:
maestro-runner --platform android server --port 9999
```

> The device test suites (`npm run test:device:*`) can also auto-start the server
> using `MAESTRO_RUNNER_BIN`, so you only need to start it manually for unit tests or
> your own scripts.

## 4. Your first script

```ts
import { MaestroClient } from "maestro-runner";

const client = new MaestroClient("http://localhost:9999");

// Create a session before executing any steps.
await client.createSession({ platformName: "android" });

try {
  await client.launchApp("com.example.app", { clearState: true });
  await client.tap({ text: "Login" });
  await client.inputText("user@example.com");
  await client.inputText("secret", { id: "password" });
  await client.tap({ text: "Sign in" });
  await client.assertVisible({ text: "Welcome" });
} finally {
  // Always close the session to free the device.
  await client.close();
}
```

`createSession()` is required before any step. `close()` sends `DELETE /session/{id}`.
Wrap work in `try/finally` (or a context helper) so the session is always released.

## 5. Selectors

`tap`, `longPress`, `assertVisible`, `assertNotVisible`, and `elementExists` accept an
options object with a selector:

| Field                | Type      | Meaning                                         |
|----------------------|-----------|-------------------------------------------------|
| `text`               | `string`  | Match by visible text                           |
| `id`                 | `string`  | Match by resource id / accessibility id         |
| `index`              | `number`  | Disambiguate multiple matches                   |
| `selector`           | `object`  | A raw `ElementSelector` (`{ text, id, ... }`)   |
| `longPress`          | `boolean` | Long-press instead of tap                       |
| `optional`           | `boolean` | Don't throw if the step fails                   |
| `timeoutMs`          | `number`  | Per-call timeout for assertions                 |
| `waitUntilVisible`   | `boolean` | Wait for the element before acting              |
| `enabled`/`checked`/… | `boolean` | State filters for the matched element           |

Example:

```ts
await client.tap({ id: "com.example.app:id/submit", enabled: true });
await client.assertVisible({ text: "Saved", timeoutMs: 10000 });
```

## 6. Page Object Model

For maintainable suites, encapsulate screens behind page objects and reuse a shared
client. The repository's device tests follow this pattern:

```ts
// setup.ts
import { MaestroClient } from "maestro-runner";

let client: MaestroClient | undefined;

export async function getClient(): Promise<MaestroClient> {
  if (!client) {
    client = new MaestroClient(process.env.MAESTRO_SERVER_URL ?? "http://localhost:9999");
    await client.createSession({ platformName: process.env.MAESTRO_PLATFORM ?? "android" });
  }
  return client;
}

export async function teardown(): Promise<void> {
  if (client) {
    await client.close();
    client = undefined;
  }
}
```

```ts
// pages/ContactListPage.ts
import type { MaestroClient } from "maestro-runner";

export class ContactListPage {
  constructor(private readonly client: MaestroClient) {}

  async launch(clear = false): Promise<void> {
    await this.client.launchApp("com.example.app", { clearState: clear });
  }

  async openCreateContact() {
    await this.client.tap({ text: "Add contact" });
    return new ContactEditPage(this.client);
  }

  async assertContactVisible(name: string): Promise<void> {
    await this.client.assertVisible({ text: name });
  }
}

class ContactEditPage {
  constructor(private readonly client: MaestroClient) {}
  async setFirstName(v: string) { await this.client.inputText(v, { id: "first_name" }); }
  async setLastName(v: string)  { await this.client.inputText(v, { id: "last_name" }); }
  async setPhone(v: string)     { await this.client.inputText(v, { id: "phone" }); }
  async save()                  { await this.client.tap({ text: "Save" }); }
}
```

```ts
// tests/add-contact.test.ts
import { getClient, teardown } from "./setup";
import { ContactListPage } from "./pages/ContactListPage";

afterAll(() => teardown());

it("adds a contact", async () => {
  const client = await getClient();
  const list = new ContactListPage(client);

  await list.launch(true);
  const edit = await list.openCreateContact();
  await edit.setFirstName("Alice");
  await edit.setLastName("Tester");
  await edit.setPhone("5550100");
  await edit.save();
  await list.assertContactVisible("Alice Tester");
});
```

## 7. Advanced calls

```ts
// Self-healing: try each selector in order, stop at the first match.
await client.tapFirstMatch([
  { text: "Continue" },
  { id: "continue_button" },
  { text: "Next" },
], "dismiss onboarding");

// Tap a coordinate.
await client.tapOnPoint("50%,50%", { longPress: true });

// Swipe on an element or direction.
await client.swipeOn({ text: "Feed", direction: "UP", durationMs: 500 });
await client.swipe("DOWN", 400);

// Screenshot / view hierarchy as raw data.
const png = await client.screenshot();        // ArrayBuffer
const xml = await client.viewHierarchy();     // string

// Device metadata.
const info = await client.deviceInfo();
console.log(info.deviceName, info.platform, info.osVersion);
```

## 8. Running the suite

```bash
# Unit tests (no device needed) — exercises the client against a mocked server.
npm run test:unit

# Device tests — require an emulator/simulator and a running server.
npm run test:device:android
npm run test:device:ios

# Animation-regression device tests.
npm run test:animation:android
npm run test:animation:ios
```

Lint and type-check:

```bash
npm run lint
npm run build      # tsc — emits dist/ with type declarations
```

## 9. Environment variables

| Variable              | Default                    | Description                     |
| --------------------- | -------------------------- | ------------------------------- |
| `MAESTRO_SERVER_URL`  | `http://localhost:9999`    | Server URL                      |
| `MAESTRO_PLATFORM`    | `android`                  | Target platform                 |
| `MAESTRO_RUNNER_BIN`  | `../../maestro-runner`     | Path to maestro-runner binary   |

## Full API reference

See the client [`README.md`](../../client/typescript/README.md) for the complete
`MaestroClient` method table.
