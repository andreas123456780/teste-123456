// Tiny toast bus. The <ToastHost /> component subscribes to events
// emitted via `toast(...)` and renders them in a stack at the bottom
// of the screen. Decoupled from React so any module can fire a toast
// without prop-drilling — the only React part is the host that
// listens.
//
// Why not a context? The store's add-to-cart action lives in App's
// useCallback; pulling a context in there would force every consumer
// to re-render on every state change. A flat event bus is simpler
// and 100% sufficient at our scale.

export type ToastKind = "info" | "success" | "error";

export type ToastInput = {
  /** Short headline (typically uppercase tracking-wide). */
  title: string;
  /** Optional subtitle / explanation. */
  description?: string;
  /** Auto-dismiss delay in ms. Defaults to 3500 (2200 for errors).
   * Pass 0 to keep the toast until the user closes it. */
  duration?: number;
  kind?: ToastKind;
  /** Optional CTA shown to the right (e.g. "Ver sacola"). When the
   * user clicks it, `onAction` runs and the toast is dismissed. */
  actionLabel?: string;
  onAction?: () => void;
};

export type Toast = ToastInput & {
  id: number;
  duration: number;
  kind: ToastKind;
};

type Listener = (toasts: Toast[]) => void;

let nextId = 1;
let queue: Toast[] = [];
const listeners = new Set<Listener>();

function emit() {
  const snapshot = queue.slice();
  for (const l of listeners) l(snapshot);
}

export function subscribeToasts(listener: Listener): () => void {
  listeners.add(listener);
  listener(queue.slice());
  return () => {
    listeners.delete(listener);
  };
}

export function toast(input: ToastInput): number {
  const kind: ToastKind = input.kind ?? "info";
  const duration =
    typeof input.duration === "number"
      ? input.duration
      : kind === "error"
        ? 5000
        : 3500;
  const id = nextId++;
  queue = [...queue, { ...input, id, duration, kind }];
  emit();
  return id;
}

export function dismissToast(id: number) {
  const before = queue.length;
  queue = queue.filter((t) => t.id !== id);
  if (queue.length !== before) emit();
}

export function clearToasts() {
  if (queue.length === 0) return;
  queue = [];
  emit();
}
