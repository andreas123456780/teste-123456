import type {
  CheckoutResponse,
  Coupon,
  PaymentIntentResponse,
  Product,
  PublicOrder,
  ValidateCouponResponse,
} from "./types";

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
    shipping?: { serviceId: number; serviceName: string; priceCents: number };
    couponCode?: string;
  }) =>
    request<CheckoutResponse>(`/api/checkout`, {
      method: "POST",
      body: JSON.stringify(payload),
    }),
  validateCoupon: (payload: {
    code: string;
    subtotalCents: number;
    shippingCents: number;
  }) =>
    request<ValidateCouponResponse>(`/api/coupons/validate`, {
      method: "POST",
      body: JSON.stringify(payload),
    }),
  createPaymentIntent: (orderId: string) =>
    request<PaymentIntentResponse>(`/api/payments/intent`, {
      method: "POST",
      body: JSON.stringify({ orderId }),
    }),
  quoteShipping: (payload: {
    zipCode: string;
    items: { productId: string; quantity: number }[];
  }) =>
    request<{ options: ShippingOption[]; cached: boolean }>(
      `/api/shipping/quote`,
      { method: "POST", body: JSON.stringify(payload) },
    ),
  getOrderByToken: (token: string) =>
    request<PublicOrder>(`/api/orders/${encodeURIComponent(token)}`),
};

// Admin API — callers supply the X-Admin-Token header. Token is stored
// in localStorage client-side (see useAdminToken). Never commit real
// tokens: operators enter them in the /admin login form.
async function adminRequest<T>(
  path: string,
  token: string,
  init?: RequestInit,
): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    ...init,
    headers: {
      ...(init?.body != null ? { "Content-Type": "application/json" } : {}),
      "X-Admin-Token": token,
      ...(init?.headers ?? {}),
    },
  });
  if (res.status === 204) return undefined as unknown as T;
  if (!res.ok) {
    const text = await res.text().catch(() => "");
    throw new Error(`API ${res.status}: ${text || res.statusText}`);
  }
  return (await res.json()) as T;
}

export type AdminProductPayload = Product & {
  hidden?: boolean;
  sortOrder?: number;
};

export const adminApi = {
  list: (token: string) =>
    adminRequest<Product[]>(`/api/admin/products`, token),
  create: (token: string, payload: AdminProductPayload) =>
    adminRequest<Product>(`/api/admin/products`, token, {
      method: "POST",
      body: JSON.stringify(payload),
    }),
  update: (token: string, id: string, payload: AdminProductPayload) =>
    adminRequest<Product>(
      `/api/admin/products/${encodeURIComponent(id)}`,
      token,
      { method: "PUT", body: JSON.stringify(payload) },
    ),
  remove: (token: string, id: string) =>
    adminRequest<void>(
      `/api/admin/products/${encodeURIComponent(id)}`,
      token,
      { method: "DELETE" },
    ),
};

// AdminCouponPayload mirrors the Go adminCouponPayload struct exactly.
// Callers may omit `active` (defaults to true) and any optional date.
export type AdminCouponPayload = {
  code: string;
  kind: "percent" | "amount" | "free_shipping";
  value: number;
  minSubtotalCents: number;
  maxUses: number;
  startsAt?: string | null;
  expiresAt?: string | null;
  active?: boolean;
  note?: string;
};

// AdminLoginResponse is returned by POST /api/admin/login on success.
export type AdminLoginResponse = {
  token: string;
  username: string;
  expiresAt: string;
};

// AdminStats mirrors the Go adminStats struct.
export type AdminStats = {
  generatedAt: string;
  orders: {
    total: number;
    pendingPayment: number;
    paid: number;
    shipped: number;
    failed: number;
    canceled: number;
  };
  revenue: {
    grossCents: number;
    shippingCents: number;
    discountCents: number;
    paidOrderCount: number;
    avgTicketCents: number;
  };
  topProducts: {
    productId: string;
    productName: string;
    quantity: number;
    grossCents: number;
  }[];
  revenueByDay: {
    day: string;
    grossCents: number;
    orderCount: number;
  }[];
  recentOrders: {
    id: string;
    status: string;
    paymentMethod: string;
    amountCents: number;
    customerName: string;
    createdAt: string;
  }[];
};

export const adminAuthApi = {
  // login returns an HMAC session token, NOT the legacy ADMIN_TOKEN.
  // Backend accepts either as X-Admin-Token on subsequent calls.
  login: (username: string, password: string) =>
    request<AdminLoginResponse>(`/api/admin/login`, {
      method: "POST",
      body: JSON.stringify({ username, password }),
    }),
  stats: (token: string) => adminRequest<AdminStats>(`/api/admin/stats`, token),
};

export const adminCouponsApi = {
  list: (token: string) =>
    adminRequest<Coupon[]>(`/api/admin/coupons`, token),
  create: (token: string, payload: AdminCouponPayload) =>
    adminRequest<Coupon>(`/api/admin/coupons`, token, {
      method: "POST",
      body: JSON.stringify(payload),
    }),
  update: (token: string, code: string, payload: AdminCouponPayload) =>
    adminRequest<Coupon>(
      `/api/admin/coupons/${encodeURIComponent(code)}`,
      token,
      { method: "PUT", body: JSON.stringify(payload) },
    ),
  remove: (token: string, code: string) =>
    adminRequest<void>(
      `/api/admin/coupons/${encodeURIComponent(code)}`,
      token,
      { method: "DELETE" },
    ),
};
