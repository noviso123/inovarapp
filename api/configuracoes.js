// Configurações do técnico/perfil da Inovar (PIX, assinatura, mensagens,
// e-mail e valores padrão). Armazenadas como JSON no bucket privado
// "documentos-inovar" (config/tecnico.json) — sincronizadas em todas as plataformas.
// GET: qualquer usuário autenticado lê. POST: somente ADMIN/TECNICO grava.
import { json, authCaller } from './_supabase.js';

const SUPABASE_URL = process.env.VITE_SUPABASE_URL || 'https://ycpswioserctavijhnre.supabase.co';
const SRK = process.env.SUPABASE_SERVICE_ROLE_KEY;
const CONFIG_PATH = 'config/tecnico.json';

const DEFAULTS = {
  businessName: 'Inovar Refrigeração',
  cnpj: '36.020.014/0001-14',
  address: 'Serra',
  name: 'Gabriel',
  phone: '27998279185',
  pixKey: 'gabrielnascimento458@gmail.com',
  pixType: 'email',
  defaultReturnMonths: 6,
  defaultWarrantyDays: 90,
  defaultPrice: 250,
  assinatura: '/inovar-brand/INOVAR_SIGNATURE_GABRIEL.png', // assinatura padrão do técnico/admin Gabriel (sai na OS)
  email_api_key: '',
  email_from: '',
  email_gmail_user: '',
  email_gmail_pass: '',
  tiposServicosCustom: [],
  mensagensWhats: {},
  lembrete_intervalo_dias: 7,
  tiposFixosRemovidos: [],
  tiposFixosEditados: [],
  calendario_token: ''
};

async function perfilDoUsuario(userId) {
  const r = await fetch(`${SUPABASE_URL}/rest/v1/profiles?id=eq.${userId}&select=nome,telefone,tipo`, {
    headers: { apikey: SRK, Authorization: `Bearer ${SRK}` }
  });
  const d = await r.json();
  return Array.isArray(d) && d[0] ? d[0] : null;
}

async function lerConfig() {
  const r = await fetch(`${SUPABASE_URL}/storage/v1/object/documentos-inovar/${CONFIG_PATH}`, {
    headers: { apikey: SRK, Authorization: `Bearer ${SRK}` }
  });
  if (!r.ok) return null;
  try {
    return await r.json();
  } catch {
    return null;
  }
}

async function salvarConfig(config) {
  const response = await fetch(`${SUPABASE_URL}/storage/v1/object/documentos-inovar/${CONFIG_PATH}`, {
    method: 'POST',
    headers: {
      apikey: SRK,
      Authorization: `Bearer ${SRK}`,
      'Content-Type': 'application/json',
      'x-upsert': 'true'
    },
    body: JSON.stringify(config)
  });
  if (!response.ok) throw new Error('Não foi possível salvar as configurações.');
}

export default async function handler(req, res) {
  const caller = await authCaller(req);
  if (!caller) return json(res, 401, { error: 'Não autenticado' });

  if (req.method === 'GET') {
    let config = await lerConfig();

    // Sem configuração salva: monta padrão a partir do perfil do técnico
    if (!config) {
      const perfil = await perfilDoUsuario(caller.userId);
      config = { ...DEFAULTS };
      if (caller.role === 'ADMIN' || caller.role === 'TECNICO') {
        config.name = perfil?.nome || 'Gabriel Nascimento';
        config.phone = perfil?.telefone || '';
      }
      await salvarConfig(config);
    }

    const chavesWhatsAppAntigas = ['whatsapp_proprio_url', 'whatsapp_proprio_token', 'whatsapp_proprio_session', 'whatsapp_evol_url', 'whatsapp_evol_api_url', 'whatsapp_evol_api_key', 'whatsapp_evolution_url', 'whatsapp_evolution_key', 'whatsapp_evolution_token', 'whatsapp_meta_token', 'whatsapp_phone_id'];
    let removeuConfiguracaoWhatsAppAntiga = false;
    for (const chave of chavesWhatsAppAntigas) {
      if (Object.hasOwn(config, chave)) {
        delete config[chave];
        removeuConfiguracaoWhatsAppAntiga = true;
      }
    }
    if (removeuConfiguracaoWhatsAppAntiga) await salvarConfig(config);

    // Preenche campos vazios com os padrões Inovar (ex.: PIX do site original)
    if (!config.pixKey) config.pixKey = DEFAULTS.pixKey;
    if (!config.pixType) config.pixType = DEFAULTS.pixType;
    if (!config.name) config.name = DEFAULTS.name;
    if (!config.phone) config.phone = DEFAULTS.phone;
    // Instala a assinatura profissional padrão também para configurações que
    // foram criadas antes deste recurso. Assim, Gabriel não precisa assinar a
    // cada Ordem de Serviço e todos os dispositivos recebem o mesmo arquivo.
    if (['ADMIN', 'TECNICO'].includes(caller.role) && config.assinatura !== DEFAULTS.assinatura) {
      config.assinatura = DEFAULTS.assinatura;
      await salvarConfig(config);
    }

    // NÃO devolve segredos completos ao navegador — só indica presença.
    // Vale para QUALQUER usuário autenticado (clientes leem o config do número
    // da empresa), por isso TODA credencial sensível sai mascarada.
    const mascarar = (cfg) => ({
      ...cfg,
      calendario_token: caller.role === 'CLIENTE' ? undefined : cfg.calendario_token,
      email_api_key: cfg.email_api_key ? '***configurada***' : '',
      email_gmail_pass: cfg.email_gmail_pass ? '***configurada***' : ''
    });
    return json(res, 200, { ok: true, config: mascarar(config) });
  }

  if (req.method === 'POST') {
    if (caller.role === 'CLIENTE') {
      return json(res, 403, { error: 'Somente a equipe Inovar altera as configurações' });
    }
    const atual = (await lerConfig()) || { ...DEFAULTS };
    const entrada = req.body || {};

    const novo = { ...atual };
    for (const chave of ['whatsapp_proprio_url', 'whatsapp_proprio_token', 'whatsapp_proprio_session', 'whatsapp_evol_url', 'whatsapp_evol_api_url', 'whatsapp_evol_api_key', 'whatsapp_evolution_url', 'whatsapp_evolution_key', 'whatsapp_evolution_token', 'whatsapp_meta_token', 'whatsapp_phone_id']) delete novo[chave];
    // aceita a chave em camelCase (do app) ou snake_case (legado)
    if (Array.isArray(entrada.tipos_servicos_custom) && !Array.isArray(entrada.tiposServicosCustom)) {
      entrada.tiposServicosCustom = entrada.tipos_servicos_custom;
      delete entrada.tipos_servicos_custom;
    }
    for (const k of Object.keys(DEFAULTS)) {
      if (entrada[k] === undefined || entrada[k] === null) continue;
      // não sobrescreve um segredo salvo com o placeholder ***
      if (typeof entrada[k] === 'string' && entrada[k].startsWith('***')) continue;
      // credenciais de e-mail nunca são apagadas por envio vazio
      if (['email_gmail_user', 'email_gmail_pass', 'email_api_key'].includes(k) && !String(entrada[k]).trim()) continue;
      novo[k] = entrada[k];
    }

    // e-mail: mantém compatibilidade com variáveis de ambiente da Vercel
    if (!novo.email_api_key && process.env.EMAIL_API_KEY) novo.email_api_key = process.env.EMAIL_API_KEY;
    if (!novo.email_from && process.env.EMAIL_FROM) novo.email_from = process.env.EMAIL_FROM;

    await salvarConfig(novo);
    // resposta também mascarada — o valor real nunca volta ao navegador
    const resposta = {
      ...novo,
      email_api_key: novo.email_api_key ? '***configurada***' : '',
      email_gmail_pass: novo.email_gmail_pass ? '***configurada***' : ''
    };
    return json(res, 200, { ok: true, config: resposta });
  }

  return json(res, 405, { error: 'Método não permitido' });
}
