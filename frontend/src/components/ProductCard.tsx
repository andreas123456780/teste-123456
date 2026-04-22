import { motion } from "framer-motion";
import { useState } from "react";
import type { Product } from "../types";
import { ProductArt } from "./ProductArt";
import { formatBRL, pixDiscountPercent } from "../utils/format";

type Props = {
  product: Product;
  index: number;
  onOpen: (p: Product) => void;
};

// Streetwear product card: photo on top (white background so the fabric
// stands out), metadata underneath. Colors + sizes listed on the card
// itself so shoppers can skim without opening the modal.
export function ProductCard({ product, index, onOpen }: Props) {
  const [color, setColor] = useState(product.colors[0] ?? "preto");
  const pixPct = pixDiscountPercent(product.priceCents, product.pixPriceCents);

  return (
    <motion.div
      initial={{ opacity: 0, y: 30 }}
      whileInView={{ opacity: 1, y: 0 }}
      viewport={{ once: true, amount: 0.2 }}
      transition={{ delay: index * 0.06, type: "spring", stiffness: 90 }}
      className="group relative flex flex-col border border-white/10 bg-[var(--color-bg-soft)] text-left transition-colors hover:border-white/30"
    >
      <button
        type="button"
        onClick={() => onOpen(product)}
        className="relative block aspect-square w-full overflow-hidden bg-white"
        aria-label={`Abrir detalhes de ${product.name}`}
      >
        <motion.div
          whileHover={{ scale: 1.03 }}
          transition={{ type: "spring", stiffness: 200, damping: 22 }}
          className="absolute inset-0 flex items-center justify-center"
        >
          <ProductArt image={product.image} alt={product.name} size={320} />
        </motion.div>

        <div className="absolute left-3 top-3 flex gap-2">
          {product.tags.slice(0, 2).map((t) => (
            <span
              key={t}
              className="bg-black/80 px-2.5 py-1 text-[10px] font-bold uppercase tracking-[0.25em] text-white backdrop-blur"
            >
              {t}
            </span>
          ))}
        </div>
        {pixPct > 0 && (
          <div className="absolute right-3 top-3 bg-[var(--color-accent)] px-2.5 py-1 text-[10px] font-bold uppercase tracking-[0.2em] text-black">
            -{pixPct}% pix
          </div>
        )}
      </button>

      <div className="flex flex-1 flex-col gap-3 p-4">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <div className="eyebrow text-white/50">{product.category}</div>
            <div className="mt-1 truncate text-base font-bold text-white">
              {product.name}
            </div>
          </div>
          <motion.button
            onClick={() => onOpen(product)}
            whileHover={{ x: 2 }}
            className="shrink-0 text-[11px] font-bold uppercase tracking-[0.3em] text-[var(--color-accent)]"
          >
            ver peça →
          </motion.button>
        </div>

        <div className="flex items-center gap-2">
          {product.colors.map((c) => (
            <button
              key={c}
              onClick={(e) => {
                e.stopPropagation();
                setColor(c);
              }}
              title={c}
              className={`h-5 w-5 border transition ${
                color === c
                  ? "border-[var(--color-accent)] scale-110"
                  : "border-white/20 hover:border-white/50"
              }`}
              style={{ background: colorSwatch(c) }}
              aria-label={`Escolher cor ${c}`}
            />
          ))}
        </div>

        <div className="flex flex-wrap gap-1.5">
          {product.sizes.map((s) => (
            <span
              key={s}
              className="border border-white/15 px-2 py-0.5 text-[10px] font-bold uppercase tracking-widest text-white/60"
            >
              {s}
            </span>
          ))}
        </div>

        <div className="mt-auto flex items-baseline justify-between gap-2 border-t border-white/10 pt-3">
          <div>
            <div className="text-lg font-black text-white">
              {formatBRL(product.priceCents)}
            </div>
            <div className="mt-0.5 text-[11px] text-white/60">
              <span className="font-bold text-[var(--color-accent)]">
                {formatBRL(product.pixPriceCents)}
              </span>{" "}
              <span className="uppercase tracking-widest">com pix</span>
            </div>
          </div>
        </div>
      </div>
    </motion.div>
  );
}

function colorSwatch(name: string): string {
  const map: Record<string, string> = {
    preto: "#0b0b12",
    branco: "#f3f3f3",
    "off-white": "#ece8de",
    cinza: "#6b7280",
    roxo: "#7c3aed",
    verde: "#4a5a2a",
    "verde-neon": "#39ff14",
    índigo: "#1c2a49",
    azul: "#1e3a8a",
    bege: "#d6b98c",
    único: "#2a2a2a",
  };
  return map[name] ?? "#444";
}
