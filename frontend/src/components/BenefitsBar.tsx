import type { ComponentType, SVGProps } from "react";
import { Truck, Refresh, Lock, Sparkle } from "./icons";
import { Reveal } from "./Reveal";

type Benefit = {
  Icon: ComponentType<SVGProps<SVGSVGElement>>;
  title: string;
  detail: string;
};

const BENEFITS: Benefit[] = [
  {
    Icon: Truck,
    title: "Envio nacional",
    detail: "Frete via SuperFrete + Correios pra todo o Brasil.",
  },
  {
    Icon: Sparkle,
    title: "Pix com desconto",
    detail: "Pagamentos via Pix saem mais barato — direto no checkout.",
  },
  {
    Icon: Refresh,
    title: "Trocas em 7 dias",
    detail: "Trocou de ideia? Você tem 7 dias após receber a peça.",
  },
  {
    Icon: Lock,
    title: "Pagamento seguro",
    detail: "Stripe + checkout criptografado. A NAST nunca vê seu cartão.",
  },
];

/**
 * BenefitsBar is the reassurance strip rendered just above the Footer.
 * It surfaces the four guarantees that move the needle on streetwear
 * conversion — shipping coverage, Pix discount, return window and
 * payment security — using the same monochrome / accent-yellow design
 * language as the rest of the storefront.
 */
export function BenefitsBar() {
  return (
    <section
      aria-label="Benefícios da NAST"
      className="border-y border-white/10 bg-black/60 px-6 py-12"
    >
      <Reveal className="mx-auto grid max-w-7xl grid-cols-2 gap-8 md:grid-cols-4">
        {BENEFITS.map(({ Icon, title, detail }) => (
          <div key={title} className="flex flex-col gap-3">
            <span className="inline-flex h-10 w-10 items-center justify-center border border-white/15 text-[var(--color-accent)]">
              <Icon className="h-5 w-5" />
            </span>
            <div className="text-[11px] font-bold uppercase tracking-[0.3em] text-white">
              {title}
            </div>
            <p className="text-xs leading-relaxed text-white/55">{detail}</p>
          </div>
        ))}
      </Reveal>
    </section>
  );
}
