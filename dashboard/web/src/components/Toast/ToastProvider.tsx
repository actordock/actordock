import {
  useCallback,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { ToastContext, type ToastTone } from "./toastContext";
import "./Toast.css";

type ToastItem = {
  id: number;
  message: string;
  tone: ToastTone;
};

const TOAST_MS = 4500;

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<ToastItem[]>([]);
  const nextId = useRef(1);

  const pushToast = useCallback((message: string, tone: ToastTone = "info") => {
    const id = nextId.current++;
    setToasts((items) => [...items, { id, message, tone }]);
    window.setTimeout(() => {
      setToasts((items) => items.filter((item) => item.id !== id));
    }, TOAST_MS);
  }, []);

  const value = useMemo(() => ({ pushToast }), [pushToast]);

  return (
    <ToastContext.Provider value={value}>
      {children}
      <div className="toast-stack" aria-live="polite" aria-relevant="additions">
        {toasts.map((toast) => (
          <div
            key={toast.id}
            className={`toast toast--${toast.tone}`}
            role={toast.tone === "error" ? "alert" : "status"}
          >
            {toast.message}
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}
