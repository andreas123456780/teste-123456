import { useCallback, useEffect, useMemo, useState } from "react";
import { api } from "./api";
import { FALLBACK_PRODUCTS } from "./data/fallback";
import type { CartItem, Product } from "./types";

import { Header } from "./components/Header";
import { Hero } from "./components/Hero";
import { Products } from "./components/Products";
import { Story } from "./components/Story";
import { Newsletter } from "./components/Newsletter";
import { Footer } from "./components/Footer";
import { Cart } from "./components/Cart";
import { ProductModal } from "./components/ProductModal";
import { ScrollProgress } from "./components/ScrollProgress";
import { LoadingScreen } from "./components/LoadingScreen";
import { ScanIntro } from "./components/ScanIntro";
import { shouldShowIntro } from "./lib/intro";
import { WhatsAppButton } from "./components/WhatsAppButton";

const WHATSAPP_NUMBER = "5511994281802";

function cartKey(id: string, size: string, color: string) {
  return `${id}|${size}|${color}`;
}

function loadCart(): CartItem[] {
  try {
    const raw = localStorage.getItem("nast:cart");
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed as CartItem[];
  } catch {
    return [];
  }
}

function App() {
  const [products, setProducts] = useState<Product[]>(FALLBACK_PRODUCTS);
  const [loading, setLoading] = useState(true);
  const [introVisible, setIntroVisible] = useState(() => shouldShowIntro());
  const [cart, setCart] = useState<CartItem[]>(() => loadCart());
  const [cartOpen, setCartOpen] = useState(false);
  const [modal, setModal] = useState<Product | null>(null);

  useEffect(() => {
    let cancelled = false;
    const start = Date.now();
    api
      .listProducts()
      .then((list) => {
        if (!cancelled && list.length > 0) setProducts(list);
      })
      .catch(() => {
        /* fallback already loaded */
      })
      .finally(() => {
        if (cancelled) return;
        const elapsed = Date.now() - start;
        const delay = Math.max(0, 900 - elapsed);
        setTimeout(() => setLoading(false), delay);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    localStorage.setItem("nast:cart", JSON.stringify(cart));
  }, [cart]);

  const addToCart = useCallback(
    (product: Product, size: string, color: string) => {
      setCart((prev) => {
        const key = cartKey(product.id, size, color);
        const idx = prev.findIndex(
          (it) => cartKey(it.product.id, it.size, it.color) === key,
        );
        if (idx >= 0) {
          const next = [...prev];
          next[idx] = { ...next[idx], quantity: next[idx].quantity + 1 };
          return next;
        }
        return [...prev, { product, size, color, quantity: 1 }];
      });
      setCartOpen(true);
    },
    [],
  );

  const updateQty = useCallback(
    (id: string, size: string, color: string, qty: number) => {
      setCart((prev) =>
        prev.map((it) =>
          cartKey(it.product.id, it.size, it.color) === cartKey(id, size, color)
            ? { ...it, quantity: qty }
            : it,
        ),
      );
    },
    [],
  );

  const removeItem = useCallback(
    (id: string, size: string, color: string) => {
      setCart((prev) =>
        prev.filter(
          (it) =>
            cartKey(it.product.id, it.size, it.color) !==
            cartKey(id, size, color),
        ),
      );
    },
    [],
  );

  const clearCart = useCallback(() => setCart([]), []);

  const cartCount = useMemo(
    () => cart.reduce((acc, it) => acc + it.quantity, 0),
    [cart],
  );

  return (
    <div className="noise relative min-h-full">
      <ScrollProgress />
      <Header cartCount={cartCount} onOpenCart={() => setCartOpen(true)} />

      <main>
        <Hero />
        <Products
          products={products}
          onOpen={setModal}
          whatsAppNumber={WHATSAPP_NUMBER}
        />
        <Story />
        <Newsletter />
      </main>

      <Footer whatsAppNumber={WHATSAPP_NUMBER} />

      <ProductModal
        product={modal}
        onClose={() => setModal(null)}
        onAdd={addToCart}
        whatsAppNumber={WHATSAPP_NUMBER}
      />
      <Cart
        open={cartOpen}
        items={cart}
        onClose={() => setCartOpen(false)}
        onUpdateQty={updateQty}
        onRemove={removeItem}
        onClear={clearCart}
      />

      <WhatsAppButton phone={WHATSAPP_NUMBER} />
      <LoadingScreen show={loading && !introVisible} />
      {introVisible && <ScanIntro onFinish={() => setIntroVisible(false)} />}
    </div>
  );
}

export default App;
