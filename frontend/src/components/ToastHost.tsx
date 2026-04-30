import { AnimatePresence, motion } from "framer-motion";
import { useEffect, useState } from "react";
import { dismissToast, subscribeToasts, type Toast } from "../lib/toast";
import { Close } from "./icons";

// ToastHost mounts once near the root of the app and renders the
// active toast queue. Toasts auto-dismiss after `duration` ms (set
// in the bus) but the user can also close them manually or click an
// optional CTA. Stacked bottom-right on desktop, full-width band at
// the top on mobile so it doesn't fight the sticky cart CTA.
export function ToastHost() {
  const [toasts, setToasts] = useState<Toast[]>([]);

  useEffect(() => subscribeToasts(setToasts), []);

  return (
    <div
      aria-live="polite"
      aria-atomic="false"
      className="pointer-events-none fixed inset-x-0 top-[80px] z-[60] flex flex-col items-center gap-2 px-4 sm:bottom-6 sm:left-auto sm:right-6 sm:top-auto sm:items-end"
    >
      <AnimatePresence>
        {toasts.map((t) => (
          <ToastItem key={t.id} toast={t} />
        ))}
      </AnimatePresence>
    </div>
  );
}

function ToastItem({ toast }: { toast: Toast }) {
  useEffect(() => {
    if (toast.duration <= 0) return;
    const handle = window.setTimeout(() => dismissToast(toast.id), toast.duration);
    return () => window.clearTimeout(handle);
  }, [toast.id, toast.duration]);

  const kindClass =
    toast.kind === "error"
      ? "border-red-500/50 bg-red-950/95 text-red-50"
      : toast.kind === "success"
        ? "border-[var(--color-accent)]/60 bg-[var(--color-bg-soft)]/95 text-white"
        : "border-white/15 bg-[var(--color-bg-soft)]/95 text-white";

  return (
    <motion.div
      layout
      initial={{ opacity: 0, y: -16, scale: 0.96 }}
      animate={{ opacity: 1, y: 0, scale: 1 }}
      exit={{ opacity: 0, y: -16, scale: 0.96 }}
      transition={{ type: "spring", stiffness: 360, damping: 30 }}
      className={`pointer-events-auto flex w-full max-w-sm items-start gap-3 border ${kindClass} px-4 py-3 shadow-lg backdrop-blur`}
      role={toast.kind === "error" ? "alert" : "status"}
    >
      <div className="min-w-0 flex-1">
        <div className="text-[11px] font-bold uppercase tracking-[0.25em]">
          {toast.title}
        </div>
        {toast.description && (
          <div className="mt-1 text-sm text-white/70">{toast.description}</div>
        )}
        {toast.actionLabel && toast.onAction && (
          <button
            type="button"
            onClick={() => {
              toast.onAction?.();
              dismissToast(toast.id);
            }}
            className="mt-2 text-[11px] font-bold uppercase tracking-[0.25em] text-[var(--color-accent)] underline-offset-4 hover:underline"
          >
            {toast.actionLabel} →
          </button>
        )}
      </div>
      <button
        type="button"
        onClick={() => dismissToast(toast.id)}
        aria-label="Fechar"
        className="shrink-0 text-white/40 transition hover:text-white"
      >
        <Close className="h-4 w-4" />
      </button>
    </motion.div>
  );
}
