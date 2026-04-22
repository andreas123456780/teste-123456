import { AnimatePresence, motion } from "framer-motion";
import { useEffect, useState } from "react";
import type { Product } from "../types";
import { ProductArt } from "./ProductArt";
import { SizeChartLink } from "./SizeChart";
import { formatBRL, pixDiscountPercent } from "../utils/format";
import { Close, Plus, WhatsApp } from "./icons";

type Props = {
  product: Product | null;
  onClose: () => void;
  onAdd: (p: Product, size: string, color: string) => void;
  whatsAppNumber: string;
};

// Promo codes the brand honors manually via WhatsApp. The displayed price
// never changes client-side (the cart and the checkout API don't understand
// coupons yet), so applying one just prepends the code to the WhatsApp
// message and shows a confirmation below the input.
const COUPONS: Record<string, number> = {
  NAST10: 10,
  BEMVINDO: 5,
};

export function ProductModal({ product, onClose, onAdd, whatsAppNumber }: Props) {
  const [size, setSize] = useState<string>(product?.sizes[0] ?? "");
  const [color, setColor] = useState<string>(product?.colors[0] ?? "");
  const [coupon, setCoupon] = useState("");
  const [couponApplied, setCouponApplied] = useState<{ code: string; pct: number } | null>(null);
  const [couponError, setCouponError] = useState<string | null>(null);
  const [prevProductId, setPrevProductId] = useState(product?.id);
  if (product && product.id !== prevProductId) {
    setPrevProductId(product.id);
    setSize(product.sizes[0] ?? "");
    setColor(product.colors[0] ?? "");
    setCoupon("");
    setCouponApplied(null);
    setCouponError(null);
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

  function applyCoupon() {
    const code = coupon.trim().toUpperCase();
    if (!code) {
      setCouponError("Digite um código.");
      return;
    }
    const pct = COUPONS[code];
    if (!pct) {
      setCouponError("Cupom inválido.");
      setCouponApplied(null);
      return;
    }
    setCouponApplied({ code, pct });
    setCouponError(null);
  }

  const waText = product
    ? [
        `Oi! Quero a ${product.name} (${color}, tam ${size || "—"}). Vi no site da NAST.`,
        couponApplied ? `Cupom: ${couponApplied.code}` : "",
      ]
        .filter(Boolean)
        .join("\n")
    : "";
  const waHref = product
    ? `https://wa.me/${whatsAppNumber}?text=${encodeURIComponent(waText)}`
    : "#";

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
                  {product.sizes.map((s) => (
                    <motion.button
                      key={s}
                      onClick={() => setSize(s)}
                      whileHover={{ y: -1 }}
                      whileTap={{ scale: 0.96 }}
                      className={`min-w-[2.75rem] border px-3 py-1.5 text-[11px] font-bold uppercase tracking-widest transition ${
                        size === s
                          ? "border-[var(--color-accent)] bg-[var(--color-accent)] text-black"
                          : "border-white/15 text-white/60 hover:border-white/40"
                      }`}
                    >
                      {s}
                    </motion.button>
                  ))}
                </div>
              </div>

              <div className="mt-5">
                <div className="eyebrow mb-3 text-white/50">Cupom</div>
                <div className="flex items-center gap-2">
                  <input
                    type="text"
                    value={coupon}
                    onChange={(e) => {
                      setCoupon(e.target.value);
                      setCouponError(null);
                    }}
                    placeholder="ex: NAST10"
                    className="flex-1 border border-white/15 bg-transparent px-3 py-2.5 text-sm uppercase tracking-[0.2em] text-white placeholder-white/30 outline-none transition focus:border-[var(--color-accent)]"
                  />
                  <motion.button
                    type="button"
                    onClick={applyCoupon}
                    whileHover={{ y: -1 }}
                    whileTap={{ scale: 0.97 }}
                    className="border border-white/20 px-4 py-2.5 text-[11px] font-bold uppercase tracking-[0.3em] text-white transition hover:border-white"
                  >
                    Aplicar
                  </motion.button>
                </div>
                {couponError && (
                  <div className="mt-2 text-xs text-[var(--color-accent-warn)]">
                    {couponError}
                  </div>
                )}
                {couponApplied && (
                  <div className="mt-2 text-xs text-[var(--color-accent)]">
                    Cupom {couponApplied.code} · -{couponApplied.pct}% ao
                    entrar em contato.
                  </div>
                )}
              </div>

              <motion.a
                href={waHref}
                target="_blank"
                rel="noreferrer"
                whileHover={{ y: -1 }}
                whileTap={{ scale: 0.98 }}
                className="mt-8 flex items-center justify-center gap-2 bg-[var(--color-accent)] px-6 py-4 text-xs font-black uppercase tracking-[0.3em] text-black transition hover:bg-white"
              >
                <WhatsApp className="h-4 w-4" />
                Entrar em contato
              </motion.a>
              <motion.button
                onClick={() => {
                  onAdd(product, size, color);
                  onClose();
                }}
                whileHover={{ y: -1 }}
                whileTap={{ scale: 0.98 }}
                className="mt-2 flex items-center justify-center gap-2 border border-white/20 bg-transparent px-6 py-3.5 text-[11px] font-bold uppercase tracking-[0.3em] text-white transition hover:border-white"
              >
                <Plus className="h-4 w-4" />
                Adicionar à sacola
              </motion.button>

              <div className="mt-4 flex items-center justify-between text-[11px] text-white/50">
                <span>Estoque: {product.stock} unidades</span>
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
