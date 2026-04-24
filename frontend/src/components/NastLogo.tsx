import { motion, useReducedMotion } from "framer-motion";

// NAST wordmark. Renders the PNG wordmark as a continuously rotating
// mark — the star sits at the center of the "Nast" word so a 360°
// rotation reads as brand-forward without spinning individual letters.
//
// Honors prefers-reduced-motion: users who opt out see the static
// wordmark with no animation.
//
// Original aspect ratio is 451 × 365 (≈ 1.24). `size` is interpreted as
// the rendered HEIGHT so callers can keep the existing prop shape.
const ASPECT = 451 / 365;

export function NastLogo({
  size = 40,
  spin = true,
}: {
  size?: number;
  spin?: boolean;
}) {
  const prefersReducedMotion = useReducedMotion();
  const shouldSpin = spin && !prefersReducedMotion;
  const width = Math.round(size * ASPECT);
  return (
    <motion.img
      src="/brand/logo-nast.png"
      alt="NAST"
      width={width}
      height={size}
      style={{ width, height: size, display: "block" }}
      // Invert so the black wordmark reads as white on dark backgrounds
      // without shipping a second asset.
      className="[filter:invert(1)]"
      initial={{ opacity: 0, rotate: 0 }}
      animate={shouldSpin ? { opacity: 1, rotate: 360 } : { opacity: 1 }}
      transition={
        shouldSpin
          ? {
              opacity: { duration: 0.6 },
              rotate: { duration: 18, ease: "linear", repeat: Infinity },
            }
          : { opacity: { duration: 0.6 } }
      }
      draggable={false}
    />
  );
}
