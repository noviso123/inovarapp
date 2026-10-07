import React, { useState, useEffect, useRef } from 'react';
import {
  Calendar,
  CalendarClock,
  Wallet,
  Settings,
  Plus,
  Users,
  AlertTriangle,
  Clock,
  ShieldCheck,
  FileText,
  LogOut,
  UserCheck,
  Wrench,
  Sparkles,
  Calculator
} from 'lucide-react';
import { Logo } from './Logo';
import { CentralNotificacoes } from './CentralNotificacoes';
import { SupabaseProfile } from '../services/supabase';
import { comprimirFotoPerfil, subirFotoPerfil, urlFotoPerfil, removerFotoPerfil } from '../services/perfilFoto';
import { Camera, X } from 'lucide-react';

interface HeaderProps {
  activeTab: 'inicio' | 'agenda' | 'proximos-agendamentos' | 'financeiro' | 'servicos' | 'orcamentos' | 'clientes' | 'proximos-retornos';
  setActiveTab: (tab: 'inicio' | 'agenda' | 'proximos-agendamentos' | 'financeiro' | 'servicos' | 'orcamentos' | 'clientes' | 'proximos-retornos') => void;
  onOpenNovoCliente: () => void;
  onOpenNovoAgendamento: () => void;
  onOpenNovoOrcamento: () => void;
  onOpenConfig: () => void;
  onOpenAccount: () => void;
  onSignOut: () => void;
  userProfile: SupabaseProfile | null;
  stats: {
    totalClientes: number;
    atrasados: number;
    estaSemana: number;
    emBreve: number;
    solicitacoesPendentes?: number;
    totalOrcamentos?: number;
  };
}

export const Header: React.FC<HeaderProps> = ({
  activeTab,
  setActiveTab,
  onOpenNovoCliente,
  onOpenNovoAgendamento,
  onOpenNovoOrcamento,
  onOpenConfig,
  onOpenAccount,
  onSignOut,
  userProfile,
  stats
}) => {
  const isTechOrAdmin = userProfile?.tipo === 'ADMIN' || userProfile?.tipo === 'TECNICO';

  // Foto de perfil do usuário logado (técnico/admin) — troca direto pelo header
  const [fotoPerfil, setFotoPerfil] = useState<string | null>(null);
  const fotoInputRef = useRef<HTMLInputElement>(null);
  const [enviandoFoto, setEnviandoFoto] = useState(false);

  useEffect(() => {
    if (userProfile) urlFotoPerfil().then(setFotoPerfil);
    else setFotoPerfil(null);
  }, [userProfile?.id]);

  const trocarFoto = async (file?: File | null) => {
    if (!file) return;
    setEnviandoFoto(true);
    try {
      const base64 = await comprimirFotoPerfil(file);
      const anterior = fotoPerfil;
      setFotoPerfil(base64);
      const r = await subirFotoPerfil(base64);
      if (r.ok) setFotoPerfil(await urlFotoPerfil());
      else setFotoPerfil(anterior);
    } catch {
      // Mantém a foto anterior se o arquivo não puder ser preparado/enviado.
    } finally {
      setEnviandoFoto(false);
      if (fotoInputRef.current) fotoInputRef.current.value = '';
    }
  };

  const removerFoto = async () => {
    if (!confirm('Remover sua foto de perfil?')) return;
    await removerFotoPerfil();
    setFotoPerfil(null);
  };

  return (
    <header className="bg-slate-950 border-b border-slate-800 sticky top-0 z-40 shadow-2xl">
      {/* Main Bar with Inovar Logo and Actions */}
      <div className="max-w-7xl mx-auto px-3 sm:px-4 py-2.5 flex items-center justify-between gap-2 sm:gap-3">
        <Logo size="md" invertido={true} className="shrink-0" />

        {userProfile && (
          <div className="ml-auto flex min-w-0 items-center gap-1.5 sm:gap-2">
            <button onClick={onOpenAccount} className="min-h-9 rounded-lg border border-slate-700 bg-slate-800/80 px-2 sm:px-3 text-[10px] sm:text-xs font-bold text-white transition-colors hover:bg-slate-700" title="Google, senha e calendários">
              <span className="hidden sm:inline">Minha conta</span><span className="sm:hidden">Conta</span>
            </button>
            {isTechOrAdmin && (
              <div className="relative flex shrink-0 items-center">
                <button onClick={() => fotoInputRef.current?.click()} title="Trocar minha foto de perfil" aria-label="Trocar minha foto de perfil" className="group relative flex h-9 w-9 items-center justify-center overflow-hidden rounded-full border-2 border-amber-300/80 bg-slate-800 shadow-lg transition-all hover:border-amber-200 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-sky-400">
                  {enviandoFoto ? <span className="h-4 w-4 rounded-full border-2 border-inovar-yellow border-t-transparent animate-spin" /> : fotoPerfil ? <img key={fotoPerfil} src={fotoPerfil} alt="Foto de perfil" className="h-full w-full object-cover" /> : <Camera className="h-4 w-4 text-inovar-yellow" />}
                </button>
                {fotoPerfil && <button onClick={removerFoto} title="Remover foto" aria-label="Remover foto de perfil" className="absolute -right-1 -top-1 flex h-4 w-4 items-center justify-center rounded-full border border-red-400/60 bg-slate-950 text-red-300"><X className="h-3 w-3" /></button>}
                <input ref={fotoInputRef} type="file" accept="image/*" className="hidden" onChange={(e) => trocarFoto(e.target.files?.[0])} />
              </div>
            )}
            <button onClick={onSignOut} className="flex min-h-9 shrink-0 items-center gap-1 rounded-lg border border-slate-700 bg-slate-800/80 px-2 text-[10px] font-medium text-slate-200 transition-colors hover:border-red-900 hover:bg-red-950/60 hover:text-red-200" title="Sair da Conta"><LogOut className="h-3 w-3" /><span>Sair</span></button>
            {isTechOrAdmin && <>
              <button onClick={onOpenConfig} className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border border-slate-700/80 bg-slate-900/70 text-slate-200 transition-colors hover:bg-slate-800 hover:text-white" title="Configurações & Perfil"><Settings className="h-4 w-4" /></button>
              <CentralNotificacoes />
            </>}
          </div>
        )}

        {/* Action Buttons for Technician */}
        {isTechOrAdmin && (
          <div className="flex items-center gap-1.5 sm:gap-2 min-w-0">
            <button
              onClick={onOpenNovoOrcamento}
              className="flex items-center gap-1.5 px-2.5 sm:px-3 py-1.5 bg-inovar-yellow hover:brightness-105 active:scale-95 text-inovar-navy rounded-lg text-xs font-bold shadow-md transition-all shrink-0"
            >
              <Calculator className="w-3.5 h-3.5" />
              <span className="hidden sm:inline">Novo </span>Orçamento
            </button>

            <button
              onClick={onOpenNovoCliente}
              className="flex items-center gap-1.5 px-2.5 sm:px-3 py-1.5 bg-slate-850 hover:bg-slate-800 text-slate-200 border border-slate-700 rounded-lg text-xs font-semibold transition-all active:scale-95 shrink-0"
            >
              <Plus className="w-3.5 h-3.5" />
              <span className="hidden sm:inline">Cliente</span>
            </button>
          </div>
        )}
      </div>

      {/* Navigation tabs for Technician */}
      {isTechOrAdmin && (
        <div className="hidden sm:flex max-w-7xl mx-auto px-4 items-center gap-1.5 overflow-x-auto border-t border-slate-800/80 pt-1.5 pb-1.5 scrollbar-none text-xs">
          <button
            onClick={() => setActiveTab('inicio')}
            className={`px-3 py-1.5 rounded-lg font-bold transition-all flex items-center gap-1.5 whitespace-nowrap ${
              activeTab === 'inicio'
                ? 'bg-sky-600 text-white shadow-sm'
                : 'text-slate-400 hover:text-white hover:bg-slate-800/60'
            }`}
          >
            <Clock className="w-3.5 h-3.5" />
            <span>Central de Atendimento</span>
            {stats.atrasados + stats.estaSemana > 0 && (
              <span className="px-1.5 py-0.2 text-[10px] bg-red-500 text-white rounded-full font-extrabold ml-0.5">
                {stats.atrasados + stats.estaSemana}
              </span>
            )}
          </button>

          <button
            onClick={() => setActiveTab('orcamentos')}
            className={`px-3 py-1.5 rounded-lg font-bold transition-all flex items-center gap-1.5 whitespace-nowrap ${
              activeTab === 'orcamentos'
                ? 'bg-sky-600 text-white shadow-sm'
                : 'text-slate-400 hover:text-white hover:bg-slate-800/60'
            }`}
          >
            <Calculator className="w-3.5 h-3.5 text-inovar-yellow" />
            <span>Propostas</span>
            {(stats.totalOrcamentos || 0) > 0 && (
              <span className="px-1.5 py-0.2 text-[10px] bg-inovar-yellow text-inovar-navy rounded-full font-black">
                {stats.totalOrcamentos}
              </span>
            )}
          </button>

          <button
            onClick={() => setActiveTab('agenda')}
            className={`px-3 py-1.5 rounded-lg font-bold transition-all flex items-center gap-1.5 whitespace-nowrap ${
              activeTab === 'agenda'
                ? 'bg-sky-600 text-white shadow-sm'
                : 'text-slate-400 hover:text-white hover:bg-slate-800/60'
            }`}
          >
            <CalendarClock className="w-3.5 h-3.5" />
            <span>Agenda</span>
          </button>

          <button
            onClick={() => setActiveTab('financeiro')}
            className={`px-3 py-1.5 rounded-lg font-bold transition-all flex items-center gap-1.5 whitespace-nowrap ${
              activeTab === 'financeiro'
                ? 'bg-emerald-600 text-white shadow-sm'
                : 'text-slate-400 hover:text-white hover:bg-slate-800/60'
            }`}
          >
            <Wallet className="w-3.5 h-3.5" />
            <span>Financeiro</span>
          </button>

          <button
            onClick={() => setActiveTab('servicos')}
            className={`px-3 py-1.5 rounded-lg font-bold transition-all flex items-center gap-1.5 whitespace-nowrap ${
              activeTab === 'servicos'
                ? 'bg-sky-600 text-white shadow-sm'
                : 'text-slate-400 hover:text-white hover:bg-slate-800/60'
            }`}
          >
            <Wrench className="w-3.5 h-3.5" />
            <span>Serviços de Campo</span>
          </button>

          <button
            onClick={() => setActiveTab('clientes')}
            className={`px-3 py-1.5 rounded-lg font-bold transition-all flex items-center gap-1.5 whitespace-nowrap ${
              activeTab === 'clientes'
                ? 'bg-sky-600 text-white shadow-sm'
                : 'text-slate-400 hover:text-white hover:bg-slate-800/60'
            }`}
          >
            <Users className="w-3.5 h-3.5" />
            <span>Clientes</span>
            <span className="px-1.5 py-0.2 text-[10px] bg-slate-800 text-slate-300 rounded-full">
              {stats.totalClientes}
            </span>
          </button>

        </div>
      )}
    </header>
  );
};
