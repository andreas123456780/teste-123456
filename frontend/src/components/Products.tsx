import { AnimatePresence, motion } from "framer-motion";
import { useMemo, useState } from "react";
import type { Product } from "../types";
import { ProductCard } from "./ProductCard";
import { SizeChartPanel, SizeChartLink } from "./SizeChart";
import { WhatsApp } from "./icons";

type Props = {
  products: Product[];
  onOpen: (p: Product, preferredSize?: string) => void;
  whatsAppNumber: string;
};

// Size tokens that identify a baby-tee/baby-look variant. Admin registers
// these under `sizes` (e.g. "Baby Look") so the filter stays data-driven —
// no product needs to be recategorized to show up under the toggle.
const BABY_TEE_SIZE_TOKENS = ["baby look", "baby tee", "baby"];

function hasBabyTeeSize(product: Product): string | null {
  for (const size of product.sizes) {
    const s = size.toLowerCase();
    if (BABY_TEE_SIZE_TOKENS.some((t) => s.includes(t))) return size;
  }
  return null;
}

type FitFilter = "regular" | "baby";

export function Products({ products, onOpen, whatsAppNumber }: Props) {
  const categories = useMemo(() => {
    const set = new Set<string>();
    products.forEach((p) => set.add(p.category));
    return ["Todas", ...Array.from(set)];
  }, [products]);
  const [activeCategory, setActiveCategory] = useState("Todas");
  const [fit, setFit] = useState<FitFilter>("regular");

  const babyCount = useMemo(
    () => products.filter((p) => hasBabyTeeSize(p) !== null).length,
    [products],
  );
  const showBabyToggle = babyCount > 0;

  const filtered = useMemo(() => {
    const byCategory =
      activeCategory === "Todas"
        ? products
        : products.filter((p) => p.category === activeCategory);
    if (fit === "baby") {
      return byCategory.filter((p) => hasBabyTeeSize(p) !== null);
    }
    return byCategory;
  }, [products, activeCategory, fit]);

  const waHref = `https://wa.me/${whatsAppNumber}?text=${encodeURIComponent(
    "Oi! Queria falar com a NAST sobre as peças.",
  )}`;

  const handleOpen = (p: Product) => {
    if (fit === "baby") {
      const size = hasBabyTeeSize(p);
      onOpen(p, size ?? undefined);
    } else {
      onOpen(p);
    }
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
          {showBabyToggle && (
            <div className="flex items-center gap-1 border border-white/15 p-1">
              {([
                ["regular", "Boxy"],
                ["baby", "Baby tee"],
              ] as const).map(([id, label]) => {
                const active = fit === id;
                return (
                  <motion.button
                    key={id}
                    type="button"
                    onClick={() => setFit(id)}
                    whileTap={{ scale: 0.96 }}
                    className={`relative px-3 py-1.5 text-[11px] font-bold uppercase tracking-[0.25em] transition ${
                      active
                        ? "bg-[var(--color-accent)] text-black"
                        : "text-white/60 hover:text-white"
                    }`}
                    aria-pressed={active}
                  >
                    {label}
                  </motion.button>
                );
              })}
            </div>
          )}
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

      {fit === "baby" && (
        <div className="mt-6 inline-flex items-center gap-2 border border-[var(--color-accent)]/40 bg-[var(--color-accent)]/10 px-3 py-1.5 text-[11px] uppercase tracking-[0.25em] text-[var(--color-accent)]">
          Mostrando só modelos com tamanho baby tee · abre já no tamanho
        </div>
      )}

      <div className="mt-12 grid grid-cols-1 gap-8 lg:grid-cols-[1fr_300px]">
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
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
          {filtered.length === 0 && (
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
