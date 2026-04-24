import { useEffect } from "react";
import type { Product } from "../types";

// Injects a schema.org Product graph for every catalog entry as
// application/ld+json. Google / Bing / Meta scrape the rendered HTML
// after JS runs, so injecting post-hydration is enough for rich
// results even without SSR. The node is keyed so hot-reload and
// product list refreshes replace cleanly.
const NODE_ID = "nast-products-jsonld";
const SITE_ORIGIN = "https://nastt.com.br";
const PRODUCTS_PREFIX = `${SITE_ORIGIN}/products`;

function absoluteImage(src: string): string {
  if (/^https?:\/\//.test(src)) return src;
  if (src.startsWith("/")) return `${SITE_ORIGIN}${src}`;
  return `${PRODUCTS_PREFIX}/${src}`;
}

function productGraph(product: Product) {
  return {
    "@type": "Product",
    "@id": `${SITE_ORIGIN}/#${product.id}`,
    name: product.name,
    description: product.description,
    image: [absoluteImage(product.image)],
    brand: { "@type": "Brand", name: "NAST" },
    category: product.category,
    sku: product.id,
    offers: {
      "@type": "Offer",
      priceCurrency: "BRL",
      price: (product.priceCents / 100).toFixed(2),
      availability:
        product.stock > 0
          ? "https://schema.org/InStock"
          : "https://schema.org/OutOfStock",
      url: SITE_ORIGIN + "/",
    },
  };
}

export function SeoJsonLd({ products }: { products: Product[] }) {
  useEffect(() => {
    if (products.length === 0) return;
    const payload = {
      "@context": "https://schema.org",
      "@graph": products.map(productGraph),
    };
    let node = document.getElementById(NODE_ID) as HTMLScriptElement | null;
    if (!node) {
      node = document.createElement("script");
      node.id = NODE_ID;
      node.type = "application/ld+json";
      document.head.appendChild(node);
    }
    node.text = JSON.stringify(payload);
    return () => {
      // Keep the tag across re-renders; only clear if unmounting.
      // (App.tsx never unmounts in practice.)
    };
  }, [products]);
  return null;
}
