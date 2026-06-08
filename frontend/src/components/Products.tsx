import { AnimatePresence, motion } from "framer-motion";
import { useMemo, useState } from "react";
import type { Product } from "../types";
import { ProductCard } from "./ProductCard";
import { SizeChartPanel, SizeChartLink } from "./SizeChart";
import { WhatsApp } from "./icons";
import { SecretUnlock } from "./SecretUnlock";
import { SECRET_PRODUCT } from "../data/fallback";

type Props = {
  products: Product[];
  onOpen: (p: Product) => void;
  whatsAppNumber: string;
};

export function Products({ products, onOpen, whatsAppNumber }: Props) {
  const [secretUnlocked, setSecretUnlocked] = useState(() => {
    return localStorage.getItem("nast:secret") === "1";
  });
  const [showUnlock, setShowUnlock] = useState(false);

  const categories = useMemo(() => {
    const set = new Set<string>();
    products.forEach((p) => set.add(p.category));
    return ["Todas", ...Array.from(set)];
  }, [products]);
  const [active, setActive] = useState("Todas");

  const filtered = useMemo(
    () =>
      active === "Todas"
        ? products
        : products.filter((p) => p.category === active),
    [products, active],
  );

  const waHref = `https://wa.me/${whatsAppNumber}?text=${encodeURIComponent(
    "Oi! Queria falar com a NAST sobre as peças.",
  )}`;

  function handleUnlocked() {
    localStorage.setItem("nast:secret", "1");
    setSecretUnlocked(true);
    setShowUnlock(false);
  }

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
                onClick={() => setActive(c)}
                whileHover={{ y: -1 }}
                whileTap={{ scale: 0.96 }}
                className="relative px-4 py-2 text-[11px] font-bold uppercase tracking-[0.3em]"
              >
                {active === c && (
                  <motion.span
                    layoutId="pill"
                    className="absolute inset-0 bg-[var(--color-accent)]"
                    transition={{ type: "spring", stiffness: 320, damping: 30 }}
                  />
                )}
                <span
                  className={`relative ${active === c ? "text-black" : "text-white/60 hover:text-white"}`}
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
                <ProductCard product={p} index={i} onOpen={onOpen} />
              </motion.div>
            ))}

            {/* Card da peça secreta */}
            {(active === "Todas") && (
              <motion.div
                key="secret-card"
                layout
                initial={{ opacity: 0, scale: 0.96 }}
                animate={{ opacity: 1, scale: 1 }}
                exit={{ opacity: 0, scale: 0.96 }}
                transition={{ type: "spring", stiffness: 140, damping: 20 }}
              >
                {secretUnlocked ? (
                  <ProductCard
                    product={SECRET_PRODUCT}
                    index={filtered.length}
                    onOpen={onOpen}
                  />
                ) : showUnlock ? (
                  <SecretUnlock onUnlocked={handleUnlocked} />
                ) : (
                  <LockedCard onReveal={() => setShowUnlock(true)} />
                )}
              </motion.div>
            )}
          </AnimatePresence>
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

function LockedCard({ onReveal }: { onReveal: () => void }) {
  return (
    <motion.div
      className="group relative flex flex-col border border-white/10 bg-[var(--color-bg-soft)] text-left transition-colors hover:border-white/30 cursor-pointer"
      whileHover={{ scale: 1.01 }}
      transition={{ type: "spring", stiffness: 200, damping: 22 }}
      onClick={onReveal}
    >
      {/* Imagem bloqueada com overlay */}
      <div className="relative aspect-square w-full overflow-hidden bg-black flex items-center justify-center">
        <motion.div
          className="absolute inset-0 flex flex-col items-center justify-center gap-3"
          animate={{ opacity: [0.6, 1, 0.6] }}
          transition={{ repeat: Infinity, duration: 2.5, ease: "easeInOut" }}
        >
          <div className="text-5xl select-none">🔒</div>
          <div className="eyebrow text-white/40 text-center px-4">ACESSO RESTRITO</div>
        </motion.div>

        {/* Scanlines effect */}
        <div
          className="absolute inset-0 pointer-events-none"
          style={{
            background:
              "repeating-linear-gradient(0deg, transparent, transparent 2px, rgba(255,255,255,0.03) 2px, rgba(255,255,255,0.03) 4px)",
          }}
        />

        <div className="absolute left-3 top-3">
          <span className="bg-red-600/90 px-2.5 py-1 text-[10px] font-bold uppercase tracking-[0.25em] text-white backdrop-blur">
            secreto
          </span>
        </div>
      </div>

      <div className="flex flex-1 flex-col gap-3 p-4">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <div className="eyebrow text-white/30">???</div>
            <div className="mt-1 text-base font-bold text-white/40 tracking-widest">
              ██████████
            </div>
          </div>
          <motion.button
            whileHover={{ x: 2 }}
            className="shrink-0 text-[11px] font-bold uppercase tracking-[0.3em] text-[var(--color-accent)]"
          >
            desbloquear →
          </motion.button>
        </div>

        <div className="mt-auto border-t border-white/10 pt-3">
          <div className="text-sm text-white/30 font-bold uppercase tracking-widest">
            tem um código?
          </div>
        </div>
      </div>
    </motion.div>
  );
}
