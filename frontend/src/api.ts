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
    // credentials:"include" is required for the /api/auth/* flows —
    // the session is a cookie set by the backend and has to travel
    // across the frontend→backend CORS boundary in prod. For
    // endpoints that don't need auth it's still safe: the browser
    // only sends the cookie when the backend has ACAO echoing the
    // Origin and ACAC=true, which our CORS middleware only does for
    // the whitelisted origins.
    credentials: "include",
    ...init,
    headers: {
      ...(init?.body != null ? { "Content-Type": "application/json" } : {}),
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

export type CountdownSettings = {
  visible: boolean;
  title: string;
  subtitle: string;
  /** RFC3339 string. Empty means "no target set" — frontend hides the
   * timer or shows the ended label depending on Visible. */
  targetAt: string;
  ctaLabel: string;
  ctaUrl: string;
  endedLabel: string;
};


export type BannerSettings = {
  visible: boolean;
  text: string;
  link?: string;
  linkLabel?: string;
  bgColor?: 'accent' | 'white' | 'black';
};
export type SecretSettings = {
  locked: boolean;
  code: string;
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
    document: string;
    address: string;
    addressNumber: string;
    addressComplement?: string;
    district: string;
    city: string;
    state: string;
    zipCode: string;
    paymentMethod: "pix" | "card";
    shipping?: { serviceId: number; serviceName: string; priceCents: number };
    couponCode?: string;
  }) =>
    request<CheckoutResponse>(`/api/checkout`, {
      method: "POST",
      body: JSON.stringify(payload),
    }),
  lookupCep: (cep: string) =>
    request<{
      cep: string;
      logradouro: string;
      bairro: string;
      cidade: string;
      uf: string;
    }>(`/api/cep/${encodeURIComponent(cep.replace(/\D/g, ""))}`),
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
  getCountdown: () =>
    request<CountdownSettings>(`/api/settings?key=countdown`),
  getSecret: () =>
    request<SecretSettings>(`/api/settings?key=secret`),
  getBanner: () =>
    request<BannerSettings>(`/api/settings?key=banner`),
};

// AuthUser mirrors backend authMeResponse. Returned by every
// /api/auth/* endpoint on success and cached client-side by the
// useAuth hook.
export type AuthUser = {
  id: string;
  email: string;
  name: string;
  emailVerified: boolean;
  hasPassword: boolean;
  hasGoogle: boolean;
};

// authApi wraps the customer-facing auth endpoints. Every call relies
// on the session cookie already being attached by credentials:"include"
// in request().
export const authApi = {
  signup: (payload: { name: string; email: string; password: string }) =>
    request<AuthUser>(`/api/auth/signup`, {
      method: "POST",
      body: JSON.stringify(payload),
    }),
  login: (payload: { email: string; password: string }) =>
    request<AuthUser>(`/api/auth/login`, {
      method: "POST",
      body: JSON.stringify(payload),
    }),
  logout: () =>
    request<{ status: string }>(`/api/auth/logout`, { method: "POST" }),
  me: () => request<AuthUser>(`/api/auth/me`),
  // googleStartUrl returns the URL the browser should navigate to in
  // order to begin the Google OAuth dance. Kept as a helper (rather
  // than a location.assign) so components can decide whether to open
  // it in a new tab or the same one.
  googleStartUrl: () => `${API_BASE}/api/auth/google/start`,
};

// MyOrder mirrors the backend myOrder shape returned by
// /api/account/orders. Used to render /minha-conta.
export type MyOrder = {
  id: string;
  orderToken?: string;
  status: string;
  paymentMethod: string;
  totalCents: number;
  shippingCents: number;
  discountCents?: number;
  amountCents: number;
  couponCode?: string;
  trackingCode?: string;
  trackingUrl?: string;
  shippingService?: string;
  createdAt: string;
  updatedAt: string;
  items: Array<{
    productId: string;
    productName: string;
    size?: string;
    color?: string;
    quantity: number;
    unitPriceCents: number;
  }>;
};

// accountApi wraps the authenticated /api/account/* surface. All
// calls require an active session cookie (sent automatically by the
// base request() helper).
export const accountApi = {
  myOrders: () =>
    request<{ orders: MyOrder[] }>(`/api/account/orders`).then(
      (r) => r.orders,
    ),
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
  // uploadImage posts a single file as multipart/form-data and returns
  // the public URL (hosted on Vercel Blob). The browser sets the
  // multipart boundary automatically when the body is a FormData, so
  // we must NOT pre-set Content-Type here.
  uploadImage: async (token: string, file: File) => {
    const fd = new FormData();
    fd.append("file", file);
    const res = await fetch(`${API_BASE}/api/admin/upload`, {
      method: "POST",
      headers: { "X-Admin-Token": token },
      body: fd,
    });
    if (!res.ok) {
      const text = await res.text().catch(() => "");
      throw new Error(`upload ${res.status}: ${text || res.statusText}`);
    }
    return (await res.json()) as { url: string; pathname: string };
  },
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

// AdminOrder mirrors the Go adminOrderDetail struct returned by
// GET /api/admin/orders and GET /api/admin/orders/:id.
export type AdminOrder = {
  orderId: string;
  status: string;
  createdAt: string;
  updatedAt: string;
  name: string;
  email: string;
  document?: string;
  address: string;
  addressNumber?: string;
  addressComplement?: string;
  district?: string;
  city?: string;
  state?: string;
  zip: string;
  paymentMethod: string;
  totalCents: number;
  shippingCents: number;
  amountCents: number;
  discountCents?: number;
  couponCode?: string;
  shippingServiceId?: number;
  shippingServiceName?: string;
  trackingCode?: string;
  trackingUrl?: string;
  labelUrl?: string;
  superfreteId?: string;
  trackingAttempts?: number;
  trackingLastError?: string;
  items: {
    productId: string;
    productName: string;
    size?: string;
    color?: string;
    quantity: number;
    unitPriceCents: number;
  }[];
};

export const adminOrdersApi = {
  list: (token: string, opts?: { status?: string; limit?: number; offset?: number }) => {
    const q = new URLSearchParams();
    if (opts?.status) q.set("status", opts.status);
    if (opts?.limit) q.set("limit", String(opts.limit));
    if (opts?.offset) q.set("offset", String(opts.offset));
    const suffix = q.toString();
    return adminRequest<{ count: number; orders: AdminOrder[] }>(
      `/api/admin/orders${suffix ? "?" + suffix : ""}`,
      token,
    );
  },
  detail: (token: string, orderId: string) =>
    adminRequest<AdminOrder>(
      `/api/admin/orders/${encodeURIComponent(orderId)}`,
      token,
    ),
  remove: (token: string, orderId: string) =>
    adminRequest<void>(
      `/api/admin/orders/${encodeURIComponent(orderId)}`,
      token,
      { method: "DELETE" },
    ),
  retryLabel: (
    token: string,
    orderId: string,
    overrides?: { name?: string; document?: string; district?: string; city?: string; state?: string },
  ) =>
    adminRequest<{
      orderId: string;
      status: string;
      trackingCode?: string;
      trackingUrl?: string;
      labelUrl?: string;
    }>(
      `/api/admin/orders/${encodeURIComponent(orderId)}/retry-label`,
      token,
      {
        method: "POST",
        body: overrides ? JSON.stringify(overrides) : undefined,
      },
    ),
  refreshTracking: (token: string, orderId: string) =>
    adminRequest<{
      orderId: string;
      status: string;
      updated: boolean;
      trackingCode?: string;
      trackingUrl?: string;
      hint?: string;
    }>(
      `/api/admin/orders/${encodeURIComponent(orderId)}/refresh-tracking`,
      token,
      { method: "POST" },
    ),
  markShipped: (
    token: string,
    orderId: string,
    payload: { trackingCode: string; trackingUrl?: string },
  ) =>
    adminRequest<{
      orderId: string;
      status: string;
      trackingCode: string;
      trackingUrl: string;
    }>(
      `/api/admin/orders/${encodeURIComponent(orderId)}/mark-shipped`,
      token,
      {
        method: "POST",
        body: JSON.stringify(payload),
      },
    ),
  markPaid: (token: string, orderId: string) =>
    adminRequest<AdminOrder>(
      `/api/admin/orders/${encodeURIComponent(orderId)}/mark-paid`,
      token,
      { method: "POST" },
    ),
  addItem: (
    token: string,
    orderId: string,
    payload: {
      productId: string;
      size: string;
      quantity: number;
      mode: "gift" | "extra";
    },
  ) =>
    adminRequest<AdminOrder>(
      `/api/admin/orders/${encodeURIComponent(orderId)}/items`,
      token,
      {
        method: "POST",
        body: JSON.stringify(payload),
      },
    ),
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

export const adminSettingsApi = {
  getCountdown: (token: string) =>
    adminRequest<CountdownSettings>(`/api/admin/settings?key=countdown`, token),
  saveCountdown: (token: string, payload: CountdownSettings) =>
    adminRequest<CountdownSettings>(`/api/admin/settings?key=countdown`, token, {
      method: "PUT",
      body: JSON.stringify(payload),
    }),
  getSecret: (token: string) =>
    adminRequest<SecretSettings>(`/api/admin/settings?key=secret`, token),
  getBanner: (token: string) =>
    adminRequest<BannerSettings>(`/api/admin/settings?key=banner`, token),
  saveBanner: (token: string, payload: BannerSettings) =>
    adminRequest<BannerSettings>(`/api/admin/settings?key=banner`, token, {
      method: 'PUT',
      body: JSON.stringify(payload),
    }),
  saveSecret: (token: string, payload: SecretSettings) =>
    adminRequest<SecretSettings>(`/api/admin/settings?key=secret`, token, {
      method: "PUT",
      body: JSON.stringify(payload),
    }),
};
