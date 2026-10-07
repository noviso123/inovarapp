import React from 'react';
import {
  X,
  Home,
  Wrench,
  UserPlus,
  Calendar,
  Calculator,
  Users,
  Wallet,
  ClipboardList
} from 'lucide-react';
import { ServiceType } from '../types';
import { montarCatalogo, TipoCatalogo } from '../services/catalogo';

const CHIP_CLASSES: Record<string, string> = {
  emerald: 'bg-emerald-50 hover:bg-emerald-100 border-emerald-200 text-emerald-800',
  sky: 'bg-sky-50 hover:bg-sky-100 border-sky-200 text-sky-800',
  amber: 'bg-amber-50 hover:bg-amber-100 border-amber-200 text-amber-800',
  purple: 'bg-purple-50 hover:bg-purple-100 border-purple-200 text-purple-800',
  blue: 'bg-blue-50 hover:bg-blue-100 border-blue-200 text-blue-800',
  slate: 'bg-slate-50 hover:bg-slate-100 border-slate-300 text-slate-700'
};

interface MobileActionSheetProps {
  isOpen: boolean;
  onClose: () => void;
  catalogo?: TipoCatalogo[];
  onOpenIniciarServico: (type?: ServiceType) => void;
  onOpenNovoCliente: () => void;
  onOpenNovoAgendamento: () => void;
  onOpenNovoOrcamento: () => void;
  onOpenServicos?: () => void;
  onOpenInicio?: () => void;
  onOpenAgenda?: () => void;
  onOpenOrcamentos?: () => void;
  onOpenClientes?: () => void;
  onOpenFinanceiro?: () => void;
  activeTab?: 'inicio' | 'agenda' | 'proximos-agendamentos' | 'financeiro' | 'servicos' | 'orcamentos' | 'clientes' | 'proximos-retornos';
}

export const MobileActionSheet: React.FC<MobileActionSheetProps> = ({
  isOpen,
  onClose,
  catalogo,
  onOpenIniciarServico,
  onOpenNovoCliente,
  onOpenNovoAgendamento,
  onOpenNovoOrcamento,
  onOpenServicos,
  onOpenInicio,
  onOpenAgenda,
  onOpenOrcamentos,
  onOpenClientes,
  onOpenFinanceiro,
  activeTab
}) => {
  if (!isOpen) return null;

  // Atalhos diretos seguem o catálogo atual (fixos editáveis/removíveis + personalizados)
  const atalhos = (catalogo && catalogo.length > 0 ? catalogo : montarCatalogo());

  return (
    <div className="fixed inset-0 z-50 flex items-end sm:items-center justify-center bg-slate-950/55 backdrop-blur-sm animate-in fade-in duration-200 p-0 sm:p-4">
      <div 
        role="dialog" aria-modal="true" aria-labelledby="acoes-rapidas-titulo"
        className="w-full sm:max-w-xl max-h-[calc(100dvh-0.5rem)] sm:max-h-[90dvh] overflow-y-auto overscroll-contain bg-white border-t sm:border border-slate-200 rounded-t-3xl sm:rounded-2xl px-4 pt-3 pb-[calc(1rem+env(safe-area-inset-bottom))] sm:p-5 shadow-2xl text-slate-900 animate-in slide-in-from-bottom-5 duration-300"
      >
        {/* Handle bar on mobile */}
        <div className="w-12 h-1.5 bg-slate-300 rounded-full mx-auto mb-4 sm:hidden"></div>

        <div className="flex items-center justify-between pb-3 border-b border-slate-200">
          <div>
            <span className="text-[10px] font-bold text-inovar-yellow uppercase tracking-wider block">
              INOVAR REFRIGERAÇÃO
            </span>
            <h3 id="acoes-rapidas-titulo" className="text-base font-bold text-slate-800">Menu principal</h3>
            <span className="text-[11px] text-slate-500">Ações e acessos rápidos</span>
          </div>
          <button
            onClick={onClose}
            className="p-2 text-slate-500 hover:text-slate-900 rounded-lg hover:bg-slate-100 transition-colors"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* O menu é a navegação principal no celular: todas as telas da equipe
            ficam visíveis antes das ações de cadastro, sem depender da barra inferior. */}
        {(onOpenInicio || onOpenServicos || onOpenAgenda || onOpenOrcamentos || onOpenClientes || onOpenFinanceiro) && (
          <section className="pt-3" aria-label="Navegação principal">
            <span className="text-[11px] font-bold text-slate-500 uppercase tracking-wider block mb-2">
              Navegar
            </span>
            <div className="grid grid-cols-3 gap-1.5">
              {onOpenInicio && <NavTile label="Central" icon={Home} active={activeTab === 'inicio'} onClick={() => { onClose(); onOpenInicio(); }} />}
              {onOpenAgenda && <NavTile label="Agenda" icon={Calendar} active={activeTab === 'agenda'} onClick={() => { onClose(); onOpenAgenda(); }} />}
              {onOpenOrcamentos && <NavTile label="Propostas" icon={Calculator} active={activeTab === 'orcamentos'} onClick={() => { onClose(); onOpenOrcamentos(); }} />}
              {onOpenServicos && <NavTile label="Serviços" icon={ClipboardList} active={activeTab === 'servicos'} onClick={() => { onClose(); onOpenServicos(); }} />}
              {onOpenClientes && <NavTile label="Clientes" icon={Users} active={activeTab === 'clientes'} onClick={() => { onClose(); onOpenClientes(); }} />}
              {onOpenFinanceiro && <NavTile label="Financeiro" icon={Wallet} active={activeTab === 'financeiro'} onClick={() => { onClose(); onOpenFinanceiro(); }} />}
            </div>
          </section>
        )}

        <div className="grid grid-cols-2 gap-2.5 py-4">
          {/* Iniciar Serviço */}
          <button
            onClick={() => {
              onClose();
              onOpenIniciarServico();
            }}
            className="min-h-36 p-3.5 bg-sky-50 hover:bg-sky-100 border border-sky-200 rounded-xl flex flex-col items-start gap-2 active:scale-[.98] transition-all text-left group"
          >
            <div className="w-9 h-9 rounded-lg bg-sky-500/20 text-sky-400 flex items-center justify-center border border-sky-500/30 group-hover:scale-105 transition-transform">
              <Wrench className="w-5 h-5" />
            </div>
            <div>
              <span className="font-bold text-sm text-slate-800 block">Ordem de Serviço</span>
              <span className="text-[11px] leading-snug text-slate-600 block">Checklist e execução</span>
            </div>
          </button>

          {/* Novo Orçamento */}
          <button
            onClick={() => {
              onClose();
              onOpenNovoOrcamento();
            }}
            className="min-h-36 p-3.5 bg-amber-50 hover:bg-amber-100 border border-amber-200 rounded-xl flex flex-col items-start gap-2 active:scale-[.98] transition-all text-left group"
          >
            <div className="w-9 h-9 rounded-lg bg-inovar-yellow/20 text-inovar-yellow flex items-center justify-center border border-inovar-yellow/30 group-hover:scale-105 transition-transform">
              <Calculator className="w-5 h-5" />
            </div>
            <div>
              <span className="font-bold text-sm text-slate-800 block">Nova proposta</span>
              <span className="text-[11px] leading-snug text-slate-600 block">Orçamento comercial</span>
            </div>
          </button>

          {/* Cadastrar Cliente */}
          <button
            onClick={() => {
              onClose();
              onOpenNovoCliente();
            }}
            className="min-h-36 p-3.5 bg-emerald-50 hover:bg-emerald-100 border border-emerald-200 rounded-xl flex flex-col items-start gap-2 active:scale-[.98] transition-all text-left group"
          >
            <div className="w-9 h-9 rounded-lg bg-emerald-500/20 text-emerald-400 flex items-center justify-center border border-emerald-500/30 group-hover:scale-105 transition-transform">
              <UserPlus className="w-5 h-5" />
            </div>
            <div>
              <span className="font-bold text-sm text-slate-800 block">Novo cliente</span>
              <span className="text-[11px] leading-snug text-slate-600 block">Cadastro sincronizado</span>
            </div>
          </button>

          {/* Agendar Retorno */}
          <button
            onClick={() => {
              onClose();
              onOpenNovoAgendamento();
            }}
            className="min-h-36 p-3.5 bg-violet-50 hover:bg-violet-100 border border-violet-200 rounded-xl flex flex-col items-start gap-2 active:scale-[.98] transition-all text-left group"
          >
            <div className="w-9 h-9 rounded-lg bg-purple-500/20 text-purple-400 flex items-center justify-center border border-purple-500/30 group-hover:scale-105 transition-transform">
              <Calendar className="w-5 h-5" />
            </div>
            <div>
              <span className="font-bold text-sm text-slate-800 block">Agendar atendimento</span>
              <span className="text-[11px] leading-snug text-slate-600 block">Retorno ou preventiva</span>
            </div>
          </button>
        </div>

        {/* Atalhos Diretos por Tipo de Serviço (segue o catálogo editável) */}
        <div className="pt-2 border-t border-slate-200">
          <span className="text-[11px] font-bold text-slate-400 uppercase tracking-wider block mb-2">
            Iniciar diretamente por serviço:
          </span>
          <div className="grid grid-cols-1 min-[380px]:grid-cols-2 gap-1.5">
            {atalhos.map((t) => {
              const IconComponent = t.card?.icon || Wrench;
              return (
                <button
                  key={t.key}
                  onClick={() => {
                    onClose();
                    onOpenIniciarServico((t.fixo || t.nome) as ServiceType);
                  }}
                  className={`min-h-11 px-2.5 py-1.5 border rounded-lg text-left text-xs font-semibold flex items-center gap-1.5 transition-colors ${
                    CHIP_CLASSES[t.card?.color || 'sky']
                  }`}
                >
                  <IconComponent className="w-3.5 h-3.5" />
                  <span className="truncate">{t.nome}</span>
                </button>
              );
            })}
          </div>
        </div>

      </div>
    </div>
  );
};

function NavTile({ label, icon: Icon, active, onClick }: { label: string; icon: React.ComponentType<{ className?: string }>; active: boolean; onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      aria-current={active ? 'page' : undefined}
      className={`min-h-14 px-2 py-2 rounded-xl border flex flex-col items-center justify-center gap-1 text-[11px] font-bold transition-colors ${
        active ? 'bg-sky-600 border-sky-600 text-white shadow-sm' : 'bg-white border-slate-200 text-slate-700 hover:bg-sky-50 hover:border-sky-200'
      }`}
    >
      <Icon className="w-4 h-4" />
      <span className="truncate max-w-full">{label}</span>
    </button>
  );
}
