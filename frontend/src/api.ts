import type { CheckoutResponse, Product } from "./types";

const API_BASE = import.meta.env.VITE_API_URL ?? "http://localhost:8080";

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    ...init,
    headers: {
      ...(init?.body != null ? { "Content-Type": "application/json" } : {}),
      ...(init?.headers ?? {}),
    },
  });
  if (!res.ok) {
    const text = await res.text().catch(() => "");
    throw new Error(`API ${res.status}: ${text || res.statusText}`);
  }
  return (await res.json()) as T;
}

export type ShippingOption = {
  serviceId: number;
  companyId: number;
  companyName: string;
  serviceName: string;
  priceCents: number;
  deliveryMinDays: number;
  deliveryMaxDays: number;
  error?: string;
};

export const api = {
  listProducts: (category?: string) =>
    request<Product[]>(
      `/api/products${category ? `?category=${encodeURIComponent(category)}` : ""}`,
    ),
  getProduct: (id: string) =>
    request<Product>(`/api/products/${encodeURIComponent(id)}`),
  checkout: (payload: {
    items: { productId: string; quantity: number; size: string; color: string }[];
    name: string;
    email: string;
    address: string;
    zipCode: string;
    paymentMethod: "pix" | "card";
  }) =>
    request<CheckoutResponse>(`/api/checkout`, {
      method: "POST",
      body: JSON.stringify(payload),
    }),
  quoteShipping: (payload: {
    zipCode: string;
    items: { productId: string; quantity: number }[];
  }) =>
    request<{ options: ShippingOption[]; cached: boolean }>(
      `/api/shipping/quote`,
      { method: "POST", body: JSON.stringify(payload) },
    ),
};
