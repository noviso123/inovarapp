import React from 'react';
import { Users, AlertTriangle, Clock, Calendar, Calculator, FileText, DollarSign, Search } from 'lucide-react';

export type AbraDestino = 'clientes' | 'proximos-retornos' | 'fila-chamados' | 'orcamentos' | 'fila-historico' | 'financeiro' | 'agenda' | 'retornos-atrasados' | 'retornos-semana' | 'retornos-sem-historico' | 'chamados-fila' | null;

interface StatsBarProps {
  stats: {
    totalClientes: number;
    atrasados: number;
    estaSemana: number;
    emBreve: number;
    semHistorico?: number;
    solicitacoesPendentes?: number;
    totalOrcamentos?: number;
  };
  totalOS: number;
  faturamento: number; // serviços concluídos + orçamentos aprovados
  onNavegar?: (destino: AbraDestino) => void; // dashboard clicável: leva à tela correspondente
}

// Barra de indicadores do painel técnico (desktop 8 colunas, mobile 2).
// Cada card leva à tela correspondente quando onNavegar é passado.
export const StatsBar: React.FC<StatsBarProps> = ({ stats, totalOS, faturamento, onNavegar }) => {
  const items: { label: string; valor: string; icon: any; cor: string; destino?: AbraDestino }[] = [
    {
      label: 'Clientes',
      valor: String(stats.totalClientes),
      icon: Users,
      cor: 'text-sky-700 bg-sky-100 border-sky-200',
      destino: 'clientes'
    },
    {
      label: 'Retornos atrasados',
      valor: String(stats.atrasados),
      icon: AlertTriangle,
      cor: stats.atrasados > 0 ? 'text-red-700 bg-red-100 border-red-200' : 'text-slate-600 bg-slate-100 border-slate-200',
      destino: 'retornos-atrasados'
    },
    {
      label: 'Vencendo esta semana',
      valor: String(stats.estaSemana),
      icon: Clock,
      cor: stats.estaSemana > 0 ? 'text-amber-700 bg-amber-100 border-amber-200' : 'text-slate-600 bg-slate-100 border-slate-200',
      destino: 'retornos-semana'
    },
    {
      label: 'Sem histórico',
      valor: String(stats.semHistorico || 0),
      icon: Search,
      cor: (stats.semHistorico || 0) > 0 ? 'text-violet-700 bg-violet-100 border-violet-200' : 'text-slate-600 bg-slate-100 border-slate-200',
      destino: 'retornos-sem-historico'
    },
    {
      label: 'Chamados abertos',
      valor: String(stats.solicitacoesPendentes || 0),
      icon: Calendar,
      cor: (stats.solicitacoesPendentes || 0) > 0 ? 'text-purple-700 bg-purple-100 border-purple-200' : 'text-slate-600 bg-slate-100 border-slate-200',
      destino: 'fila-chamados'
    },
    {
      label: 'Orçamentos',
      valor: String(stats.totalOrcamentos || 0),
      icon: Calculator,
      cor: 'text-amber-700 bg-amber-100 border-amber-200',
      destino: 'orcamentos'
    },
    {
      label: 'Ordens de serviço',
      valor: String(totalOS),
      icon: FileText,
      cor: 'text-emerald-700 bg-emerald-100 border-emerald-200',
      destino: 'fila-historico'
    },
    {
      label: 'Faturamento',
      valor: 'R$ ' + faturamento.toLocaleString('pt-BR', { maximumFractionDigits: 0 }),
      icon: DollarSign,
      cor: 'text-emerald-700 bg-emerald-100 border-emerald-200',
      destino: 'financeiro'
    }
  ];

  return (
    <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-8 gap-2">
      {items.map((it) => {
        const clicavel = !!onNavegar && !!it.destino;
        return (
          <div
            key={it.label}
            onClick={() => clicavel && onNavegar!(it.destino!)}
            role={clicavel ? 'button' : undefined}
            tabIndex={clicavel ? 0 : undefined}
            onKeyDown={(event) => { if (clicavel && (event.key === 'Enter' || event.key === ' ')) { event.preventDefault(); onNavegar!(it.destino!); } }}
            title={clicavel ? `Abrir ${it.label}` : undefined}
            className={`bg-white border border-slate-200 rounded-2xl p-3 shadow-sm flex items-center gap-2.5 ${clicavel ? 'cursor-pointer hover:border-sky-300 hover:bg-sky-50/50 active:scale-[0.98] transition-all focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-sky-400' : ''}`}
          >
            <div className={`w-9 h-9 rounded-xl border flex items-center justify-center shrink-0 ${it.cor}`}>
              <it.icon className="w-4 h-4" />
            </div>
            <div className="min-w-0">
              <span className="block text-xl font-black text-slate-950 leading-none truncate">{it.valor}</span>
              <span className="block text-[11px] text-slate-600 font-bold leading-tight mt-1">{it.label}</span>
            </div>
          </div>
        );
      })}
    </div>
  );
};
