import { motion } from "framer-motion";

// NAST mark: outer frame + 5-pointed star with an accent square. Reused
// across the header, loading screen and footer so there's a single source
// of truth for the brand mark.
export function NastLogo({ size = 32 }: { size?: number }) {
  return (
    <motion.svg
      width={size}
      height={size}
      viewBox="0 0 64 64"
      initial={{ opacity: 0, scale: 0.7 }}
      animate={{ opacity: 1, scale: 1 }}
      transition={{ type: "spring", stiffness: 180, damping: 16 }}
      aria-hidden="true"
    >
      <rect
        x="4"
        y="4"
        width="56"
        height="56"
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
      />
      <motion.path
        d="M32 14 L37.2 27.4 L51.6 28.5 L40.4 37.6 L44.1 51.5 L32 43.8 L19.9 51.5 L23.6 37.6 L12.4 28.5 L26.8 27.4 Z"
        fill="currentColor"
        initial={{ scale: 0, opacity: 0 }}
        animate={{ scale: 1, opacity: 1 }}
        transition={{ delay: 0.35, type: "spring", stiffness: 220, damping: 18 }}
        style={{ transformOrigin: "32px 32px" }}
      />
      <motion.rect
        x="46"
        y="10"
        width="6"
        height="6"
        fill="var(--color-accent)"
        initial={{ scale: 0 }}
        animate={{ scale: 1 }}
        transition={{ delay: 0.9, type: "spring", stiffness: 260 }}
        style={{ transformOrigin: "49px 13px" }}
      />
    </motion.svg>
  );
}
