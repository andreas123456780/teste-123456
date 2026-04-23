import { Header } from "../components/Header";
import { Footer } from "../components/Footer";

// Returns / exchanges / shipping policy, split from /termos so the
// customer-facing content isn't buried inside a legal document. Same
// editing approach as Privacy.tsx: rare updates, version-controlled.

type Props = {
  whatsAppNumber: string;
  supportEmail: string;
};

export function Returns({ whatsAppNumber, supportEmail }: Props) {
  const waLink = (text: string) =>
    `https://wa.me/${whatsAppNumber}?text=${encodeURIComponent(text)}`;
  return (
    <div className="noise relative min-h-full">
      <Header cartCount={0} onOpenCart={() => { /* cart hidden */ }} />
      <main className="mx-auto max-w-3xl px-6 py-16 text-white/80">
        <div className="eyebrow text-white/40">ajuda</div>
        <h1 className="mt-2 text-3xl font-black tracking-tight text-white md:text-4xl">
          Trocas, devoluções e envio
        </h1>
        <p className="mt-3 text-sm text-white/50">
          Tudo que você precisa saber pra receber sua peça e, se precisar,
          trocar. Atendimento humano por{" "}
          <a className="underline" href={waLink("Oi, preciso de ajuda com uma peça NAST.")} target="_blank" rel="noreferrer noopener">
            WhatsApp
          </a>
          {" "}ou{" "}
          <a className="underline" href={`mailto:${supportEmail}`}>
            {supportEmail}
          </a>
          .
        </p>

        <Section title="Direito de arrependimento (7 dias)">
          Pelo Código de Defesa do Consumidor (art. 49), você tem 7 dias
          corridos a partir do recebimento pra desistir da compra sem precisar
          dar motivo. Devolução 100% gratuita: nós mandamos a etiqueta por
          email após sua solicitação.
        </Section>

        <Section title="Trocas por defeito ou tamanho">
          <List
            items={[
              "Defeito de fabricação: troca integral dentro de 30 dias. Nós cobrimos o frete ida e volta.",
              "Troca de tamanho: até 15 dias após o recebimento, peça sem uso, com etiquetas e embalagem original. Frete de ida por conta da NAST, volta por conta do cliente (usando etiqueta SuperFrete que geramos).",
              "Trocas não são feitas em peças usadas, lavadas ou com sinais de perfume.",
            ]}
          />
        </Section>

        <Section title="Como solicitar">
          <List
            items={[
              "Chame a gente no WhatsApp com foto da peça e o número do pedido (ex: NAST-123).",
              "A gente confirma a elegibilidade e manda a etiqueta de devolução por email (SuperFrete).",
              "Despachar em qualquer agência dos Correios em até 5 dias úteis.",
              "Assim que a peça chegar aqui e passar na inspeção, o reembolso / a nova peça sai em até 3 dias úteis.",
            ]}
          />
        </Section>

        <div id="envio" />
        <Section title="Envio e prazos">
          <List
            items={[
              "Saímos de São Paulo/SP (08503-000).",
              "Transportadora: SuperFrete / Correios (PAC, SEDEX e LOGGI disponíveis conforme CEP).",
              "Prazo começa a contar após a confirmação do pagamento. Cartão: imediato. Pix via WhatsApp: assim que enviarmos a confirmação.",
              "Prazo típico SEDEX: 1–3 dias úteis em capitais; PAC: 3–7 dias úteis. CEPs remotos podem exceder.",
              "Rastreio é enviado por email e também disponível na página do seu pedido (link no email de confirmação).",
            ]}
          />
        </Section>

        <Section title="Pagamento Pix via WhatsApp">
          Enquanto nossa conta Stripe está em homologação para Pix
          automático, o pagamento Pix é feito por WhatsApp: você escolhe
          Pix no carrinho, seu pedido fica reservado, e abrimos uma conversa
          com a chave/QR Code. Assim que você paga, a gente confirma e o
          pedido entra em produção/envio igual ao cartão.
        </Section>

        <Section title="Cancelamento">
          Pedidos que ainda não foram despachados podem ser cancelados por
          WhatsApp a qualquer momento. Pedidos já em trânsito são tratados
          como devolução (regra acima).
        </Section>

        <div className="mt-12 flex flex-col gap-3">
          <a
            href={waLink("Oi, preciso de ajuda com trocas/devolução NAST.")}
            target="_blank"
            rel="noreferrer noopener"
            className="inline-flex w-fit items-center gap-2 bg-[var(--color-accent)] px-6 py-3 text-xs font-bold uppercase tracking-[0.3em] text-black hover:bg-white"
          >
            Falar no WhatsApp
          </a>
          <a
            href="/"
            className="text-xs uppercase tracking-widest text-white/60 hover:text-white"
          >
            ← voltar à loja
          </a>
        </div>
      </main>
      <Footer whatsAppNumber={whatsAppNumber} />
    </div>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="mt-10">
      <h2 className="text-lg font-black text-white">{title}</h2>
      <div className="mt-2 text-sm leading-relaxed text-white/70">{children}</div>
    </section>
  );
}

function List({ items }: { items: string[] }) {
  return (
    <ul className="mt-2 list-disc space-y-1 pl-5">
      {items.map((i) => (
        <li key={i}>{i}</li>
      ))}
    </ul>
  );
}
