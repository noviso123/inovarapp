import React, { useState, useEffect, useRef } from 'react';
import { Bell, X, CheckCheck, Clock3, Inbox, Trash2, Check, Sparkles } from 'lucide-react';

// Histórico de notificações do dispositivo (localStorage) — cada portal
// (técnico/cliente) mantém sua própria lista.
const KEY = 'inovarapp_notifs_v1';
const MAX = 60;

export interface Notificacao {
  id: string;
  quando: string; // ISO
  titulo: string;
  texto: string;
  lida: boolean;
  url?: string;
}

const notificarAtualizacao = () => window.dispatchEvent(new Event('inovar:notificacoes-atualizadas'));

export const NotificacoesStore = {
  listar(): Notificacao[] {
    try {
      return JSON.parse(localStorage.getItem(KEY) || '[]');
    } catch {
      return [];
    }
  },
  registrar(titulo: string, texto: string, url?: string): Notificacao {
    const lista = this.listar();
    const nova: Notificacao = {
      id: Math.random().toString(36).slice(2) + Date.now().toString(36),
      quando: new Date().toISOString(),
      titulo,
      texto,
      lida: false,
      ...(url && url.startsWith('/') && !url.startsWith('//') ? { url } : {})
    };
    const atualizada = [nova, ...lista].slice(0, MAX);
    localStorage.setItem(KEY, JSON.stringify(atualizada));
    notificarAtualizacao();
    return nova;
  },
  marcarTodasLidas(): void {
    const lista = this.listar().map((n) => ({ ...n, lida: true }));
    localStorage.setItem(KEY, JSON.stringify(lista));
    notificarAtualizacao();
  },
  marcarLida(id: string): void {
    const lista = this.listar().map((n) => n.id === id ? { ...n, lida: true } : n);
    localStorage.setItem(KEY, JSON.stringify(lista));
    notificarAtualizacao();
  },
  limpar(): void {
    localStorage.setItem(KEY, '[]');
    notificarAtualizacao();
  }
};

export const naoLidas = (): number => NotificacoesStore.listar().filter((n) => !n.lida).length;

const tempoRelativo = (valor: string): string => {
  const data = new Date(valor);
  const diferenca = Date.now() - data.getTime();
  if (!Number.isFinite(data.getTime()) || diferenca < 0) return '';
  if (diferenca < 60_000) return 'Agora';
  if (diferenca < 3_600_000) return `Há ${Math.floor(diferenca / 60_000)} min`;
  if (diferenca < 86_400_000) return `Hoje, ${data.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' })}`;
  if (diferenca < 172_800_000) return `Ontem, ${data.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' })}`;
  return data.toLocaleDateString('pt-BR', { day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit' });
};

interface CentralNotificacoesProps {
  dark?: boolean; // painel técnico usa fundo escuro; portal cliente também
}

export const CentralNotificacoes: React.FC<CentralNotificacoesProps> = () => {
  const [aberto, setAberto] = useState(false);
  const [lista, setLista] = useState<Notificacao[]>(NotificacoesStore.listar());
  const [naoLidasQtd, setNaoLidasQtd] = useState(naoLidas());
  const [filtro, setFiltro] = useState<'todas' | 'nao-lidas'>('todas');
  const containerRef = useRef<HTMLDivElement>(null);

  // atualiza quando novas notificações chegam (o localStorage muda)
  useEffect(() => {
    const atualizar = () => {
      setLista(NotificacoesStore.listar());
      setNaoLidasQtd(naoLidas());
    };
    const aoMudarArmazenamento = (event: StorageEvent) => { if (event.key === KEY) atualizar(); };
    window.addEventListener('inovar:notificacoes-atualizadas', atualizar);
    window.addEventListener('storage', aoMudarArmazenamento);
    return () => {
      window.removeEventListener('inovar:notificacoes-atualizadas', atualizar);
      window.removeEventListener('storage', aoMudarArmazenamento);
    };
  }, []);

  useEffect(() => {
    if (!aberto) return;
    const fecharAoClicarFora = (event: PointerEvent) => {
      if (!containerRef.current?.contains(event.target as Node)) setAberto(false);
    };
    const fecharComEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setAberto(false);
    };
    document.addEventListener('pointerdown', fecharAoClicarFora);
    document.addEventListener('keydown', fecharComEscape);
    return () => {
      document.removeEventListener('pointerdown', fecharAoClicarFora);
      document.removeEventListener('keydown', fecharComEscape);
    };
  }, [aberto]);

  const atualizar = () => { setLista(NotificacoesStore.listar()); setNaoLidasQtd(naoLidas()); };
  const abrir = () => setAberto((a) => !a);
  const notificacoesVisiveis = filtro === 'nao-lidas' ? lista.filter((n) => !n.lida) : lista;

  return (
    <div ref={containerRef} className="relative z-[70]">
      <button
        onClick={abrir}
        title="Notificações"
        type="button"
        aria-label={naoLidasQtd ? `Notificações, ${naoLidasQtd} não lidas` : 'Notificações'}
        aria-expanded={aberto}
        aria-haspopup="dialog"
        className={`relative flex h-10 w-10 shrink-0 items-center justify-center rounded-xl border transition-all duration-200 ${aberto ? 'border-sky-400/40 bg-sky-500/15 text-white shadow-lg shadow-sky-950/30' : 'border-slate-700/80 bg-slate-900/70 text-slate-300 hover:border-slate-600 hover:bg-slate-800 hover:text-white'}`}
      >
        <Bell className={`h-[18px] w-[18px] transition-transform duration-200 ${naoLidasQtd > 0 ? 'animate-pulse-subtle' : ''}`} />
        {naoLidasQtd > 0 && (
          <span className="absolute -right-1 -top-1 flex h-[18px] min-w-[18px] items-center justify-center rounded-full border-2 border-slate-950 bg-rose-500 px-1 text-[10px] font-black leading-none text-white shadow-md shadow-rose-950/40">
            {naoLidasQtd > 9 ? '9+' : naoLidasQtd}
          </span>
        )}
      </button>

      {aberto && (
        <div role="dialog" aria-label="Central de notificações" aria-modal="false" className="fixed inset-x-2 top-[calc(env(safe-area-inset-top)+4rem)] z-[80] mx-auto flex max-h-[min(44rem,calc(100dvh-5rem-env(safe-area-inset-top)))] w-auto max-w-lg flex-col overflow-hidden rounded-[1.35rem] border border-slate-700/90 bg-slate-950 shadow-2xl shadow-slate-950/60 ring-1 ring-white/[0.04] animate-in fade-in slide-in-from-top-2 duration-200 sm:absolute sm:inset-x-auto sm:right-0 sm:top-[calc(100%+0.65rem)] sm:mx-0 sm:max-h-[min(42rem,calc(100dvh-7rem))] sm:w-[min(27rem,calc(100vw-2rem))]">
          <div className="relative flex shrink-0 items-center justify-between gap-3 overflow-hidden border-b border-slate-800 bg-gradient-to-br from-slate-900 via-slate-900 to-sky-950/50 px-4 py-4">
            <span className="pointer-events-none absolute -right-6 -top-10 h-28 w-28 rounded-full bg-sky-500/10 blur-2xl" />
            <div className="flex min-w-0 items-center gap-2.5">
              <span className="relative flex h-10 w-10 shrink-0 items-center justify-center rounded-2xl border border-sky-300/10 bg-sky-400/10 text-sky-200 shadow-inner shadow-sky-300/10"><Bell className="h-[18px] w-[18px]" />{naoLidasQtd > 0 && <span className="absolute right-1 top-1 h-2 w-2 rounded-full border border-slate-900 bg-rose-400" />}</span>
              <div className="min-w-0">
                <span className="block text-[15px] font-extrabold leading-tight tracking-tight text-white">Central de notificações</span>
                <span className="mt-1 block text-[11px] text-slate-400">{naoLidasQtd ? `${naoLidasQtd} novidade${naoLidasQtd === 1 ? '' : 's'} para você` : 'Você está em dia'}</span>
              </div>
            </div>
            <div className="flex shrink-0 items-center gap-1">
              <button
                type="button"
                onClick={() => { if (window.confirm('Limpar todas as notificações deste dispositivo?')) { NotificacoesStore.limpar(); atualizar(); } }}
                disabled={lista.length === 0}
                aria-label="Limpar todas as notificações"
                title="Limpar histórico"
                className="flex h-9 w-9 items-center justify-center rounded-xl text-slate-400 transition-colors hover:bg-rose-500/10 hover:text-rose-300 disabled:cursor-not-allowed disabled:opacity-30"
              >
                <Trash2 className="h-4 w-4" />
              </button>
              <button type="button" aria-label="Fechar notificações" onClick={() => setAberto(false)} className="flex h-9 w-9 items-center justify-center rounded-lg text-slate-400 transition-colors hover:bg-slate-800 hover:text-white">
                <X className="h-4 w-4" />
              </button>
            </div>
          </div>

          {lista.length > 0 && <div className="flex shrink-0 items-center justify-between gap-2 border-b border-slate-800/80 bg-slate-950 px-4 py-2.5">
            <div className="flex items-center gap-1 rounded-xl bg-slate-900 p-1" role="tablist" aria-label="Filtrar notificações">
              <button type="button" role="tab" aria-selected={filtro === 'todas'} onClick={() => setFiltro('todas')} className={`rounded-lg px-3 py-1.5 text-[11px] font-bold transition-all ${filtro === 'todas' ? 'bg-slate-700 text-white shadow-sm' : 'text-slate-400 hover:text-slate-200'}`}>Todas <span className="ml-1 text-[10px] opacity-70">{lista.length}</span></button>
              <button type="button" role="tab" aria-selected={filtro === 'nao-lidas'} onClick={() => setFiltro('nao-lidas')} className={`rounded-lg px-3 py-1.5 text-[11px] font-bold transition-all ${filtro === 'nao-lidas' ? 'bg-slate-700 text-white shadow-sm' : 'text-slate-400 hover:text-slate-200'}`}>Não lidas <span className="ml-1 text-[10px] opacity-70">{naoLidasQtd}</span></button>
            </div>
            {naoLidasQtd > 0 && <button type="button" onClick={() => { NotificacoesStore.marcarTodasLidas(); atualizar(); }} className="inline-flex min-h-9 items-center gap-1.5 rounded-lg px-2 text-[10px] font-bold text-sky-300 transition-colors hover:bg-sky-500/10 hover:text-white"><CheckCheck className="h-3.5 w-3.5" /><span className="hidden min-[380px]:inline">Marcar todas como lidas</span><span className="min-[380px]:hidden">Ler todas</span></button>}
          </div>}

          <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain bg-slate-950/70">
            {lista.length === 0 ? (
              <div className="flex min-h-64 flex-col items-center justify-center px-7 py-10 text-center">
                <span className="relative mb-4 flex h-[4.25rem] w-[4.25rem] items-center justify-center rounded-[1.4rem] border border-slate-700/70 bg-gradient-to-br from-slate-800 to-slate-900 text-slate-400 shadow-xl shadow-black/20"><Inbox className="h-7 w-7" /><Sparkles className="absolute -right-1 -top-1 h-4 w-4 text-sky-300" /></span>
                <span className="text-sm font-bold text-slate-200">Sua caixa está tranquila</span>
                <span className="mt-1.5 max-w-[17rem] text-xs leading-relaxed text-slate-500">Quando houver novidades sobre chamados, orçamentos ou serviços, elas aparecerão aqui.</span>
              </div>
            ) : notificacoesVisiveis.length === 0 ? (
              <div className="flex min-h-52 flex-col items-center justify-center px-6 py-8 text-center"><span className="mb-3 flex h-11 w-11 items-center justify-center rounded-2xl bg-emerald-500/10 text-emerald-300"><Check className="h-5 w-5" /></span><span className="text-xs font-bold text-slate-300">Tudo foi lido</span><span className="mt-1 text-[11px] text-slate-500">Não há notificações novas.</span></div>
            ) : (
              notificacoesVisiveis.map((n) => (
                <article key={n.id} className={`group relative border-b border-slate-800/70 transition-colors hover:bg-slate-900/60 ${!n.lida ? 'bg-gradient-to-r from-amber-300/[0.055] via-slate-900/20 to-transparent' : ''}`}>
                  {!n.lida && <span className="absolute bottom-3 left-0 top-3 w-[2px] rounded-r bg-gradient-to-b from-amber-200 to-amber-500" />}
                  <div className="flex gap-3 px-4 py-3.5">
                    <span className={`mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-xl border ${n.lida ? 'border-slate-700/70 bg-slate-900 text-slate-500' : 'border-amber-200/10 bg-amber-300/10 text-amber-200'}`}><Bell className="h-4 w-4" /></span>
                    <button type="button" onClick={() => {
                      if (!n.lida) { NotificacoesStore.marcarLida(n.id); atualizar(); }
                      if (n.url) { setAberto(false); window.location.assign(n.url); }
                    }} className="min-w-0 flex-1 text-left focus-visible:outline-none" title={n.url ? 'Abrir atualização' : n.lida ? 'Notificação lida' : 'Marcar como lida'}>
                      <div className="flex items-start justify-between gap-2">
                        <span className={`min-w-0 break-words text-xs leading-snug ${n.lida ? 'font-semibold text-slate-300' : 'font-extrabold text-white'}`}>{n.titulo}</span>
                        {!n.lida && <span className="mt-0.5 h-2 w-2 shrink-0 rounded-full bg-sky-400 shadow-[0_0_10px_rgba(56,189,248,.65)]" aria-label="Não lida" />}
                      </div>
                      <p className="mt-1.5 break-words text-[11px] leading-relaxed text-slate-400">{n.texto}</p>
                      <span className="mt-2 inline-flex items-center gap-1 text-[10px] font-medium text-slate-500"><Clock3 className="h-3 w-3" />{tempoRelativo(n.quando)}</span>
                      {n.url && <span className="ml-2 text-[10px] font-bold text-sky-300 opacity-80 transition-opacity group-hover:opacity-100">Abrir →</span>}
                    </button>
                    {!n.lida && <button type="button" onClick={() => { NotificacoesStore.marcarLida(n.id); atualizar(); }} aria-label="Marcar notificação como lida" title="Marcar como lida" className="flex h-9 w-9 shrink-0 items-center justify-center self-center rounded-xl text-slate-500 transition-colors hover:bg-emerald-500/10 hover:text-emerald-300"><Check className="h-4 w-4" /></button>}
                  </div>
                </article>
              ))
            )}
          </div>

          {lista.length > 0 && (
            <div className="flex shrink-0 items-center justify-between gap-2 border-t border-slate-800 bg-slate-900/80 px-4 py-2.5 text-[10px] text-slate-500">
              <span className="inline-flex items-center gap-1.5"><CheckCheck className="h-3.5 w-3.5 text-emerald-400/80" /> Sincronizado neste dispositivo</span><span>{lista.length}/{MAX}</span>
            </div>
          )}
        </div>
      )}
    </div>
  );
};
