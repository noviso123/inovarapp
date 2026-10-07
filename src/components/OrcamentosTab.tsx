import React, { useState } from 'react';
import { BudgetEstimate, Client, TechnicianProfile } from '../types';
import { BudgetService } from '../services/budgetService';
import { WhatsAppService } from '../services/whatsapp';
import {
  FileText,
  Plus,
  Search,
  Clock,
  CheckCircle2,
  XCircle,
  Share2,
  DollarSign,
  ChevronRight,
  Filter
} from 'lucide-react';
import { format, parseISO } from 'date-fns';

interface OrcamentosTabProps {
  budgets: BudgetEstimate[];
  clients: Client[];
  profile: TechnicianProfile;
  onOpenNovoOrcamento: () => void;
  onSelectBudget: (budget: BudgetEstimate) => void;
  onUpdateStatusOrc?: (id: string, status: 'aprovado' | 'recusado') => void;
  onMarcarRecebido?: (budget: BudgetEstimate) => void;
}

export const OrcamentosTab: React.FC<OrcamentosTabProps> = ({
  budgets,
  clients,
  profile,
  onOpenNovoOrcamento,
  onSelectBudget,
  onUpdateStatusOrc,
  onMarcarRecebido
}) => {
  const [searchTerm, setSearchTerm] = useState('');
  const [mesRef, setMesRef] = useState(new Date().toISOString().slice(0, 7));
  const [filterStatus, setFilterStatus] = useState<'todos' | 'pendente' | 'aprovado' | 'recusado'>('todos');

  const filteredBudgets = budgets.filter((b) => {
    const matchesSearch =
      b.clientName.toLowerCase().includes(searchTerm.toLowerCase()) ||
      b.clientPhone.includes(searchTerm) ||
      (b.applianceDesc && b.applianceDesc.toLowerCase().includes(searchTerm.toLowerCase()));

    const matchesStatus = filterStatus === 'todos' || b.status === filterStatus;
    return matchesSearch && matchesStatus;
  });

  // Metrics
  const totalOrcados = budgets.length;
  const totalPendentes = budgets.filter((b) => b.status === 'pendente').length;
  const totalAprovados = budgets.filter((b) => b.status === 'aprovado').length;
  const valorTotalAprovado = budgets
    .filter((b) => b.status === 'aprovado')
    .reduce((acc, b) => acc + (b.finalValue || 0), 0);

  return (
    <div className="operational-screen space-y-4">
      {/* Top Banner de Orçamentos */}
      <div className="bg-white p-4 sm:p-5 rounded-2xl border border-slate-200 shadow-sm text-slate-900">
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
          <div>
            <div className="flex items-center gap-2 mb-1">
              <span className="px-2 py-0.5 rounded-full bg-inovar-yellow/20 text-inovar-yellow text-[10px] uppercase font-bold tracking-wider">
                Propostas & Orçamentos
              </span>
              <span className="text-xs text-slate-500">• Inovar Refrigeração</span>
            </div>
            <h2 className="text-lg sm:text-xl font-black text-slate-800">
              Propostas comerciais
            </h2>
            <p className="text-xs text-slate-600 mt-1 max-w-xl">
              Crie, envie, acompanhe e converta propostas em ordens de serviço.
            </p>
          </div>

          <button
            onClick={onOpenNovoOrcamento}
            className="px-4 py-2.5 bg-inovar-yellow hover:brightness-105 active:scale-95 text-inovar-navy rounded-xl text-xs font-bold shadow-lg flex items-center justify-center gap-2 transition-all shrink-0"
          >
            <Plus className="w-4 h-4" />
            <span>Criar Novo Orçamento</span>
          </button>
        </div>

        {/* Metrics Grid */}
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-2.5 mt-4 pt-4 border-t border-slate-200 text-xs">
          <div className="bg-slate-50 p-2.5 rounded-xl border border-slate-200">
            <span className="text-[10px] text-slate-500 block font-semibold">Propostas emitidas</span>
            <span className="text-base font-extrabold text-slate-800">{totalOrcados} propostas</span>
          </div>

          <div className="bg-amber-50 p-2.5 rounded-xl border border-amber-100">
            <span className="text-[10px] text-amber-400 block font-semibold">Aguardando Aprovação</span>
            <span className="text-base font-extrabold text-amber-400">{totalPendentes} pendentes</span>
          </div>

          <div className="bg-emerald-50 p-2.5 rounded-xl border border-emerald-100">
            <span className="text-[10px] text-emerald-400 block font-semibold">Orçamentos Aprovados</span>
            <span className="text-base font-extrabold text-emerald-400">{totalAprovados} fechados</span>
          </div>

          <div className="bg-sky-50 p-2.5 rounded-xl border border-sky-100">
            <span className="text-[10px] text-sky-400 block font-semibold">Faturamento Aprovado</span>
            <span className="text-base font-extrabold text-sky-400">
              R$ {valorTotalAprovado.toLocaleString('pt-BR', { minimumFractionDigits: 2 })}
            </span>
          </div>
        </div>
      </div>

      {/* FINANCEIRO DO MÊS */}
      <div className="bg-emerald-50 rounded-2xl border border-emerald-100 p-4 space-y-3">
        <div className="flex items-center justify-between gap-2 flex-wrap">
          <span className="text-[11px] font-extrabold text-emerald-300 uppercase tracking-wider">💰 Financeiro do Mês</span>
          <input
            type="month"
            value={mesRef}
            onChange={(e) => setMesRef(e.target.value)}
            className="px-2 py-1 bg-white border border-emerald-200 rounded-lg text-xs text-slate-700"
          />
        </div>
        {(() => {
          const doMes = budgets.filter((b) => (b.pagoEm || '').slice(0, 7) === mesRef);
          const recebido = doMes.reduce((s, b) => s + (b.finalValue || 0), 0);
          const aReceber = budgets.filter((b) => b.status === 'aprovado' && !b.pago).reduce((s, b) => s + (b.finalValue || 0), 0);
          const emitidoMes = budgets.filter((b) => (b.date || '').slice(0, 7) === mesRef).reduce((s, b) => s + (b.finalValue || 0), 0);
          const porCliente: Record<string, number> = {};
          doMes.forEach((b) => { porCliente[b.clientName] = (porCliente[b.clientName] || 0) + (b.finalValue || 0); });
          return (
            <div className="grid grid-cols-3 gap-2">
              <div className="bg-white p-2.5 rounded-xl border border-emerald-100">
                <span className="text-[10px] text-emerald-400 block font-semibold">Recebido no mês</span>
                <span className="text-base font-extrabold text-emerald-400">R$ {recebido.toFixed(2)}</span>
              </div>
              <div className="bg-white p-2.5 rounded-xl border border-emerald-100">
                <span className="text-[10px] text-amber-400 block font-semibold">A receber (aprovados)</span>
                <span className="text-base font-extrabold text-amber-400">R$ {aReceber.toFixed(2)}</span>
              </div>
              <div className="bg-white p-2.5 rounded-xl border border-emerald-100">
                <span className="text-[10px] text-sky-400 block font-semibold">Emitido no mês</span>
                <span className="text-base font-extrabold text-sky-400">R$ {emitidoMes.toFixed(2)}</span>
              </div>
              {Object.keys(porCliente).length > 0 && (
                <div className="col-span-3 pt-1 space-y-1">
                  <span className="text-[10px] text-slate-400 font-semibold uppercase">Recebido por cliente:</span>
                  {Object.entries(porCliente).map(([nome, valor]) => (
                    <div key={nome} className="flex justify-between text-[11px] text-slate-700 bg-white rounded-lg px-2.5 py-1">
                      <span>{nome}</span>
                      <span className="font-bold text-emerald-400">R$ {valor.toFixed(2)}</span>
                    </div>
                  ))}
                </div>
              )}
            </div>
          );
        })()}
      </div>

      {/* Filter and Search Bar */}
      <div className="flex flex-col sm:flex-row gap-2 items-center justify-between">
        <div className="relative w-full sm:w-80">
          <Search className="w-4 h-4 text-slate-400 absolute left-3 top-1/2 -translate-y-1/2" />
          <input
            type="text"
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
            placeholder="Buscar por cliente, aparelho ou telefone..."
            className="w-full pl-9 pr-3 py-2 bg-white border border-slate-300 rounded-xl text-xs text-slate-800 placeholder-slate-400 focus:outline-none focus:border-sky-500"
          />
        </div>

        <div className="flex items-center gap-1.5 w-full sm:w-auto overflow-x-auto text-xs scrollbar-none">
          <button
            onClick={() => setFilterStatus('todos')}
            className={`px-3 py-1.5 rounded-lg font-semibold whitespace-nowrap transition-colors ${
              filterStatus === 'todos'
                ? 'bg-sky-600 text-white shadow-sm'
                : 'bg-white text-slate-600 hover:text-slate-900 border border-slate-300'
            }`}
          >
            Todos ({budgets.length})
          </button>
          <button
            onClick={() => setFilterStatus('pendente')}
            className={`px-3 py-1.5 rounded-lg font-semibold whitespace-nowrap transition-colors ${
              filterStatus === 'pendente'
                ? 'bg-amber-600 text-white shadow-sm'
                : 'bg-white text-slate-600 hover:text-slate-900 border border-slate-300'
            }`}
          >
            Pendentes ({totalPendentes})
          </button>
          <button
            onClick={() => setFilterStatus('aprovado')}
            className={`px-3 py-1.5 rounded-lg font-semibold whitespace-nowrap transition-colors ${
              filterStatus === 'aprovado'
                ? 'bg-emerald-600 text-white shadow-sm'
                : 'bg-white text-slate-600 hover:text-slate-900 border border-slate-300'
            }`}
          >
            Aprovados ({totalAprovados})
          </button>
        </div>
      </div>

      {/* List of Budgets */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-3 items-start">
        {filteredBudgets.length === 0 ? (
          <div className="bg-white rounded-2xl border border-slate-200 p-8 text-center text-slate-500 text-xs">
            Nenhum orçamento encontrado com os filtros selecionados. Clique em "Criar Novo Orçamento" para iniciar!
          </div>
        ) : (
          filteredBudgets.map((b) => (
            <div
              key={b.id}
              onClick={() => onSelectBudget(b)}
              className="bg-white hover:bg-sky-50 border border-slate-200 hover:border-sky-300 rounded-2xl p-4 cursor-pointer transition-all shadow-sm group"
            >
              <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
                <div className="space-y-1">
                  <div className="flex items-center gap-2">
                    <span className="font-bold text-sm text-slate-800 group-hover:text-sky-700 transition-colors">
                      {b.clientName}
                    </span>
                    <span
                      className={`text-[10px] px-2 py-0.2 rounded-full font-bold uppercase ${
                        b.status === 'aprovado'
                          ? 'bg-emerald-500/20 text-emerald-400 border border-emerald-500/30'
                          : b.status === 'recusado'
                          ? 'bg-red-500/20 text-red-400 border border-red-500/30'
                          : 'bg-amber-500/20 text-amber-400 border border-amber-500/30'
                      }`}
                    >
                      {b.status}
                    </span>
                    <span className="text-[10px] text-slate-500 font-mono">
                      #{b.id.toUpperCase().slice(0, 8)}
                    </span>
                  </div>

                  <p className="text-xs text-slate-400">
                    {b.applianceDesc || 'Ar-Condicionado'} • {b.items.length} item(ns)
                  </p>

                  <div className="flex flex-wrap items-center gap-2 text-[11px] text-slate-400 pt-0.5">
                    <span>Emitido: {format(new Date(b.date), 'dd/MM/yyyy')}</span>
                    <span>•</span>
                    <span>Validade: {format(new Date(b.validUntil), 'dd/MM/yyyy')}</span>
                    <span>•</span>
                    <span>Garantia: {b.warrantyTerms}</span>
                  </div>
                </div>

                <div className="flex sm:flex-col items-center sm:items-end justify-between sm:justify-center border-t sm:border-t-0 border-slate-800 pt-2 sm:pt-0">
                  <div className="text-left sm:text-right">
                    <span className="text-[10px] text-slate-400 block">Valor Total</span>
                    <span className="text-base font-extrabold text-emerald-400">
                      R$ {b.finalValue.toFixed(2)}
                    </span>
                  </div>

                  <div className="flex items-center gap-1.5 mt-1 flex-wrap">
                    {/* Ações rápidas de status direto no card */}
                    {b.status === 'pendente' && onUpdateStatusOrc && (
                      <>
                        <button
                          onClick={(e) => { e.stopPropagation(); onUpdateStatusOrc(b.id, 'aprovado'); }}
                          title="Marcar como aprovado"
                          className="px-2 py-1 bg-emerald-500/20 hover:bg-emerald-500/40 text-emerald-300 rounded-lg text-[10px] font-bold border border-emerald-500/30 transition-colors"
                        >
                          ✓ Aprovar
                        </button>
                        <button
                          onClick={(e) => { e.stopPropagation(); if (confirm('Recusar este orçamento?')) onUpdateStatusOrc(b.id, 'recusado'); }}
                          title="Marcar como recusado"
                          className="px-2 py-1 bg-red-500/20 hover:bg-red-500/40 text-red-300 rounded-lg text-[10px] font-bold border border-red-500/30 transition-colors"
                        >
                          ✕ Recusar
                        </button>
                      </>
                    )}
                    {b.status === 'aprovado' && !b.pago && onMarcarRecebido && (
                      <button
                        onClick={(e) => { e.stopPropagation(); if (confirm('Confirmar recebimento de R$ ' + b.finalValue.toFixed(2) + ' de ' + b.clientName + '?')) onMarcarRecebido(b); }}
                        title="Marcar valor como recebido"
                        className="px-2 py-1 bg-emerald-600 hover:bg-emerald-500 text-white rounded-lg text-[10px] font-extrabold transition-colors active:scale-95"
                      >
                        💰 Recebido
                      </button>
                    )}
                    <button
                      onClick={(e) => {
                        e.stopPropagation();
                        const msg = BudgetService.generateWhatsAppBudget(b, profile);
                        WhatsAppService.openWhatsApp(b.clientPhone, msg);
                      }}
                      title="Enviar no WhatsApp"
                      className="p-1.5 bg-emerald-600/20 hover:bg-emerald-600/40 text-emerald-400 rounded-lg border border-emerald-500/30 transition-colors"
                    >
                      <Share2 className="w-3.5 h-3.5" />
                    </button>
                    <span className="text-xs font-semibold text-sky-400 flex items-center gap-0.5 group-hover:translate-x-0.5 transition-transform">
                      Ver Proposta <ChevronRight className="w-3.5 h-3.5" />
                    </span>
                  </div>
                </div>
              </div>
            </div>
          ))
        )}
      </div>
    </div>
  );
};
