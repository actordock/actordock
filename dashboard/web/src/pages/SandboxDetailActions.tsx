import { useState } from "react";
import { useNavigate } from "react-router-dom";
import {
  createSandboxSnapshot,
  killSandbox,
  pauseSandbox,
  refreshSandboxTTL,
  resumeSandbox,
  updateSandboxNetwork,
} from "../api/platform";
import type { SandboxDetail } from "../api/types";
import { ConfirmDialog, Modal } from "../components/Modal/Modal";
import { useToast } from "../components/Toast/useToast";
import { SandboxNetworkDrawer } from "./SandboxNetworkDrawer";

type SandboxDetailActionsProps = {
  sandbox: SandboxDetail;
  reload: () => void;
};

export function SandboxDetailActions({
  sandbox,
  reload,
}: SandboxDetailActionsProps) {
  const navigate = useNavigate();
  const { pushToast } = useToast();
  const [busy, setBusy] = useState(false);
  const [killOpen, setKillOpen] = useState(false);
  const [extendOpen, setExtendOpen] = useState(false);
  const [snapshotOpen, setSnapshotOpen] = useState(false);
  const [networkOpen, setNetworkOpen] = useState(false);
  const [extendSec, setExtendSec] = useState("300");
  const [snapshotName, setSnapshotName] = useState("");

  const isRunning = sandbox.state === "running";
  const isPaused = sandbox.state === "paused";
  const canManage = isRunning || isPaused;

  async function runAction(
    label: string,
    action: () => Promise<void>,
    onSuccess?: () => void,
  ): Promise<boolean> {
    setBusy(true);
    try {
      await action();
      pushToast(`${label} succeeded`, "success");
      onSuccess?.();
      reload();
      return true;
    } catch (err) {
      pushToast(
        err instanceof Error ? err.message : `${label} failed`,
        "error",
      );
      reload();
      return false;
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <div className="sandbox-detail-actions">
        <button
          type="button"
          className="btn btn--ghost"
          disabled={busy}
          onClick={() => setKillOpen(true)}
        >
          Kill
        </button>
        <button
          type="button"
          className="btn btn--ghost"
          disabled={busy || !isRunning}
          onClick={() =>
            void runAction("Pause", () => pauseSandbox(sandbox.sandboxID))
          }
        >
          Pause
        </button>
        <button
          type="button"
          className="btn btn--ghost"
          disabled={busy || !isPaused}
          onClick={() =>
            void runAction("Resume", () => resumeSandbox(sandbox.sandboxID))
          }
        >
          Resume
        </button>
        <button
          type="button"
          className="btn btn--ghost"
          disabled={busy || !canManage}
          onClick={() => setExtendOpen(true)}
        >
          Extend timeout
        </button>
        <button
          type="button"
          className="btn btn--ghost"
          disabled={busy || !isRunning}
          onClick={() => setSnapshotOpen(true)}
        >
          Snapshot
        </button>
        <button
          type="button"
          className="btn btn--ghost"
          disabled={busy || !canManage}
          onClick={() => setNetworkOpen(true)}
        >
          Network
        </button>
      </div>

      <ConfirmDialog
        open={killOpen}
        title="Kill sandbox"
        message={`Permanently delete sandbox ${sandbox.sandboxID}? This cannot be undone.`}
        confirmLabel="Kill sandbox"
        danger
        busy={busy}
        onCancel={() => setKillOpen(false)}
        onConfirm={() => {
          void runAction(
            "Kill",
            () => killSandbox(sandbox.sandboxID),
            () => navigate("/sandboxes"),
          ).then((ok) => {
            if (ok) {
              setKillOpen(false);
            }
          });
        }}
      />

      <Modal
        open={extendOpen}
        title="Extend timeout"
        subtitle="Add seconds to the sandbox TTL."
        onClose={busy ? () => undefined : () => setExtendOpen(false)}
        footer={
          <>
            <button
              type="button"
              className="btn btn--ghost"
              onClick={() => setExtendOpen(false)}
              disabled={busy}
            >
              Cancel
            </button>
            <button
              type="button"
              className="btn btn--primary"
              disabled={busy}
              onClick={() => {
                const duration = Number(extendSec);
                if (!Number.isFinite(duration) || duration <= 0) {
                  pushToast("Duration must be a positive number", "error");
                  return;
                }
                void runAction("Extend timeout", () =>
                  refreshSandboxTTL(sandbox.sandboxID, duration),
                ).then((ok) => {
                  if (ok) {
                    setExtendOpen(false);
                  }
                });
              }}
            >
              {busy ? "Saving…" : "Extend"}
            </button>
          </>
        }
      >
        <label className="modal-field">
          <span>Additional seconds</span>
          <input
            type="number"
            min={1}
            value={extendSec}
            onChange={(e) => setExtendSec(e.target.value)}
            disabled={busy}
          />
        </label>
      </Modal>

      <Modal
        open={snapshotOpen}
        title="Create snapshot"
        subtitle="The sandbox will be paused after the snapshot is taken."
        onClose={busy ? () => undefined : () => setSnapshotOpen(false)}
        footer={
          <>
            <button
              type="button"
              className="btn btn--ghost"
              onClick={() => setSnapshotOpen(false)}
              disabled={busy}
            >
              Cancel
            </button>
            <button
              type="button"
              className="btn btn--primary"
              disabled={busy}
              onClick={() => {
                const name = snapshotName.trim();
                void runAction("Snapshot", async () => {
                  await createSandboxSnapshot(
                    sandbox.sandboxID,
                    name || undefined,
                  );
                }).then((ok) => {
                  if (ok) {
                    setSnapshotOpen(false);
                    setSnapshotName("");
                  }
                });
              }}
            >
              {busy ? "Creating…" : "Create snapshot"}
            </button>
          </>
        }
      >
        <label className="modal-field">
          <span>Name (optional)</span>
          <input
            type="text"
            value={snapshotName}
            onChange={(e) => setSnapshotName(e.target.value)}
            disabled={busy}
            placeholder="my-snapshot"
          />
        </label>
      </Modal>

      <SandboxNetworkDrawer
        open={networkOpen}
        sandbox={sandbox}
        busy={busy}
        onClose={() => setNetworkOpen(false)}
        onSave={async (body) => {
          setBusy(true);
          try {
            await updateSandboxNetwork(sandbox.sandboxID, body);
            pushToast("Network updated", "success");
            setNetworkOpen(false);
            reload();
          } catch (err) {
            pushToast(
              err instanceof Error ? err.message : "Network update failed",
              "error",
            );
            reload();
            throw err;
          } finally {
            setBusy(false);
          }
        }}
      />
    </>
  );
}
