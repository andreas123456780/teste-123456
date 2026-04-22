import { NastLogo } from "./NastLogo";
import { Instagram, TikTok, WhatsApp } from "./icons";

type Props = { whatsAppNumber?: string };

export function Footer({ whatsAppNumber }: Props = {}) {
  const waHref = whatsAppNumber ? `https://wa.me/${whatsAppNumber}` : "https://wa.me/";
  return (
    <footer className="relative border-t border-white/10 bg-black/80 px-6 py-16 text-sm text-white/50">
      <div className="mx-auto grid max-w-7xl grid-cols-2 gap-10 md:grid-cols-4">
        <div>
          <div className="flex items-center gap-3 text-white">
            <NastLogo size={28} />
            <span className="text-lg font-black tracking-[0.4em]">NAST</span>
          </div>
          <p className="mt-4 text-xs text-white/50">
            Streetwear autoral feito no Brasil. Drops limitados e produção
            consciente.
          </p>
        </div>
        <div>
          <div className="mb-4 text-[11px] font-bold uppercase tracking-[0.3em] text-white/70">
            Loja
          </div>
          <ul className="space-y-2">
            <li><a href="#products" className="hover:text-white">Camisetas</a></li>
            <li><a href="#products" className="hover:text-white">Boxy</a></li>
            <li><a href="#products" className="hover:text-white">Baby Look</a></li>
            <li><a href="#products" className="hover:text-white">Tabela de medidas</a></li>
          </ul>
        </div>
        <div>
          <div className="mb-4 text-[11px] font-bold uppercase tracking-[0.3em] text-white/70">
            Ajuda
          </div>
          <ul className="space-y-2">
            <li>Trocas e devoluções</li>
            <li>Envio e rastreio</li>
            <li>Guia de medidas</li>
            <li>Contato</li>
          </ul>
        </div>
        <div>
          <div className="mb-4 text-[11px] font-bold uppercase tracking-[0.3em] text-white/70">
            Social
          </div>
          <ul className="flex items-center gap-3">
            <li>
              <a
                href="https://instagram.com"
                target="_blank"
                rel="noreferrer"
                className="inline-flex h-9 w-9 items-center justify-center border border-white/15 text-white/60 transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
                aria-label="Instagram"
              >
                <Instagram className="h-4 w-4" />
              </a>
            </li>
            <li>
              <a
                href="https://tiktok.com"
                target="_blank"
                rel="noreferrer"
                className="inline-flex h-9 w-9 items-center justify-center border border-white/15 text-white/60 transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
                aria-label="TikTok"
              >
                <TikTok className="h-4 w-4" />
              </a>
            </li>
            <li>
              <a
                href={waHref}
                target="_blank"
                rel="noreferrer"
                className="inline-flex h-9 w-9 items-center justify-center border border-white/15 text-white/60 transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
                aria-label="WhatsApp"
              >
                <WhatsApp className="h-4 w-4" />
              </a>
            </li>
          </ul>
        </div>
      </div>
      <div className="mx-auto mt-12 flex max-w-7xl flex-col items-start justify-between gap-3 border-t border-white/10 pt-6 text-[11px] uppercase tracking-[0.25em] text-white/40 md:flex-row md:items-center">
        <div>© {new Date().getFullYear()} NAST — todos os direitos reservados.</div>
        <div className="flex items-center gap-2">
          <span className="pulse-dot" />
          Streetwear autoral
        </div>
      </div>
    </footer>
  );
}
