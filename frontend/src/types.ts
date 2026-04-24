export type Product = {
  id: string;
  name: string;
  description: string;
  priceCents: number;
  pixPriceCents: number;
  category: string;
  image: string;
  backImage: string;
  colors: string[];
  sizes: string[];
  tags: string[];
  stock: number;
};

export type CartItem = {
  product: Product;
  quantity: number;
  size: string;
  color: string;
};

// CheckoutResponse is what /api/checkout returns once the order has been
// validated and stored on the backend. Since payments are handled by
// Stripe, the order starts as `pending_payment`; the browser exchanges
// `orderId` for a `clientSecret` via /api/payments/intent.
export type CheckoutResponse = {
  orderId: string;
  orderToken: string;
  totalCents: number;
  shippingCents: number;
  amountCents: number;
  discountCents?: number;
  couponCode?: string;
  status: string;
  createdAt: string;
  paymentMethod: "pix" | "card";
};

// PublicOrder mirrors the backend's sanitized order view. Fields that are
// optional on the server are optional here too.
export type PublicOrder = {
  orderId: string;
  status: string;
  paymentMethod: "pix" | "card" | string;
  totalCents: number;
  shippingCents: number;
  amountCents: number;
  shippingName?: string;
  trackingCode?: string;
  trackingUrl?: string;
  createdAt: string;
  updatedAt: string;
  customer: string;
  items: Array<{
    productId: string;
    productName: string;
    size?: string;
    color?: string;
    quantity: number;
    unitPriceCents: number;
  }>;
};

export type PaymentIntentResponse = {
  clientSecret: string;
  orderId: string;
  status: string;
  amountCents: number;
  method: "pix" | "card";
};

// Discount coupon, admin-side shape. The public validate endpoint
// returns a narrower `PublicCoupon` subset that omits usedCount/note.
export type CouponKind = "percent" | "amount" | "free_shipping";

export type Coupon = {
  code: string;
  kind: CouponKind;
  value: number;
  minSubtotalCents: number;
  maxUses: number;
  usedCount: number;
  startsAt?: string | null;
  expiresAt?: string | null;
  active: boolean;
  note?: string;
  createdAt: string;
  updatedAt: string;
};

export type DiscountSummary = {
  itemDiscountCents: number;
  shippingDiscountCents: number;
  totalDiscountCents: number;
  newSubtotalCents: number;
  newShippingCents: number;
  newAmountCents: number;
};

export type ValidateCouponResponse = {
  coupon: {
    code: string;
    kind: CouponKind;
    value: number;
    minSubtotalCents: number;
    expiresAt?: string | null;
  };
  discount: DiscountSummary;
};
