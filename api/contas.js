// Contas da InovarApp — função única com ações.
// acao: 'minha_conta' (público: auto-cadastro do cliente, login imediato)
//       'do_tecnico' (ADMIN/TECNICO: cria conta para um cliente cadastrado)
// Ambos disparam BOAS-VINDAS no WhatsApp do cliente (via sistema próprio de WhatsApp).
const SUPABASE_URL = process.env.VITE_SUPABASE_URL || 'https://ycpswioserctavijhnre.supabase.co';
const SRK = process.env.SUPABASE_SERVICE_ROLE_KEY;
const ANON = process.env.VITE_SUPABASE_ANON_KEY;
import { enviarWhatsapp } from './_whatsapp.js';
import { lerConfigServidor } from './_config.js';
import { randomInt } from 'node:crypto';

const json = (res, status, body) => {
  res.statusCode = status;
  res.setHeader('Content-Type', 'application/json; charset=utf-8');
  res.end(JSON.stringify(body));
};

// ---- Central de Mensagens: usa o texto salvo pelo admin (mensagensWhats) ou o padrão ----
function aplicarVarsContas(texto, vars) {
  return String(texto || '').replace(/\{\{(\w+)\}\}/g, (_, k) => (vars[k] === undefined || vars[k] === null ? '' : String(vars[k])));
}
async function textoBoasVindas(chave, padrao, vars) {
  try {
    const cfgRaw = await lerConfigServidor();
    const salvo = cfgRaw && cfgRaw.mensagensWhats && cfgRaw.mensagensWhats[chave];
    return aplicarVarsContas(salvo && salvo.trim() ? salvo : padrao, vars);
  } catch {
    return aplicarVarsContas(padrao, vars);
  }
}

function bearerToken(req) {
  const h = req.headers.authorization || '';
  return h.startsWith('Bearer ') ? h.slice(7) : null;
}

function decodeJwtPayload(token) {
  try {
    return JSON.parse(Buffer.from(token.split('.')[1], 'base64url').toString('utf8'));
  } catch {
    return null;
  }
}

// Cria usuário já confirmado no Auth (login imediato, sem e-mail de confirmação)
async function criarUsuario(opts) {
  const emailLimpo = String(opts.email || '').trim().toLowerCase();
  const r = await fetch(`${SUPABASE_URL}/auth/v1/admin/users`, {
    method: 'POST',
    headers: { apikey: SRK, Authorization: `Bearer ${SRK}`, 'Content-Type': 'application/json' },
    body: JSON.stringify({
      email: emailLimpo,
      password: String(opts.senha),
      email_confirm: true,
      user_metadata: {
        nome: String(opts.nome || '').trim(),
        telefone: String(opts.telefone || '').trim(),
        must_change_password: opts.trocaObrigatoria === true
      }
    })
  });
  const created = await r.json().catch(() => ({}));
  return { ok: r.ok, created, emailLimpo, msg: created?.msg || created?.message || '' };
}

async function validarEquipe(req) {
  const token = bearerToken(req);
  if (!token) return { error: 'Não autenticado', status: 401 };
  const payload = decodeJwtPayload(token);
  if (!payload?.sub || (payload.exp && payload.exp * 1000 < Date.now())) return { error: 'Sessão inválida', status: 401 };
  const profiles = await fetch(`${SUPABASE_URL}/rest/v1/profiles?id=eq.${payload.sub}&select=id,tipo`, {
    headers: { apikey: ANON, Authorization: `Bearer ${token}` }
  }).then((x) => x.json()).catch(() => null);
  if (!Array.isArray(profiles) || profiles.length !== 1 || !['ADMIN', 'TECNICO'].includes(profiles[0].tipo)) {
    return { error: 'Somente técnicos e administradores podem realizar esta ação', status: 403 };
  }
  return { token, profile: profiles[0] };
}

function gerarSenhaTemporaria() {
  const letras = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz';
  const numeros = '23456789';
  const especiais = '!@#';
  const escolher = (chars) => chars[randomInt(0, chars.length)];
  const base = [escolher(letras.toUpperCase()), escolher(letras.toLowerCase()), escolher(numeros), escolher(especiais)];
  while (base.length < 10) base.push(escolher(letras + numeros));
  for (let i = base.length - 1; i > 0; i--) {
    const j = randomInt(0, i + 1);
    [base[i], base[j]] = [base[j], base[i]];
  }
  return base.join('');
}

// Vincula o cadastro real do cliente à conta criada (remove o customer vazio do trigger)
async function vincularCustomer(customerId, userId, nome) {
  if (!customerId || !userId) return;
  await fetch(`${SUPABASE_URL}/rest/v1/customers?profile_id=eq.${userId}&nome=eq.${encodeURIComponent(nome || '')}`, {
    method: 'DELETE',
    headers: { apikey: SRK, Authorization: `Bearer ${SRK}` }
  });
  await fetch(`${SUPABASE_URL}/rest/v1/customers?id=eq.${customerId}`, {
    method: 'PATCH',
    headers: { apikey: SRK, Authorization: `Bearer ${SRK}`, 'Content-Type': 'application/json' },
    body: JSON.stringify({ profile_id: userId })
  });
}

export default async function handler(req, res) {
  if (req.method !== 'POST') return json(res, 405, { error: 'Método não permitido' });
  if (!SRK) return json(res, 500, { error: 'Backend não configurado' });

  const { acao } = req.body || {};

  // ---------- Auto-cadastro público do cliente ----------
  if (acao === 'minha_conta') {
    const { email, senha, nome, whatsapp, endereco, bairro, cidade } = req.body || {};
    const emailLimpo = String(email || '').trim().toLowerCase();

    if (!/^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(emailLimpo)) return json(res, 400, { error: 'Informe um e-mail válido' });
    if (!senha || String(senha).length < 6) return json(res, 400, { error: 'A senha deve ter no mínimo 6 caracteres' });
    if (!nome || !String(nome).trim()) return json(res, 400, { error: 'Informe seu nome completo' });

    const { ok, created, emailLimpo: e, msg } = await criarUsuario({ email: emailLimpo, senha, nome, telefone: whatsapp });
    if (!ok) {
      if (/already|registered|exists/i.test(msg)) {
        return json(res, 409, { error: 'Este e-mail já possui conta. Faça login normalmente.', ja_existe: true });
      }
      return json(res, 400, { error: msg || 'Não foi possível criar sua conta' });
    }

    if (whatsapp || endereco || bairro || cidade) {
      const cust = await fetch(`${SUPABASE_URL}/rest/v1/customers?profile_id=eq.${created.id}&select=id`, {
        headers: { apikey: SRK, Authorization: `Bearer ${SRK}` }
      }).then((x) => x.json()).catch(() => null);
      if (Array.isArray(cust) && cust[0]?.id) {
        await fetch(`${SUPABASE_URL}/rest/v1/customers?id=eq.${cust[0].id}`, {
          method: 'PATCH',
          headers: { apikey: SRK, Authorization: `Bearer ${SRK}`, 'Content-Type': 'application/json' },
          body: JSON.stringify({
            whatsapp: String(whatsapp || '').trim() || undefined,
            endereco: String(endereco || '').trim() || undefined,
            bairro: String(bairro || '').trim() || undefined,
            cidade: String(cidade || '').trim() || undefined
          })
        });
      }
    }

    if (whatsapp) {
      const primeiroNome = String(nome).trim().split(' ')[0];
      // texto salvo pelo admin na Central de Mensagens (ou o padrão abaixo)
      const boasVindas = await textoBoasVindas('boas_vindas_autocadastro',
        '🎉 *Bem-vindo(a) à {{empresa}}*, {{cliente}}!\n\n' +
        'Sua conta no *InovarApp* foi criada com o e-mail:\n' +
        '📧 {{email}}\n\n' +
        '📲 *Acesse:* {{app}}\n\n' +
        'No app você acompanha seus aparelhos, solicita atendimentos, recebe orçamentos com assinatura e consulta suas garantias! ❄️',
        { cliente: primeiroNome, email: e, empresa: 'Inovar Refrigeração', app: 'https://inovarapp.vercel.app' }
      );
      enviarWhatsapp({ telefone: whatsapp, texto: boasVindas }).catch(() => {});
    }

    return json(res, 200, { ok: true, user_id: created.id, email: e, mensagem: 'Conta criada! Entrando no seu portal...' });
  }

  // ---------- Técnico cria conta para um cliente ----------
  if (acao === 'do_tecnico') {
    const equipe = await validarEquipe(req);
    if (equipe.error) return json(res, equipe.status, { error: equipe.error });

    const { email, senha, nome, telefone, customer_id } = req.body || {};
    if (!email || !/^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(email)) {
      return json(res, 400, { error: 'Informe um e-mail válido para o cliente' });
    }
    const senhaFinal = senha && String(senha).length >= 6 ? String(senha) : '123456';

    const { ok, created, emailLimpo, msg } = await criarUsuario({ email, senha: senhaFinal, nome, telefone, trocaObrigatoria: true });
    if (!ok) {
      if (/already|registered|exists/i.test(msg)) {
        return json(res, 409, { error: 'O e-mail ' + emailLimpo + ' já possui conta. O cliente pode entrar normalmente.', ja_existe: true });
      }
      return json(res, 400, { error: msg || 'Não foi possível criar a conta do cliente' });
    }

    if (customer_id && created.id) {
      await vincularCustomer(customer_id, created.id, nome);
    }

    if (telefone) {
      const primeiroNome = String(nome || 'Cliente').trim().split(' ')[0];
      // texto salvo pelo admin na Central de Mensagens (ou o padrão abaixo)
      const boasVindas = await textoBoasVindas('boas_vindas_tecnico',
        '🎉 *Bem-vindo(a) à {{empresa}}*, {{cliente}}!\n\n' +
        'Sua conta de acesso ao *InovarApp* foi criada:\n' +
        '📧 *Login:* {{email}}\n' +
        '🔑 *Senha:* {{senha}}\n\n' +
        '📲 *Acesse aqui:* {{app}}\n\n' +
        '*Como entrar:*\n' +
        '1. Abra o link acima\n' +
        '2. Toque em *Entrar no App*\n' +
        '3. Use o login e a senha acima\n\n' +
        '🔐 No primeiro acesso, o app solicitará a criação de uma senha pessoal.\n\n' +
        '*{{empresa}}* ❄️',
        { cliente: primeiroNome, email: emailLimpo, senha: senhaFinal, empresa: 'Inovar Refrigeração', app: 'https://inovarapp.vercel.app' }
      );
      enviarWhatsapp({ telefone, texto: boasVindas }).catch(() => {});
    }

    return json(res, 200, {
      ok: true,
      user_id: created.id,
      email: emailLimpo,
      senha_inicial: senhaFinal,
      mensagem: 'Conta criada! O cliente entra com ' + emailLimpo + ' e a senha ' + senhaFinal + '.'
    });
  }

  // ---------- Equipe redefine o acesso de um cliente ----------
  if (acao === 'redefinir_cliente') {
    const equipe = await validarEquipe(req);
    if (equipe.error) return json(res, equipe.status, { error: equipe.error });

    const customerId = String(req.body?.customer_id || '').trim();
    if (!customerId) return json(res, 400, { error: 'Cliente não informado' });

    const clientes = await fetch(`${SUPABASE_URL}/rest/v1/customers?id=eq.${encodeURIComponent(customerId)}&select=id,nome,whatsapp,profile_id`, {
      headers: { apikey: SRK, Authorization: `Bearer ${SRK}` }
    }).then((x) => x.json()).catch(() => null);
    const cliente = Array.isArray(clientes) ? clientes[0] : null;
    if (!cliente?.profile_id) return json(res, 409, { error: 'Este cliente ainda não possui uma conta de acesso vinculada.' });

    const perfis = await fetch(`${SUPABASE_URL}/rest/v1/profiles?id=eq.${cliente.profile_id}&select=id,email,tipo`, {
      headers: { apikey: SRK, Authorization: `Bearer ${SRK}` }
    }).then((x) => x.json()).catch(() => null);
    const perfil = Array.isArray(perfis) ? perfis[0] : null;
    if (!perfil?.email || perfil.tipo !== 'CLIENTE') return json(res, 409, { error: 'A conta vinculada não é uma conta válida de cliente.' });

    const senhaTemporaria = gerarSenhaTemporaria();
    const usuarioAtual = await fetch(`${SUPABASE_URL}/auth/v1/admin/users/${cliente.profile_id}`, {
      headers: { apikey: SRK, Authorization: `Bearer ${SRK}` }
    }).then((x) => x.json()).catch(() => ({}));
    const atualizacao = await fetch(`${SUPABASE_URL}/auth/v1/admin/users/${cliente.profile_id}`, {
      method: 'PUT',
      headers: { apikey: SRK, Authorization: `Bearer ${SRK}`, 'Content-Type': 'application/json' },
      body: JSON.stringify({
        password: senhaTemporaria,
        user_metadata: { ...(usuarioAtual?.user_metadata || {}), must_change_password: true }
      })
    });
    if (!atualizacao.ok) {
      const detalhe = await atualizacao.json().catch(() => ({}));
      return json(res, 400, { error: detalhe?.message || detalhe?.msg || 'Não foi possível redefinir a senha' });
    }

    if (cliente.whatsapp) {
      const primeiroNome = String(cliente.nome || 'Cliente').trim().split(' ')[0];
      const mensagem = `🔐 *Acesso temporário ao InovarApp*\n\nOlá, ${primeiroNome}. Sua senha foi redefinida pela equipe.\n\n📧 *Login:* ${perfil.email}\n🔑 *Senha temporária:* ${senhaTemporaria}\n📲 https://inovarapp.vercel.app\n\nNo próximo acesso você deverá criar uma nova senha pessoal.`;
      enviarWhatsapp({ telefone: cliente.whatsapp, texto: mensagem }).catch(() => {});
    }

    return json(res, 200, {
      ok: true,
      email: perfil.email,
      senha_temporaria: senhaTemporaria,
      mensagem: 'Senha temporária criada. O cliente deverá alterá-la no próximo acesso.'
    });
  }

  return json(res, 400, { error: 'Ação inválida' });
}
