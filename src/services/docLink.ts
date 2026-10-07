import type jsPDF from 'jspdf';
import { supabase } from './supabase';

// Gera o PDF, hospeda no bucket privado da Inovar e devolve um link
// assinado (7 dias) que pode ser enviado por WhatsApp/e-mail.
export const DocLinkService = {
  async pdfParaLink(doc: jsPDF, nomeArquivo: string): Promise<string | null> {
    try {
      const base64 = doc.output('datauristring');

      const { data: sessionData } = await supabase.auth.getSession();
      const token = sessionData?.session?.access_token;
      if (!token) return null;

      const up = await fetch('/api/documentos', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
        body: JSON.stringify({ acao: 'upload', nome: nomeArquivo, base64 })
      });
      if (!up.ok) return null;
      const { path } = await up.json();

      const lk = await fetch('/api/documentos', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
        body: JSON.stringify({ acao: 'link', path, dias: 7 })
      });
      if (!lk.ok) return null;
      const { shortUrl, url } = await lk.json();
      return shortUrl || url || null;
    } catch (err) {
      console.warn('Não foi possível hospedar o PDF:', err);
      return null;
    }
  }
};
