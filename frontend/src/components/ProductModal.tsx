import { AnimatePresence, motion } from "framer-motion";
import { useEffect, useState } from "react";
import type { Product } from "../types";
import { ProductArt } from "./ProductArt";
import { SizeChartLink } from "./SizeChart";
import { formatBRL, pixDiscountPercent } from "../utils/format";
import { Close, Plus } from "./icons";

type Props = {
  product: Product | null;
  /** Pre-select this size when the modal opens (e.g. from the baby-tee
   * filter). Falls back to the product's first size when absent. */
  preferredSize?: string;
  onClose: () => void;
  onAdd: (
    p: Product,
    size: string,
    color: string,
    options?: { openCart?: boolean },
  ) => void;
};

/** Products migrated after the stock-per-size rollout carry a populated
 * `stockBySize` map. Legacy rows have `{}` — those should NOT show
 * "esgotado" ghosting (we don't know the real inventory per size). */
function hasPerSizeStock(product: Product | null | undefined): boolean {
  if (!product || !product.stockBySize) return false;
  return Object.keys(product.stockBySize).length > 0;
}

function sizeStock(product: Product, size: string): number {
  if (!hasPerSizeStock(product)) return Number.POSITIVE_INFINITY;
  return product.stockBySize[size] ?? 0;
}

function pickInitialSize(
  product: Product | null | undefined,
  preferred: string | undefined,
): string {
  if (!product) return "";
  const inStock = (s: string) => sizeStock(product, s) > 0;
  if (preferred && product.sizes.includes(preferred) && inStock(preferred)) {
    return preferred;
  }
  const firstAvailable = product.sizes.find(inStock);
  if (firstAvailable) return firstAvailable;
  // All sizes sold out — default to the first so the UI still renders.
  return product.sizes[0] ?? "";
}

export function ProductModal({ product, preferredSize, onClose, onAdd }: Props) {
  const [size, setSize] = useState<string>(pickInitialSize(product, preferredSize));
  const [color, setColor] = useState<string>(product?.colors[0] ?? "");
  const [prevProductId, setPrevProductId] = useState(product?.id);
  const [prevPreferred, setPrevPreferred] = useState(preferredSize);
  if (
    product &&
    (product.id !== prevProductId || preferredSize !== prevPreferred)
  ) {
    setPrevProductId(product.id);
    setPrevPreferred(preferredSize);
    setSize(pickInitialSize(product, preferredSize));
    setColor(product.colors[0] ?? "");
  }

  useEffect(() => {
    const onEsc = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onEsc);
    return () => window.removeEventListener("keydown", onEsc);
  }, [onClose]);

  const pixPct = product
    ? pixDiscountPercent(product.priceCents, product.pixPriceCents)
    : 0;

  return (
    <AnimatePresence>
      {product && (
        <motion.div
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          className="fixed inset-0 z-50 flex items-center justify-center overflow-y-auto bg-black/80 px-4 py-10 backdrop-blur"
          onClick={onClose}
        >
          <motion.div
            onClick={(e) => e.stopPropagation()}
            initial={{ y: 60, opacity: 0, scale: 0.96 }}
            animate={{ y: 0, opacity: 1, scale: 1 }}
            exit={{ y: 40, opacity: 0, scale: 0.96 }}
            transition={{ type: "spring", stiffness: 240, damping: 26 }}
            className="relative grid w-full max-w-5xl grid-cols-1 overflow-hidden border border-white/10 bg-[var(--color-bg-soft)] shadow-2xl md:grid-cols-2"
          >
            <button
              aria-label="Fechar"
              onClick={onClose}
              className="absolute right-4 top-4 z-10 border border-white/10 bg-black/50 p-2 text-white/70 backdrop-blur hover:text-white"
            >
              <Close className="h-4 w-4" />
            </button>

            <div className="relative flex items-center justify-center bg-white p-6 md:p-10">
              <ProductArt image={product.image} alt={product.name} size={420} />
            </div>

            <div className="flex flex-col overflow-y-auto p-8">
              <div className="flex items-center justify-between">
                <div className="eyebrow text-white/50">{product.category}</div>
                <SizeChartLink />
              </div>
              <h3 className="mt-3 text-3xl font-black tracking-tight text-white md:text-4xl">
                {product.name}
              </h3>
              <div className="mt-4 flex items-baseline gap-3">
                <span className="text-3xl font-black text-white">
                  {formatBRL(product.priceCents)}
                </span>
                {pixPct > 0 && (
                  <span className="bg-[var(--color-accent)] px-2 py-0.5 text-[10px] font-bold uppercase tracking-widest text-black">
                    -{pixPct}% pix
                  </span>
                )}
              </div>
              <div className="mt-1 text-sm text-white/70">
                ou{" "}
                <span className="font-bold text-[var(--color-accent)]">
                  {formatBRL(product.pixPriceCents)}
                </span>{" "}
                no pix
              </div>

              <p className="mt-6 text-sm leading-relaxed text-white/70">
                {product.description}
              </p>

              <div className="mt-6">
                <div className="eyebrow mb-3 text-white/50">Cor · {color}</div>
                <div className="flex flex-wrap gap-2">
                  {product.colors.map((c) => (
                    <motion.button
                      key={c}
                      onClick={() => setColor(c)}
                      whileHover={{ y: -1 }}
                      whileTap={{ scale: 0.96 }}
                      className={`border px-3 py-1.5 text-[11px] font-semibold uppercase tracking-widest transition ${
                        color === c
                          ? "border-[var(--color-accent)] bg-[var(--color-accent)]/10 text-white"
                          : "border-white/15 text-white/60 hover:border-white/40"
                      }`}
                    >
                      {c}
                    </motion.button>
                  ))}
                </div>
              </div>

              <div className="mt-5">
                <div className="mb-3 flex items-center justify-between">
                  <span className="eyebrow text-white/50">Tamanho</span>
                  <SizeChartLink label="ver medidas" />
                </div>
                <div className="flex flex-wrap gap-2">
                  {product.sizes.map((s) => {
                    const soldOut = sizeStock(product, s) <= 0;
                    const selected = size === s;
                    return (
                      <motion.button
                        key={s}
                        type="button"
                        onClick={() => {
                          if (!soldOut) setSize(s);
                        }}
                        disabled={soldOut}
                        aria-disabled={soldOut}
                        whileHover={soldOut ? undefined : { y: -1 }}
                        whileTap={soldOut ? undefined : { scale: 0.96 }}
                        className={`min-w-[2.75rem] border px-3 py-1.5 text-[11px] font-bold uppercase tracking-widest transition ${
                          soldOut
                            ? "cursor-not-allowed border-white/10 text-white/25 line-through"
                            : selected
                              ? "border-[var(--color-accent)] bg-[var(--color-accent)] text-black"
                              : "border-white/15 text-white/60 hover:border-white/40"
                        }`}
                        title={soldOut ? "Esgotado" : undefined}
                      >
                        {s}
                      </motion.button>
                    );
                  })}
                </div>
              </div>

              {(() => {
                const currentStock = sizeStock(product, size);
                const disabled = currentStock <= 0;
                return (
                  <>
                    <motion.button
                      type="button"
                      onClick={() => {
                        if (disabled) return;
                        onAdd(product, size, color, { openCart: true });
                        onClose();
                      }}
                      disabled={disabled}
                      aria-disabled={disabled}
                      whileHover={disabled ? undefined : { y: -1 }}
                      whileTap={disabled ? undefined : { scale: 0.98 }}
                      className={`mt-8 flex items-center justify-center gap-2 px-6 py-4 text-xs font-black uppercase tracking-[0.3em] transition ${
                        disabled
                          ? "cursor-not-allowed bg-white/10 text-white/40"
                          : "bg-[var(--color-accent)] text-black hover:bg-white"
                      }`}
                    >
                      <Plus className="h-4 w-4" />
                      {disabled ? "Esgotado" : "Comprar agora"}
                    </motion.button>
                    <motion.button
                      type="button"
                      onClick={() => {
                        if (disabled) return;
                        onAdd(product, size, color, { openCart: false });
                        onClose();
                      }}
                      disabled={disabled}
                      aria-disabled={disabled}
                      whileHover={disabled ? undefined : { y: -1 }}
                      whileTap={disabled ? undefined : { scale: 0.98 }}
                      className={`mt-2 flex items-center justify-center gap-2 border bg-transparent px-6 py-3.5 text-[11px] font-bold uppercase tracking-[0.3em] transition ${
                        disabled
                          ? "cursor-not-allowed border-white/10 text-white/30"
                          : "border-white/20 text-white hover:border-white"
                      }`}
                    >
                      <Plus className="h-4 w-4" />
                      Adicionar à sacola
                    </motion.button>
                  </>
                );
              })()}

              <div className="mt-4 flex items-center justify-between text-[11px] text-white/50">
                <span>
                  {hasPerSizeStock(product)
                    ? size
                      ? sizeStock(product, size) > 0
                        ? `Estoque ${size}: ${sizeStock(product, size)} un`
                        : `Tamanho ${size} esgotado`
                      : `Estoque total: ${product.stock} un`
                    : `Estoque: ${product.stock} unidades`}
                </span>
                <span className="flex gap-1">
                  {product.tags.map((t) => (
                    <span
                      key={t}
                      className="border border-white/15 px-2 py-0.5 uppercase tracking-widest"
                    >
                      {t}
                    </span>
                  ))}
                </span>
              </div>
            </div>
          </motion.div>
        </motion.div>
      )}
    </AnimatePresence>
  );
}
