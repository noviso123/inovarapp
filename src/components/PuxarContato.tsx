import React, { useEffect, useState } from 'react';
import { supabase } from '../services/supabase';
import { buscarContatosGoogle } from '../services/googleContacts';
import { authorizeGoogle } from '../services/googleConnection';
import { Loader2, Search, Smartphone, ChevronRight, UsersRound } from 'lucide-react';

export interface ContatoPuxado {
  nome?: string;
  telefone?: string;
  email?: string;
}

// AUTO-PREENCHIMENTO POR CONTATOS — duas integrações, usadas em todos os cadastros:
//   1. Agenda NATIVA do celular (Contact Picker API — Android/Chrome), com busca
//      nativa e seleção múltipla;
//   2. CONTATOS DO GOOGLE (People API) — busca pelo nome em qualquer navegador
//      (PC e iPhone incluídos) quando a sessão foi criada por "Entrar com Google".
// Ao escolher, onSelecionar recebe { nome, telefone, email } para preencher o formulário.
export const PuxarContato: React.FC<{ onSelecionar: (c: ContatoPuxado) => void }> = ({ onSelecionar }) => {
  const suportaAgenda = typeof (navigator as any).contacts?.select === 'function';
  const [googleOn, setGoogleOn] = useState(false);
  const [busca, setBusca] = useState('');
  const [buscando, setBuscando] = useState(false);
  const [puxando, setPuxando] = useState(false);
  const [conectandoGoogle, setConectandoGoogle] = useState(false);
  const [resultados, setResultados] = useState<ContatoPuxado[] | null>(null);
  const [aviso, setAviso] = useState<string | null>(null);

  useEffect(() => {
    supabase.auth.getSession().then(({ data }) => {
      setGoogleOn(!!data.session?.provider_token);
    });
  }, []);

  const puxarAgenda = async () => {
    if (!suportaAgenda) return;
    try {
      setPuxando(true);
      const sel: any[] = await (navigator as any).contacts.select(['name', 'tel', 'email'], { multiple: true });
      const contatos: ContatoPuxado[] = (sel || [])
        .map((c: any) => ({
          nome: c.name?.[0] || '',
          telefone: String(c.tel?.[0] || '').replace(/[^\d+]/g, ''),
          email: c.email?.[0] || undefined
        }))
        .filter((c) => c.nome || c.telefone || c.email);
      if (!contatos.length) return;
      if (contatos.length === 1) onSelecionar(contatos[0]);
      else setResultados(contatos.sort((a, b) => (a.nome || '').localeCompare(b.nome || '')));
    } catch { /* usuário cancelou ou negou */ }
    finally { setPuxando(false); }
  };

  const buscarGoogle = async () => {
    const q = busca.trim();
    if (!q) return;
    try {
      setBuscando(true);
      setAviso(null);
      const r = await buscarContatosGoogle(q);
      if (!r.length) {
        setAviso('Nenhum contato do Google encontrado para "' + q + '".');
        setResultados(null);
        return;
      }
      setResultados(r);
    } catch (e: any) {
      const code = String(e?.message || '');
      if (code.includes('sem-login-google')) {
        setGoogleOn(false);
        setAviso('Sessão do Google expirada — toque em "Entrar com Google" para reconectar.');
      } else if (code.includes('sem-permissao-contatos')) {
        setAviso('Seu login Google ainda não tem a permissão de contatos. Toque em "Entrar com Google" novamente e autorize — ou use a agenda do celular.');
      } else {
        setAviso('Não foi possível buscar nos contatos do Google agora.');
      }
    } finally {
      setBuscando(false);
    }
  };

  const conectarContatosGoogle = async () => {
    try {
      setConectandoGoogle(true);
      // A autorização abre o Google e retorna para o app. Ao voltar, basta
      // reabrir este cadastro para pesquisar e aproveitar os dados do contato.
      await authorizeGoogle('contacts');
    } catch (error: any) {
      setAviso(error?.message || 'Não foi possível abrir a autorização do Google.');
      setConectandoGoogle(false);
    }
  };

  return (
    <div className="mb-3 space-y-2">
      {/* 1) Agenda nativa do celular */}
      {suportaAgenda && (
        <button
          type="button"
          onClick={puxarAgenda}
          disabled={puxando}
          className="w-full p-3 bg-sky-50 hover:bg-sky-100 active:scale-[0.99] border border-sky-200 rounded-xl flex items-center gap-3 text-left transition-all disabled:opacity-60"
        >
          <span className="p-2.5 bg-sky-600 text-white rounded-xl shrink-0">
            {puxando ? <Loader2 className="w-5 h-5 animate-spin" /> : <Smartphone className="w-5 h-5" />}
          </span>
          <span className="flex-1 min-w-0">
            <span className="block text-xs font-extrabold text-sky-800">Puxar contato do celular</span>
            <span className="block text-[10px] text-slate-500 leading-snug">Abre sua agenda (com busca) — nome, WhatsApp e e-mail preenchem sozinhos</span>
          </span>
          <ChevronRight className="w-4 h-4 text-sky-400 shrink-0" />
        </button>
      )}

      {/* 2) Contatos do Google (People API) */}
      {googleOn && (
        <div className="flex gap-2">
          <input
            type="text"
            value={busca}
            onChange={(e) => setBusca(e.target.value)}
            onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); buscarGoogle(); } }}
            placeholder="Buscar nos contatos do Google..."
            className="flex-1 p-2 bg-white border border-slate-300 rounded-lg text-xs"
          />
          <button
            type="button"
            onClick={buscarGoogle}
            disabled={buscando || !busca.trim()}
            className="px-3 bg-white hover:bg-slate-50 border border-slate-300 text-slate-700 rounded-lg text-xs font-bold flex items-center gap-1.5 disabled:opacity-50"
          >
            {buscando ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Search className="w-3.5 h-3.5" />}
            Google
          </button>
        </div>
      )}

      {!googleOn && (
        <button
          type="button"
          onClick={conectarContatosGoogle}
          disabled={conectandoGoogle}
          className="w-full p-3 bg-white hover:bg-sky-50 active:scale-[0.99] border border-sky-200 rounded-xl flex items-center gap-3 text-left transition-all disabled:opacity-60"
        >
          <span className="p-2.5 bg-sky-100 text-sky-700 rounded-xl shrink-0">
            {conectandoGoogle ? <Loader2 className="w-5 h-5 animate-spin" /> : <UsersRound className="w-5 h-5" />}
          </span>
          <span className="flex-1 min-w-0">
            <span className="block text-xs font-extrabold text-sky-900">Conectar contatos do Google</span>
            <span className="block text-[10px] text-slate-500 leading-snug">Pesquise e aproveite nome, telefone e e-mail em qualquer aparelho.</span>
          </span>
          <ChevronRight className="w-4 h-4 text-sky-400 shrink-0" />
        </button>
      )}

      {aviso && (
        <p className="text-[10px] text-amber-800 bg-amber-50 border border-amber-200 rounded-lg p-2 leading-snug">{aviso}</p>
      )}

      {/* Resultados da busca/seleção */}
      {resultados && (
        <div className="border border-slate-200 rounded-xl divide-y divide-slate-100 max-h-52 overflow-y-auto bg-white">
          <div className="px-2.5 py-1.5 bg-slate-50 text-[10px] font-bold text-slate-500 uppercase tracking-wide flex items-center justify-between">
            <span>{resultados.length} contato(s) — escolha um</span>
            <button type="button" onClick={() => setResultados(null)} className="text-slate-400 hover:text-slate-600 font-bold">✕</button>
          </div>
          {resultados.map((c, i) => (
            <button
              key={i}
              type="button"
              onClick={() => { onSelecionar(c); setResultados(null); setBusca(''); }}
              className="w-full p-2.5 text-left hover:bg-sky-50 transition-colors"
            >
              <span className="block text-xs font-bold text-slate-800">{c.nome || 'Sem nome'}</span>
              <span className="block text-[11px] text-slate-500">{c.telefone || 'sem telefone'}{c.email ? ` • ${c.email}` : ''}</span>
            </button>
          ))}
        </div>
      )}

    </div>
  );
};
