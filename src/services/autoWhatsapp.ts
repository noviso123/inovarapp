import { supabase } from './supabase';

// Envios AUTOMÁTICOS de WhatsApp para os clientes.
// Usa o serviço próprio em Go através do endpoint autenticado /api/whatsapp.
// O backend registra a mensagem em uma fila persistente no Supabase e tenta
// enviá-la novamente quando o provedor voltar a ficar disponível.

export const AutoWhatsapp = {
  async enviar({
    telefone,
    texto,
    documento_url,
    documento_nome
  }: {
    telefone?: string;
    texto: string;
    documento_url?: string;
    documento_nome?: string;
  }): Promise<boolean> {
    try {
      if (!telefone || !telefone.replace(/\D/g, '')) return false;
      const { data: sessionData } = await supabase.auth.getSession();
      const r = await fetch('/api/whatsapp', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${sessionData?.session?.access_token || ''}`
        },
        body: JSON.stringify({ acao: 'enviar', telefone, texto, documento_url, documento_nome })
      });
      if (!r.ok) return false;
      const result = await r.json().catch(() => ({}));
      // A API confirma explicitamente o envio para não indicar sucesso falso.
      return result?.ok === true;
    } catch {
      return false;
    }
  },

  // Orçamento criado -> proposta em PDF automaticamente para o cliente
  async orcamento({ telefone, numero, clienteNome, total, pdfBase64, nomePdf, texto }: {
    telefone?: string;
    numero: string;
    clienteNome: string;
    total: number;
    pdfBase64: string;
    nomePdf: string;
    texto?: string; // template personalizado da Central de Mensagens (placeholders já aplicados)
  }) {
    try {
      const { data: sessionData } = await supabase.auth.getSession();
      const up = await fetch('/api/documentos', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${sessionData?.session?.access_token || ''}`
        },
        body: JSON.stringify({ acao: 'upload', nome: nomePdf, base64: pdfBase64 })
      });
      if (!up.ok) return false;
      const { path } = await up.json();

      const lk = await fetch('/api/documentos', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${sessionData?.session?.access_token || ''}`
        },
        body: JSON.stringify({ acao: 'link', path, dias: 7 })
      });
      if (!lk.ok) return false;
      const { url } = await lk.json();

      const textoPadrao = `Olá, ${clienteNome.split(' ')[0]}! Tudo bem? 😊\n\nAqui é a *Inovar Refrigeração*.\n\nSegue a sua *Proposta/Orçamento nº ${numero}*\n💰 *Valor total: R$ ${total.toFixed(2)}*\n\n📄 A proposta completa em PDF está anexada nesta mensagem.\n\nQualquer dúvida estou à disposição! ❄️🤝

📲 Acompanhe tudo pelo app: https://inovarapp.vercel.app`;

      return await this.enviar({ telefone, texto: (texto && texto.trim()) || textoPadrao, documento_url: url, documento_nome: nomePdf });
    } catch {
      return false;
    }
  },

  // OS concluída -> comprovante/garantia em PDF automaticamente
  async ordemServico({ telefone, osId, clienteNome, servico, dataServico, garantiaDias, pdfBase64, nomePdf, texto }: {
    telefone?: string;
    osId: string;
    clienteNome: string;
    servico: string;
    dataServico: string;
    garantiaDias: number;
    pdfBase64: string;
    nomePdf: string;
    texto?: string; // template personalizado da Central de Mensagens
  }) {
    try {
      const { data: sessionData } = await supabase.auth.getSession();
      const up = await fetch('/api/documentos', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${sessionData?.session?.access_token || ''}`
        },
        body: JSON.stringify({ acao: 'upload', nome: nomePdf, base64: pdfBase64 })
      });
      if (!up.ok) return false;
      const { path } = await up.json();

      const lk = await fetch('/api/documentos', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${sessionData?.session?.access_token || ''}`
        },
        body: JSON.stringify({ acao: 'link', path, dias: 30 })
      });
      if (!lk.ok) return false;
      const { url } = await lk.json();

      const textoPadrao = `Olá, ${clienteNome.split(' ')[0]}! ✅\n\nSua *Ordem de Serviço ${osId}* foi concluída com sucesso!\n\n❄️ Serviço: ${servico}\n📅 Data: ${dataServico}\n🛡️ *Garantia: ${garantiaDias} dias*\n\n📄 A OS completa em PDF (comprovante e garantia) está anexada.\n\nObrigado pela confiança! *Inovar Refrigeração* ❄️

📲 Acesse seu portal: https://inovarapp.vercel.app`;

      return await this.enviar({ telefone, texto: (texto && texto.trim()) || textoPadrao, documento_url: url, documento_nome: nomePdf });
    } catch {
      return false;
    }
  }
};
