// Leitura das preferências compartilhadas (mensagens, e-mail e dados da empresa).
// Credenciais do WhatsApp ficam apenas nas variáveis de ambiente do serviço Go.
const SUPABASE_URL = process.env.VITE_SUPABASE_URL || 'https://ycpswioserctavijhnre.supabase.co';
const SRK = process.env.SUPABASE_SERVICE_ROLE_KEY;

export async function lerConfigServidor() {
  try {
    const r = await fetch(`${SUPABASE_URL}/storage/v1/object/documentos-inovar/config/tecnico.json`, {
      headers: { apikey: SRK, Authorization: `Bearer ${SRK}` }
    });
    if (!r.ok) return null;
    return await r.json();
  } catch {
    return null;
  }
}

export function emailConfigFrom(cfg) {
  return {
    apiKey: cfg?.email_api_key || process.env.EMAIL_API_KEY || '',
    from: cfg?.email_from || process.env.EMAIL_FROM || '',
    gmailUser: cfg?.email_gmail_user || '',
    gmailPass: cfg?.email_gmail_pass || ''
  };
}
