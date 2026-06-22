import { useState } from "react";
import type { SandboxDetail, SandboxNetworkUpdate } from "../api/types";
import { Modal } from "../components/Modal/Modal";

type SandboxNetworkDrawerProps = {
  open: boolean;
  sandbox: SandboxDetail;
  busy: boolean;
  onClose: () => void;
  onSave: (body: SandboxNetworkUpdate) => Promise<void>;
};

function joinList(values?: string[]): string {
  return values?.join(", ") ?? "";
}

function parseList(raw: string): string[] {
  return raw
    .split(",")
    .map((part) => part.trim())
    .filter(Boolean);
}

function NetworkForm({
  sandbox,
  busy,
  onClose,
  onSave,
}: Omit<SandboxNetworkDrawerProps, "open">) {
  const [allowOut, setAllowOut] = useState(() =>
    joinList(sandbox.network?.allowOut),
  );
  const [denyOut, setDenyOut] = useState(() =>
    joinList(sandbox.network?.denyOut),
  );
  const [allowInternet, setAllowInternet] = useState(
    () => sandbox.allowInternetAccess ?? true,
  );
  const [error, setError] = useState("");

  async function handleSave() {
    setError("");
    const body: SandboxNetworkUpdate = {
      allowOut: parseList(allowOut),
      denyOut: parseList(denyOut),
      allow_internet_access: allowInternet,
    };
    try {
      await onSave(body);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to update network");
    }
  }

  return (
    <Modal
      open
      title="Network settings"
      subtitle={`Egress rules for ${sandbox.sandboxID}`}
      wide
      onClose={busy ? () => undefined : onClose}
      footer={
        <>
          <button
            type="button"
            className="btn btn--ghost"
            onClick={onClose}
            disabled={busy}
          >
            Cancel
          </button>
          <button
            type="button"
            className="btn btn--primary"
            onClick={() => void handleSave()}
            disabled={busy}
          >
            {busy ? "Saving…" : "Save"}
          </button>
        </>
      }
    >
      {error ? (
        <div className="modal-error" role="alert">
          {error}
        </div>
      ) : null}

      <label className="modal-field">
        <span>Allow outbound (comma-separated CIDRs or hosts)</span>
        <input
          type="text"
          value={allowOut}
          onChange={(e) => setAllowOut(e.target.value)}
          disabled={busy}
        />
      </label>

      <label className="modal-field">
        <span>Deny outbound (comma-separated)</span>
        <input
          type="text"
          value={denyOut}
          onChange={(e) => setDenyOut(e.target.value)}
          disabled={busy}
        />
      </label>

      <label className="modal-field">
        <span>
          <input
            type="checkbox"
            checked={allowInternet}
            onChange={(e) => setAllowInternet(e.target.checked)}
            disabled={busy}
            style={{ marginRight: 8 }}
          />
          Allow internet access
        </span>
      </label>
    </Modal>
  );
}

export function SandboxNetworkDrawer({
  open,
  sandbox,
  busy,
  onClose,
  onSave,
}: SandboxNetworkDrawerProps) {
  if (!open) {
    return null;
  }

  return (
    <NetworkForm
      key={sandbox.sandboxID}
      sandbox={sandbox}
      busy={busy}
      onClose={onClose}
      onSave={onSave}
    />
  );
}
