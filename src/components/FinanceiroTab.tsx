import React, { useMemo, useState } from 'react';
import { Client, BudgetEstimate, TechnicianProfile } from '../types';
import {
  Wallet,
  TrendingUp,
  Clock,
  CheckCircle2,
  User,
  CalendarDays,
  AlertTriangle,
  Copy,
  ExternalLink,
  BadgeCheck,
  Eye
} from 'lucide-react';
import { format, parseISO } from 'date-fns';

interface FinanceiroTabProps {
  services: any[];
  budgets: BudgetEstimate[];
  clients: Client[];
  profile: TechnicianProfile;
  onMarcarRecebido?: (budget: BudgetEstimate) => void;
  onAbrirOrcamento?: (budget: BudgetEstimate) => void;
  onAbrirOS?: (serviceId: string) => void;
  onUpdateStatus?: (serviceId: string, status: string) => void;
  onFinalizar?: (serviceId: string) => void;
}

const brl = (v: number) => 'R$ ' + (Number(v) || 0).toLocaleString('pt-BR', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
const dataBR = (iso?: string) => {
  if (!iso) return '—';
  const soData = iso.slice(0, 10);
  return soData.includes('-') ? soData.split('-').reverse().join('/') : soData;
};

// FINANCEIRO DO TÉCNICO — recebido, a receber, atrasado, PIX e atalhos
// para marcar recebimento e abrir o orçamento/OS de cada valor.
export const FinanceiroTab: React.FC<FinanceiroTabProps> = ({
  services,
  budgets,
  clients,
  profile,
  onMarcarRecebido,
  onAbrirOrcamento,
  onAbrirOS,
  onUpdateStatus,
  onFinalizar
}) => {
  const hoje = format(new Date(), 'yyyy-MM-dd');
  const [mesRef, setMesRef] = useState(new Date().toISOString().slice(0, 7));
  const [verTudo, setVerTudo] = useState(false);
  const [pixCopiado, setPixCopiado] = useState(false);

  const clienteNome = (id: string) => clients.find((c) => c.id === id)?.name || 'Cliente';

  // Serviços executados (CONCLUIDO) — receita real.
  // Mês pela data de CONCLUSÃO (não da agenda): migration sem a coluna usa o marcador.
  const conclusaoDe = (s: any) =>
    s.data_conclusao
    || String(s.observacoes || '').match(/\[DATA_CONCLUSAO:(\d{4}-\d{2}-\d{2})\]/)?.[1]
    || '';
  const concluidos = useMemo(
    () => services.filter((s: any) => s.status === 'CONCLUIDO' && (Number(s.valor) || 0) > 0),
    [services]
  );
  const listaExecutados = verTudo
    ? concluidos
    : concluidos.filter((s: any) => (conclusaoDe(s) || s.data_agendamento || s.data_solicitacao || '').slice(0, 7) === mesRef);
  const recebidoMes = listaExecutados.reduce((s, x) => s + (Number(x.valor) || 0), 0);

  // A receber: serviços AGENDADOS/EM ANDAMENTO com valor + orçamentos aprovados ainda não pagos
  const agendados = services.filter((s: any) => (s.status === 'AGENDADO' || s.status === 'EM_ANDAMENTO') && (Number(s.valor) || 0) > 0);
  const atrasados = agendados.filter((s: any) => (s.data_agendamento || '') && (s.data_agendamento || '').slice(0, 10) < hoje);
  const aReceberAgendado = agendados.reduce((s, x) => s + (Number(x.valor) || 0), 0);
  // orçamento já convertido em OS concluída não é mais "a receber" (já entrou na receita)
  const orcamentosAReceber = budgets.filter((b) => {
    if (b.status !== 'aprovado' || b.pago) return false;
    const svc = b.serviceId ? services.find((x: any) => x.id === b.serviceId) : null;
    return !svc || svc.status !== 'CONCLUIDO';
  });
  const aReceberOrc = orcamentosAReceber.reduce((s, b) => s + (b.finalValue || 0), 0);
  const orcPagos = budgets.filter((b) => b.pago);

  const totalAReceber = aReceberAgendado + aReceberOrc;

  const porCliente = useMemo(() => {
    const map: Record<string, { total: number; itens: any[] }> = {};
    listaExecutados.forEach((s) => {
      const nome = clienteNome(s.cliente_id);
      map[nome] = map[nome] || { total: 0, itens: [] };
      map[nome].total += Number(s.valor) || 0;
      map[nome].itens.push(s);
    });
    return Object.entries(map).sort((a, b) => b[1].total - a[1].total);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [listaExecutados, clients]);

  const copiarPix = async () => {
    try {
      await navigator.clipboard.writeText(profile.pixKey || '');
      setPixCopiado(true);
      setTimeout(() => setPixCopiado(false), 2000);
    } catch { /* clipboard bloqueado */ }
  };

  const kpi = (label: string, valor: string, cor: string, icon: React.ComponentType<{ className?: string }>, destaque?: boolean) => {
    const Icon = icon;
    return (
      <div className={`rounded-2xl p-3 shadow-sm border ${destaque ? 'bg-emerald-50 border-emerald-200' : 'bg-white border-slate-200'}`}>
        <div className={`w-9 h-9 rounded-xl flex items-center justify-center mb-1.5 ${cor}`}>
          <Icon className="w-4 h-4" />
        </div>
        <span className={`block text-base font-extrabold leading-none ${destaque ? 'text-emerald-700' : 'text-slate-800'}`}>{valor}</span>
        <span className="block text-[10px] text-slate-500 font-semibold mt-0.5">{label}</span>
      </div>
    );
  };

  return (
    <div className="operational-screen space-y-4">
      {/* Header + seletor de mês + PIX */}
      <div className="bg-white rounded-2xl border border-slate-200 shadow-sm p-4 flex items-center justify-between gap-3 flex-wrap">
        <div className="flex items-center gap-2">
          <div className="p-2 bg-emerald-100 text-emerald-700 rounded-xl">
            <Wallet className="w-5 h-5" />
          </div>
          <div>
            <h3 className="font-bold text-sm text-slate-800">Financeiro</h3>
            <p className="text-[11px] text-slate-500">Tudo que entrou, o que falta receber e o que atrasou</p>
          </div>
        </div>
        <label className="flex items-center gap-2 text-xs font-semibold text-slate-600 flex-wrap">
          <CalendarDays className="w-4 h-4" />
          <input type="month" value={mesRef} onChange={(e) => setMesRef(e.target.value)} className="px-2 py-1.5 border border-slate-300 rounded-lg text-xs" />
          <button
            onClick={() => setVerTudo(!verTudo)}
            className={`px-2.5 py-1.5 rounded-lg text-[11px] font-bold border transition-colors ${verTudo ? 'bg-slate-800 text-white border-slate-800' : 'bg-white text-slate-600 border-slate-300'}`}
          >
            {verTudo ? 'Histórico completo ✓' : 'Ver tudo'}
          </button>
        </label>
      </div>

      {/* KPIs */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-2">
        {kpi(verTudo ? 'Recebido (histórico)' : 'Recebido em ' + mesRef.split('-').reverse().join('/'), brl(recebidoMes), 'bg-emerald-100 text-emerald-700', CheckCircle2, true)}
        {kpi('A receber (total)', brl(totalAReceber), 'bg-blue-100 text-blue-700', Clock)}
        {kpi('Atrasado (agendados vencidos)', brl(atrasados.reduce((s, x) => s + (Number(x.valor) || 0), 0)), 'bg-red-100 text-red-700', AlertTriangle)}
        {kpi('Orçamentos aprovados a faturar', brl(aReceberOrc), 'bg-amber-100 text-amber-700', TrendingUp)}
      </div>

      {/* Receber via PIX */}
      <div className="bg-emerald-50 rounded-2xl border border-emerald-200 p-4 flex items-center justify-between gap-3 flex-wrap text-slate-900">
        <div className="flex items-center gap-2.5">
          <div className="p-2 bg-emerald-500/20 text-emerald-400 rounded-xl border border-emerald-500/30">
            <BadgeCheck className="w-5 h-5" />
          </div>
          <div>
            <span className="text-[10px] font-bold text-emerald-400 uppercase tracking-wider block">Receber via PIX — {profile.businessName || 'Inovar Refrigeração'}</span>
            <span className="text-xs font-bold text-slate-800 break-all">{profile.pixKey || 'Configure sua chave em Configurações'}</span>
          </div>
        </div>
        <button
          onClick={copiarPix}
          disabled={!profile.pixKey}
          className="px-3.5 py-2 bg-emerald-500 hover:bg-emerald-400 disabled:opacity-40 text-slate-950 rounded-xl text-[11px] font-extrabold flex items-center gap-1.5 transition-all active:scale-95"
        >
          {pixCopiado ? <CheckCircle2 className="w-4 h-4" /> : <Copy className="w-4 h-4" />}
          <span>{pixCopiado ? 'Chave copiada!' : 'Copiar chave PIX'}</span>
        </button>
      </div>

      {/* Por cliente — serviços executados */}
      <div className="bg-white rounded-2xl border border-slate-200 shadow-sm overflow-hidden">
        <div className="px-4 py-3 border-b border-slate-100 flex items-center justify-between">
          <span className="text-[11px] font-extrabold text-slate-700 uppercase tracking-wide flex items-center gap-1.5">
            <CheckCircle2 className="w-3.5 h-3.5 text-emerald-600" /> Serviços executados {verTudo ? '(histórico completo)' : 'em ' + mesRef.split('-').reverse().join('/')}
          </span>
          <span className="text-xs font-bold text-emerald-700">{brl(recebidoMes)}</span>
        </div>
        {listaExecutados.length === 0 ? (
          <p className="p-6 text-center text-xs text-slate-400">Nenhum serviço executado neste período ainda.</p>
        ) : (
          <div className="divide-y divide-slate-100">
            {porCliente.map(([nome, info]) => (
              <div key={nome} className="px-4 py-3">
                <div className="flex items-center justify-between mb-1">
                  <span className="font-bold text-xs text-slate-800 flex items-center gap-1.5"><User className="w-3.5 h-3.5 text-slate-400" /> {nome}</span>
                  <span className="text-xs font-extrabold text-emerald-600">{brl(info.total)}</span>
                </div>
                <div className="space-y-1">
                  {info.itens.map((s: any, i: number) => (
                    <div key={i} className="flex justify-between items-center text-[11px] text-slate-500 pl-4 gap-2">
                      <span className="min-w-0 truncate">{(s.descricao || s.tipo || '').replace(/_/g, ' ')} • {dataBR(s.data_agendamento || s.data_solicitacao)}</span>
                      <span className="flex items-center gap-2 shrink-0">
                        <span className="font-semibold text-slate-600">{brl(Number(s.valor || 0))}</span>
                        {onAbrirOS && (
                          <button
                            onClick={() => onAbrirOS(s.id)}
                            title="Abrir OS deste serviço"
                            className="p-1 text-slate-400 hover:text-sky-600 hover:bg-sky-50 rounded transition-colors"
                          >
                            <ExternalLink className="w-3.5 h-3.5" />
                          </button>
                        )}
                      </span>
                    </div>
                  ))}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* A receber — agendados (inclui atrasados) */}
      <div className="bg-white rounded-2xl border border-slate-200 shadow-sm overflow-hidden">
        <div className="px-4 py-3 border-b border-slate-100 flex items-center justify-between flex-wrap gap-1">
          <span className="text-[11px] font-extrabold text-slate-700 uppercase tracking-wide flex items-center gap-1.5">
            <Clock className="w-3.5 h-3.5 text-amber-600" /> A receber — serviços agendados
          </span>
          <span className="text-xs font-bold text-amber-600">{brl(aReceberAgendado)}</span>
        </div>
        {agendados.length === 0 ? (
          <p className="p-5 text-center text-xs text-slate-400">Nenhum serviço agendado pendente de recebimento.</p>
        ) : (
          <div className="divide-y divide-slate-100">
            {agendados
              .slice()
              .sort((a: any, b: any) => (a.data_agendamento || '').localeCompare(b.data_agendamento || ''))
              .map((s: any) => {
                const atrasado = (s.data_agendamento || '').slice(0, 10) < hoje;
                return (
                  <div key={s.id} className="px-4 py-2.5 flex items-center justify-between gap-2 text-xs">
                    <div className="min-w-0">
                      <span className="text-slate-700 font-semibold">{clienteNome(s.cliente_id)}</span>
                      <span className="block text-[10px] text-slate-400 truncate">{(s.descricao || s.tipo || '').replace(/_/g, ' ')}</span>
                    </div>
                    <div className="flex items-center gap-2 shrink-0">
                      <span className={`text-[10px] font-bold px-1.5 py-0.5 rounded ${atrasado && s.status === 'AGENDADO' ? 'bg-red-50 text-red-600' : 'text-slate-500'}`}>
                        {dataBR(s.data_agendamento)}{atrasado && s.status === 'AGENDADO' ? ' • atrasado' : s.status === 'EM_ANDAMENTO' ? ' • em andamento' : ''}
                      </span>
                      <span className="font-bold text-slate-800">{brl(Number(s.valor || 0))}</span>
                      {/* Ações rápidas de status direto na linha */}
                      {(onUpdateStatus || onFinalizar) && (
                        <div className="flex items-center gap-1">
                          {s.status === 'AGENDADO' && onUpdateStatus && (
                            <button
                              onClick={() => onUpdateStatus(s.id, 'EM_ANDAMENTO')}
                              title="Iniciar execução agora"
                              className="px-2 py-1 bg-purple-50 hover:bg-purple-100 text-purple-700 rounded-lg text-[10px] font-bold border border-purple-100 transition-colors"
                            >
                              ▶ Iniciar
                            </button>
                          )}
                          {onFinalizar && (
                            <button
                              onClick={() => onFinalizar(s.id)}
                              title="Finalizar: checklist, valor e garantia nesta mesma OS"
                              className="px-2 py-1 bg-emerald-50 hover:bg-emerald-100 text-emerald-700 rounded-lg text-[10px] font-bold border border-emerald-100 transition-colors"
                            >
                              ✓ Finalizar
                            </button>
                          )}
                          {onUpdateStatus && (
                            <button
                              onClick={() => { if (confirm('Cancelar este serviço?')) onUpdateStatus(s.id, 'CANCELADO'); }}
                              title="Cancelar serviço"
                              className="px-2 py-1 bg-red-50 hover:bg-red-100 text-red-600 rounded-lg text-[10px] font-bold border border-red-100 transition-colors"
                            >
                              ✕
                            </button>
                          )}
                        </div>
                      )}
                    </div>
                  </div>
                );
              })}
          </div>
        )}
      </div>

      {/* Orçamentos aprovados — marcar recebido / abrir */}
      <div className="bg-white rounded-2xl border border-slate-200 shadow-sm overflow-hidden">
        <div className="px-4 py-3 border-b border-slate-100 flex items-center justify-between flex-wrap gap-1">
          <span className="text-[11px] font-extrabold text-slate-700 uppercase tracking-wide flex items-center gap-1.5">
            <TrendingUp className="w-3.5 h-3.5 text-sky-600" /> Orçamentos aprovados (a receber)
          </span>
          <span className="text-xs font-bold text-sky-700">{brl(aReceberOrc)}</span>
        </div>
        {orcamentosAReceber.length === 0 ? (
          <p className="p-5 text-center text-xs text-slate-400">Nenhum orçamento aprovado aguardando recebimento.</p>
        ) : (
          <div className="divide-y divide-slate-100">
            {orcamentosAReceber.map((b) => (
              <div key={b.id} className="px-4 py-2.5 flex items-center justify-between gap-2 text-xs flex-wrap">
                <div className="min-w-0">
                  <span className="font-semibold text-slate-700">{b.clientName}</span>
                  <span className="block text-[10px] text-slate-400">
                    {b.items?.[0]?.description || b.applianceDesc || 'Serviço'} • criado em {dataBR(b.date)}
                  </span>
                </div>
                <div className="flex items-center gap-1.5 shrink-0">
                  <span className="font-bold text-slate-800">{brl(b.finalValue)}</span>
                  {onAbrirOrcamento && (
                    <button
                      onClick={() => onAbrirOrcamento(b)}
                      title="Abrir orçamento"
                      className="p-1.5 text-slate-400 hover:text-sky-600 hover:bg-sky-50 rounded-lg transition-colors"
                    >
                      <Eye className="w-4 h-4" />
                    </button>
                  )}
                  {onMarcarRecebido && (
                    <button
                      onClick={() => { if (confirm('Confirmar recebimento de ' + brl(b.finalValue) + ' de ' + b.clientName + '?')) onMarcarRecebido(b); }}
                      title="Marcar como recebido"
                      className="px-2.5 py-1.5 bg-emerald-600 hover:bg-emerald-500 text-white rounded-lg text-[10px] font-extrabold flex items-center gap-1 transition-colors active:scale-95"
                    >
                      <BadgeCheck className="w-3.5 h-3.5" />
                      Recebido
                    </button>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Histórico de recebimentos (orçamentos pagos) */}
      {orcPagos.length > 0 && (
        <div className="bg-white rounded-2xl border border-slate-200 shadow-sm overflow-hidden">
          <div className="px-4 py-3 border-b border-slate-100 flex items-center justify-between">
            <span className="text-[11px] font-extrabold text-slate-700 uppercase tracking-wide flex items-center gap-1.5">
              <BadgeCheck className="w-3.5 h-3.5 text-emerald-600" /> Recebimentos registrados (orçamentos)
            </span>
            <span className="text-xs font-bold text-emerald-700">{brl(orcPagos.reduce((s, b) => s + (b.valorRecebido ?? b.finalValue), 0))}</span>
          </div>
          <div className="divide-y divide-slate-100">
            {orcPagos.map((b) => (
              <div key={b.id} className="px-4 py-2.5 flex items-center justify-between text-xs">
                <span className="text-slate-700 font-semibold">{b.clientName}</span>
                <span className="text-slate-500">{b.pagoEm ? dataBR(b.pagoEm) : '—'}</span>
                <span className="font-bold text-emerald-600">{brl(b.valorRecebido ?? b.finalValue)}</span>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
};
