import { motion } from "framer-motion";
import { WhatsApp } from "./icons";

type Props = {
  phone: string; // E.164 no plus, ex: "5511999999999"
  message?: string;
};

// Floating WhatsApp CTA — sticks to the bottom-right and gently pulses.
// Hidden from viewers who prefer reduced motion (no pulse).
export function WhatsAppButton({
  phone,
  message = "Oi! Queria falar sobre uma peça da NAST.",
}: Props) {
  const href = `https://wa.me/${phone}?text=${encodeURIComponent(message)}`;
  return (
    <motion.a
      href={href}
      target="_blank"
      rel="noreferrer"
      aria-label="Falar no WhatsApp"
      initial={{ opacity: 0, scale: 0.6, y: 40 }}
      animate={{ opacity: 1, scale: 1, y: 0 }}
      transition={{ delay: 1.6, type: "spring", stiffness: 180, damping: 18 }}
      whileHover={{ scale: 1.08 }}
      whileTap={{ scale: 0.94 }}
      className="group fixed bottom-6 right-6 z-40 inline-flex items-center gap-2"
    >
      <motion.span
        aria-hidden="true"
        className="pointer-events-none absolute inset-0 rounded-full bg-[var(--color-accent)]/30"
        animate={{ scale: [1, 1.6, 1], opacity: [0.7, 0, 0.7] }}
        transition={{ duration: 2.2, repeat: Infinity, ease: "easeOut" }}
      />
      <span className="relative flex h-14 w-14 items-center justify-center rounded-full bg-[var(--color-accent)] text-black shadow-[0_8px_32px_-8px_rgba(123,211,255,0.6)] transition group-hover:bg-white">
        <WhatsApp className="h-6 w-6" />
      </span>
      <span className="pointer-events-none hidden whitespace-nowrap rounded-full bg-white px-3 py-1 text-[10px] font-bold uppercase tracking-[0.3em] text-black opacity-0 shadow group-hover:opacity-100 md:inline-block">
        chama no zap
      </span>
    </motion.a>
  );
}
