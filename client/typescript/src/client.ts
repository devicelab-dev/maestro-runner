/**
 * MaestroClient — main client class for maestro-runner REST API.
 *
 * Usage:
 *
 * ```ts
 * const client = new MaestroClient("http://localhost:9999");
 * await client.createSession({ platformName: "android" });
 * try {
 *   await client.tap({ text: "Login" });
 *   await client.inputText("user@example.com");
 * } finally {
 *   await client.close();
 * }
 * ```
 */

import * as commands from "./commands";
import { MaestroError, SessionError, StepError } from "./exceptions";
import type { ElementSelector} from "./models";
import { DeviceInfo, ExecutionResult } from "./models";

type Step = Record<string, unknown>;

export interface MaestroClientOptions {
  baseUrl?: string;
  capabilities?: Record<string, unknown>;
  timeout?: number;
}

export class MaestroClient {
  readonly baseUrl: string;
  readonly timeout: number;
  private sessionId: string | undefined;

  constructor(
    baseUrl: string = "http://localhost:9999",
    options?: { capabilities?: Record<string, unknown>; timeout?: number },
  ) {
    this.baseUrl = baseUrl.replace(/\/+$/, "");
    this.timeout = options?.timeout ?? 60_000;
  }

  /** Initialize a session. Must be called before executing steps. */
  async createSession(capabilities: Record<string, unknown>): Promise<void> {
    const resp = await this.fetch("/session", {
      method: "POST",
      body: JSON.stringify(capabilities),
    });
    if (!resp.ok) {
      const text = await resp.text();
      throw new SessionError(`Failed to create session: ${text}`, resp.status);
    }
    const data = (await resp.json()) as { sessionId: string };
    this.sessionId = data.sessionId;
  }

  getSessionId(): string | undefined {
    return this.sessionId;
  }

  /** Delete the server session. */
  async close(): Promise<void> {
    if (!this.sessionId) return;
    try {
      await this.fetch(`/session/${this.sessionId}`, { method: "DELETE" });
    } catch {
      // swallow
    }
    this.sessionId = undefined;
  }

  // --- Low-level ---

  async executeStep(step: Step): Promise<ExecutionResult> {
    const sid = this.requireSession();
    const resp = await this.fetch(`/session/${sid}/execute`, {
      method: "POST",
      body: JSON.stringify(step),
    });
    if (!resp.ok) {
      const text = await resp.text();
      throw new MaestroError(`Execute failed: ${text}`, resp.status);
    }
    return ExecutionResult.fromDict((await resp.json()) as Record<string, unknown>);
  }

  private async exec(step: Step): Promise<ExecutionResult> {
    const result = await this.executeStep(step);
    if (!result.success && !step.optional) {
      throw new StepError(result.message ?? "step failed");
    }
    return result;
  }

  // --- App lifecycle ---

  async launchApp(
    appId: string,
    opts?: { clearState?: boolean; stopApp?: boolean; label?: string },
  ): Promise<ExecutionResult> {
    return this.exec(commands.launchApp(appId, opts));
  }

  async stopApp(appId: string, label?: string): Promise<ExecutionResult> {
    return this.exec(commands.stopApp(appId, label));
  }

  async clearState(appId: string, label?: string): Promise<ExecutionResult> {
    return this.exec(commands.clearState(appId, label));
  }

  async openLink(link: string, label?: string): Promise<ExecutionResult> {
    return this.exec(commands.openLink(link, label));
  }

  async setPermissions(
    appId: string,
    permissions: Record<string, string>,
    label?: string,
  ): Promise<ExecutionResult> {
    return this.exec(commands.setPermissions(appId, permissions, label));
  }

  async resetPermissions(label?: string): Promise<ExecutionResult> {
    return this.exec(commands.resetPermissions(label));
  }

  // --- Tap ---

  async tap(opts: {
    text?: string;
    id?: string;
    index?: number;
    selector?: ElementSelector;
    longPress?: boolean;
    waitUntilVisible?: boolean;
    retryIfNoChange?: boolean;
    enabled?: boolean;
    checked?: boolean;
    focused?: boolean;
    selected?: boolean;
    optional?: boolean;
    label?: string;
  }): Promise<ExecutionResult> {
    return this.exec(commands.tapOn(opts));
  }

  async longPress(opts: {
    text?: string;
    id?: string;
    selector?: ElementSelector;
    label?: string;
  }): Promise<ExecutionResult> {
    return this.exec(commands.tapOn({ ...opts, longPress: true }));
  }

  async tapOnPoint(
    point: string,
    opts?: { longPress?: boolean; label?: string },
  ): Promise<ExecutionResult> {
    const step: Step = { type: "tapOnPoint", point };
    if (opts?.longPress) step.longPress = true;
    if (opts?.label) step.label = opts.label;
    return this.exec(step);
  }

  // --- Input ---

  async inputText(text: string, label?: string): Promise<ExecutionResult> {
    return this.exec(commands.inputText(text, label));
  }

  async eraseText(characters?: number, label?: string): Promise<ExecutionResult> {
    return this.exec(commands.eraseText(characters, label));
  }

  async pressKey(code: string, label?: string): Promise<ExecutionResult> {
    return this.exec(commands.pressKey(code, label));
  }

  async back(label?: string): Promise<ExecutionResult> {
    return this.exec(commands.back(label));
  }

  async hideKeyboard(strategy?: string, label?: string): Promise<ExecutionResult> {
    return this.exec(commands.hideKeyboard(strategy, label));
  }

  async waitForAnimationToEnd(
    sleepMs?: number,
    threshold?: number,
    label?: string,
  ): Promise<ExecutionResult> {
    return this.exec(commands.waitForAnimationToEnd(sleepMs, threshold, label));
  }

  // --- Scroll / Swipe ---

  async scroll(label?: string): Promise<ExecutionResult> {
    return this.exec(commands.scroll(label));
  }

  async swipe(
    direction: string,
    durationMs: number = 400,
    label?: string,
  ): Promise<ExecutionResult> {
    return this.exec(commands.swipe(direction, durationMs, label));
  }

  async swipeOn(opts: {
    text?: string;
    id?: string;
    direction?: string;
    durationMs?: number;
    label?: string;
  }): Promise<ExecutionResult> {
    const step: Step = {
      type: "swipe",
      direction: (opts.direction ?? "UP").toUpperCase(),
      duration: opts.durationMs ?? 400,
    };
    if (opts.text != null) step.selector = { text: opts.text };
    else if (opts.id != null) step.selector = { id: opts.id };
    if (opts.label) step.label = opts.label;
    return this.exec(step);
  }

  // --- Assertions ---

  async assertVisible(opts: {
    text?: string;
    id?: string;
    selector?: ElementSelector;
    timeoutMs?: number;
    label?: string;
  }): Promise<ExecutionResult> {
    return this.exec(commands.assertVisible(opts));
  }

  async assertNotVisible(opts: {
    text?: string;
    id?: string;
    selector?: ElementSelector;
    timeoutMs?: number;
    label?: string;
  }): Promise<ExecutionResult> {
    return this.exec(commands.assertNotVisible(opts));
  }

  async elementExists(opts: { text?: string; id?: string }): Promise<boolean> {
    const step = commands.assertVisible({ ...opts, optional: true });
    const result = await this.executeStep(step);
    return result.success;
  }

  // --- Self-healing multi-selector tap ---

  async tapFirstMatch(
    selectors: Record<string, unknown>[],
    step: string = "",
  ): Promise<ExecutionResult> {
    let lastResult: ExecutionResult | undefined;
    for (const sel of selectors) {
      const tapStep: Step = { type: "tapOn", optional: true, ...sel };
      const result = await this.executeStep(tapStep);
      if (result.success) return result;
      lastResult = result;
    }
    if (!lastResult) {
      throw new StepError("tapFirstMatch: no selectors provided");
    }
    throw new StepError(
      `tapFirstMatch: none of ${selectors.length} selectors matched (step=${step})`,
    );
  }

  // --- Device queries ---

  async deviceInfo(): Promise<DeviceInfo> {
    const sid = this.requireSession();
    const resp = await this.fetch(`/session/${sid}/device-info`);
    if (!resp.ok) {
      const text = await resp.text();
      throw new MaestroError(`device-info failed: ${text}`, resp.status);
    }
    return DeviceInfo.fromDict((await resp.json()) as Record<string, unknown>);
  }

  async screenshot(): Promise<ArrayBuffer> {
    const sid = this.requireSession();
    const resp = await this.fetch(`/session/${sid}/screenshot`);
    if (!resp.ok) {
      const text = await resp.text();
      throw new MaestroError(`screenshot failed: ${text}`, resp.status);
    }
    return resp.arrayBuffer();
  }

  async viewHierarchy(): Promise<string> {
    const sid = this.requireSession();
    const resp = await this.fetch(`/session/${sid}/source`);
    if (!resp.ok) {
      const text = await resp.text();
      throw new MaestroError(`source failed: ${text}`, resp.status);
    }
    return resp.text();
  }

  // --- WebView (mobile WebView via CDP) ---

  async evalWebViewScript(
    script: string,
    opts?: { output?: string; label?: string },
  ): Promise<ExecutionResult> {
    return this.exec(commands.evalWebViewScript(script, opts));
  }

  async runWebViewScript(
    file: string,
    opts?: { env?: Record<string, string>; output?: string; label?: string },
  ): Promise<ExecutionResult> {
    return this.exec(commands.runWebViewScript(file, opts));
  }

  // --- Internals ---

  private requireSession(): string {
    if (!this.sessionId) {
      throw new SessionError(
        "No active session. Call createSession() first.",
      );
    }
    return this.sessionId;
  }

  private fetch(path: string, init?: RequestInit): Promise<Response> {
    const url = `${this.baseUrl}${path}`;
    const headers: Record<string, string> = {
      "Content-Type": "application/json",
    };
    return fetch(url, {
      ...init,
      headers: { ...headers, ...(init?.headers as Record<string, string>) },
      signal: AbortSignal.timeout(this.timeout),
    });
  }
}
