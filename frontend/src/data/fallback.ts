// Fallback catalog used when the API is unreachable so the landing still
// showcases the full experience standalone. Mirrors the Go backend seed.
import type { Product } from "../types";

export const FALLBACK_PRODUCTS: Product[] = [
  {
    id: "p-tee-bw-black",
    name: "CAMISETA BLACK & WHITE",
    description:
      "Camiseta preta em algodão 30.1 penteado com print cursivo frontal em branco. Corte regular, gola reforçada.",
    priceCents: 8990,
    pixPriceCents: 8541,
    category: "Camisetas",
    image: "tee-cursive-black.jpg",
    backImage: "tee-cursive-black.jpg",
    colors: ["preto"],
    sizes: ["P", "M", "G", "Baby Look"],
    tags: ["edição limitada"],
    stock: 24,
  },
  {
    id: "p-tee-bw-white",
    name: "CAMISA BLACK & WHITE",
    description:
      "Camiseta branca em algodão 30.1 penteado com print cursivo frontal em preto. Corte regular, gola reforçada.",
    priceCents: 8990,
    pixPriceCents: 8541,
    category: "Camisetas",
    image: "tee-cursive-white.png",
    backImage: "tee-cursive-white.png",
    colors: ["branco"],
    sizes: ["P", "M", "G", "Baby Look"],
    tags: ["edição limitada"],
    stock: 24,
  },
  {
    id: "p-boxy-black",
    name: "CAMISA BOXY NAST PRETA",
    description:
      "Camiseta boxy preta em algodão pesado 240g com modelagem oversized, ombro caído e etiqueta tecida NAST.",
    priceCents: 9990,
    pixPriceCents: 9491,
    category: "Boxy",
    image: "boxy-black.jpg",
    backImage: "boxy-black.jpg",
    colors: ["preto"],
    sizes: ["P", "M", "G"],
    tags: ["boxy fit"],
    stock: 18,
  },
  {
    id: "p-boxy-white",
    name: "CAMISETA BOXY NAST BRANCA",
    description:
      "Camiseta boxy branca em algodão pesado 240g com modelagem oversized, ombro caído e etiqueta tecida NAST.",
    priceCents: 9990,
    pixPriceCents: 9491,
    category: "Boxy",
    image: "boxy-white.jpg",
    backImage: "boxy-white.jpg",
    colors: ["branco"],
    sizes: ["P", "M", "G"],
    tags: ["boxy fit"],
    stock: 18,
  },
];
