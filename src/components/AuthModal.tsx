import React, { useState } from 'react';
import { X, Lock, Mail, User, Phone, MapPin, CheckCircle2, AlertCircle, Loader2 } from 'lucide-react';
import { SupabaseService } from '../services/supabase';
import { buscarCep, mascaraCep } from '../services/cep';
import { Logo } from './Logo';

interface AuthModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSuccess: () => void;
  initialMode?: 'login' | 'signup';
}

export const AuthModal: React.FC<AuthModalProps> = ({
  isOpen,
  onClose,
  onSuccess,
  initialMode = 'login'
}) => {
  const [mode, setMode] = useState<'login' | 'signup' | 'forgot'>(initialMode);
  const [loading, setLoading] = useState(false);
  const [errorMsg, setErrorMsg] = useState<string | null>(null);
  const [successMsg, setSuccessMsg] = useState<string | null>(null);

  // Form states
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [nome, setNome] = useState('');
  const [whatsapp, setWhatsapp] = useState('');
  const [endereco, setEndereco] = useState('');
  const [bairro, setBairro] = useState('');
  const [cidade, setCidade] = useState('Vitória');
  const [cep, setCep] = useState('');
  const [buscandoCep, setBuscandoCep] = useState(false);

  // CEP: preenche rua/bairro/cidade automaticamente (tudo continua editável)
  const aplicarCep = async (v: string) => {
    const m = mascaraCep(v);
    setCep(m);
    if (m.replace(/\D/g, '').length !== 8) return;
    setBuscandoCep(true);
    const end = await buscarCep(m);
    setBuscandoCep(false);
    if (end) {
      if (end.rua) setEndereco(end.rua);
      if (end.bairro) setBairro(end.bairro);
      if (end.cidade) setCidade(end.cidade);
    }
  };

  if (!isOpen) return null;

  const handleGoogleLogin = async () => {
    try {
      setLoading(true);
      setErrorMsg(null);
      const { error } = await SupabaseService.signInWithGoogle();
      if (error) throw error;
      // Redirecionamento é automático pelo Supabase
    } catch (err: any) {
      setErrorMsg(err.message || 'Erro ao realizar login com o Google.');
      setLoading(false);
    }
  };

  const handleForgot = async (e: React.FormEvent) => {
    e.preventDefault();
    if (loading) return;
    setLoading(true); setErrorMsg(null); setSuccessMsg(null);
    try {
      const { error } = await SupabaseService.requestPasswordReset(email.trim());
      if (error) throw error;
      setSuccessMsg('Se houver uma conta com este e-mail, você receberá um link para criar uma nova senha. Confira também o spam.');
    } catch { setErrorMsg('Não foi possível enviar o link agora. Aguarde alguns minutos e tente novamente.'); }
    finally { setLoading(false); }
  };

  const handleLogin = async (e: React.FormEvent) => {
    e.preventDefault();
    setErrorMsg(null);
    setSuccessMsg(null);
    setLoading(true);

    try {
      const { data, error } = await SupabaseService.signIn(email.trim(), password);
      if (error) throw error;

      setSuccessMsg('Login realizado com sucesso!');
      setTimeout(() => {
        onSuccess();
        onClose();
      }, 700);
    } catch (err: any) {
      setErrorMsg(err.message || 'Erro ao realizar login. Verifique seu email e senha.');
    } finally {
      setLoading(false);
    }
  };

  const handleSignUp = async (e: React.FormEvent) => {
    e.preventDefault();
    setErrorMsg(null);
    setSuccessMsg(null);
    setLoading(true);

    if (password.length < 6) {
      setErrorMsg('A senha deve conter no mínimo 6 caracteres.');
      setLoading(false);
      return;
    }

    try {
      // Cria a conta já confirmada e entra direto (sem precisar confirmar e-mail)
      const resp = await fetch('/api/contas', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          acao: 'minha_conta',
          email: email.trim(),
          senha: password,
          nome: nome.trim(),
          whatsapp: whatsapp.trim(),
          endereco: endereco.trim(),
          bairro: bairro.trim(),
          cidade: cidade.trim()
        })
      });
      const data = await resp.json().catch(() => ({}));

      if (!resp.ok) {
        setErrorMsg(data.error || 'Erro ao criar sua conta. Tente novamente.');
        setLoading(false);
        return;
      }

      // Login imediato com a senha escolhida
      const { error: loginErr } = await SupabaseService.signIn(email.trim(), password);
      if (loginErr) throw loginErr;

      setSuccessMsg('Conta criada com sucesso! Entrando no seu portal...');
      setTimeout(() => {
        onSuccess();
        onClose();
      }, 800);
    } catch (err: any) {
      setErrorMsg(err.message || 'Erro ao cadastrar cliente. Tente novamente.');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/75 backdrop-blur-sm animate-in fade-in duration-200">
      <div className="bg-slate-900 border border-slate-700/80 rounded-2xl w-full max-w-md overflow-hidden shadow-2xl text-slate-100 flex flex-col max-h-[90dvh]">
        {/* Header */}
        <div className="p-4 bg-gradient-to-r from-inovar-navy via-slate-900 to-slate-950 border-b border-slate-800 flex items-center justify-between">
          <Logo size="sm" invertido={true} />
          <button
            onClick={onClose}
            className="p-1.5 text-slate-400 hover:text-white rounded-lg hover:bg-slate-800 transition-colors"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Mode Selector Tabs */}
        <div className="grid grid-cols-2 p-1.5 bg-slate-950 border-b border-slate-800/80 text-sm font-semibold">
          <button
            type="button"
            onClick={() => { setMode('login'); setErrorMsg(null); }}
            className={`py-2 rounded-lg transition-all ${
              mode === 'login'
                ? 'bg-inovar-yellow text-inovar-navy shadow-md font-bold'
                : 'text-slate-400 hover:text-white'
            }`}
          >
            Entrar no App
          </button>
          <button
            type="button"
            onClick={() => { setMode('signup'); setErrorMsg(null); }}
            className={`py-2 rounded-lg transition-all ${
              mode === 'signup'
                ? 'bg-inovar-yellow text-inovar-navy shadow-md font-bold'
                : 'text-slate-400 hover:text-white'
            }`}
          >
            Novo Cliente
          </button>
        </div>

        <div className="p-5 overflow-y-auto space-y-4">
          {errorMsg && (
            <div role="alert" className="p-3 bg-red-950/60 border border-red-800/80 rounded-xl text-red-200 text-xs flex items-center gap-2.5">
              <AlertCircle className="w-4 h-4 text-red-400 shrink-0" />
              <span>{errorMsg}</span>
            </div>
          )}

          {successMsg && (
            <div role="status" className="p-3 bg-emerald-950/60 border border-emerald-800/80 rounded-xl text-emerald-200 text-xs flex items-center gap-2.5">
              <CheckCircle2 className="w-4 h-4 text-emerald-400 shrink-0" />
              <span>{successMsg}</span>
            </div>
          )}

          {mode === 'forgot' ? (
            <form onSubmit={handleForgot} className="space-y-4">
              <h2 className="text-lg font-bold">Recuperar minha senha</h2>
              <p className="text-sm text-slate-300">Informe o e-mail da sua conta para receber o link de recuperação.</p>
              <label className="block text-sm">E-mail
                <input aria-label="E-mail de recuperação" type="email" autoComplete="email" required value={email} onChange={e => setEmail(e.target.value)} className="mt-2 w-full p-3 bg-slate-800 border border-slate-600 rounded-xl" />
              </label>
              <button disabled={loading} className="w-full p-3 bg-inovar-yellow text-slate-950 rounded-xl font-bold disabled:opacity-50">{loading ? 'Enviando...' : 'Enviar link de recuperação'}</button>
              <button type="button" onClick={() => { setMode('login'); setSuccessMsg(null); setErrorMsg(null); }} className="w-full p-3 text-slate-200">Voltar ao login</button>
            </form>
          ) : mode === 'login' ? (
            <form onSubmit={handleLogin} className="space-y-3.5">
              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1.5">E-mail</label>
                <div className="relative">
                  <Mail className="w-4 h-4 text-slate-400 absolute left-3 top-3" />
                  <input
                    type="email"
                    required
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    placeholder="seuemail@exemplo.com"
                    className="w-full pl-9 pr-3 py-2.5 bg-slate-800/90 border border-slate-700 rounded-xl text-sm text-white placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-inovar-yellow/50"
                  />
                </div>
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1.5">Senha</label>
                <div className="relative">
                  <Lock className="w-4 h-4 text-slate-400 absolute left-3 top-3" />
                  <input
                    type="password"
                    required
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    placeholder="••••••••"
                    className="w-full pl-9 pr-3 py-2.5 bg-slate-800/90 border border-slate-700 rounded-xl text-sm text-white placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-inovar-yellow/50"
                  />
                </div>
              </div>



              <button
                type="submit"
                disabled={loading}
                className="w-full py-3 bg-inovar-yellow hover:brightness-105 active:scale-[0.99] text-inovar-navy font-bold rounded-xl text-sm transition-all flex items-center justify-center gap-2 shadow-lg disabled:opacity-50"
              >
                {loading ? <Loader2 className="w-4 h-4 animate-spin" /> : 'Entrar no Sistema'}
              </button>
              <button type="button" disabled={loading} onClick={() => { setMode('forgot'); setErrorMsg(null); setSuccessMsg(null); }} className="w-full min-h-11 text-sm text-sky-300 hover:text-white">Esqueci minha senha</button>
              <div className="relative flex items-center py-2">
                <div className="flex-grow border-t border-slate-700"></div>
                <span className="flex-shrink-0 mx-4 text-slate-500 text-xs">ou</span>
                <div className="flex-grow border-t border-slate-700"></div>
              </div>

              <button
                type="button"
                onClick={handleGoogleLogin}
                disabled={loading}
                className="w-full py-2.5 bg-white hover:bg-gray-100 text-gray-900 font-bold rounded-xl text-sm transition-all flex items-center justify-center gap-2 shadow disabled:opacity-50"
              >
                <svg className="w-5 h-5" viewBox="0 0 24 24">
                  <path fill="#4285F4" d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z"/>
                  <path fill="#34A853" d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"/>
                  <path fill="#FBBC05" d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z"/>
                  <path fill="#EA4335" d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z"/>
                </svg>
                Entrar com Google
              </button>
            </form>
          ) : (
            <form onSubmit={handleSignUp} className="space-y-3">
              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">Nome Completo</label>
                <div className="relative">
                  <User className="w-4 h-4 text-slate-400 absolute left-3 top-2.5" />
                  <input
                    type="text"
                    required
                    value={nome}
                    onChange={(e) => setNome(e.target.value)}
                    placeholder="Ex: João Ferreira"
                    className="w-full pl-9 pr-3 py-2 bg-slate-800/90 border border-slate-700 rounded-xl text-sm text-white placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-inovar-yellow/50"
                  />
                </div>
              </div>

              <div className="grid grid-cols-1 sm:grid-cols-2 gap-2.5">
                <div>
                  <label className="block text-xs font-semibold text-slate-300 mb-1">E-mail</label>
                  <div className="relative">
                    <Mail className="w-4 h-4 text-slate-400 absolute left-3 top-2.5" />
                    <input
                      type="email"
                      required
                      value={email}
                      onChange={(e) => setEmail(e.target.value)}
                      placeholder="seu@email.com"
                      className="w-full pl-9 pr-3 py-2 bg-slate-800/90 border border-slate-700 rounded-xl text-sm text-white placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-inovar-yellow/50"
                    />
                  </div>
                </div>

                <div>
                  <label className="block text-xs font-semibold text-slate-300 mb-1">WhatsApp</label>
                  <div className="relative">
                    <Phone className="w-4 h-4 text-slate-400 absolute left-3 top-2.5" />
                    <input
                      type="tel"
                      required
                      value={whatsapp}
                      onChange={(e) => setWhatsapp(e.target.value)}
                      placeholder="(27) 99999-9999"
                      className="w-full pl-9 pr-3 py-2 bg-slate-800/90 border border-slate-700 rounded-xl text-sm text-white placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-inovar-yellow/50"
                    />
                  </div>
                </div>
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">Criar Senha</label>
                <div className="relative">
                  <Lock className="w-4 h-4 text-slate-400 absolute left-3 top-2.5" />
                  <input
                    type="password"
                    required
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    placeholder="Mínimo 6 dígitos"
                    className="w-full pl-9 pr-3 py-2 bg-slate-800/90 border border-slate-700 rounded-xl text-sm text-white placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-inovar-yellow/50"
                  />
                </div>
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">CEP (preenche o endereço automático)</label>
                <div className="relative">
                  <input
                    type="text"
                    value={cep}
                    onChange={(e) => aplicarCep(e.target.value)}
                    placeholder="Ex: 29060-270"
                    inputMode="numeric"
                    className="w-full px-3 py-2 bg-slate-800/90 border border-slate-700 rounded-xl text-sm text-white placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-inovar-yellow/50"
                  />
                  {buscandoCep && <Loader2 className="w-4 h-4 text-inovar-yellow animate-spin absolute right-3 top-2.5" />}
                </div>
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">Endereço (Rua e Número)</label>
                <div className="relative">
                  <MapPin className="w-4 h-4 text-slate-400 absolute left-3 top-2.5" />
                  <input
                    type="text"
                    value={endereco}
                    onChange={(e) => setEndereco(e.target.value)}
                    placeholder="Ex: Av. Dante Michelini, 1200"
                    className="w-full pl-9 pr-3 py-2 bg-slate-800/90 border border-slate-700 rounded-xl text-sm text-white placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-inovar-yellow/50"
                  />
                </div>
              </div>

              <div className="grid grid-cols-2 gap-2">
                <div>
                  <label className="block text-xs font-semibold text-slate-300 mb-1">Bairro</label>
                  <input
                    type="text"
                    value={bairro}
                    onChange={(e) => setBairro(e.target.value)}
                    placeholder="Ex: Jardim da Penha"
                    className="w-full px-3 py-2 bg-slate-800/90 border border-slate-700 rounded-xl text-sm text-white placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-inovar-yellow/50"
                  />
                </div>
                <div>
                  <label className="block text-xs font-semibold text-slate-300 mb-1">Cidade</label>
                  <input
                    type="text"
                    value={cidade}
                    onChange={(e) => setCidade(e.target.value)}
                    placeholder="Ex: Vitória"
                    className="w-full px-3 py-2 bg-slate-800/90 border border-slate-700 rounded-xl text-sm text-white placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-inovar-yellow/50"
                  />
                </div>
              </div>

              <button
                type="submit"
                disabled={loading}
                className="w-full mt-2 py-3 bg-inovar-yellow hover:brightness-105 active:scale-[0.99] text-inovar-navy font-bold rounded-xl text-sm transition-all flex items-center justify-center gap-2 shadow-lg disabled:opacity-50"
              >
                {loading ? <Loader2 className="w-4 h-4 animate-spin" /> : 'Cadastrar e Conectar'}
              </button>

              <div className="relative flex items-center py-2">
                <div className="flex-grow border-t border-slate-700"></div>
                <span className="flex-shrink-0 mx-4 text-slate-500 text-xs">ou</span>
                <div className="flex-grow border-t border-slate-700"></div>
              </div>

              <button
                type="button"
                onClick={handleGoogleLogin}
                disabled={loading}
                className="w-full py-2.5 bg-white hover:bg-gray-100 text-gray-900 font-bold rounded-xl text-sm transition-all flex items-center justify-center gap-2 shadow disabled:opacity-50"
              >
                <svg className="w-5 h-5" viewBox="0 0 24 24">
                  <path fill="#4285F4" d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z"/>
                  <path fill="#34A853" d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"/>
                  <path fill="#FBBC05" d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z"/>
                  <path fill="#EA4335" d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z"/>
                </svg>
                Cadastrar com Google
              </button>
            </form>
          )}
        </div>
      </div>
    </div>
  );
};
