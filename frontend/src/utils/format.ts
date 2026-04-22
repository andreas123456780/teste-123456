export function formatBRL(cents: number): string {
  return new Intl.NumberFormat("pt-BR", {
    style: "currency",
    currency: "BRL",
  }).format(cents / 100);
}

export function pixDiscountPercent(priceCents: number, pixCents: number): number {
  if (priceCents <= 0) return 0;
  const pct = ((priceCents - pixCents) / priceCents) * 100;
  return Math.round(pct);
}
