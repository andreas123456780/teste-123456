import { AnimatePresence, motion } from "framer-motion";
import { useMemo, useState } from "react";
import type { Product } from "../types";
import { ProductCard } from "./ProductCard";
import { ProductCardSkeleton } from "./ProductCardSkeleton";
import { SizeChartPanel, SizeChartLink } from "./SizeChart";
import { WhatsApp } from "./icons";

type Props = {
  products: Product[];
  onOpen: (p: Product, preferredSize?: string) => void;
  whatsAppNumber: string;
  /** When true the grid renders skeleton tiles instead of real cards.
   * Used while the catalog is being fetched from the API the first time. */
  loading?: boolean;
};

// Normalize category strings from the API (trim whitespace, title-case)
// so admin-created variants like "baby look" and "Baby Look" are merged.
function normalizeCategory(raw: string): string {
  const trimmed = raw.trim();
  if (!trimmed) return trimmed;
  return trimmed
    .split(/\s+/)
    .map((w) => w.charAt(0).toUpperCase() + w.slice(1).toLowerCase())
    .join(" ");
}

export function Products({ products, onOpen, whatsAppNumber, loading }: Props) {
  const categories = useMemo(() => {
    const seen = new Set<string>();
    products.forEach((p) => seen.add(normalizeCategory(p.category)));
    return ["Todas", ...Array.from(seen)];
  }, [products]);
  const [activeCategory, setActiveCategory] = useState("Todas");

  const filtered = useMemo(() => {
    return activeCategory === "Todas"
      ? products
      : products.filter(
          (p) => normalizeCategory(p.category) === activeCategory,
        );
  }, [products, activeCategory]);

  const waHref = `https://wa.me/${whatsAppNumber}?text=${encodeURIComponent(
    "Oi! Queria falar com a NAST sobre as peças.",
  )}`;

  const handleOpen = (p: Product) => {
    onOpen(p);
  };

  return (
    <section id="products" className="relative mx-auto max-w-7xl px-6 py-28">
      <motion.div
        initial={{ opacity: 0, y: 20 }}
        whileInView={{ opacity: 1, y: 0 }}
        viewport={{ once: true }}
        className="flex flex-col items-start justify-between gap-6 md:flex-row md:items-end"
      >
        <div>
          <div className="eyebrow text-white/50">Drop 01</div>
          <h2 className="mt-3 text-5xl font-black tracking-tighter text-white md:text-7xl">
            A COLEÇÃO <span className="acid">INTEIRA.</span>
          </h2>
          <p className="mt-4 max-w-md text-sm text-white/60">
            Quatro peças, uma declaração. Edição limitada, produção em pequena
            escala e acabamento cuidado.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <div className="flex flex-wrap gap-2">
            {categories.map((c) => (
              <motion.button
                key={c}
                onClick={() => setActiveCategory(c)}
                whileHover={{ y: -1 }}
                whileTap={{ scale: 0.96 }}
                className="relative px-4 py-2 text-[11px] font-bold uppercase tracking-[0.3em]"
              >
                {activeCategory === c && (
                  <motion.span
                    layoutId="pill"
                    className="absolute inset-0 bg-[var(--color-accent)]"
                    transition={{ type: "spring", stiffness: 320, damping: 30 }}
                  />
                )}
                <span
                  className={`relative ${activeCategory === c ? "text-black" : "text-white/60 hover:text-white"}`}
                >
                  {c}
                </span>
              </motion.button>
            ))}
          </div>
          <div className="lg:hidden">
            <SizeChartLink />
          </div>
        </div>
      </motion.div>

      <div className="mt-12 grid grid-cols-1 gap-8 lg:grid-cols-[1fr_300px]">
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          {loading && filtered.length === 0 ? (
            <>
              {Array.from({ length: 4 }).map((_, i) => (
                <ProductCardSkeleton key={`sk-${i}`} />
              ))}
            </>
          ) : (
            <AnimatePresence mode="popLayout">
              {filtered.map((p, i) => (
                <motion.div
                  key={p.id}
                  layout
                  initial={{ opacity: 0, scale: 0.96 }}
                  animate={{ opacity: 1, scale: 1 }}
                  exit={{ opacity: 0, scale: 0.96 }}
                  transition={{ type: "spring", stiffness: 140, damping: 20 }}
                >
                  <ProductCard product={p} index={i} onOpen={handleOpen} />
                </motion.div>
              ))}
            </AnimatePresence>
          )}
          {!loading && filtered.length === 0 && (
            <div className="col-span-full border border-white/10 bg-white/5 px-6 py-10 text-center text-sm text-white/60">
              Nenhum modelo encontrado pro filtro atual.
            </div>
          )}
        </div>

        <SizeChartPanel />
      </div>

      <motion.a
        href={waHref}
        target="_blank"
        rel="noreferrer"
        whileHover={{ y: -2 }}
        whileTap={{ scale: 0.97 }}
        className="mt-12 inline-flex items-center gap-3 border border-[var(--color-accent)] px-6 py-4 text-xs font-bold uppercase tracking-[0.3em] text-[var(--color-accent)] transition hover:bg-[var(--color-accent)] hover:text-black"
      >
        <WhatsApp className="h-4 w-4" />
        Ficou com dúvida? chama no zap
      </motion.a>
    </section>
  );
}
