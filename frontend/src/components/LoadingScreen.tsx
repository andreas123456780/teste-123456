import { AnimatePresence, motion } from "framer-motion";
import { NastLogo } from "./NastLogo";

export function LoadingScreen({ show }: { show: boolean }) {
  return (
    <AnimatePresence>
      {show && (
        <motion.div
          initial={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          transition={{ duration: 0.6 }}
          className="fixed inset-0 z-[100] flex flex-col items-center justify-center bg-[var(--color-bg)]"
        >
          <motion.div
            animate={{ rotate: [0, 0, 180, 180, 360] }}
            transition={{ duration: 2.4, repeat: Infinity, ease: "easeInOut" }}
          >
            <NastLogo size={64} />
          </motion.div>
          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            transition={{ delay: 0.3 }}
            className="mt-6 text-[11px] font-bold uppercase tracking-[0.6em] text-white/60"
          >
            NAST · carregando
          </motion.div>
        </motion.div>
      )}
    </AnimatePresence>
  );
}
