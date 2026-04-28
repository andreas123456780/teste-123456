import { motion } from "framer-motion";

// Real product photography lives in /public/products/. The `image` field on
// each Product is the file name (e.g. "boxy-black.jpg") which this component
// turns into a responsive <img>. When `hoverImage` is provided and differs
// from `image`, a second <img> is layered on top and fades in on hover —
// the streetwear "back/front swap" that lets shoppers preview the print
// without opening the modal.
type Props = {
  image: string;
  hoverImage?: string;
  color?: string;
  size?: number;
  className?: string;
  alt?: string;
};

function resolveSrc(image: string): string {
  return /^https?:\/\//.test(image) || image.startsWith("/")
    ? image
    : `/products/${image}`;
}

export function ProductArt({
  image,
  hoverImage,
  size = 260,
  className,
  alt = "",
}: Props) {
  const src = resolveSrc(image);
  const hoverSrc =
    hoverImage && hoverImage !== image ? resolveSrc(hoverImage) : null;
  const baseClass =
    (className ?? "") + " block h-full w-full object-contain bg-white";
  return (
    <div className="relative h-full w-full">
      <motion.img
        src={src}
        alt={alt}
        width={size}
        height={size}
        className={
          baseClass +
          (hoverSrc
            ? " transition-opacity duration-500 group-hover:opacity-0"
            : "")
        }
        initial={{ opacity: 0, scale: 0.985 }}
        whileInView={{ opacity: 1, scale: 1 }}
        viewport={{ once: true, amount: 0.3 }}
        transition={{ duration: 0.5, ease: "easeOut" }}
        loading="lazy"
        draggable={false}
      />
      {hoverSrc && (
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
        />
      )}
    </div>
  );
}
