import { motion } from "framer-motion";
import { Header } from "../components/Header";
import { Footer } from "../components/Footer";
import { BenefitsBar } from "../components/BenefitsBar";
import { Reveal } from "../components/Reveal";
import { Instagram, WhatsApp } from "../components/icons";

// /sobre — institutional page that expands the manifesto bullet on the
// home page into a full editorial spread (story, founder, behind the
// scenes, contact). Pure storytelling: no products, no carts. Loaded
// from the footer link "Sobre" and from the header on internal nav.

type Props = {
  whatsAppNumber: string;
};

const INSTAGRAM_URL = "https://www.instagram.com/nast.comm/";

export function Sobre({ whatsAppNumber }: Props) {
  const waLink = `https://wa.me/${whatsAppNumber}?text=${encodeURIComponent(
    "Oi! Quero saber mais sobre a NAST.",
  )}`;
  return (
    <div className="noise relative min-h-full">
      <Header cartCount={0} onOpenCart={() => { /* cart hidden on Sobre */ }} />

      <main>
        <section className="relative mx-auto max-w-5xl px-6 pb-16 pt-32 md:pt-40">
          <Reveal>
            <div className="eyebrow text-white/40">Manifesto</div>
            <h1 className="mt-3 text-5xl font-black tracking-tighter text-white md:text-7xl">
              FEITO PRA QUEM <span className="acid">VESTE O QUE ACREDITA.</span>
            </h1>
          </Reveal>
          <Reveal delay={0.15}>
            <p className="mt-8 max-w-2xl text-lg leading-relaxed text-white/70">
              NAST nasceu na periferia leste de São Paulo como manifesto
              silencioso: peças com construção honesta, tiragem limitada e zero
              barulho. Cada drop é numerado. Cada peça passa por inspeção
              manual. Nada é descartável.
            </p>
          </Reveal>
        </section>

        <section className="relative mx-auto max-w-5xl px-6 py-16">
          <div className="grid grid-cols-1 gap-10 md:grid-cols-3">
            {[
              [
                "01",
                "Design",
                "Estética geométrica, sem excessos. Cada estampa é estudada e revisada — nada vira drop só pra ocupar espaço no feed.",
              ],
              [
                "02",
                "Materiais",
                "Algodão 30.1 penteado, malha boxy 240g/m². Costura reforçada e acabamento double-needle. A peça envelhece bem, lavagem após lavagem.",
              ],
              [
                "03",
                "Produção",
                "Costureiras parceiras pagas por peça acabada, não por hora. Pequenos lotes (40-80 unidades) pra evitar estoque parado e desperdício.",
              ],
            ].map(([n, t, d], i) => (
              <Reveal key={n} delay={i * 0.1}>
                <div>
                  <div className="num-display text-5xl font-black text-[var(--color-accent)] md:text-6xl">
                    {n}
                  </div>
                  <div className="mt-3 text-lg font-bold uppercase tracking-widest text-white">
                    {t}
                  </div>
                  <p className="mt-2 text-sm leading-relaxed text-white/60">
                    {d}
                  </p>
                </div>
              </Reveal>
            ))}
          </div>
        </section>

        <section className="relative bg-white/[0.02] py-20">
          <div className="mx-auto grid max-w-5xl grid-cols-1 gap-12 px-6 md:grid-cols-2 md:items-center">
            <Reveal>
              <div>
                <div className="eyebrow text-white/40">Bastidores</div>
                <h2 className="mt-3 text-4xl font-black tracking-tight text-white md:text-5xl">
                  Drop pequeno, atenção grande.
                </h2>
                <p className="mt-6 text-base leading-relaxed text-white/70">
                  Cada lote leva entre 3 e 5 semanas pra sair do desenho até a
                  embalagem. A gente desenha, manda pro estampador, recebe a
                  prova, ajusta a saturação, costura, inspeciona, fotografa e
                  só então solta. Não terceiriza voz da marca.
                </p>
                <ul className="mt-6 space-y-2 text-sm text-white/60">
                  <li>· Tecido cortado e costurado em São Paulo capital.</li>
                  <li>· Estampa serigrafada à mão, base resistente a lavagem.</li>
                  <li>· Embalagem com kraft reciclado e selo numerado.</li>
                  <li>· Inspeção peça-a-peça antes do envio.</li>
                </ul>
              </div>
            </Reveal>
            <Reveal delay={0.15}>
              <div className="grid grid-cols-2 gap-3">
                {[
                  "/carousel/lookbook-01.jpg",
                  "/carousel/lookbook-02.jpg",
                  "/carousel/lookbook-03.jpg",
                  "/carousel/lookbook-04.jpg",
                ].map((src, i) => (
                  <motion.img
                    key={src}
                    src={src}
                    alt=""
                    aria-hidden
                    className="aspect-[4/5] w-full object-cover"
                    initial={{ opacity: 0, y: 12 }}
                    whileInView={{ opacity: 1, y: 0 }}
                    viewport={{ once: true, amount: 0.3 }}
                    transition={{ delay: i * 0.07, duration: 0.5 }}
                    loading="lazy"
                  />
                ))}
              </div>
            </Reveal>
          </div>
        </section>

        <section className="relative mx-auto max-w-5xl px-6 py-20">
          <Reveal>
            <div className="border border-white/10 bg-white/[0.02] p-8 md:p-12">
              <div className="eyebrow text-white/40">Contato</div>
              <h2 className="mt-3 text-3xl font-black tracking-tight text-white md:text-4xl">
                Fala direto com a gente.
              </h2>
              <p className="mt-4 max-w-xl text-sm leading-relaxed text-white/60">
                Dúvida sobre tamanho, parceria, troca ou só pra trocar uma
                ideia: a gente responde pessoalmente.
              </p>
              <div className="mt-6 flex flex-wrap gap-3">
                <a
                  href={waLink}
                  target="_blank"
                  rel="noreferrer"
                  className="inline-flex items-center gap-2 bg-[var(--color-accent)] px-5 py-3 text-xs font-black uppercase tracking-[0.3em] text-black transition hover:bg-white"
                >
                  <WhatsApp className="h-4 w-4" />
                  WhatsApp
                </a>
                <a
                  href={INSTAGRAM_URL}
                  target="_blank"
                  rel="noreferrer"
                  className="inline-flex items-center gap-2 border border-white/20 px-5 py-3 text-xs font-bold uppercase tracking-[0.3em] text-white transition hover:border-white"
                >
                  <Instagram className="h-4 w-4" />
                  @nast.comm
                </a>
              </div>
            </div>
          </Reveal>
        </section>
      </main>

      <BenefitsBar />
      <Footer whatsAppNumber={whatsAppNumber} />
    </div>
  );
}
