import { motion } from "framer-motion";

// Real product photography lives in /public/products/. The `image` field on
// each Product is the file name (e.g. "boxy-black.jpg") which this component
// turns into a responsive <img>. Back/front hover swap is supported when
// backImage differs from image, otherwise it's a no-op.
type Props = {
  image: string;
  color?: string;
  size?: number;
  className?: string;
  alt?: string;
};

export function ProductArt({
  image,
  size = 260,
  className,
  alt = "",
}: Props) {
  const src = image.startsWith("/") ? image : `/products/${image}`;
  return (
    <motion.img
      src={src}
      alt={alt}
      width={size}
      height={size}
      className={
        (className ?? "") +
        " block h-full w-full object-contain bg-white"
      }
      initial={{ opacity: 0, scale: 0.985 }}
      whileInView={{ opacity: 1, scale: 1 }}
      viewport={{ once: true, amount: 0.3 }}
      transition={{ duration: 0.5, ease: "easeOut" }}
      loading="lazy"
      draggable={false}
    />
  );
}
