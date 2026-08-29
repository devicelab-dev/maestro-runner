/** Command builders — produce step JSON for the REST API. */

import type { ElementSelector } from "./models";

type Step = Record<string, unknown>;
type SelectorValue = string | Record<string, unknown>;

function selectorValue(opts: {
  text?: string;
  id?: string;
  index?: number;
  selector?: ElementSelector;
  enabled?: boolean;
  checked?: boolean;
  focused?: boolean;
  selected?: boolean;
}): SelectorValue {
  const d: Record<string, unknown> = {};
  if (opts.selector) Object.assign(d, opts.selector.toDict());
  if (opts.text != null) d.text = opts.text;
  if (opts.id != null) d.id = opts.id;
  if (opts.index != null) d.index = String(opts.index);
  if (opts.enabled != null) d.enabled = opts.enabled;
  if (opts.checked != null) d.checked = opts.checked;
  if (opts.focused != null) d.focused = opts.focused;
  if (opts.selected != null) d.selected = opts.selected;
  // Compact form: text-only selector → plain string
  const keys = Object.keys(d);
  if (keys.length === 1 && keys[0] === "text") return d.text as string;
  return d;
}

export function tapOn(opts: {
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
  timeout?: number;
  label?: string;
}): Step {
  const step: Step = { type: "tapOn" };
  step.selector = selectorValue(opts);
  if (opts.longPress) step.longPress = true;
  if (opts.waitUntilVisible != null) step.waitUntilVisible = opts.waitUntilVisible;
  if (opts.retryIfNoChange != null) step.retryTapIfNoChange = opts.retryIfNoChange;
  if (opts.optional) step.optional = true;
  if (opts.timeout != null) step.timeout = opts.timeout;
  if (opts.label != null) step.label = opts.label;
  return step;
}

export function inputText(text: string, label?: string): Step {
  const step: Step = { type: "inputText", text };
  if (label != null) step.label = label;
  return step;
}

export function eraseText(characters?: number, label?: string): Step {
  const step: Step = { type: "eraseText" };
  if (characters != null) step.charactersToErase = characters;
  if (label != null) step.label = label;
  return step;
}

export function pressKey(code: string, label?: string): Step {
  const step: Step = { type: "pressKey", key: code };
  if (label != null) step.label = label;
  return step;
}

export function back(label?: string): Step {
  const step: Step = { type: "back" };
  if (label != null) step.label = label;
  return step;
}

export function scroll(label?: string): Step {
  const step: Step = { type: "scroll" };
  if (label != null) step.label = label;
  return step;
}

export function swipe(direction: string, durationMs: number = 400, label?: string): Step {
  const step: Step = { type: "swipe", direction: direction.toUpperCase(), duration: durationMs };
  if (label != null) step.label = label;
  return step;
}

export function hideKeyboard(strategy?: string, label?: string): Step {
  const step: Step = { type: "hideKeyboard" };
  if (strategy != null) step.strategy = strategy;
  if (label != null) step.label = label;
  return step;
}

export function waitForAnimationToEnd(
  sleepMs?: number,
  threshold?: number,
  label?: string,
): Step {
  const step: Step = { type: "waitForAnimationToEnd" };
  if (sleepMs != null) step.sleepMs = sleepMs;
  if (threshold != null) step.threshold = threshold;
  if (label != null) step.label = label;
  return step;
}

export function assertVisible(opts: {
  text?: string;
  id?: string;
  selector?: ElementSelector;
  timeoutMs?: number;
  optional?: boolean;
  label?: string;
}): Step {
  const step: Step = { type: "assertVisible" };
  step.selector = selectorValue({ text: opts.text, id: opts.id, selector: opts.selector });
  if (opts.timeoutMs != null) step.timeout = opts.timeoutMs;
  if (opts.optional) step.optional = true;
  if (opts.label != null) step.label = opts.label;
  return step;
}

export function assertNotVisible(opts: {
  text?: string;
  id?: string;
  selector?: ElementSelector;
  timeoutMs?: number;
  label?: string;
}): Step {
  const step: Step = { type: "assertNotVisible" };
  step.selector = selectorValue({ text: opts.text, id: opts.id, selector: opts.selector });
  if (opts.timeoutMs != null) step.timeout = opts.timeoutMs;
  if (opts.label != null) step.label = opts.label;
  return step;
}

export function launchApp(
  appId: string,
  opts?: { clearState?: boolean; stopApp?: boolean; label?: string },
): Step {
  const step: Step = { type: "launchApp", appId };
  if (opts?.clearState != null) step.clearState = opts.clearState;
  if (opts?.stopApp != null) step.stopApp = opts.stopApp;
  if (opts?.label != null) step.label = opts.label;
  return step;
}

export function stopApp(appId: string, label?: string): Step {
  const step: Step = { type: "stopApp", appId };
  if (label != null) step.label = label;
  return step;
}

export function clearState(appId: string, label?: string): Step {
  const step: Step = { type: "clearState", appId };
  if (label != null) step.label = label;
  return step;
}

export function openLink(link: string, label?: string): Step {
  const step: Step = { type: "openLink", link };
  if (label != null) step.label = label;
  return step;
}

export function setPermissions(
  appId: string,
  permissions: Record<string, string>,
  label?: string,
): Step {
  const step: Step = { type: "setPermissions", appId, permissions };
  if (label != null) step.label = label;
  return step;
}

export function resetPermissions(label?: string): Step {
  const step: Step = { type: "resetPermissions" };
  if (label != null) step.label = label;
  return step;
}

export function evalWebViewScript(
  script: string,
  opts?: { output?: string; label?: string },
): Step {
  const step: Step = { type: "evalWebViewScript", script };
  if (opts?.output != null) step.output = opts.output;
  if (opts?.label != null) step.label = opts.label;
  return step;
}

export function runWebViewScript(
  file: string,
  opts?: { env?: Record<string, string>; output?: string; label?: string },
): Step {
  const step: Step = { type: "runWebViewScript", file };
  if (opts?.env != null) step.env = opts.env;
  if (opts?.output != null) step.output = opts.output;
  if (opts?.label != null) step.label = opts.label;
  return step;
}

// ---------------------------------------------------------------------------
// Gestures
// ---------------------------------------------------------------------------

function coordOrSelector(
  v: string | { text?: string; id?: string; selector?: ElementSelector },
): SelectorValue {
  return typeof v === "string" ? v : selectorValue(v);
}

export function doubleTapOn(opts: {
  text?: string;
  id?: string;
  selector?: ElementSelector;
  optional?: boolean;
  retryTapIfNoChange?: boolean;
  waitUntilVisible?: boolean;
  waitToSettleTimeoutMs?: number;
  label?: string;
}): Step {
  const step: Step = { type: "doubleTapOn" };
  step.selector = selectorValue(opts);
  if (opts.optional) step.optional = true;
  if (opts.retryTapIfNoChange != null) step.retryTapIfNoChange = opts.retryTapIfNoChange;
  if (opts.waitUntilVisible != null) step.waitUntilVisible = opts.waitUntilVisible;
  if (opts.waitToSettleTimeoutMs != null) step.waitToSettleTimeoutMs = opts.waitToSettleTimeoutMs;
  if (opts.label != null) step.label = opts.label;
  return step;
}

export function longPressOn(opts: {
  text?: string;
  id?: string;
  selector?: ElementSelector;
  durationMs?: number;
  optional?: boolean;
  retryTapIfNoChange?: boolean;
  waitUntilVisible?: boolean;
  waitToSettleTimeoutMs?: number;
  label?: string;
}): Step {
  const step: Step = { type: "longPressOn" };
  step.selector = selectorValue(opts);
  if (opts.durationMs != null) step.duration = opts.durationMs;
  if (opts.optional) step.optional = true;
  if (opts.retryTapIfNoChange != null) step.retryTapIfNoChange = opts.retryTapIfNoChange;
  if (opts.waitUntilVisible != null) step.waitUntilVisible = opts.waitUntilVisible;
  if (opts.waitToSettleTimeoutMs != null) step.waitToSettleTimeoutMs = opts.waitToSettleTimeoutMs;
  if (opts.label != null) step.label = opts.label;
  return step;
}

export function dragAndDrop(opts: {
  from: string | { text?: string; id?: string; selector?: ElementSelector };
  to: string | { text?: string; id?: string; selector?: ElementSelector };
  holdDuration?: number;
  duration?: number;
  label?: string;
}): Step {
  const step: Step = { type: "dragAndDrop" };
  step.from = coordOrSelector(opts.from);
  step.to = coordOrSelector(opts.to);
  if (opts.holdDuration != null) step.holdDuration = opts.holdDuration;
  if (opts.duration != null) step.duration = opts.duration;
  if (opts.label != null) step.label = opts.label;
  return step;
}

export function scrollUntilVisible(opts: {
  element: string | { text?: string; id?: string; selector?: ElementSelector };
  from?: string | { text?: string; id?: string; selector?: ElementSelector };
  direction?: string;
  maxScrolls?: number;
  speed?: number;
  visibilityPercentage?: number;
  centerElement?: boolean;
  waitToSettleTimeoutMs?: number;
  optional?: boolean;
  label?: string;
}): Step {
  const step: Step = { type: "scrollUntilVisible" };
  step.element = coordOrSelector(opts.element);
  if (opts.from != null) step.from = coordOrSelector(opts.from);
  if (opts.direction != null) step.direction = opts.direction;
  if (opts.maxScrolls != null) step.maxScrolls = opts.maxScrolls;
  if (opts.speed != null) step.speed = opts.speed;
  if (opts.visibilityPercentage != null) step.visibilityPercentage = opts.visibilityPercentage;
  if (opts.centerElement != null) step.centerElement = opts.centerElement;
  if (opts.waitToSettleTimeoutMs != null) step.waitToSettleTimeoutMs = opts.waitToSettleTimeoutMs;
  if (opts.optional) step.optional = true;
  if (opts.label != null) step.label = opts.label;
  return step;
}

// ---------------------------------------------------------------------------
// Assertions & media
// ---------------------------------------------------------------------------

export function assertScreenshot(opts: {
  path?: string;
  cropOn?: string | { text?: string; id?: string; selector?: ElementSelector };
  thresholdPercentage?: number;
  optional?: boolean;
  label?: string;
}): Step {
  const step: Step = { type: "assertScreenshot" };
  if (opts.path != null) step.path = opts.path;
  if (opts.cropOn != null) {
    step.cropOn = typeof opts.cropOn === "string" ? opts.cropOn : selectorValue(opts.cropOn);
  }
  if (opts.thresholdPercentage != null) step.thresholdPercentage = opts.thresholdPercentage;
  if (opts.optional) step.optional = true;
  if (opts.label != null) step.label = opts.label;
  return step;
}

export function takeScreenshot(opts: {
  path?: string;
  cropOn?: string | { text?: string; id?: string; selector?: ElementSelector };
  label?: string;
}): Step {
  const step: Step = { type: "takeScreenshot" };
  if (opts.path != null) step.path = opts.path;
  if (opts.cropOn != null) {
    step.cropOn = typeof opts.cropOn === "string" ? opts.cropOn : selectorValue(opts.cropOn);
  }
  if (opts.label != null) step.label = opts.label;
  return step;
}

export function copyTextFrom(opts: {
  text?: string;
  id?: string;
  selector?: ElementSelector;
  label?: string;
}): Step {
  const step: Step = { type: "copyTextFrom" };
  step.selector = selectorValue(opts);
  if (opts.label != null) step.label = opts.label;
  return step;
}

export function pasteText(label?: string): Step {
  const step: Step = { type: "pasteText" };
  if (label != null) step.label = label;
  return step;
}

export function setClipboard(text: string, label?: string): Step {
  const step: Step = { type: "setClipboard", text };
  if (label != null) step.label = label;
  return step;
}

// ---------------------------------------------------------------------------
// AI & scripting
// ---------------------------------------------------------------------------

export function assertWithAI(assertion: string, label?: string): Step {
  const step: Step = { type: "assertWithAI", assertion };
  if (label != null) step.label = label;
  return step;
}

export function evalScript(script: string, label?: string): Step {
  const step: Step = { type: "evalScript", script };
  if (label != null) step.label = label;
  return step;
}

export function runScript(opts: {
  script?: string;
  file?: string;
  env?: Record<string, string>;
  label?: string;
}): Step {
  const step: Step = { type: "runScript" };
  if (opts.script != null) step.script = opts.script;
  if (opts.file != null) step.file = opts.file;
  if (opts.env != null) step.env = opts.env;
  if (opts.label != null) step.label = opts.label;
  return step;
}

export function evalBrowserScript(
  script: string,
  opts?: { output?: string; label?: string },
): Step {
  const step: Step = { type: "evalBrowserScript", script };
  if (opts?.output != null) step.output = opts.output;
  if (opts?.label != null) step.label = opts.label;
  return step;
}

// ---------------------------------------------------------------------------
// Device control
// ---------------------------------------------------------------------------

export function setLocation(latitude: string, longitude: string, label?: string): Step {
  const step: Step = { type: "setLocation", latitude, longitude };
  if (label != null) step.label = label;
  return step;
}

export function setAirplaneMode(enabled: boolean, label?: string): Step {
  const step: Step = { type: "setAirplaneMode", enabled };
  if (label != null) step.label = label;
  return step;
}

export function toggleAirplaneMode(label?: string): Step {
  const step: Step = { type: "toggleAirplaneMode" };
  if (label != null) step.label = label;
  return step;
}

export function setNetworkConditions(opts: {
  offline?: boolean;
  latency?: number;
  downloadSpeed?: number;
  uploadSpeed?: number;
  label?: string;
}): Step {
  const step: Step = { type: "setNetworkConditions" };
  if (opts.offline != null) step.offline = opts.offline;
  if (opts.latency != null) step.latency = opts.latency;
  if (opts.downloadSpeed != null) step.downloadSpeed = opts.downloadSpeed;
  if (opts.uploadSpeed != null) step.uploadSpeed = opts.uploadSpeed;
  if (opts.label != null) step.label = opts.label;
  return step;
}

export function openNotifications(label?: string): Step {
  const step: Step = { type: "openNotifications" };
  if (label != null) step.label = label;
  return step;
}

export function setDarkMode(enabled: boolean, label?: string): Step {
  const step: Step = { type: "setDarkMode", enabled };
  if (label != null) step.label = label;
  return step;
}

export function setOrientation(orientation: string, label?: string): Step {
  const step: Step = { type: "setOrientation", orientation };
  if (label != null) step.label = label;
  return step;
}

// ---------------------------------------------------------------------------
// Browser (web platform)
// ---------------------------------------------------------------------------

export function openBrowser(url?: string, label?: string): Step {
  const step: Step = { type: "openBrowser" };
  if (url != null) step.url = url;
  if (label != null) step.label = label;
  return step;
}

export function switchTab(opts: {
  tabLabel?: string;
  index?: number;
  url?: string;
  label?: string;
}): Step {
  const step: Step = { type: "switchTab" };
  if (opts.tabLabel != null) step.tabLabel = opts.tabLabel;
  if (opts.index != null) step.index = opts.index;
  if (opts.url != null) step.url = opts.url;
  if (opts.label != null) step.label = opts.label;
  return step;
}

export function closeTab(label?: string): Step {
  const step: Step = { type: "closeTab" };
  if (label != null) step.label = label;
  return step;
}

export function getConsoleLogs(output: string, label?: string): Step {
  const step: Step = { type: "getConsoleLogs", output };
  if (label != null) step.label = label;
  return step;
}

export function clearConsoleLogs(label?: string): Step {
  const step: Step = { type: "clearConsoleLogs" };
  if (label != null) step.label = label;
  return step;
}

export function assertNoJSErrors(label?: string): Step {
  const step: Step = { type: "assertNoJSErrors" };
  if (label != null) step.label = label;
  return step;
}

export function mockNetwork(opts: {
  url: string;
  method?: string;
  response: { status?: number; headers?: Record<string, string>; body?: string };
  label?: string;
}): Step {
  const step: Step = { type: "mockNetwork", url: opts.url };
  if (opts.method != null) step.method = opts.method;
  const response: Record<string, unknown> = {};
  if (opts.response.status != null) response.status = opts.response.status;
  if (opts.response.headers != null) response.headers = opts.response.headers;
  if (opts.response.body != null) response.body = opts.response.body;
  step.response = response;
  if (opts.label != null) step.label = opts.label;
  return step;
}
