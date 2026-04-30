import type { Product } from "../types";

// productFilters owns the data shape and pure helpers behind the
// catalog filter strip. Lives outside the React component so the
// component file can stay focused on rendering and so unit tests (or
// Storybook) can exercise the logic without mounting React.

export type ProductFilterState = {
  sizes: string[];
  colors: string[];
  /** When null, the price filter is open-ended on the upper bound (i.e.
   * "no max"). When set, products priced above this are excluded. */
  priceMaxCents: number | null;
  /** When true, hide products with stock=0. */
  inStockOnly: boolean;
};

export type ProductFilterBounds = {
  sizes: string[];
  colors: string[];
  priceCents: [number, number];
};

export function deriveFilterBounds(products: Product[]): ProductFilterBounds {
  const sizes = new Set<string>();
  const colors = new Set<string>();
  let min = Number.POSITIVE_INFINITY;
  let max = 0;
  for (const p of products) {
    p.sizes.forEach((s) => sizes.add(s));
    p.colors.forEach((c) => colors.add(c));
    if (p.priceCents > 0) {
      if (p.priceCents < min) min = p.priceCents;
      if (p.priceCents > max) max = p.priceCents;
    }
  }
  if (!Number.isFinite(min)) min = 0;
  if (max < min) max = min;
  return {
    sizes: Array.from(sizes),
    colors: Array.from(colors),
    priceCents: [min, max],
  };
}

export function emptyFilterState(): ProductFilterState {
  return {
    sizes: [],
    colors: [],
    priceMaxCents: null,
    inStockOnly: false,
  };
}

export function applyFilters(
  products: Product[],
  filters: ProductFilterState,
  bounds: ProductFilterBounds,
): Product[] {
  const effectiveMax =
    filters.priceMaxCents === null
      ? bounds.priceCents[1]
      : Math.min(filters.priceMaxCents, bounds.priceCents[1]);
  return products.filter((p) => {
    if (filters.sizes.length > 0) {
      const hit = p.sizes.some((s) => filters.sizes.includes(s));
      if (!hit) return false;
    }
    if (filters.colors.length > 0) {
      const hit = p.colors.some((c) => filters.colors.includes(c));
      if (!hit) return false;
    }
    if (p.priceCents > effectiveMax) return false;
    if (filters.inStockOnly && p.stock <= 0) return false;
    return true;
  });
}

export function activeFilterCount(
  filters: ProductFilterState,
  bounds: ProductFilterBounds,
): number {
  let n = 0;
  if (filters.sizes.length > 0) n += 1;
  if (filters.colors.length > 0) n += 1;
  if (
    filters.priceMaxCents !== null &&
    filters.priceMaxCents < bounds.priceCents[1]
  ) {
    n += 1;
  }
  if (filters.inStockOnly) n += 1;
  return n;
}

/** Drop sizes/colors that the catalog no longer offers so a stale
 * filter doesn't make the grid empty. Pure — caller decides whether
 * to apply the result. */
export function pruneFiltersToBounds(
  filters: ProductFilterState,
  bounds: ProductFilterBounds,
): ProductFilterState {
  const cleanedSizes = filters.sizes.filter((s) => bounds.sizes.includes(s));
  const cleanedColors = filters.colors.filter((c) => bounds.colors.includes(c));
  if (
    cleanedSizes.length === filters.sizes.length &&
    cleanedColors.length === filters.colors.length
  ) {
    return filters;
  }
  return { ...filters, sizes: cleanedSizes, colors: cleanedColors };
}
