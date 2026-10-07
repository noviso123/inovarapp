import React, { useEffect, useState } from 'react';
import { ativarNotificacoesPush, pushDisponivel } from '../services/pushNotifications';

export function PwaStatus() {
  const [online, setOnline] = useState(navigator.onLine);
  const [install, setInstall] = useState<any>(null);
  const [dismissed, setDismissed] = useState(false);
  const [pushState, setPushState] = useState<'idle' | 'loading' | 'ready' | 'error'>('idle');
  const [pushMessage, setPushMessage] = useState('');
  useEffect(() => {
    const update = () => setOnline(navigator.onLine);
    const prompt = (e: Event) => { e.preventDefault(); setInstall(e); };
    const installed = () => setInstall(null);
    window.addEventListener('online', update); window.addEventListener('offline', update);
    window.addEventListener('beforeinstallprompt', prompt); window.addEventListener('appinstalled', installed);
    return () => { window.removeEventListener('online', update); window.removeEventListener('offline', update); window.removeEventListener('beforeinstallprompt', prompt); window.removeEventListener('appinstalled', installed); };
  }, []);
  if (!online) return <div role="status" className="bg-amber-100 text-amber-950 p-3 text-center text-sm">Sem conexão. Os dados exibidos podem estar desatualizados. Reconecte para salvar alterações.</div>;
  const podeAtivarPush = pushDisponivel() && Notification.permission !== 'granted';
  if ((!install && !podeAtivarPush) || dismissed) return null;
  return <div className="flex flex-wrap items-center justify-center gap-2 sm:gap-3 bg-sky-700 text-white p-3 text-sm">
    {install && <><span>Instale o InovarApp para receber avisos mesmo fechado.</span><button onClick={async () => { await install.prompt(); const result = await install.userChoice; if (result.outcome === 'accepted') setInstall(null); }} className="px-4 py-2 bg-white text-sky-800 rounded-lg font-bold">Instalar aplicativo</button></>}
    {podeAtivarPush && <><span className="font-medium">Ative avisos de OS, documentos, chamados, orçamentos, agenda e atrasos.</span><button disabled={pushState === 'loading'} onClick={async () => { setPushState('loading'); setPushMessage(''); try { await ativarNotificacoesPush(); setPushState('ready'); setPushMessage('Notificações ativadas neste dispositivo.'); } catch (error: any) { setPushState('error'); setPushMessage(error.message || 'Não foi possível ativar notificações.'); } }} className="px-4 py-2 bg-inovar-yellow text-inovar-navy rounded-lg font-bold disabled:opacity-60">{pushState === 'loading' ? 'Ativando…' : 'Ativar notificações'}</button></>}
    {pushMessage && <span role="status" className={pushState === 'error' ? 'text-amber-100' : 'text-sky-100'}>{pushMessage}</span>}
    <button onClick={() => setDismissed(true)} className="px-3 py-2 underline underline-offset-2">Agora não</button>
  </div>;
}
