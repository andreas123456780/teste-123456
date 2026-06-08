import { motion, AnimatePresence } from "framer-motion";
import { useState } from "react";
import { SECRET_CODE } from "../data/fallback";

type Props = {
  onUnlocked: () => void;
};

export function SecretUnlock({ onUnlocked }: Props) {
  const [code, setCode] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [shake, setShake] = useState(false);

  function tryUnlock() {
    if (code.trim().toUpperCase() === SECRET_CODE) {
      onUnlocked();
    } else {
      setError("Código inválido. Tente novamente.");
      setShake(true);
      setTimeout(() => setShake(false), 500);
    }
  }

  return (
    <motion.div
      className="flex flex-col items-center justify-center border border-white/10 bg-[var(--color-bg-soft)] p-8 text-center"
      initial={{ opacity: 0, scale: 0.97 }}
      animate={{ opacity: 1, scale: 1 }}
      transition={{ type: "spring", stiffness: 200, damping: 24 }}
    >
      <div className="mb-2 text-4xl">🔒</div>
      <div className="eyebrow mb-2 text-white/40">ACESSO RESTRITO</div>
      <h3 className="text-xl font-black uppercase tracking-tight text-white">
        Peça Secreta
      </h3>
      <p className="mt-2 max-w-xs text-sm text-white/50">
        Você encontrou algo raro. Digite o código para desbloquear.
      </p>

      <motion.div
        className="mt-6 flex w-full max-w-xs flex-col gap-3"
        animate={shake ? { x: [0, -8, 8, -6, 6, 0] } : {}}
        transition={{ duration: 0.4 }}
      >
        <input
          type="text"
          value={code}
          onChange={(e) => {
            setCode(e.target.value);
            setError(null);
          }}
          onKeyDown={(e) => e.key === "Enter" && tryUnlock()}
          placeholder="CÓDIGO SECRETO"
          className="w-full border border-white/15 bg-transparent px-4 py-3 text-center text-sm font-bold uppercase tracking-[0.3em] text-white placeholder-white/20 outline-none transition focus:border-[var(--color-accent)]"
          autoFocus
        />
        <motion.button
          onClick={tryUnlock}
          whileHover={{ y: -1 }}
          whileTap={{ scale: 0.97 }}
          className="w-full bg-[var(--color-accent)] px-4 py-3 text-xs font-black uppercase tracking-[0.3em] text-black transition hover:bg-white"
        >
          Desbloquear
        </motion.button>
      </motion.div>

      <AnimatePresence>
        {error && (
          <motion.p
            initial={{ opacity: 0, y: -4 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0 }}
            className="mt-3 text-xs text-red-400"
          >
            {error}
          </motion.p>
        )}
      </AnimatePresence>
    </motion.div>
  );
}
