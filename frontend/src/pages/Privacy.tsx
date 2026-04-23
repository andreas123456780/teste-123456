import { Header } from "../components/Header";
import { Footer } from "../components/Footer";

// Static privacy policy page (LGPD compliance). Text is authored here
// rather than being edited through an admin UI because updates are rare
// and version control gives us clearer history for legal review.

type Props = {
  whatsAppNumber: string;
  supportEmail: string;
};

export function Privacy({ whatsAppNumber, supportEmail }: Props) {
  return (
    <div className="noise relative min-h-full">
      <Header cartCount={0} onOpenCart={() => { /* cart hidden */ }} />
      <main className="mx-auto max-w-3xl px-6 py-16 text-white/80">
        <div className="eyebrow text-white/40">legal</div>
        <h1 className="mt-2 text-3xl font-black tracking-tight text-white md:text-4xl">
          Política de Privacidade
        </h1>
        <p className="mt-3 text-sm text-white/50">
          Última atualização: {lastUpdated()}
        </p>

        <Section title="1. Quem somos">
          NAST é uma marca de streetwear brasileira. Este site é mantido pela
          NAST para permitir a compra das peças da nossa coleção. Em caso de
          dúvida, fale com a gente por{" "}
          <a className="underline" href={`mailto:${supportEmail}`}>
            {supportEmail}
          </a>
          {" "}ou pelo WhatsApp{" "}
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

        <Section title="2. Que dados coletamos">
          Para processar uma compra coletamos apenas o estritamente necessário:
          <List
            items={[
              "Nome completo — para emissão da etiqueta de envio.",
              "Email — para confirmação do pedido e envio do código de rastreio.",
              "Endereço e CEP — para envio e cálculo de frete.",
              "Número de WhatsApp, apenas quando você escolhe Pix (pra fecharmos o pagamento por mensagem).",
              "Dados do cartão são processados DIRETAMENTE pela Stripe; NAST não armazena número de cartão.",
            ]}
          />
          Também armazenamos metadados do pedido (itens, valor, status) até o
          cumprimento do pedido e por obrigações legais (5 anos para fins
          fiscais).
        </Section>

        <Section title="3. Como usamos seus dados">
          <List
            items={[
              "Processar a compra (pagamento, emissão de etiqueta, envio).",
              "Enviar o email de confirmação com o link de rastreio.",
              "Atender a solicitações de suporte.",
              "Cumprir obrigações legais (emissão de nota fiscal, resposta a autoridades).",
            ]}
          />
          Seus dados NÃO são usados para marketing de terceiros nem vendidos.
        </Section>

        <Section title="4. Com quem compartilhamos">
          Apenas com os fornecedores necessários para realizar a compra:
          <List
            items={[
              "Stripe — para processar pagamentos (cartão e Pix).",
              "SuperFrete — para cotar e emitir a etiqueta de envio.",
              "Resend — para enviar email transacional de confirmação, quando configurado.",
              "Correios — transportadora final.",
            ]}
          />
          Todos operam conforme suas próprias políticas de privacidade e
          estão sujeitos à LGPD / legislação equivalente.
        </Section>

        <Section title="5. Cookies">
          Usamos cookies estritamente necessários para o funcionamento do site
          (carrinho, sessão). Se qualquer analytics ou tracking opcional for
          ativado futuramente, você será avisado pelo banner de
          consentimento e poderá recusar sem prejuízo da compra.
        </Section>

        <Section title="6. Seus direitos (LGPD)">
          Você tem direito a (art. 18 da Lei 13.709/2018):
          <List
            items={[
              "Confirmação da existência de tratamento e acesso aos seus dados.",
              "Correção de dados incompletos, inexatos ou desatualizados.",
              "Anonimização, bloqueio ou eliminação de dados desnecessários.",
              "Portabilidade dos dados.",
              "Eliminação dos dados tratados com base no seu consentimento.",
              "Revogação do consentimento.",
            ]}
          />
          Para exercer qualquer um desses direitos, envie um email para{" "}
          <a className="underline" href={`mailto:${supportEmail}`}>
            {supportEmail}
          </a>
          . Respondemos em até 15 dias úteis.
        </Section>

        <Section title="7. Segurança">
          Todos os dados trafegam sob HTTPS. O processamento de pagamento via
          cartão é feito pela Stripe (PCI-DSS nível 1). Dados de pedido ficam
          em banco criptografado em repouso (provedor gerenciado) com acesso
          restrito à equipe operacional da NAST.
        </Section>

        <Section title="8. Encarregado (DPO)">
          Caso deseje falar diretamente com o Encarregado pelo Tratamento de
          Dados Pessoais da NAST, use o email acima; respondemos dentro do
          prazo legal.
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

function List({ items }: { items: string[] }) {
  return (
    <ul className="mt-3 list-disc space-y-1 pl-6">
      {items.map((it) => (
        <li key={it}>{it}</li>
      ))}
    </ul>
  );
}

function lastUpdated(): string {
  // Build-time constant so users don't see a shifting date on every visit.
  return "22/04/2026";
}
