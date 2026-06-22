import { useEffect, useRef, useState } from "react";
import { Terminal } from "xterm";
import { FitAddon } from "@xterm/addon-fit";
import { connectSandbox } from "../api/platform";
import { startSandboxTerminal } from "../lib/sandboxTerminal";
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

export function SandboxDetailTerminal() {
  const { sandbox, reload } = useSandboxDetail();
  const containerRef = useRef<HTMLDivElement>(null);
  const terminalRef = useRef<Terminal | null>(null);
  const sessionRef = useRef<Awaited<ReturnType<typeof startSandboxTerminal>> | null>(
    null,
  );
  const resizeObserverRef = useRef<ResizeObserver | null>(null);
  const [state, setState] = useState<TerminalState>({ kind: "idle" });
  const [copied, setCopied] = useState(false);

  const canConnect =
    sandbox?.state === "running" || sandbox?.state === "paused";

  const connectUrl = sandbox
    ? buildSandboxConnectUrl(sandbox.sandboxID, sandbox.domain ?? "localhost")
    : "";

  useEffect(() => {
    if (!sandbox || !canConnect) {
      return;
    }

    const sandboxID = sandbox.sandboxID;
    let disposed = false;

    async function openTerminal() {
      setState({ kind: "connecting" });
      try {
        await connectSandbox(sandboxID, CONNECT_TIMEOUT_SEC);
        if (disposed || !containerRef.current) {
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
        terminal.open(containerRef.current);
        fitAddon.fit();

        const session = await startSandboxTerminal({
          routerBaseUrl: getRouterBaseUrl(),
          sandboxID,
          cols: terminal.cols,
          rows: terminal.rows,
          onData: (data) => {
            terminal.write(data);
          },
          onExit: (message) => {
            if (!disposed) {
              setState({
                kind: "exited",
                message: message ?? "Terminal session ended",
              });
            }
          },
        });

        if (disposed) {
          await session.dispose();
          terminal.dispose();
          return;
        }

        terminalRef.current = terminal;
        sessionRef.current = session;

        terminal.onData((data) => {
          void session.write(data);
        });

        const resizeObserver = new ResizeObserver(() => {
          fitAddon.fit();
          void session.resize(terminal.cols, terminal.rows);
        });
        resizeObserver.observe(containerRef.current);
        resizeObserverRef.current = resizeObserver;

        terminal.writeln("Connected to sandbox shell.");
        setState({ kind: "connected" });
      } catch (err) {
        if (!disposed) {
          setState({
            kind: "error",
            message:
              err instanceof Error ? err.message : "Failed to open terminal",
          });
        }
      }
    }

    void openTerminal();

    return () => {
      disposed = true;
      resizeObserverRef.current?.disconnect();
      resizeObserverRef.current = null;
      void sessionRef.current?.dispose();
      sessionRef.current = null;
      terminalRef.current?.dispose();
      terminalRef.current = null;
    };
  }, [sandbox, canConnect]);

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
        <CopyConnectButton connectUrl={connectUrl} copied={copied} onCopy={setCopied} />
      </div>

      {state.kind === "connecting" ? (
        <p className="sandbox-terminal-muted">Connecting terminal…</p>
      ) : null}

      {state.kind === "error" ? (
        <div className="sandbox-terminal-error" role="alert">
          {state.message}
          <p className="sandbox-terminal-muted">
            Fallback: use the connect URL with router port-forward on :8081.
          </p>
        </div>
      ) : null}

      {state.kind === "exited" ? (
        <p className="sandbox-terminal-muted">{state.message}</p>
      ) : null}

      <div
        ref={containerRef}
        className={`sandbox-terminal-surface${state.kind === "connected" ? "" : " sandbox-terminal-surface--hidden"}`}
      />
    </div>
  );
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
