import { useCallback, useEffect, useMemo, useState } from "react";
import { api } from "./api";
import { FALLBACK_PRODUCTS } from "./data/fallback";
import type { CartItem, Product } from "./types";

import { Header } from "./components/Header";
import { ToastHost } from "./components/ToastHost";
import { toast } from "./lib/toast";
import { Hero } from "./components/Hero";
import { HeroCarousel } from "./components/HeroCarousel";
import { Countdown } from "./components/Countdown";
import { Products } from "./components/Products";
import { Story } from "./components/Story";
import { Newsletter } from "./components/Newsletter";
import { BenefitsBar } from "./components/BenefitsBar";
import { Footer } from "./components/Footer";
import { Cart } from "./components/Cart";
import { CookieBanner } from "./components/CookieBanner";
import { ProductModal } from "./components/ProductModal";
import { ScrollProgress } from "./components/ScrollProgress";
import { LoadingScreen } from "./components/LoadingScreen";
import { ScanIntro } from "./components/ScanIntro";
import { shouldShowIntro } from "./lib/intro";
import { WhatsAppButton } from "./components/WhatsAppButton";
import { InstagramFeed } from "./components/InstagramFeed";
import { SeoJsonLd } from "./components/SeoJsonLd";
import { OrderStatus } from "./pages/OrderStatus";
import { Privacy } from "./pages/Privacy";
import { Terms } from "./pages/Terms";
import { Returns } from "./pages/Returns";
import { AdminPage } from "./pages/Admin";
import { AuthPage } from "./pages/Auth";
import { MinhaConta } from "./pages/MinhaConta";
import { Sobre } from "./pages/Sobre";

const WHATSAPP_NUMBER = "5511910859392";
const SUPPORT_EMAIL = "contato@nast.com.br";
const INSTAGRAM_HANDLE = "nast.oficial";

// Minimal route matcher. We avoid react-router to keep the bundle lean;
// the app only has three top-level routes plus the storefront. Each route
// reads window.location once on mount and renders accordingly. SPA
// navigation is not needed — users land on these via email links or
// direct navigation.
type Route =
  | { kind: "home" }
  | { kind: "order"; token: string }
  | { kind: "privacy" }
  | { kind: "terms" }
  | { kind: "returns" }
  | { kind: "sobre" }
  | { kind: "admin" }
  | { kind: "login" }
  | { kind: "signup" }
  | { kind: "minha-conta" };

function parseRoute(pathname: string): Route {
  const orderMatch = pathname.match(/^\/pedido\/([^/?#]+)\/?$/);
  if (orderMatch) return { kind: "order", token: decodeURIComponent(orderMatch[1]) };
  if (pathname === "/privacidade" || pathname === "/privacidade/") {
    return { kind: "privacy" };
  }
  if (pathname === "/termos" || pathname === "/termos/") {
    return { kind: "terms" };
  }
  if (pathname === "/trocas" || pathname === "/trocas/") {
    return { kind: "returns" };
  }
  if (
    pathname === "/sobre" ||
    pathname === "/sobre/" ||
    pathname === "/manifesto" ||
    pathname === "/manifesto/"
  ) {
    return { kind: "sobre" };
  }
  if (pathname === "/admin" || pathname === "/admin/") {
    return { kind: "admin" };
  }
  if (pathname === "/login" || pathname === "/login/") {
    return { kind: "login" };
  }
  if (pathname === "/cadastro" || pathname === "/cadastro/") {
    return { kind: "signup" };
  }
  if (pathname === "/minha-conta" || pathname === "/minha-conta/") {
    return { kind: "minha-conta" };
  }
  return { kind: "home" };
}

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
  const route = useMemo(() => parseRoute(window.location.pathname), []);

  if (route.kind === "order") {
    return <OrderStatus token={route.token} whatsAppNumber={WHATSAPP_NUMBER} />;
  }
  if (route.kind === "privacy") {
    return (
      <Privacy
        whatsAppNumber={WHATSAPP_NUMBER}
        supportEmail={SUPPORT_EMAIL}
      />
    );
  }
  if (route.kind === "terms") {
    return (
      <Terms whatsAppNumber={WHATSAPP_NUMBER} supportEmail={SUPPORT_EMAIL} />
    );
  }
  if (route.kind === "returns") {
    return (
      <Returns whatsAppNumber={WHATSAPP_NUMBER} supportEmail={SUPPORT_EMAIL} />
    );
  }
  if (route.kind === "sobre") {
    return <Sobre whatsAppNumber={WHATSAPP_NUMBER} />;
  }
  if (route.kind === "admin") {
    return <AdminPage />;
  }
  if (route.kind === "minha-conta") {
    return <MinhaConta whatsAppNumber={WHATSAPP_NUMBER} />;
  }
  if (route.kind === "login" || route.kind === "signup") {
    // Preserve ?next=/some/path so the auth page sends the user back
    // where they came from after a successful login.
    const next = new URL(window.location.href).searchParams.get("next");
    return (
      <AuthPage
        mode={route.kind}
        whatsAppNumber={WHATSAPP_NUMBER}
        redirectTo={next || "/"}
      />
    );
  }
  return <Home />;
}

function Home() {
  // Start with an empty catalog so the Products section renders skeleton
  // tiles while the API call is in flight. If the API fails we fall back
  // to the hard-coded catalog (offline safety net).
  const [products, setProducts] = useState<Product[]>([]);
  const [loading, setLoading] = useState(true);
  const [banner, setBanner] = useState<BannerSettings | null>(null);
  const [secretSettings, setSecretSettings] = useState<SecretSettings | null>(null);
  const [productsLoading, setProductsLoading] = useState(true);
  const [introVisible, setIntroVisible] = useState(() => shouldShowIntro());
  const [cart, setCart] = useState<CartItem[]>(() => loadCart());
  // `?checkout=1` is the signal the auth page tacks onto its
  // redirect: it means "I just came back from logging in, open the
  // cart so I can finish paying." Parsed once on mount to avoid a
  // reopen loop.
  const [cartOpen, setCartOpen] = useState(() => {
    if (typeof window === "undefined") return false;
    return new URL(window.location.href).searchParams.get("checkout") === "1";
  });
  const [modal, setModal] = useState<Product | null>(null);
  const [modalPreferredSize, setModalPreferredSize] = useState<
    string | undefined
  >(undefined);

  const openModal = useCallback((p: Product, preferredSize?: string) => {
    setModal(p);
    setModalPreferredSize(preferredSize);
  }, []);
  const closeModal = useCallback(() => {
    setModal(null);
    setModalPreferredSize(undefined);
  }, []);

  useEffect(() => {
    let cancelled = false;
    const start = Date.now();
    api.getBanner().then(setBanner).catch(() => {});
    api.getSecret().then(setSecretSettings).catch(() => {});
    api
      .listProducts()
      .then((list) => {
        if (cancelled) return;
        const catalog = list.length === 0 ? FALLBACK_PRODUCTS : list;
        setProducts(catalog);
        // Drop cart lines whose product no longer exists in the
        // catalog — e.g. an admin-deleted product still cached in
        // localStorage. Otherwise the checkout throws "unknown
        // product" on submit with no recovery path for the customer.
        const ids = new Set(catalog.map((p) => p.id));
        setCart((prev) => {
          const next = prev.filter((c) => ids.has(c.product.id));
          return next.length === prev.length ? prev : next;
        });
      })
      .catch(() => {
        if (cancelled) return;
        // API unreachable — ship the offline catalog so the storefront
        // still has something to show.
        setProducts(FALLBACK_PRODUCTS);
      })
      .finally(() => {
        if (cancelled) return;
        setProductsLoading(false);
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
    (
      product: Product,
      size: string,
      color: string,
      options?: { openCart?: boolean },
    ) => {
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
      // "Comprar agora" still pops the drawer open so the user can
      // finish checkout in one motion. "Adicionar à sacola" only
      // fires a toast — the cart counter in the header pulses to
      // confirm the add and the toast offers an explicit CTA to
      // open the drawer when ready.
      if (options?.openCart) {
        setCartOpen(true);
      } else {
        const detail = [size, color].filter(Boolean).join(" · ");
        toast({
          kind: "success",
          title: "Adicionado à sacola",
          description: `${product.name}${detail ? " — " + detail : ""}`,
          actionLabel: "Ver sacola",
          onAction: () => setCartOpen(true),
        });
      }
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
      <AnnouncementBanner settings={banner} />
      <ScrollProgress />
      <Header cartCount={cartCount} onOpenCart={() => setCartOpen(true)} />

      <main>
        <Hero />
        <HeroCarousel />
        <Countdown />
        <Products
          products={products}
          onOpen={openModal}
          whatsAppNumber={WHATSAPP_NUMBER}
          loading={productsLoading}
          secretProductId={secretSettings?.productId}
        />
        <Story />
        <InstagramFeed handle={INSTAGRAM_HANDLE} />
        <Newsletter />
      </main>

      <BenefitsBar />
      <Footer whatsAppNumber={WHATSAPP_NUMBER} />

      <ProductModal
        product={modal}
        preferredSize={modalPreferredSize}
        onClose={closeModal}
        onAdd={addToCart}
      />
      <Cart
        open={cartOpen}
        items={cart}
        onClose={() => setCartOpen(false)}
        onUpdateQty={updateQty}
        onRemove={removeItem}
        onClear={clearCart}
        whatsAppNumber={WHATSAPP_NUMBER}
      />

      <WhatsAppButton phone={WHATSAPP_NUMBER} />
      <CookieBanner />
      <ToastHost />
      <SeoJsonLd products={products} />
      <LoadingScreen show={loading && !introVisible} />
      {introVisible && <ScanIntro onFinish={() => setIntroVisible(false)} />}
    </div>
  );
}

export default App;
