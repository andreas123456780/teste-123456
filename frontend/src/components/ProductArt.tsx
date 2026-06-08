import { motion } from "framer-motion";
import { useState } from "react";
import { NastLogo } from "./NastLogo";

// Real product photography lives in /public/products/. The `image` field on
// each Product is the file name (e.g. "boxy-black.jpg") which this component
// turns into a responsive <img>. When `hoverImage` is provided and differs
// from `image`, a second <img> is layered on top and fades in on hover —
// the streetwear "back/front swap" that lets shoppers preview the print
// without opening the modal.
// If an image fails to load an animated NAST placeholder is shown instead.
type Props = {
  image: string;
  hoverImage?: string;
  color?: string;
  size?: number;
  className?: string;
  alt?: string;
  /** When true, render the image without the white photo backdrop so a
   * transparent-PNG cutout floats on whatever color the parent provides.
   * Legacy products with JPG photos that have a baked-in white studio
   * background should leave this false. */
  transparent?: boolean;
  /** Controls CSS object-fit. Use "cover" for cards (fills square, no bars)
   * and "contain" for modals (shows full image). Defaults to "contain". */
  fit?: "contain" | "cover";
};

function resolveSrc(image: string): string {
  return /^https?:\/\//.test(image) || image.startsWith("/")
    ? image
    : `/products/${image}`;
}

function ImageFallback() {
  return (
    <motion.div
      className="absolute inset-0 flex flex-col items-center justify-center gap-3 bg-[#0b0b12]"
      initial={{ opacity: 0 }}
      animate={{ opacity: 1 }}
      transition={{ duration: 0.4 }}
    >
      <NastLogo size={48} spin />
      <span className="text-[10px] font-bold uppercase tracking-[0.3em] text-white/30">
        imagem indisponível
      </span>
    </motion.div>
  );
}

export function ProductArt({
  image,
  hoverImage,
  size = 260,
  className,
  alt = "",
  transparent = false,
  fit = "contain",
}: Props) {
  const [frontError, setFrontError] = useState(false);
  const [hoverError, setHoverError] = useState(false);

  const src = resolveSrc(image);
  const hoverSrc =
    hoverImage && hoverImage !== image ? resolveSrc(hoverImage) : null;
  const bgClass = transparent ? "" : " bg-white";
  const fitClass = fit === "cover" ? " object-cover object-center" : " object-contain";
  const baseClass =
    (className ?? "") + " block h-full w-full" + fitClass + bgClass;

  return (
    <div className="relative h-full w-full">
      {frontError ? (
        <ImageFallback />
      ) : (
        <motion.img
          src={src}
          alt={alt}
          width={size}
          height={size}
          className={
            baseClass +
            (hoverSrc && !hoverError
              ? " transition-opacity duration-500 group-hover:opacity-0"
              : "")
          }
          initial={{ opacity: 0, scale: 0.985 }}
          whileInView={{ opacity: 1, scale: 1 }}
          viewport={{ once: true, amount: 0.3 }}
          transition={{ duration: 0.5, ease: "easeOut" }}
          loading="lazy"
          draggable={false}
          onError={() => setFrontError(true)}
        />
      )}
      {hoverSrc && !hoverError && !frontError && (
        <img
          src={hoverSrc}
          alt=""
          aria-hidden="true"
          width={size}
          height={size}
          className={
            baseClass +
            " absolute inset-0 opacity-0 transition-opacity duration-500 group-hover:opacity-100"
          }
          loading="lazy"
          draggable={false}
          onError={() => setHoverError(true)}
        />
      )}
    </div>
  );
}
