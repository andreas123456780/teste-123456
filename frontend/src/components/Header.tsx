import { motion, useScroll, useTransform } from "framer-motion";
import { NastLogo } from "./NastLogo";
import { Instagram, Linktree, ShoppingBag } from "./icons";
import { useAuth } from "../lib/useAuth";

const INSTAGRAM_URL = "https://www.instagram.com/nast.comm/";
const LINKTREE_URL =
  "https://linktr.ee/nast.comm?utm_source=ig&utm_medium=social&utm_content=link_in_bio";

type Props = {
  cartCount: number;
  onOpenCart: () => void;
};

export function Header({ cartCount, onOpenCart }: Props) {
  // Auth state decides whether the top bar shows "Entrar" or the
  // signed-in user's first name with a Sair button. When auth is
  // disabled server-side (503), the affordance is hidden entirely.
  const auth = useAuth();
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
          aria-label="NAST — ir ao topo"
          className="flex items-center text-white"
          whileHover={{ opacity: 0.85 }}
          whileTap={{ scale: 0.97 }}
        >
          <NastLogo size={44} />
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

        <div className="flex items-center gap-2">
          {!auth.disabled && !auth.loading && (
            auth.user ? (
              <div className="hidden items-center gap-2 sm:flex">
                <a
                  href="/minha-conta"
                  className="max-w-[160px] truncate border border-white/20 px-3 py-2 text-[11px] font-semibold uppercase tracking-[0.2em] text-white/80 transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
                  aria-label="Minha conta"
                >
                  {firstName(auth.user.name, auth.user.email)}
                </a>
                <button
                  type="button"
                  onClick={async () => {
                    await auth.logout();
                    window.location.reload();
                  }}
                  className="border border-white/20 px-3 py-2 text-[11px] font-semibold uppercase tracking-[0.3em] text-white/70 transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
                >
                  Sair
                </button>
              </div>
            ) : (
              <a
                href="/login"
                className="hidden border border-white/20 px-3 py-2 text-[11px] font-semibold uppercase tracking-[0.3em] text-white/70 transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)] sm:inline-block"
              >
                Entrar
              </a>
            )
          )}
          <motion.a
            href={INSTAGRAM_URL}
            target="_blank"
            rel="noreferrer"
            aria-label="Instagram NAST"
            whileHover={{ y: -1 }}
            whileTap={{ scale: 0.95 }}
            className="hidden h-9 w-9 items-center justify-center border border-white/20 text-white/70 transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)] sm:inline-flex"
          >
            <Instagram className="h-4 w-4" />
          </motion.a>
          <motion.a
            href={LINKTREE_URL}
            target="_blank"
            rel="noreferrer"
            aria-label="Linktree NAST"
            whileHover={{ y: -1 }}
            whileTap={{ scale: 0.95 }}
            className="hidden h-9 w-9 items-center justify-center border border-white/20 text-white/70 transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)] sm:inline-flex"
          >
            <Linktree className="h-4 w-4" />
          </motion.a>
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
      </div>
    </motion.header>
  );
}

// firstName returns the customer's display chip for the header.
// Prefers the first token of the full name; falls back to the local
// part of the email when name is empty (Google OAuth with missing
// given_name falls into this path).
function firstName(name: string, email: string): string {
  const n = (name || "").trim().split(/\s+/)[0];
  if (n) return n;
  const local = (email || "").split("@")[0];
  return local || "conta";
}
