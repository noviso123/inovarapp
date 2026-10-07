import React, { useState } from 'react';
import { supabase } from '../services/supabase';

export function RecuperarSenhaModal({ onClose, obrigatoria = false }: { onClose: () => void; obrigatoria?: boolean }) {
  const [password, setPassword] = useState('');
  const [confirmation, setConfirmation] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [done, setDone] = useState(false);
  async function save(e: React.FormEvent) {
    e.preventDefault();
    if (busy) return;
    setError('');
    if (password.length < 8) return setError('Use pelo menos 8 caracteres.');
    if (password !== confirmation) return setError('As senhas não são iguais.');
    setBusy(true);
    try {
      const { data } = await supabase.auth.getSession();
      if (!data.session) throw new Error('Este link expirou. Solicite outro em Esqueci minha senha.');
      const { error } = await supabase.auth.updateUser({ password, data: { must_change_password: false } });
      if (error) throw error;
      setPassword(''); setConfirmation(''); setDone(true);
      window.history.replaceState({}, '', window.location.pathname);
    } catch (err: any) { setError(err.message || 'Não foi possível salvar a senha.'); }
    finally { setBusy(false); }
  }
  return <div className="fixed inset-0 z-[100] bg-slate-950/90 p-4 flex items-center justify-center">
    <section role="dialog" aria-modal="true" aria-labelledby="recovery-title" className="w-full max-w-md rounded-2xl bg-slate-900 text-white border border-slate-600 p-6 max-h-[90dvh] overflow-auto">
      <h2 id="recovery-title" className="text-xl font-bold">{done ? 'Senha atualizada' : obrigatoria ? 'Troca de senha obrigatória' : 'Criar nova senha'}</h2>
      {done ? <><p className="my-4 text-slate-300">Sua nova senha já pode ser usada para acessar o app.</p><button onClick={onClose} className="w-full p-3 bg-blue-600 rounded-xl font-bold">Continuar no app</button></> : <form onSubmit={save} className="mt-4 space-y-4">
        <p className="text-sm text-slate-300">{obrigatoria ? 'Por segurança, crie uma senha pessoal antes de acessar o painel.' : 'Escolha uma senha de pelo menos 8 caracteres.'}</p>
        <label className="block text-sm">Nova senha<input autoFocus type="password" autoComplete="new-password" minLength={8} required value={password} onChange={e => setPassword(e.target.value)} className="mt-2 w-full p-3 rounded-xl bg-slate-800 border border-slate-600" /></label>
        <label className="block text-sm">Confirmar nova senha<input type="password" autoComplete="new-password" minLength={8} required value={confirmation} onChange={e => setConfirmation(e.target.value)} className="mt-2 w-full p-3 rounded-xl bg-slate-800 border border-slate-600" /></label>
        {error && <p role="alert" className="text-red-300 text-sm">{error}</p>}
        <button disabled={busy} className="w-full p-3 bg-blue-600 rounded-xl font-bold disabled:opacity-50">{busy ? 'Salvando...' : 'Salvar nova senha'}</button>
        {!obrigatoria && <button type="button" disabled={busy} onClick={onClose} className="w-full p-3 text-slate-300">Cancelar</button>}
      </form>}
    </section>
  </div>;
}
