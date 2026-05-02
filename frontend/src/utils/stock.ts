import type { Product } from "../types";

const LOW_STOCK_THRESHOLD = 3;

/** Sum of per-size inventory. Falls back to legacy `stock` when the
 * product was created before the per-size rollout (empty `stockBySize`). */
export function totalStock(product: Product): number {
  const map = product.stockBySize;
  if (map && Object.keys(map).length > 0) {
    let sum = 0;
    for (const v of Object.values(map)) sum += Math.max(0, v ?? 0);
    return sum;
  }
  return Math.max(0, product.stock ?? 0);
}

/** Pretty label for the storefront card/modal. Returns null when stock is
 * either above the threshold (no urgency needed) or the product uses the
 * legacy shape with unknown per-size data — in which case we'd rather say
 * nothing than guess. */
export function lowStockLabel(product: Product): string | null {
  const map = product.stockBySize;
  const hasPerSize = map && Object.keys(map).length > 0;
  if (!hasPerSize) return null;
  const total = totalStock(product);
  if (total <= 0) return "Esgotado";
  if (total === 1) return "Última unidade!";
  if (total <= LOW_STOCK_THRESHOLD) return `Últimas ${total} unidades`;
  return null;
}

/** Label for a specific size (used inside the modal next to the size
 * selector / CTA). Returns null when stock is unknown or above the
 * threshold. */
export function lowStockLabelForSize(
  product: Product,
  size: string,
): string | null {
  const map = product.stockBySize;
  if (!map || Object.keys(map).length === 0) return null;
  const count = map[size];
  if (count === undefined) return null;
  if (count <= 0) return null; // sold-out is already shown via the disabled state
  if (count === 1) return `Última unidade no tamanho ${size}`;
  if (count <= LOW_STOCK_THRESHOLD) return `Últimas ${count} no tamanho ${size}`;
  return null;
}
