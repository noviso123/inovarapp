import React, { useEffect, useState } from 'react';
import { supabase, SupabaseService } from '../services/supabase';
import { authorizeGoogle, automaticGoogleSync, googleCalendarRequest } from '../services/googleConnection';

export function ContaModal({ onClose, equipe }: { onClose: () => void; equipe: boolean }) {
  const [email, setEmail] = useState(''); const [linked, setLinked] = useState(false);
  const [busy, setBusy] = useState(false); const [message, setMessage] = useState('');
  const [calendar, setCalendar] = useState<any>({});
  useEffect(() => {
    supabase.auth.getUser().then(({ data }) => { setEmail(data.user?.email || ''); setLinked(!!data.user?.identities?.some(i => i.provider === 'google')); });
    if (equipe) googleCalendarRequest().then(setCalendar).catch(e => setMessage(e.message));
    const status = (e: Event) => { const detail = (e as CustomEvent).detail; setCalendar((c: any) => ({ ...c, ...detail })); };
    window.addEventListener('inovar-calendar-status', status);
    return () => window.removeEventListener('inovar-calendar-status', status);
  }, [equipe]);
  async function run(fn: () => Promise<any>) {
    if (busy) return; setBusy(true); setMessage('');
    try { await fn(); } catch (err: any) { setMessage(err.message || 'Não foi possível concluir. Tente novamente.'); }
    finally { setBusy(false); }
  }
  const button = 'w-full min-h-11 p-3 rounded-xl bg-blue-600 hover:bg-blue-500 text-white font-bold disabled:opacity-50';
  return <div className="fixed inset-0 z-[70] bg-slate-950/85 p-3 flex items-center justify-center">
    <section role="dialog" aria-modal="true" aria-labelledby="account-title" className="bg-white text-slate-900 rounded-2xl max-w-lg w-full max-h-[92dvh] overflow-y-auto shadow-xl">
      <header className="p-5 border-b flex justify-between gap-3 items-center"><div><h2 id="account-title" className="text-lg font-bold">Minha conta e conexões</h2><p className="text-sm text-slate-500 break-all">{email}</p></div><button aria-label="Fechar minha conta" onClick={onClose} className="p-3 rounded-xl bg-slate-100">✕</button></header>
      <div className="p-5 space-y-5">
        {message && <p role="status" className="p-3 bg-amber-50 text-amber-900 rounded-xl text-sm">{message}</p>}
        <section className="space-y-3"><h3 className="font-bold">Acesso à conta</h3><p className="text-sm text-slate-600">{linked ? 'Sua conta Google está vinculada. Você pode entrar com e-mail e senha ou com o Google.' : 'Conecte o Google à sua conta atual para manter seus cadastros e histórico no mesmo lugar.'}</p>
          {!linked && <button disabled={busy} onClick={() => run(() => authorizeGoogle())} className={button}>Vincular minha conta Google</button>}
          <button disabled={busy || !email} onClick={() => run(async () => { const { error } = await SupabaseService.requestPasswordReset(email); if (error) throw error; setMessage('Link de alteração de senha solicitado. Confira seu e-mail e a pasta de spam.'); })} className="w-full p-3 min-h-11 border border-slate-300 rounded-xl font-semibold">Alterar minha senha por e-mail</button>
        </section>
        {equipe && <section className="space-y-3 border-t pt-5"><h3 className="font-bold">Google Agenda</h3>
          <p className="text-sm text-slate-600">Após autorizar, o app sincroniza agendamentos, reagendamentos e encerramentos enquanto você trabalha. Os eventos aparecem nos aparelhos conectados à mesma conta Google.</p>
          {calendar.error && <p role="alert" className="text-red-700 text-sm">{calendar.error}</p>}
          {calendar.lastSync && <p className="text-xs text-slate-500">Última sincronização: {new Date(calendar.lastSync).toLocaleString('pt-BR')}</p>}
          <button disabled={busy} onClick={() => run(() => authorizeGoogle('calendar'))} className={button}>{calendar.connected ? 'Renovar autorização Google Agenda' : 'Conectar Google Agenda'}</button>
          {calendar.connected && <div className="grid grid-cols-2 gap-2"><button disabled={busy} onClick={() => run(() => automaticGoogleSync())} className="p-3 border rounded-xl font-semibold">Sincronizar agora</button><button disabled={busy} onClick={() => run(async () => { await googleCalendarRequest('disconnect'); setCalendar({ connected: false }); setMessage('Sincronização desativada. Os eventos já criados permanecem no Google.'); })} className="p-3 border rounded-xl text-slate-600">Desconectar agenda</button></div>}
        </section>}
        <p className="text-xs text-slate-500">No iPhone, instale pelo Safari: Compartilhar → Adicionar à Tela de Início. No Android e computador, use Instalar aplicativo quando disponível.</p>
      </div>
    </section>
  </div>;
}
