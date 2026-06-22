import { useCallback, useEffect, useRef, useState } from "react";
import { Terminal } from "xterm";
import { FitAddon } from "@xterm/addon-fit";
import { connectSandbox } from "../api/platform";
import { startSandboxTerminal } from "../lib/sandboxTerminal";
import { writePtyToTerminal } from "../lib/ptyOutput";
import {
  buildSandboxConnectUrl,
  getRouterBaseUrl,
} from "../lib/routerUrl";
import { useSandboxDetail } from "./sandboxDetailContext";
import "xterm/css/xterm.css";
import "./SandboxDetailTerminal.css";

type TerminalState =
  | { kind: "idle" }
  | { kind: "connecting" }
  | { kind: "connected" }
  | { kind: "exited"; message?: string }
  | { kind: "error"; message: string };

const CONNECT_TIMEOUT_SEC = 600;

type Runtime = {
  terminal: Terminal;
  fitAddon: FitAddon;
  resizeObserver: ResizeObserver;
  dispose: () => Promise<void>;
};

export function SandboxDetailTerminal() {
  const { sandbox, reload } = useSandboxDetail();
  const containerRef = useRef<HTMLDivElement>(null);
  const runtimeRef = useRef<Runtime | null>(null);
  const connectGenRef = useRef(0);
  const [state, setState] = useState<TerminalState>({ kind: "idle" });
  const [copied, setCopied] = useState(false);
  const [connectAttempt, setConnectAttempt] = useState(0);

  const sandboxID = sandbox?.sandboxID ?? "";
  const canConnect =
    sandbox?.state === "running" || sandbox?.state === "paused";

  const connectUrl = sandbox
    ? buildSandboxConnectUrl(sandboxID, sandbox.domain ?? "localhost")
    : "";

  const startConnection = useCallback(() => {
    setConnectAttempt((attempt) => attempt + 1);
  }, []);

  const teardownRuntime = useCallback(async () => {
    const runtime = runtimeRef.current;
    runtimeRef.current = null;
    if (!runtime) {
      return;
    }
    runtime.resizeObserver.disconnect();
    await runtime.dispose();
    runtime.terminal.dispose();
  }, []);

  useEffect(() => {
    if (!sandboxID || !canConnect || connectAttempt === 0) {
      return;
    }

    const container = containerRef.current;
    if (!container) {
      return;
    }

    const containerEl = container;

    const gen = ++connectGenRef.current;
    let inputDisposable: { dispose: () => void } | null = null;

    async function connect() {
      setState({ kind: "connecting" });
      await teardownRuntime();

      if (gen !== connectGenRef.current) {
        return;
      }

      const terminal = new Terminal({
        cursorBlink: true,
        fontFamily: "JetBrains Mono, ui-monospace, monospace",
        fontSize: 13,
        theme: {
          background: "#f0ebe3",
          foreground: "#2c2a26",
          cursor: "#8a6f4a",
        },
      });
      const fitAddon = new FitAddon();
      terminal.loadAddon(fitAddon);
      terminal.open(containerEl);
      fitAddon.fit();

      try {
        await connectSandbox(sandboxID, CONNECT_TIMEOUT_SEC);
        if (gen !== connectGenRef.current) {
          terminal.dispose();
          return;
        }

        fitAddon.fit();

        const session = await startSandboxTerminal({
          routerBaseUrl: getRouterBaseUrl(),
          sandboxID,
          cols: terminal.cols,
          rows: terminal.rows,
          onData: (data) => {
            if (gen === connectGenRef.current) {
              writePtyToTerminal((chunk) => terminal.write(chunk), data);
            }
          },
          onExit: (message) => {
            if (gen !== connectGenRef.current) {
              return;
            }
            setState({
              kind: "exited",
              message: message ?? "Terminal session ended",
            });
          },
        });

        if (gen !== connectGenRef.current) {
          await session.dispose();
          terminal.dispose();
          return;
        }

        inputDisposable = terminal.onData((data) => {
          void session.write(data);
        });

        const resizeObserver = new ResizeObserver(() => {
          fitAddon.fit();
          void session.resize(terminal.cols, terminal.rows);
        });
        resizeObserver.observe(containerEl);

        runtimeRef.current = {
          terminal,
          fitAddon,
          resizeObserver,
          dispose: () => session.dispose(),
        };

        fitAddon.fit();
        await session.resize(terminal.cols, terminal.rows);
        terminal.focus();
        setState({ kind: "connected" });
      } catch (err) {
        terminal.dispose();
        if (gen !== connectGenRef.current) {
          return;
        }
        setState({
          kind: "error",
          message:
            err instanceof Error ? err.message : "Failed to open terminal",
        });
      }
    }

    void connect();

    return () => {
      connectGenRef.current += 1;
      inputDisposable?.dispose();
      void teardownRuntime();
    };
  }, [sandboxID, canConnect, connectAttempt, teardownRuntime]);

  useEffect(() => {
    if (!sandboxID || !canConnect) {
      return;
    }
    const timer = window.setTimeout(() => {
      setConnectAttempt((attempt) => (attempt === 0 ? 1 : attempt));
    }, 0);
    return () => window.clearTimeout(timer);
  }, [sandboxID, canConnect]);

  useEffect(() => {
    return () => {
      connectGenRef.current += 1;
      void teardownRuntime();
    };
  }, [teardownRuntime]);

  if (!sandbox) {
    return null;
  }

  if (!canConnect) {
    return (
      <div className="sandbox-terminal-panel">
        <p className="sandbox-terminal-muted">
          Terminal is available when the sandbox is running or paused.
        </p>
        <CopyConnectButton connectUrl={connectUrl} copied={copied} onCopy={setCopied} />
      </div>
    );
  }

  return (
    <div className="sandbox-terminal-panel">
      <div className="sandbox-terminal-toolbar">
        <span className="sandbox-terminal-status" data-state={state.kind}>
          {terminalStatusLabel(state)}
        </span>
        <span className="sandbox-terminal-muted">
          Router: {getRouterBaseUrl()}
        </span>
        <button
          type="button"
          className="btn btn--ghost"
          onClick={() => reload()}
        >
          Refresh sandbox
        </button>
        <button type="button" className="btn btn--ghost" onClick={startConnection}>
          {state.kind === "connected" ? "Reconnect" : "Connect"}
        </button>
        <CopyConnectButton connectUrl={connectUrl} copied={copied} onCopy={setCopied} />
      </div>

      {state.kind === "error" ? (
        <div className="sandbox-terminal-error" role="alert">
          {state.message}
          <p className="sandbox-terminal-muted">
            Ensure platform (:8080) and router (:8081) port-forwards are running.
          </p>
        </div>
      ) : null}

      {state.kind === "exited" ? (
        <p className="sandbox-terminal-muted">{state.message}</p>
      ) : null}

      <div ref={containerRef} className="sandbox-terminal-surface" />
    </div>
  );
}

function terminalStatusLabel(state: TerminalState): string {
  switch (state.kind) {
    case "idle":
      return "Idle";
    case "connecting":
      return "Connecting…";
    case "connected":
      return "Connected";
    case "exited":
      return "Session ended";
    case "error":
      return "Error";
    default:
      return "";
  }
}

function CopyConnectButton({
  connectUrl,
  copied,
  onCopy,
}: {
  connectUrl: string;
  copied: boolean;
  onCopy: (copied: boolean) => void;
}) {
  return (
    <button
      type="button"
      className="btn btn--ghost"
      onClick={() => {
        void navigator.clipboard.writeText(connectUrl).then(() => {
          onCopy(true);
          window.setTimeout(() => onCopy(false), 1500);
        });
      }}
    >
      {copied ? "Copied connect URL" : "Copy connect URL"}
    </button>
  );
}
