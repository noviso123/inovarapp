// Contatos do GOOGLE (People API) — busca nos contatos da conta Google do
// usuário logado. Funciona em qualquer navegador (PC, iPhone, Android) quando
// a sessão foi criada por "Entrar com Google" com o escopo contacts.readonly.
import { supabase } from './supabase';

export interface ContatoGoogle {
  nome: string;
  telefone: string;
  email?: string;
}

async function providerToken(): Promise<string | null> {
  const { data } = await supabase.auth.getSession();
  return data.session?.provider_token || null;
}

// Há sessão Google com token de provedor? (sensível a escopos concedidos)
export async function googleContatosDisponivel(): Promise<boolean> {
  return !!(await providerToken());
}

// Busca por nome/telefone/e-mail nos contatos do Google
export async function buscarContatosGoogle(query: string): Promise<ContatoGoogle[]> {
  const token = await providerToken();
  if (!token) throw new Error('sem-login-google');

  const params = new URLSearchParams({
    query,
    pageSize: '10',
    readMask: 'names,phoneNumbers,emailAddresses'
  });
  const r = await fetch('https://people.googleapis.com/v1/people:searchContacts?' + params.toString(), {
    headers: { Authorization: `Bearer ${token}` }
  });

  if (r.status === 401 || r.status === 403) throw new Error('sem-permissao-contatos');
  if (!r.ok) throw new Error('falha-google');

  const j = await r.json();
  return (j.results || [])
    .map((res: any) => {
      const p = res.person || {};
      return {
        nome: p.names?.[0]?.displayName || '',
        telefone: String(p.phoneNumbers?.[0]?.value || '').replace(/[^\d+]/g, ''),
        email: p.emailAddresses?.[0]?.value || undefined
      };
    })
    .filter((c: ContatoGoogle) => c.nome || c.telefone || c.email);
}
