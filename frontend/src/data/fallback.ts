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
    image: "tee-cursive-black.jpeg",
    backImage: "tee-cursive-black-back.jpeg",
    colors: ["preto"],
    sizes: ["P", "M", "G", "Baby Look"],
    tags: ["edição limitada"],
    stock: 24,
    stockBySize: { P: 6, M: 6, G: 6, "Baby Look": 6 },
  },
  {
    id: "p-tee-bw-white",
    name: "CAMISA BLACK & WHITE",
    description:
      "Camiseta branca em algodão 30.1 penteado com print cursivo frontal em preto. Corte regular, gola reforçada.",
    priceCents: 8990,
    pixPriceCents: 8541,
    category: "Camisetas",
    image: "tee-cursive-white.jpeg",
    backImage: "tee-cursive-white-back.jpeg",
    colors: ["branco"],
    sizes: ["P", "M", "G", "Baby Look"],
    tags: ["edição limitada"],
    stock: 24,
    stockBySize: { P: 6, M: 6, G: 6, "Baby Look": 6 },
  },
  {
    id: "p-boxy-black",
    name: "CAMISA BOXY NAST PRETA",
    description:
      "Camiseta boxy preta em algodão pesado 240g com modelagem oversized, ombro caído e etiqueta tecida NAST.",
    priceCents: 9990,
    pixPriceCents: 9491,
    category: "Boxy",
    image: "boxy-black.jpeg",
    backImage: "boxy-black-back.jpeg",
    colors: ["preto"],
    sizes: ["P", "M", "G"],
    tags: ["boxy fit"],
    stock: 18,
    stockBySize: { P: 6, M: 6, G: 6 },
  },
  {
    id: "p-boxy-white",
    name: "CAMISETA BOXY NAST BRANCA",
    description:
      "Camiseta boxy branca em algodão pesado 240g com modelagem oversized, ombro caído e etiqueta tecida NAST.",
    priceCents: 9990,
    pixPriceCents: 9491,
    category: "Boxy",
    image: "boxy-white.jpeg",
    backImage: "boxy-white-back.jpeg",
    colors: ["branco"],
    sizes: ["P", "M", "G"],
    tags: ["boxy fit"],
    stock: 18,
    stockBySize: { P: 6, M: 6, G: 6 },
  },
  {
    id: "p-bb-look-black",
    name: "BABY LOOK NAST TEE",
    description:
      'Baby look preta em algodão 30.1 penteado com print cursivo "Just be You" frontal em branco. Corte ajustado feminino, gola reforçada e etiqueta tecida NAST.',
    priceCents: 7990,
    pixPriceCents: 7591,
    category: "Baby Look",
    image: "bb-look-black.jpeg",
    backImage: "bb-look-black-back.jpeg",
    colors: ["preto"],
    sizes: ["Baby Look"],
    tags: ["edição limitada"],
    stock: 12,
    stockBySize: { "Baby Look": 12 },
  },
  {
    id: "p-jorge-black",
    name: "CAMISA NAST JORGE",
    description:
      "Camiseta boxy preta com arte exclusiva de São Jorge. Algodão pesado 240g, modelagem oversized e etiqueta tecida NAST.",
    priceCents: 9990,
    pixPriceCents: 9491,
    category: "Boxy",
    image: "tee-jorge.png",
    backImage: "tee-jorge.png",
    transparentImage: true,
    colors: ["preto"],
    sizes: ["P", "M", "G"],
    tags: ["boxy fit", "edição limitada"],
    stock: 10,
    stockBySize: { P: 4, M: 4, G: 2 },
  },
  {
    id: "p-bb-look-white",
    name: "BABY LOOK NAST TEE",
    description:
      'Baby look branca em algodão 30.1 penteado com print cursivo "Just be You" frontal em preto. Corte ajustado feminino, gola reforçada e etiqueta tecida NAST.',
    priceCents: 7990,
    pixPriceCents: 7591,
    category: "Baby Look",
    image: "bb-look-white.jpeg",
    backImage: "bb-look-white-back.jpeg",
    colors: ["branco"],
    sizes: ["Baby Look"],
    tags: ["edição limitada"],
    stock: 12,
    stockBySize: { "Baby Look": 12 },
  },
];

// Peça secreta — só aparece após desbloqueio com código
export const SECRET_PRODUCT: Product = {
  id: "p-secret-red",
  name: "NST VERMELHA",
  description:
    "Peça exclusiva em vermelho. Produção limitadíssima, reservada para quem sabe onde procurar.",
  priceCents: 12990,
  pixPriceCents: 12341,
  category: "Secreto",
  image: "tee-red.jpg",
  backImage: "tee-red.jpg",
  colors: ["vermelho"],
  sizes: ["P", "M", "G"],
  tags: ["secreto", "exclusivo"],
  stock: 5,
  stockBySize: { P: 2, M: 2, G: 1 },
};

// Código de desbloqueio da peça secreta
export const SECRET_CODE = "10820";
