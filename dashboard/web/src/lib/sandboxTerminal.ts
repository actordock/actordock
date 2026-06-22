import { Code, ConnectError, createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { Process, Signal } from "../envd/process/process_pb";
import { decodePtyOutput } from "./ptyOutput";
import { SANDBOX_ID_HEADER } from "./routerUrl";

export const DEFAULT_TERMINAL_COLS = 80;
export const DEFAULT_TERMINAL_ROWS = 24;

export type TerminalSession = {
  pid: number;
  write: (data: string) => Promise<void>;
  resize: (cols: number, rows: number) => Promise<void>;
  dispose: () => Promise<void>;
};

type StartTerminalOpts = {
  routerBaseUrl: string;
  sandboxID: string;
  cols: number;
  rows: number;
  onData: (data: Uint8Array) => void;
  onExit?: (message?: string) => void;
};

export function normalizeTerminalSize(cols: number, rows: number) {
  return {
    cols: cols > 0 ? cols : DEFAULT_TERMINAL_COLS,
    rows: rows > 0 ? rows : DEFAULT_TERMINAL_ROWS,
  };
}

export async function startSandboxTerminal(
  opts: StartTerminalOpts,
): Promise<TerminalSession> {
  const size = normalizeTerminalSize(opts.cols, opts.rows);
  const transport = createConnectTransport({
    baseUrl: opts.routerBaseUrl,
    useBinaryFormat: false,
    fetch: (url, init) =>
      fetch(url, {
        ...init,
        redirect: "follow",
      }),
    interceptors: [
      (next) => async (req) => {
        req.header.set(SANDBOX_ID_HEADER, opts.sandboxID);
        return next(req);
      },
    ],
  });
  const client = createClient(Process, transport);
  const abort = new AbortController();

  const events = client.start(
    {
      process: {
        cmd: "/bin/sh",
        args: ["-i"],
        envs: {
          TERM: "xterm-256color",
          LANG: "C.UTF-8",
          LC_ALL: "C.UTF-8",
        },
      },
      pty: {
        size,
      },
    },
    { signal: abort.signal },
  );

  let pid = 0;
  let streamError: Error | undefined;

  const streamDone = consumeTerminalStream(events, {
    onStart: (nextPid) => {
      pid = nextPid;
    },
    onData: opts.onData,
    onExit: (message) => {
      opts.onExit?.(message);
    },
    onError: (err) => {
      streamError = err;
      if (
        err instanceof ConnectError &&
        err.code === Code.Canceled
      ) {
        return;
      }
      opts.onExit?.(formatTerminalError(err));
    },
  });

  const deadline = Date.now() + 15_000;
  while (pid === 0 && !streamError && Date.now() < deadline) {
    await sleep(50);
  }

  if (streamError) {
    abort.abort();
    await streamDone.catch(() => undefined);
    throw streamError;
  }
  if (pid === 0) {
    abort.abort();
    await streamDone.catch(() => undefined);
    throw new Error("Timed out waiting for terminal start event");
  }

  return {
    pid,
    write: async (data: string) => {
      await client.sendInput({
        process: {
          selector: { case: "pid", value: pid },
        },
        input: {
          input: {
            case: "pty",
            value: new TextEncoder().encode(data),
          },
        },
      });
    },
    resize: async (cols: number, rows: number) => {
      const next = normalizeTerminalSize(cols, rows);
      try {
        await client.update({
          process: {
            selector: { case: "pid", value: pid },
          },
          pty: {
            size: next,
          },
        });
      } catch (err) {
        if (!isUnimplementedError(err)) {
          throw err;
        }
      }
    },
    dispose: async () => {
      abort.abort();
      try {
        await client.sendSignal({
          process: {
            selector: { case: "pid", value: pid },
          },
          signal: Signal.SIGKILL,
        });
      } catch {
        // process may already be gone
      }
      await streamDone.catch(() => undefined);
    },
  };
}

async function consumeTerminalStream(
  events: AsyncIterable<{ event?: { event?: { case?: string; value?: unknown } } }>,
  handlers: {
    onStart: (pid: number) => void;
    onData: (data: Uint8Array) => void;
    onExit: (message?: string) => void;
    onError: (err: Error) => void;
  },
) {
  try {
    for await (const message of events) {
      const event = message.event?.event;
      if (!event) {
        continue;
      }
      if (event.case === "start") {
        const start = event.value as { pid?: number };
        if (start.pid) {
          handlers.onStart(start.pid);
        }
        continue;
      }
      if (event.case === "data") {
        const data = event.value as {
          output?: { case?: string; value?: Uint8Array };
        };
        if (data.output?.case === "pty" && data.output.value) {
          handlers.onData(decodePtyOutput(data.output.value));
        }
        continue;
      }
      if (event.case === "end") {
        const end = event.value as { exitCode?: number; error?: string };
        handlers.onExit(
          end.error ||
            (end.exitCode !== undefined ? `Process exited (${end.exitCode})` : undefined),
        );
        break;
      }
    }
  } catch (err) {
    handlers.onError(
      err instanceof Error ? err : new Error("Terminal stream failed"),
    );
  }
}

function isUnimplementedError(err: unknown): boolean {
  return err instanceof ConnectError && err.code === Code.Unimplemented;
}

function formatTerminalError(err: unknown): string {
  if (err instanceof ConnectError) {
    if (err.code === Code.Canceled) {
      return "Terminal connection canceled";
    }
    if (err.code === Code.Unavailable || err.code === Code.NotFound) {
      return "Sandbox is not reachable. Ensure router port-forward is running on :8081.";
    }
    return err.message;
  }
  if (err instanceof Error) {
    return err.message;
  }
  return "Terminal connection failed";
}

function sleep(ms: number) {
  return new Promise((resolve) => window.setTimeout(resolve, ms));
}
