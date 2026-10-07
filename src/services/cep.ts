// Endereço automático pelo CEP (ViaCEP) — campos continuam editáveis depois.
export interface EnderecoCep {
  rua: string;
  bairro: string;
  cidade: string;
  uf: string;
}

export const mascaraCep = (v: string): string => {
  const d = (v || '').replace(/\D/g, '').slice(0, 8);
  return d.length > 5 ? d.slice(0, 5) + '-' + d.slice(5) : d;
};

export async function buscarCep(cep: string): Promise<EnderecoCep | null> {
  const digits = (cep || '').replace(/\D/g, '');
  if (digits.length !== 8) return null;
  try {
    const r = await fetch('https://viacep.com.br/ws/' + digits + '/json/');
    const j = await r.json();
    if (!r.ok || j.erro) return null;
    return {
      rua: j.logradouro || '',
      bairro: j.bairro || '',
      cidade: j.localidade || '',
      uf: j.uf || ''
    };
  } catch {
    return null;
  }
}
