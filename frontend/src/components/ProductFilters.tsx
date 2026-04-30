import { motion } from "framer-motion";
import { useState } from "react";
import {
  activeFilterCount,
  emptyFilterState,
  type ProductFilterBounds,
  type ProductFilterState,
} from "../lib/productFilters";
import { formatBRL } from "../utils/format";

// ProductFilters renders the catalog filter strip (sizes, colors,
// price ceiling, in-stock toggle). All logic lives in
// `lib/productFilters` — this file is purely presentational.
//
// Filters stay collapsed on every viewport so the catalog stays the
// star of the page and shoppers who don't need to filter aren't taxed
// with a tall control surface they have to scroll past. A counter
// badge surfaces how many filters are active when collapsed.

type Props = {
  bounds: ProductFilterBounds;
  filters: ProductFilterState;
  onChange: (next: ProductFilterState) => void;
};

export function ProductFilters({ bounds, filters, onChange }: Props) {
  const [open, setOpen] = useState(false);
  const activeCount = activeFilterCount(filters, bounds);

  const noPriceRange = bounds.priceCents[0] >= bounds.priceCents[1];
  const sliderMax = bounds.priceCents[1];
  const sliderValue =
    filters.priceMaxCents === null
      ? sliderMax
      : Math.min(filters.priceMaxCents, sliderMax);

  const toggleSize = (s: string) =>
    onChange({
      ...filters,
      sizes: filters.sizes.includes(s)
        ? filters.sizes.filter((x) => x !== s)
        : [...filters.sizes, s],
    });
  const toggleColor = (c: string) =>
    onChange({
      ...filters,
      colors: filters.colors.includes(c)
        ? filters.colors.filter((x) => x !== c)
        : [...filters.colors, c],
    });
  const setPriceMax = (cents: number) =>
    onChange({
      ...filters,
      priceMaxCents: cents >= sliderMax ? null : cents,
    });

  const reset = () => onChange(emptyFilterState());

  return (
    <div className="border border-white/10 bg-white/[0.02]">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="flex w-full items-center justify-between gap-3 px-4 py-3 text-[11px] font-bold uppercase tracking-[0.3em] text-white/70 transition hover:text-white"
        aria-expanded={open}
      >
        <span>
          Filtros
          {activeCount > 0 && (
            <span className="ml-2 inline-flex h-5 min-w-5 items-center justify-center bg-[var(--color-accent)] px-1 text-[10px] font-bold text-black">
              {activeCount}
            </span>
          )}
        </span>
        <span aria-hidden className="text-white/40">{open ? "−" : "+"}</span>
      </button>

      <motion.div
        initial={false}
        animate={{ height: open ? "auto" : 0, opacity: open ? 1 : 0 }}
        transition={{ duration: 0.25, ease: "easeOut" }}
        className="overflow-hidden"
      >
        <div className="space-y-5 border-t border-white/10 px-4 py-4">
          {bounds.sizes.length > 0 && (
            <div>
              <div className="eyebrow mb-2 text-white/50">Tamanho</div>
              <div className="flex flex-wrap gap-1.5">
                {bounds.sizes.map((s) => {
                  const active = filters.sizes.includes(s);
                  return (
                    <button
                      key={s}
                      type="button"
                      onClick={() => toggleSize(s)}
                      className={`min-w-[2.5rem] border px-2.5 py-1 text-[11px] font-bold uppercase tracking-widest transition ${
                        active
                          ? "border-[var(--color-accent)] bg-[var(--color-accent)] text-black"
                          : "border-white/15 text-white/60 hover:border-white/40"
                      }`}
                    >
                      {s}
                    </button>
                  );
                })}
              </div>
            </div>
          )}

          {bounds.colors.length > 0 && (
            <div>
              <div className="eyebrow mb-2 text-white/50">Cor</div>
              <div className="flex flex-wrap gap-1.5">
                {bounds.colors.map((c) => {
                  const active = filters.colors.includes(c);
                  return (
                    <button
                      key={c}
                      type="button"
                      onClick={() => toggleColor(c)}
                      className={`border px-2.5 py-1 text-[11px] font-semibold uppercase tracking-widest transition ${
                        active
                          ? "border-[var(--color-accent)] bg-[var(--color-accent)]/10 text-white"
                          : "border-white/15 text-white/60 hover:border-white/40"
                      }`}
                    >
                      {c}
                    </button>
                  );
                })}
              </div>
            </div>
          )}

          {!noPriceRange && (
            <div>
              <div className="eyebrow mb-2 flex items-center justify-between text-white/50">
                <span>Preço até</span>
                <span className="font-mono text-[11px] text-white">
                  {formatBRL(sliderValue)}
                </span>
              </div>
              <input
                type="range"
                min={bounds.priceCents[0]}
                max={sliderMax}
                step={500}
                value={sliderValue}
                onChange={(e) => setPriceMax(Number(e.target.value))}
                className="w-full accent-[var(--color-accent)]"
                aria-label="Preço máximo"
              />
              <div className="mt-1 flex justify-between text-[10px] text-white/40">
                <span>{formatBRL(bounds.priceCents[0])}</span>
                <span>{formatBRL(sliderMax)}</span>
              </div>
            </div>
          )}

          <label className="flex cursor-pointer items-center gap-2 text-[11px] font-semibold uppercase tracking-widest text-white/70">
            <input
              type="checkbox"
              checked={filters.inStockOnly}
              onChange={(e) =>
                onChange({ ...filters, inStockOnly: e.target.checked })
              }
              className="h-4 w-4 accent-[var(--color-accent)]"
            />
            <span>Mostrar apenas com estoque</span>
          </label>

          {activeCount > 0 && (
            <button
              type="button"
              onClick={reset}
              className="text-[11px] font-bold uppercase tracking-[0.3em] text-white/60 underline-offset-4 hover:text-white hover:underline"
            >
              Limpar filtros
            </button>
          )}
        </div>
      </motion.div>
    </div>
  );
}
