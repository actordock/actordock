import { useEffect, useState } from "react";
import { fetchHealth } from "../api/platform";
import type { ConnectionStatus } from "../components/AppShell/AppShell";

export function useConnectionStatus(pollMs = 30_000): ConnectionStatus {
  const [status, setStatus] = useState<ConnectionStatus>("checking");

  useEffect(() => {
    let cancelled = false;

    const check = async () => {
      try {
        await fetchHealth();
        if (!cancelled) {
          setStatus("connected");
        }
      } catch {
        if (!cancelled) {
          setStatus("disconnected");
        }
      }
    };

    void check();
    const id = window.setInterval(() => void check(), pollMs);
    return () => {
      cancelled = true;
      window.clearInterval(id);
    };
  }, [pollMs]);

  return status;
}
