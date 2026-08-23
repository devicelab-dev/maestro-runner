/**
 * Shared test setup — auto-start maestro-runner server when needed.
 *
 * Equivalent of Python conftest.py.
 *
 * Supports Jest parallel execution: each worker gets its own server
 * on a unique port, targeting a specific device via JEST_WORKER_ID.
 *
 * Env vars:
 *   MAESTRO_SERVER_URL   (default: http://localhost:9999)
 *   MAESTRO_PLATFORM     (default: android)
 *   MAESTRO_RUNNER_BIN   (path to binary, auto-detected by default)
 */

import { ChildProcess, execSync, spawn } from "child_process";
import * as path from "path";
import * as fs from "fs";
import { MaestroClient } from "../src";

const BASE_SERVER_URL = process.env.MAESTRO_SERVER_URL ?? "http://localhost:9999";
const PLATFORM = process.env.MAESTRO_PLATFORM ?? "android";
const EXPLICIT_DEVICE_ID = process.env.MAESTRO_DEVICE_ID;
const BASE_PORT = parseInt(new URL(BASE_SERVER_URL).port || "9999", 10);

// Jest assigns JEST_WORKER_ID starting at 1 for each parallel worker
const WORKER_ID = parseInt(process.env.JEST_WORKER_ID ?? "1", 10);
const WORKER_NAME = `jw${Math.max(WORKER_ID - 1, 0)}`;
const SERVER_PORT = BASE_PORT + WORKER_ID - 1;
const SERVER_URL = `http://localhost:${SERVER_PORT}`;

const DEFAULT_BIN = path.resolve(__dirname, "..", "..", "..", "maestro-runner");
const MAESTRO_RUNNER_BIN = process.env.MAESTRO_RUNNER_BIN ?? DEFAULT_BIN;
const REPORTS_DIR = path.resolve(__dirname, "..", "reports");

let runId = "";
let serverLogPath = "";
let serverLogStream: fs.WriteStream | undefined;
let runLogPath = "";
let runLogStream: fs.WriteStream | undefined;

function utcTimestamp(): string {
  const date = new Date();
  const pad = (n: number): string => String(n).padStart(2, "0");
  return [
    date.getUTCFullYear(),
    pad(date.getUTCMonth() + 1),
    pad(date.getUTCDate()),
  ].join("") +
    "-" +
    [pad(date.getUTCHours()), pad(date.getUTCMinutes()), pad(date.getUTCSeconds())].join("");
}

function persistLatestServerMetadata(mode: "spawned" | "reused-existing-server"): void {
  fs.mkdirSync(REPORTS_DIR, { recursive: true });
  const latestPath = path.join(REPORTS_DIR, "server-latest.json");
  let payload: { updatedAt: string; workers: Record<string, Record<string, string>> } = {
    updatedAt: new Date().toISOString(),
    workers: {},
  };

  if (fs.existsSync(latestPath)) {
    try {
      payload = JSON.parse(fs.readFileSync(latestPath, "utf-8"));
    } catch {
      payload = {
        updatedAt: new Date().toISOString(),
        workers: {},
      };
    }
  }

  payload.workers[WORKER_NAME] = {
    workerId: WORKER_NAME,
    runId,
    mode,
    serverUrl: SERVER_URL,
    serverPort: String(SERVER_PORT),
    serverLogPath,
    ...(assignedDevice ? { deviceId: assignedDevice } : {}),
    startedAt: new Date().toISOString(),
  };
  payload.updatedAt = new Date().toISOString();
  fs.writeFileSync(latestPath, `${JSON.stringify(payload, null, 2)}\n`, "utf-8");
}

function currentNodeId(): string {
  const maybeExpect = (globalThis as { expect?: { getState?: () => { currentTestName?: string } } }).expect;
  const name = maybeExpect?.getState?.().currentTestName;
  return name && name.trim().length > 0 ? name : "-";
}

function appendRunLog(level: "INFO" | "DEBUG" | "WARN", message: string): void {
  if (!runLogStream) {
    return;
  }
  const ts = new Date().toISOString();
  runLogStream.write(
    `${ts} [${level}] [worker=${WORKER_NAME}] [node=${currentNodeId()}] ${message}\n`,
  );
}

function tailFile(filePath: string, maxLines = 120): string {
  if (!fs.existsSync(filePath)) {
    return "";
  }
  const lines = fs.readFileSync(filePath, "utf-8").split(/\r?\n/);
  return lines.slice(Math.max(0, lines.length - maxLines)).join("\n");
}

function writeArtifactSummary(status: "passed" | "failed"): void {
  if (!runId) {
    return;
  }

  const artifacts: Array<{ name: string; path: string; sizeBytes: number }> = [];
  const known = [
    path.join(REPORTS_DIR, "report.html"),
    path.join(REPORTS_DIR, "junit-report.xml"),
    serverLogPath,
    runLogPath,
  ];

  for (const artifactPath of known) {
    if (!artifactPath || !fs.existsSync(artifactPath)) {
      continue;
    }
    const stats = fs.statSync(artifactPath);
    artifacts.push({
      name: path.basename(artifactPath),
      path: artifactPath,
      sizeBytes: stats.size,
    });
  }

  const summary: Record<string, unknown> = {
    runId,
    workerId: WORKER_NAME,
    platform: PLATFORM,
    serverUrl: SERVER_URL,
    serverPort: String(SERVER_PORT),
    sessionStatus: status,
    generatedAt: new Date().toISOString(),
    artifacts,
    tails: {
      server: tailFile(serverLogPath),
      jest: tailFile(runLogPath),
    },
  };

  const summaryPath = path.join(REPORTS_DIR, `artifact-summary-${runId}.json`);
  fs.writeFileSync(summaryPath, `${JSON.stringify(summary, null, 2)}\n`, "utf-8");
}

async function serverIsReady(url: string): Promise<boolean> {
  try {
    const resp = await fetch(`${url}/status`, {
      signal: AbortSignal.timeout(2000),
    });
    return resp.ok;
  } catch {
    return false;
  }
}

/** Sleeps for `ms` milliseconds. */
function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

/** Discover connected Android device serials via adb. */
function discoverDevices(): string[] {
  try {
    const out = execSync("adb devices", { encoding: "utf-8" });
    return out
      .split("\n")
      .slice(1)
      .filter((line) => line.match(/^\S+\s+device$/))
      .map((line) => line.split("\t")[0]);
  } catch {
    return [];
  }
}

let serverProcess: ChildProcess | undefined;
let sharedClient: MaestroClient | undefined;
let assignedDevice: string | undefined;

function serverCommand(port: number, deviceId?: string): string[] {
  const command = ["--platform", PLATFORM];
  if (deviceId) {
    command.push("--device", deviceId);
  }
  command.push("server", "--port", String(port));
  return command;
}

/**
 * Ensure a maestro-runner server is available. Starts one if needed.
 * Returns the server URL.
 */
export async function ensureServer(): Promise<string> {
  runId = `${utcTimestamp()}-${WORKER_NAME}-${process.pid}`;
  fs.mkdirSync(REPORTS_DIR, { recursive: true });
  serverLogPath = path.join(REPORTS_DIR, `server-run-${utcTimestamp()}-${WORKER_NAME}.log`);
  runLogPath = path.join(REPORTS_DIR, `jest-run-${runId}.log`);
  if (!runLogStream) {
    runLogStream = fs.createWriteStream(runLogPath, { flags: "a", encoding: "utf-8" });
  }
  appendRunLog("INFO", `run initialized runId=${runId} platform=${PLATFORM}`);

  if (await serverIsReady(SERVER_URL)) {
    fs.writeFileSync(
      serverLogPath,
      `runId=${runId} workerId=${WORKER_NAME} mode=reused-existing-server\n`,
      "utf-8",
    );
    persistLatestServerMetadata("reused-existing-server");
    appendRunLog("INFO", "reusing existing maestro-runner server");
    return SERVER_URL;
  }

  const binary = MAESTRO_RUNNER_BIN;
  if (!fs.existsSync(binary)) {
    throw new Error(
      `maestro-runner binary not found at ${binary}. ` +
        "Set MAESTRO_RUNNER_BIN or add it to PATH.",
    );
  }

  // Discover devices and assign one to this worker
  if (EXPLICIT_DEVICE_ID) {
    assignedDevice = EXPLICIT_DEVICE_ID;
  } else if (PLATFORM === "android") {
    const devices = discoverDevices();
    const idx = WORKER_ID - 1;
    if (idx < devices.length) {
      assignedDevice = devices[idx];
    }
  }

  serverProcess = spawn(
    binary,
    serverCommand(SERVER_PORT, assignedDevice),
    {
      stdio: "pipe",
      env: {
        ...process.env,
        MAESTRO_WORKER_ID: WORKER_NAME,
        ...(assignedDevice && PLATFORM === "android" ? { ANDROID_SERIAL: assignedDevice } : {}),
      },
    },
  );

  serverLogStream = fs.createWriteStream(serverLogPath, { flags: "a", encoding: "utf-8" });
  serverLogStream.write(
    `runId=${runId} workerId=${WORKER_NAME} platform=${PLATFORM}` +
      `${assignedDevice ? ` deviceId=${assignedDevice}` : ""}\n`,
  );
  serverProcess.stdout?.pipe(serverLogStream);
  serverProcess.stderr?.pipe(serverLogStream);
  persistLatestServerMetadata("spawned");
  appendRunLog("INFO", `spawned maestro-runner server on ${SERVER_URL}`);

  const deadline = Date.now() + 30_000;
  while (Date.now() < deadline) {
    if (serverProcess.exitCode != null) {
      throw new Error(
        `maestro-runner exited early (code ${serverProcess.exitCode})`,
      );
    }
    if (await serverIsReady(SERVER_URL)) return SERVER_URL;
    await sleep(500);
  }

  serverProcess.kill();
  throw new Error("maestro-runner server did not become ready within 30 s");
}

/** Get a shared MaestroClient, creating session on first call. */
export async function getClient(): Promise<MaestroClient> {
  if (sharedClient) return sharedClient;

  const url = await ensureServer();
  const client = new MaestroClient(url);
  const caps: Record<string, unknown> = { platformName: PLATFORM };
  if (assignedDevice) {
    caps.deviceId = assignedDevice;
  }
  await client.createSession(caps);
  appendRunLog("INFO", `client session created for ${SERVER_URL}`);
  sharedClient = client;
  return client;
}

/** Tear down the shared client and server process. */
export async function teardown(): Promise<void> {
  let failed = false;
  if (sharedClient) {
    try {
      await sharedClient.close();
    } catch {
      failed = true;
      appendRunLog("WARN", "client close failed during teardown");
    }
    sharedClient = undefined;
  }
  if (serverProcess) {
    // Detach the child's output pipes *before* closing the log stream so a
    // final flush as the process exits can't write to an already-ended stream
    // (which throws "write after end" and crashes the test runner).
    serverProcess.stdout?.unpipe(serverLogStream);
    serverProcess.stderr?.unpipe(serverLogStream);
    serverProcess.kill();
    serverProcess = undefined;
  }
  if (serverLogStream) {
    serverLogStream.write(`terminated runId=${runId} workerId=${WORKER_NAME}\n`);
    serverLogStream.end();
    serverLogStream = undefined;
  }
  appendRunLog("INFO", "teardown completed");
  writeArtifactSummary(failed ? "failed" : "passed");
  if (runLogStream) {
    runLogStream.end();
    runLogStream = undefined;
  }
}
