import { motion, useScroll, useTransform } from "framer-motion";
import { NastLogo } from "./NastLogo";
import { ShoppingBag } from "./icons";

type Props = {
  cartCount: number;
  onOpenCart: () => void;
};

export function Header({ cartCount, onOpenCart }: Props) {
  const { scrollY } = useScroll();
  const bg = useTransform(scrollY, [0, 120], [
    "rgba(6,6,6,0)",
    "rgba(6,6,6,0.9)",
  ]);
  const border = useTransform(scrollY, [0, 120], [
    "rgba(255,255,255,0)",
    "rgba(255,255,255,0.12)",
  ]);
  const blur = useTransform(scrollY, [0, 120], ["blur(0px)", "blur(14px)"]);

  return (
    <motion.header
      style={{
        backgroundColor: bg,
        borderColor: border,
        backdropFilter: blur,
        WebkitBackdropFilter: blur,
      }}
      className="fixed inset-x-0 top-[30px] z-40 border-b"
    >
      <div className="mx-auto flex max-w-7xl items-center justify-between px-6 py-4">
        <motion.a
          href="#top"
          className="flex items-center gap-3 text-white"
          whileHover={{ opacity: 0.8 }}
          whileTap={{ scale: 0.97 }}
        >
          <NastLogo size={28} />
          <span className="text-lg font-black tracking-[0.4em]">NAST</span>
        </motion.a>

        <nav className="hidden items-center gap-10 text-xs font-semibold uppercase tracking-[0.25em] text-white/70 md:flex">
          {[
            ["Loja", "#products"],
            ["Medidas", "#products"],
            ["Manifesto", "#story"],
            ["Contato", "#newsletter"],
          ].map(([label, href]) => (
            <motion.a
              key={label}
              href={href}
              className="relative transition-colors hover:text-white"
              whileHover="hover"
            >
              {label}
              <motion.span
                className="absolute -bottom-1 left-0 h-[2px] w-full origin-left bg-[var(--color-accent)]"
                initial={{ scaleX: 0 }}
                variants={{ hover: { scaleX: 1 } }}
                transition={{ type: "spring", stiffness: 260, damping: 22 }}
              />
            </motion.a>
          ))}
        </nav>

        <motion.button
          onClick={onOpenCart}
          whileHover={{ y: -1 }}
          whileTap={{ scale: 0.97 }}
          className="relative flex items-center gap-2 border border-white/20 px-4 py-2 text-xs font-semibold uppercase tracking-[0.3em] text-white transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
        >
          <ShoppingBag className="h-4 w-4" />
          <span className="hidden sm:inline">Sacola</span>
          {cartCount > 0 && (
            <motion.span
              key={cartCount}
              initial={{ scale: 0.4, opacity: 0 }}
              animate={{ scale: 1, opacity: 1 }}
              transition={{ type: "spring", stiffness: 500, damping: 20 }}
              className="ml-1 flex h-5 min-w-5 items-center justify-center rounded-none bg-[var(--color-accent)] px-1.5 text-[11px] font-bold text-black"
            >
              {cartCount}
            </motion.span>
          )}
        </motion.button>
      </div>
    </motion.header>
  );
}
