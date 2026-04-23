import { Header } from "../components/Header";
import { Footer } from "../components/Footer";

type Props = {
  whatsAppNumber: string;
  supportEmail: string;
};

export function Terms({ whatsAppNumber, supportEmail }: Props) {
  return (
    <div className="noise relative min-h-full">
      <Header cartCount={0} onOpenCart={() => { /* cart hidden */ }} />
      <main className="mx-auto max-w-3xl px-6 py-16 text-white/80">
        <div className="eyebrow text-white/40">legal</div>
        <h1 className="mt-2 text-3xl font-black tracking-tight text-white md:text-4xl">
          Termos de Uso
        </h1>
        <p className="mt-3 text-sm text-white/50">
          Ao finalizar um pedido você concorda com estes termos.
        </p>

        <Section title="1. Produtos">
          Todos os produtos comercializados são peças NAST de edição limitada.
          Ilustrações, medidas e cores são meramente informativas; pequenas
          variações são inerentes à produção artesanal.
        </Section>

        <Section title="2. Pagamento">
          Cartão é processado pela Stripe (PCI-DSS nível 1). Pix é
          finalizado via WhatsApp com o atendimento, enquanto nossa conta
          Stripe está em homologação para Pix automático. Pedidos são
          reservados e só entram em produção após a confirmação do
          pagamento. Pedidos Pix não pagos em 24 h são cancelados e o
          estoque é liberado.
        </Section>

        <Section title="3. Entrega">
          O envio é realizado pelos Correios através da SuperFrete. O prazo
          começa a contar a partir da emissão da etiqueta (tipicamente 1 dia
          útil após o pagamento). Atrasos por parte dos Correios fogem ao
          nosso controle, mas estamos à disposição para auxiliar no
          acompanhamento em{" "}
          <a
            className="underline"
            href={`https://wa.me/${whatsAppNumber}`}
            target="_blank"
            rel="noreferrer noopener"
          >
            +{whatsAppNumber}
          </a>
          .
        </Section>

        <Section title="4. Trocas e devoluções">
          Direito de arrependimento: 7 dias corridos a contar do recebimento
          (Código de Defesa do Consumidor, art. 49). Regras completas,
          prazos e passo a passo da solicitação estão em{" "}
          <a className="underline" href="/trocas">
            /trocas
          </a>
          . Para iniciar, escreva para{" "}
          <a className="underline" href={`mailto:${supportEmail}`}>
            {supportEmail}
          </a>
          {" "}ou pelo WhatsApp. A peça deve estar sem uso, com etiquetas e na
          embalagem original.
        </Section>

        <Section title="5. Propriedade intelectual">
          Todos os designs, fotografias e conteúdo deste site são de
          propriedade da NAST. Reprodução não autorizada é proibida.
        </Section>

        <Section title="6. Foro">
          Estes termos são regidos pela legislação brasileira. Fica eleito o
          foro da comarca de São Paulo/SP para dirimir qualquer controvérsia.
        </Section>

        <div className="mt-12">
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
