import { AnimatePresence, motion } from "framer-motion";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { api, type ShippingOption } from "../api";
import type { CartItem } from "../types";
import { formatBRL } from "../utils/format";

type Props = {
  items: CartItem[];
  zipCode: string;
  onZipChange: (zip: string) => void;
  selectedServiceId: number | null;
  onSelect: (option: ShippingOption | null) => void;
};

// formatCep visually normalizes 01310100 → "01310-100" while keeping the
// underlying value as digits-only so the API always receives a clean value.
function formatCep(value: string) {
  const d = value.replace(/\D/g, "").slice(0, 8);
  if (d.length <= 5) return d;
  return `${d.slice(0, 5)}-${d.slice(5)}`;
}

// ShippingQuote is a self-contained form + option list. It triggers a quote
// lookup whenever the CEP becomes 8 digits (debounced) or when the cart
// composition changes. Selected option is lifted to the parent so checkout
// can include it in the total and payload.
export function ShippingQuote({
  items,
  zipCode,
  onZipChange,
  selectedServiceId,
  onSelect,
}: Props) {
  const [options, setOptions] = useState<ShippingOption[] | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const cancelRef = useRef<AbortController | null>(null);

  const digits = useMemo(() => zipCode.replace(/\D/g, ""), [zipCode]);
  const cartKey = useMemo(
    () =>
      items
        .map((it) => `${it.product.id}:${it.quantity}`)
        .sort()
        .join("|"),
    [items],
  );

  const fetchQuote = useCallback(async () => {
    if (digits.length !== 8 || items.length === 0) {
      setOptions(null);
      return;
    }
    cancelRef.current?.abort();
    const ctrl = new AbortController();
    cancelRef.current = ctrl;
    setLoading(true);
    setError(null);
    try {
      const payload = {
        zipCode: digits,
        items: items.map((it) => ({
          productId: it.product.id,
          quantity: it.quantity,
        })),
      };
      const res = await api.quoteShipping(payload);
      if (ctrl.signal.aborted) return;
      // Keep only carriers with a valid price; dropped carriers aren't
      // selectable anyway, but we still surface errors below the list.
      const sorted = [...res.options].sort(
        (a, b) => (a.priceCents || Infinity) - (b.priceCents || Infinity),
      );
      setOptions(sorted);
      // Auto-pick cheapest working option if nothing selected yet.
      const firstValid = sorted.find((o) => !o.error && o.priceCents > 0);
      if (selectedServiceId == null && firstValid) {
        onSelect(firstValid);
      } else if (
        selectedServiceId != null &&
        !sorted.some((o) => o.serviceId === selectedServiceId)
      ) {
        onSelect(firstValid ?? null);
      }
    } catch (e) {
      if (ctrl.signal.aborted) return;
      setError(
        e instanceof Error
          ? "Não foi possível calcular o frete agora."
          : "Erro ao calcular frete.",
      );
      setOptions(null);
    } finally {
      if (!ctrl.signal.aborted) setLoading(false);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [digits, cartKey]);

  // Debounce: wait 300ms after the last keystroke / cart edit.
  useEffect(() => {
    const t = window.setTimeout(fetchQuote, 300);
    return () => window.clearTimeout(t);
  }, [fetchQuote]);

  return (
    <div className="flex flex-col gap-2 border border-white/10 bg-white/[0.03] p-3">
      <div className="flex items-center justify-between gap-2">
        <label className="text-[11px] font-bold uppercase tracking-[0.3em] text-white/60">
          frete
        </label>
        <input
          inputMode="numeric"
          placeholder="CEP · 00000-000"
          className="w-40 border border-white/15 bg-transparent px-2 py-1 text-right text-sm text-white placeholder-white/40 outline-none focus:border-[var(--color-accent)]"
          value={formatCep(zipCode)}
          onChange={(e) => onZipChange(e.target.value)}
          aria-label="CEP de entrega"
          maxLength={9}
        />
      </div>

      <AnimatePresence initial={false}>
        {digits.length === 8 && loading && (
          <motion.div
            key="loading"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            className="text-[11px] uppercase tracking-[0.3em] text-white/50"
          >
            calculando…
          </motion.div>
        )}

        {error && (
          <motion.div
            key="error"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            className="text-[11px] text-[var(--color-accent-warn)]"
          >
            {error}
          </motion.div>
        )}

        {options && options.length > 0 && (
          <motion.ul
            key="options"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            className="flex flex-col gap-1"
          >
            {options.map((o) => {
              const disabled = !!o.error || o.priceCents <= 0;
              const active = o.serviceId === selectedServiceId;
              return (
                <li key={`${o.companyId}-${o.serviceId}`}>
                  <button
                    type="button"
                    disabled={disabled}
                    onClick={() => onSelect(o)}
                    className={`flex w-full items-center justify-between gap-2 border px-3 py-2 text-left text-[12px] transition ${
                      active
                        ? "border-[var(--color-accent)] bg-[var(--color-accent)]/10 text-white"
                        : disabled
                          ? "cursor-not-allowed border-white/5 text-white/30"
                          : "border-white/10 text-white/80 hover:border-white/30 hover:text-white"
                    }`}
                  >
                    <span className="flex min-w-0 flex-col">
                      <span className="truncate font-bold uppercase tracking-wider">
                        {o.companyName} · {o.serviceName}
                      </span>
                      <span className="text-[10px] uppercase tracking-widest text-white/50">
                        {o.error
                          ? o.error
                          : `${o.deliveryMinDays}${
                              o.deliveryMaxDays > o.deliveryMinDays
                                ? `–${o.deliveryMaxDays}`
                                : ""
                            } dias úteis`}
                      </span>
                    </span>
                    <span className="shrink-0 font-mono text-sm">
                      {disabled ? "—" : formatBRL(o.priceCents)}
                    </span>
                  </button>
                </li>
              );
            })}
          </motion.ul>
        )}
      </AnimatePresence>

      {digits.length > 0 && digits.length < 8 && (
        <div className="text-[10px] uppercase tracking-[0.3em] text-white/40">
          informe o cep completo
        </div>
      )}
    </div>
  );
}
