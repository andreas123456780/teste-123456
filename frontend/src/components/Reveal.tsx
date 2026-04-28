import { motion } from "framer-motion";
import type { ReactNode } from "react";

type Props = {
  children: ReactNode;
  /** Delay in seconds before the reveal starts. */
  delay?: number;
  /** Distance in pixels the element travels up while fading in. */
  y?: number;
  /** Tailwind class names applied to the wrapping motion div. */
  className?: string;
  /** Pixels to translate horizontally while revealing (rare). */
  x?: number;
  /** Replay the animation every time the element scrolls into view. */
  repeat?: boolean;
};

/**
 * Reveal is a thin wrapper around `motion.div` that fades and slides its
 * children into view as soon as they intersect the viewport. Used to
 * introduce sections (benefits bar, headings, secondary CTAs) without
 * each one wiring up its own `initial` / `whileInView` triple.
 */
export function Reveal({
  children,
  delay = 0,
  y = 24,
  x = 0,
  className,
  repeat = false,
}: Props) {
  return (
    <motion.div
      className={className}
      initial={{ opacity: 0, y, x }}
      whileInView={{ opacity: 1, y: 0, x: 0 }}
      viewport={{ once: !repeat, amount: 0.2 }}
      transition={{ duration: 0.6, delay, ease: [0.21, 0.78, 0.39, 1] }}
    >
      {children}
    </motion.div>
  );
}
