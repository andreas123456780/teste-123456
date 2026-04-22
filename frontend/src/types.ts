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
  totalCents: number;
  shippingCents: number;
  amountCents: number;
  status: string;
  createdAt: string;
  paymentMethod: "pix" | "card";
};

export type PaymentIntentResponse = {
  clientSecret: string;
  orderId: string;
  status: string;
  amountCents: number;
  method: "pix" | "card";
};
