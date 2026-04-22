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

export type CheckoutResponse = {
  orderId: string;
  totalCents: number;
  status: string;
  createdAt: string;
  estimatedAt: string;
};
