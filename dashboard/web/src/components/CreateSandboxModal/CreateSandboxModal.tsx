import { useEffect, useState } from "react";
import { createSandbox, fetchTemplates } from "../../api/platform";
import type { SandboxVolumeMount, Template } from "../../api/types";
import { Modal } from "../Modal/Modal";
import { useToast } from "../Toast/useToast";

type CreateSandboxModalProps = {
  open: boolean;
  onClose: () => void;
  onCreated: (sandboxID: string) => void;
};

const DEFAULT_TIMEOUT = 300;

export function CreateSandboxModal({
  open,
  onClose,
  onCreated,
}: CreateSandboxModalProps) {
  const { pushToast } = useToast();
  const [templates, setTemplates] = useState<Template[]>([]);
  const [templateID, setTemplateID] = useState("");
  const [timeoutSec, setTimeoutSec] = useState(String(DEFAULT_TIMEOUT));
  const [volumeMountsJson, setVolumeMountsJson] = useState("[]");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!open) {
      return;
    }
    let cancelled = false;
    void fetchTemplates()
      .then((items) => {
        if (cancelled) {
          return;
        }
        setTemplates(items);
        if (items.length > 0) {
          setTemplateID(items[0].templateID);
        }
      })
      .catch((err) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : "Failed to load templates");
        }
      });
    return () => {
      cancelled = true;
    };
  }, [open]);

  function handleClose() {
    setError("");
    setVolumeMountsJson("[]");
    setTimeoutSec(String(DEFAULT_TIMEOUT));
    setBusy(false);
    onClose();
  }

  async function handleSubmit() {
    setError("");
    const timeout = Number(timeoutSec);
    if (!templateID) {
      setError("Template is required");
      return;
    }
    if (!Number.isFinite(timeout) || timeout <= 0) {
      setError("Timeout must be a positive number of seconds");
      return;
    }

    let volumeMounts: SandboxVolumeMount[] | undefined;
    const trimmed = volumeMountsJson.trim();
    if (trimmed && trimmed !== "[]") {
      try {
        const parsed = JSON.parse(trimmed) as unknown;
        if (!Array.isArray(parsed)) {
          setError("volumeMounts must be a JSON array");
          return;
        }
        volumeMounts = parsed as SandboxVolumeMount[];
      } catch {
        setError("volumeMounts must be valid JSON");
        return;
      }
    }

    setBusy(true);
    try {
      const resp = await createSandbox({
        templateID,
        timeout,
        volumeMounts,
      });
      pushToast(`Sandbox ${resp.sandboxID} created`, "success");
      handleClose();
      onCreated(resp.sandboxID);
    } catch (err) {
      const message =
        err instanceof Error ? err.message : "Failed to create sandbox";
      setError(message);
      pushToast(message, "error");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal
      open={open}
      title="Create sandbox"
      subtitle="Spawn a new sandbox from a template."
      wide
      onClose={busy ? () => undefined : handleClose}
      footer={
        <>
          <button
            type="button"
            className="btn btn--ghost"
            onClick={handleClose}
            disabled={busy}
          >
            Cancel
          </button>
          <button
            type="button"
            className="btn btn--primary"
            onClick={() => void handleSubmit()}
            disabled={busy || templates.length === 0}
          >
            {busy ? "Creating…" : "Create"}
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
        <span>Template</span>
        <select
          value={templateID}
          onChange={(e) => setTemplateID(e.target.value)}
          disabled={busy || templates.length === 0}
        >
          {templates.length === 0 ? (
            <option value="">No templates available</option>
          ) : (
            templates.map((tmpl) => (
              <option key={tmpl.templateID} value={tmpl.templateID}>
                {tmpl.templateID}
              </option>
            ))
          )}
        </select>
      </label>

      <label className="modal-field">
        <span>Timeout (seconds)</span>
        <input
          type="number"
          min={1}
          value={timeoutSec}
          onChange={(e) => setTimeoutSec(e.target.value)}
          disabled={busy}
        />
      </label>

      <label className="modal-field">
        <span>Volume mounts (JSON array, optional)</span>
        <textarea
          value={volumeMountsJson}
          onChange={(e) => setVolumeMountsJson(e.target.value)}
          disabled={busy}
          spellCheck={false}
        />
      </label>
    </Modal>
  );
}
