import { motion } from "framer-motion";
import { useState } from "react";
import { Close } from "./icons";

// Size chart based on the official NAST measurements (width, length and bust
// in cm). Rendered both as a compact inline table on the products section
// and as a modal dialog opened from product cards / modal.

type Row = { size: string; largura: number; comprimento: number; busto?: number };

const CAMISETA: Row[] = [
  { size: "P", largura: 49, comprimento: 59, busto: 72 },
  { size: "M", largura: 51, comprimento: 60, busto: 76 },
  { size: "G", largura: 54, comprimento: 62, busto: 82 },
];

const BOXY: Row[] = [
  { size: "P", largura: 57, comprimento: 57, busto: 120 },
  { size: "M", largura: 59, comprimento: 60, busto: 124 },
  { size: "G", largura: 62, comprimento: 62, busto: 126 },
];

const BABY_LOOK: Row[] = [
  { size: "P", largura: 46, comprimento: 59 },
  { size: "M", largura: 48, comprimento: 61 },
];

function Table({ title, rows, showBusto = true }: { title: string; rows: Row[]; showBusto?: boolean }) {
  return (
    <div>
      <div className="mb-3 text-[11px] font-bold uppercase tracking-[0.3em] text-white">
        {title}
      </div>
      <div className="overflow-hidden border border-white/10">
        <table className="w-full border-collapse text-left text-[12px]">
          <thead className="bg-white/[0.04] text-[10px] uppercase tracking-[0.2em] text-white/50">
            <tr>
              <th className="px-3 py-2 font-semibold">Tamanho</th>
              <th className="px-3 py-2 font-semibold">Largura</th>
              <th className="px-3 py-2 font-semibold">Comprimento</th>
              {showBusto && <th className="px-3 py-2 font-semibold">Busto</th>}
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => (
              <tr key={r.size} className="border-t border-white/10 text-white/80">
                <td className="px-3 py-2 font-bold text-white">★ {r.size}</td>
                <td className="px-3 py-2">{r.largura} cm</td>
                <td className="px-3 py-2">{r.comprimento} cm</td>
                {showBusto && <td className="px-3 py-2">{r.busto ?? "—"} cm</td>}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function Content({ dense = false }: { dense?: boolean }) {
  return (
    <div className={dense ? "space-y-6" : "space-y-8"}>
      <Table title="Camisetas (modelagem normal)" rows={CAMISETA} />
      <Table title="Camiseta Boxy" rows={BOXY} />
      <Table title="Baby Look" rows={BABY_LOOK} showBusto={false} />
      <ul className="space-y-1.5 border-t border-white/10 pt-4 text-[11px] leading-relaxed text-white/60">
        <li>• Medidas podem variar ±2 cm.</li>
        <li>• Largura = medida de axila a axila.</li>
        <li>• Comprimento = do ombro até a barra.</li>
        <li>• Boxy = mais larga, ombro caído, caimento quadrado.</li>
        <li>• Baby Look = mais ajustada ao corpo.</li>
      </ul>
    </div>
  );
}

// Inline panel for the products section.
export function SizeChartPanel() {
  return (
    <aside className="sticky top-28 hidden lg:block">
      <div className="border border-white/10 bg-[var(--color-bg-soft)] p-6">
        <div className="eyebrow mb-2 text-white/50">Tabela de Medidas</div>
        <h3 className="mb-5 text-xl font-black tracking-tight text-white">
          Como vestir certo.
        </h3>
        <Content dense />
      </div>
    </aside>
  );
}

// Compact button + modal dialog for use inside product cards / modal.
export function SizeChartLink({ label = "Tabela de medidas" }: { label?: string }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button
        type="button"
        onClick={(e) => {
          e.stopPropagation();
          setOpen(true);
        }}
        className="text-[11px] font-bold uppercase tracking-[0.25em] text-white/60 underline underline-offset-4 transition hover:text-white"
      >
        {label}
      </button>
      {open && (
        <motion.div
          role="dialog"
          aria-label="Tabela de medidas"
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          className="fixed inset-0 z-[60] flex items-center justify-center bg-black/80 px-4 py-10 backdrop-blur"
          onClick={() => setOpen(false)}
        >
          <motion.div
            onClick={(e) => e.stopPropagation()}
            initial={{ y: 40, opacity: 0, scale: 0.97 }}
            animate={{ y: 0, opacity: 1, scale: 1 }}
            transition={{ type: "spring", stiffness: 220, damping: 24 }}
            className="relative max-h-[85vh] w-full max-w-xl overflow-y-auto border border-white/10 bg-[var(--color-bg-soft)] p-8"
          >
            <button
              aria-label="Fechar"
              onClick={() => setOpen(false)}
              className="absolute right-4 top-4 border border-white/10 bg-black/40 p-2 text-white/70 hover:text-white"
            >
              <Close className="h-4 w-4" />
            </button>
            <div className="eyebrow mb-2 text-white/50">NAST</div>
            <h2 className="mb-6 text-2xl font-black tracking-tight text-white">
              Tabela de Medidas
            </h2>
            <Content />
          </motion.div>
        </motion.div>
      )}
    </>
  );
}
