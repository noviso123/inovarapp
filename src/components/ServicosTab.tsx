import React, { useState } from 'react';
import { ServiceType, Client, MaintenanceRecord, DadosTipoServico } from '../types';
import { TipoCatalogo, normalizarNomeServico } from '../services/catalogo';
import {
  Edit as EditIcon,
  Trash2,
  Wrench,
  Plus,
  CheckCircle2,
  ChevronRight,
  X,
  Save,
  Tag
} from 'lucide-react';

interface ServicosTabProps {
  services: any[];
  clients: Client[];
  catalogo: TipoCatalogo[];
  onAddTipo?: (dados: DadosTipoServico) => void;
  onEditTipo?: (key: string, dados: DadosTipoServico) => void;
  onDeleteTipo?: (key: string) => void;
  onEditServico?: (item: { id: string; tipo: string; data: string; valor: number; cliente: string; aparelho: string; observacoes?: string }) => void;
  onDeleteServico?: (serviceId: string) => void;
  onNovoServicoDireto?: () => void;
  onIniciarServico: (type: ServiceType) => void;
  onUpdateStatus?: (serviceId: string, status: string) => void;
  onFinalizar?: (serviceId: string) => void;
  maintenances: MaintenanceRecord[];
}

interface ModalTipoState {
  key: string;
  isFixo: boolean;
  nome: string;
  preco: string;
  desc: string;
  tempoMedio: string;
  garantiaPadrao: string;
  badge: string;
  itensTexto: string;
}

const estadoInicial = { key: '', isFixo: false, nome: '', preco: '250', desc: '', tempoMedio: '', garantiaPadrao: '', badge: '', itensTexto: '' };

export const ServicosTab: React.FC<ServicosTabProps> = ({
  onIniciarServico,
  maintenances,
  services,
  clients,
  catalogo,
  onAddTipo,
  onEditTipo,
  onDeleteTipo,
  onEditServico,
  onDeleteServico,
  onNovoServicoDireto
  , onUpdateStatus
  , onFinalizar
}) => {
  const [modalTipo, setModalTipo] = useState<ModalTipoState | null>(null);
  // O catálogo é a única origem para criar, editar e excluir tipos de
  // serviço. Ele fica disponível nesta tela em desktop e mobile; as cópias que
  // existiam em Configurações continuam removidas.
  const permitirGerenciarCatalogo = true;

  // Compute metrics
  const totalExecutados = maintenances.length;
  const faturamentoTotal = maintenances.reduce((acc, m) => acc + (m.price || 0), 0);

  const abrirNovoTipo = () => setModalTipo({ ...estadoInicial });

  const abrirEdicao = (t: TipoCatalogo) =>
    setModalTipo({
      key: t.key,
      isFixo: !!t.fixo,
      nome: t.nome,
      preco: String(t.preco || ''),
      desc: t.desc || t.card?.desc || '',
      tempoMedio: t.tempoMedio || t.card?.tempoMedio || '',
      garantiaPadrao: t.garantiaPadrao || t.card?.garantiaPadrao || '',
      badge: t.badge || t.card?.badge || '',
      itensTexto: (t.itens || t.card?.itensInclusos || []).join('\n')
    });

  const salvarTipo = () => {
    if (!modalTipo) return;
    const nome = modalTipo.nome.trim();
    if (!nome) return;
    const dados: DadosTipoServico = {
      nome,
      preco: Number(modalTipo.preco.replace(',', '.')) || 0,
      desc: modalTipo.desc.trim() || undefined,
      tempoMedio: modalTipo.tempoMedio.trim() || undefined,
      garantiaPadrao: modalTipo.garantiaPadrao.trim() || undefined,
      badge: modalTipo.badge.trim() || undefined,
      itens: modalTipo.itensTexto.split('\n').map((s) => s.trim()).filter(Boolean)
    };
    if (!modalTipo.key) onAddTipo?.(dados);
    else onEditTipo?.(modalTipo.key, dados);
    setModalTipo(null);
  };

  const excluirTipo = (t: TipoCatalogo) => {
    const msg = t.fixo
      ? 'Remover "' + t.nome + '" do catálogo?\n\nEle deixará de aparecer no checklist, nos orçamentos e nos agendamentos futuros.'
      : 'Remover "' + t.nome + '" do catálogo de serviços?';
    if (confirm(msg)) onDeleteTipo?.(t.key);
  };

  const osDoTipo = (t: TipoCatalogo) =>
    maintenances.filter(
      (m) => m.serviceType === t.nome || (t.fixo !== undefined && m.serviceType === t.fixo)
    ).length;

  const inputCls = 'w-full p-2.5 bg-white border border-slate-300 rounded-xl text-xs text-slate-800 placeholder-slate-400 focus:outline-none focus:border-sky-500';

  return (
    <div className="operational-screen space-y-4">
      {/* Top Banner de Serviços */}
      <div className="bg-white p-4 sm:p-5 rounded-2xl border border-slate-200 shadow-sm text-slate-900">
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
          <div>
            <div className="flex items-center gap-2 mb-1">
              <span className="px-2 py-0.5 rounded-full bg-inovar-yellow/20 text-inovar-yellow text-[10px] uppercase font-bold tracking-wider">
                Serviços de Campo
              </span>
              <span className="text-xs text-slate-400">• Inovar Refrigeração</span>
            </div>
            <h2 className="text-lg sm:text-xl font-black text-slate-900">
              Serviços de Climatização & Refrigeração
            </h2>
            <p className="text-xs text-slate-600 mt-1 max-w-xl leading-relaxed">
              Serviços padronizados da Inovar para orçamento, agendamento e Ordem de Serviço.
            </p>
          </div>

          <div className="flex items-center gap-3 shrink-0 flex-wrap">
            <div className="bg-slate-50 px-3.5 py-2 rounded-xl border border-slate-200 text-center">
              <span className="text-[10px] text-slate-600 block font-semibold">Ordens de Serviço concluídas</span>
              <span className="text-base font-extrabold text-sky-700">{totalExecutados}</span>
            </div>
            <div className="bg-slate-50 px-3.5 py-2 rounded-xl border border-slate-200 text-center">
              <span className="text-[10px] text-slate-600 block font-semibold">Faturamento</span>
              <span className="text-base font-extrabold text-emerald-700">
                R$ {faturamentoTotal.toLocaleString('pt-BR', { minimumFractionDigits: 2 })}
              </span>
            </div>
            {permitirGerenciarCatalogo && <div className="flex flex-col gap-1.5">
              <button
                onClick={abrirNovoTipo}
                className="px-3.5 py-2.5 bg-inovar-yellow hover:brightness-105 text-inovar-navy rounded-xl text-[11px] font-extrabold flex items-center justify-center gap-1.5 shadow-lg transition-all active:scale-95"
                title="Cadastrar um novo tipo de serviço no catálogo"
              >
                <Plus className="w-4 h-4 stroke-[3]" />
                <span>Novo Tipo de Serviço</span>
              </button>
            </div>}
          </div>
        </div>
      </div>

      {/* Grid com Todos os Serviços */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-3.5">
        {catalogo.map((t) => {
          const cardBase = t.card;
          const IconComponent = cardBase?.icon || Tag;
          const colorCls = cardBase?.accentColor || 'text-sky-400';
          // valores efetivos: edição do admin sobrepõe o padrão do tipo fixo
          const titulo = t.nome;
          const subtitulo = cardBase?.subtitle || 'Serviço personalizado da equipe';
          const desc = t.desc || cardBase?.desc || 'Tipo de serviço cadastrado pela equipe Inovar. Disponível no checklist de campo, orçamentos e agendamentos.';
          const tempo = t.tempoMedio || cardBase?.tempoMedio;
          const garantia = t.garantiaPadrao || cardBase?.garantiaPadrao;
          const badge = t.badge || cardBase?.badge;
          const itens = t.itens?.length ? t.itens : cardBase?.itensInclusos || [];
          const precoTexto = t.preco > 0
            ? 'R$ ' + t.preco.toLocaleString('pt-BR', { minimumFractionDigits: 2 })
            : cardBase?.valorMedioRef || 'Sob consulta';
          const countDone = osDoTipo(t);

          const acoesEdicao = permitirGerenciarCatalogo && (
            <div className="flex items-center gap-1 shrink-0">
              {badge && (
                <span className="px-2 py-0.5 rounded-md bg-slate-100 text-slate-700 text-[10px] font-bold border border-slate-200">
                  {badge}
                </span>
              )}
              <button
                onClick={() => abrirEdicao(t)}
                title="Editar este tipo de serviço (todos os campos)"
                className="min-w-10 min-h-10 p-2 text-slate-500 hover:text-sky-700 hover:bg-sky-50 rounded-lg transition-colors"
              >
                <EditIcon className="w-4 h-4" />
              </button>
              <button
                onClick={() => excluirTipo(t)}
                title="Excluir este tipo de serviço do catálogo"
                className="min-w-10 min-h-10 p-2 text-slate-500 hover:text-red-700 hover:bg-red-50 rounded-lg transition-colors"
              >
                <Trash2 className="w-4 h-4" />
              </button>
            </div>
          );

          return (
            <div
              key={t.key}
              className="rounded-2xl p-4 flex flex-col justify-between bg-white border border-slate-200 shadow-sm transition-all hover:border-sky-300 hover:shadow-md"
            >
              <div>
                <div className="flex items-start justify-between gap-2 mb-2.5">
                  <div className="flex items-center gap-2.5">
                    <div className={`p-2.5 rounded-xl bg-slate-100 border border-slate-200 ${colorCls}`}>
                      <IconComponent className="w-5 h-5" />
                    </div>
                    <div>
                      <h3 className="font-bold text-sm text-slate-900">{titulo}</h3>
                      <p className="text-[11px] text-slate-600">{subtitulo}</p>
                    </div>
                  </div>
                  {acoesEdicao}
                </div>

                <p className="text-xs text-slate-600 mb-3 leading-relaxed">{desc}</p>

                {itens.length > 0 && (
                  <div className="bg-slate-50 rounded-xl p-3 border border-slate-200 space-y-1.5 mb-3.5">
                    <span className="text-[10px] font-bold text-slate-700 uppercase tracking-wider block mb-1">
                      Procedimento Padrão:
                    </span>
                    {itens.map((item, idx) => (
                      <div key={idx} className="flex items-start gap-1.5 text-[11px] text-slate-600">
                        <CheckCircle2 className="w-3.5 h-3.5 text-sky-600 shrink-0 mt-0.5" />
                        <span>{item}</span>
                      </div>
                    ))}
                  </div>
                )}

                <div className="grid grid-cols-3 gap-2 text-[11px] py-2 border-t border-slate-200 text-slate-600">
                  <div>
                    <span className="block text-[10px] text-slate-500 font-semibold">TEMPO</span>
                    <span className="font-bold text-slate-800">{tempo || '—'}</span>
                  </div>
                  <div>
                    <span className="block text-[10px] text-slate-500 font-semibold">GARANTIA</span>
                    <span className="font-bold text-emerald-700">{garantia || '—'}</span>
                  </div>
                  <div>
                    <span className="block text-[10px] text-slate-500 font-semibold">PREÇO PADRÃO</span>
                    <span className="font-bold text-sky-700">{precoTexto}</span>
                  </div>
                </div>
              </div>

              <div className="pt-3 border-t border-slate-200 mt-2 flex flex-col sm:flex-row sm:items-center justify-between gap-2">
                <span className="text-[10px] text-slate-600 font-medium">
                  {countDone > 0 ? `${countDone} ordens realizadas` : 'Pronto para atendimento'}
                </span>
                <button
                  onClick={() => onIniciarServico((t.fixo || t.nome) as ServiceType)}
                  className="w-full sm:w-auto min-h-11 px-4 py-2 bg-sky-600 hover:bg-sky-500 text-white rounded-xl text-xs font-bold flex items-center justify-center gap-1.5 transition-all shadow-md active:scale-95 group"
                >
                  <Plus className="w-3.5 h-3.5" />
                  <span>Ordem de Serviço</span>
                  <ChevronRight className="w-3.5 h-3.5 group-hover:translate-x-0.5 transition-transform" />
                </button>
              </div>
            </div>
          );
        })}

      </div>

      {/* SERVIÇOS EM ANDAMENTO — pendentes e agendados, ordenados por prioridade */}
      <div className="bg-white rounded-2xl border border-slate-200 shadow-sm overflow-hidden mt-4">
        <div className="px-4 py-3 border-b border-slate-100 flex items-center justify-between flex-wrap gap-2">
          <div className="flex items-center gap-2">
            <div className="p-1.5 bg-slate-100 text-slate-700 rounded-lg">
              <Wrench className="w-4 h-4" />
            </div>
            <div>
              <span className="text-[11px] font-extrabold text-slate-700 uppercase tracking-wide block">Serviços em Andamento</span>
              <span className="text-[10px] text-slate-400">Pendentes e agendados — atrasado primeiro, depois o que caiu hoje</span>
            </div>
          </div>
          <button
            onClick={onNovoServicoDireto}
            className="px-3 py-1.5 bg-sky-600 hover:bg-sky-500 text-white rounded-lg text-[11px] font-bold flex items-center gap-1.5"
          >
            <Plus className="w-3.5 h-3.5" />
            Cadastrar Serviço
          </button>
        </div>

        {(() => {
          const hoje = new Date().toISOString().slice(0, 10);
          const emAndamento = (services || [])
            .filter((s: any) => s.status === 'PENDENTE' || s.status === 'AGENDADO' || s.status === 'EM_ANDAMENTO')
            .slice()
            .sort((a: any, b: any) => {
              const pri = (s: any) => {
                const d = (s.data_agendamento || '').slice(0, 10);
                if (d && d < hoje) return 0; // atrasado
                if (d === hoje) return 1; // caiu hoje
                if (d) return 2; // agendado futuro
                return 3; // sem data
              };
              const pa = pri(a);
              const pb = pri(b);
              if (pa !== pb) return pa - pb;
              const da = (a.data_agendamento || '9999-12-31').slice(0, 10);
              const db = (b.data_agendamento || '9999-12-31').slice(0, 10);
              return da.localeCompare(db);
            });

          if (emAndamento.length === 0) {
            return (
              <p className="p-6 text-center text-xs text-slate-400">
                Nenhum serviço em andamento. Os concluídos ficam no histórico de Ordens de Serviço.
              </p>
            );
          }
          return (
            <div className="divide-y divide-slate-100">
              {emAndamento.map((s: any) => {
                const nome = clients.find((cl) => cl.id === s.cliente_id)?.name || 'Cliente';
                const statusCor: Record<string, string> = {
                  AGENDADO: 'bg-blue-50 text-blue-700 border-blue-100',
                  PENDENTE: 'bg-amber-50 text-amber-700 border-amber-100',
                  EM_ANDAMENTO: 'bg-purple-50 text-purple-700 border-purple-100'
                };
                const data = (s.data_agendamento || '').slice(0, 10);
                const atrasado = data && data < hoje;
                const ehHoje = data === hoje;
                return (
                  <div key={s.id} className={`px-4 py-3 flex items-center justify-between gap-3 flex-wrap ${atrasado ? 'bg-red-50/60' : ''}`}>
                    <div className="min-w-0">
                      <div className="flex items-center gap-2 flex-wrap">
                        <span className="text-xs font-bold text-slate-800">{nome}</span>
                        {atrasado && (
                          <span className="px-2 py-0.5 rounded-full text-[10px] font-bold border bg-red-100 text-red-700 border-red-200">
                            ATRASADO
                          </span>
                        )}
                        {ehHoje && (
                          <span className="px-2 py-0.5 rounded-full text-[10px] font-bold border bg-inovar-yellow/30 text-inovar-navy border-inovar-yellow">
                            HOJE
                          </span>
                        )}
                        <span className={`px-2 py-0.5 rounded-full text-[10px] font-bold border ${statusCor[s.status] || 'bg-slate-100 text-slate-500 border-slate-200'}`}>
                          {s.status}
                        </span>
                      </div>
                      <p className="text-[11px] text-slate-500 mt-0.5">
                        {normalizarNomeServico(s.descricao || s.tipo || '').replace(/_/g, ' ')} • {data ? data.split('-').reverse().join('/') : 'sem data'} • R$ {Number(s.valor || 0).toFixed(2)}
                      </p>
                    </div>
                    <div className="flex items-center gap-1.5">
                      {s.status === 'AGENDADO' && onUpdateStatus && (
                        <button onClick={() => onUpdateStatus(s.id, 'EM_ANDAMENTO')} className="px-2.5 py-1.5 bg-purple-50 hover:bg-purple-100 text-purple-700 rounded-lg text-[10px] font-bold border border-purple-100">Iniciar serviço</button>
                      )}
                      {(s.status === 'PENDENTE' || s.status === 'EM_ANDAMENTO') && onUpdateStatus && (
                        <button onClick={() => (onFinalizar ? onFinalizar(s.id) : onUpdateStatus!(s.id, 'CONCLUIDO'))} title="Fim do serviço: abre o checklist e conclui esta mesma OS" className="px-2.5 py-1.5 bg-emerald-50 hover:bg-emerald-100 text-emerald-700 rounded-lg text-[10px] font-bold border border-emerald-100">Finalizar serviço</button>
                      )}
                      {s.status !== 'CONCLUIDO' && s.status !== 'CANCELADO' && onUpdateStatus && (
                        <button onClick={() => { if (confirm('Cancelar este serviço?')) onUpdateStatus(s.id, 'CANCELADO'); }} className="px-2.5 py-1.5 bg-red-50 hover:bg-red-100 text-red-700 rounded-lg text-[10px] font-bold border border-red-100">Cancelar</button>
                      )}
                      {onEditServico && (
                        <button
                          onClick={() => onEditServico({ id: s.id, tipo: normalizarNomeServico(s.descricao || s.tipo), data: (s.data_agendamento || '').slice(0, 10), valor: Number(s.valor || 0), cliente: nome, aparelho: '', observacoes: s.observacoes })}
                          title="Editar serviço"
                          className="p-1.5 text-slate-400 hover:text-sky-600 hover:bg-sky-50 rounded-lg transition-colors"
                        >
                          <EditIcon className="w-4 h-4" />
                        </button>
                      )}
                      {onDeleteServico && (
                        <button
                          onClick={() => { if (confirm('Excluir este serviço permanentemente do banco?')) onDeleteServico(s.id); }}
                          title="Excluir serviço"
                          className="p-1.5 text-slate-400 hover:text-red-600 hover:bg-red-50 rounded-lg transition-colors"
                        >
                          <Trash2 className="w-4 h-4" />
                        </button>
                      )}
                    </div>
                  </div>
                );
              })}
            </div>
          );
        })()}
      </div>

      {/* MODAL: cadastrar / editar tipo de serviço — TODOS os campos */}
      {permitirGerenciarCatalogo && modalTipo && (
        <div className="fixed inset-0 z-[70] flex items-center justify-center p-3 sm:p-4 bg-black/85 backdrop-blur-sm animate-in fade-in duration-200">
          <div className="bg-white border border-slate-200 rounded-2xl w-full max-w-md shadow-2xl text-slate-900 overflow-hidden flex flex-col max-h-[92vh]">
            <div className="p-4 bg-sky-50 border-b border-slate-200 flex items-center justify-between shrink-0">
              <div className="flex items-center gap-2.5">
                <div className="p-2 bg-sky-100 text-sky-700 rounded-xl border border-sky-200">
                  <Tag className="w-4 h-4" />
                </div>
                <div>
                  <span className="text-[10px] font-bold text-sky-700 uppercase tracking-wider block">
                    CATÁLOGO INOVAR
                  </span>
                  <h3 className="text-sm font-bold text-slate-900">
                    {modalTipo.key ? 'Editar Tipo de Serviço' : 'Cadastrar Tipo de Serviço'}
                  </h3>
                </div>
              </div>
              <button
                onClick={() => setModalTipo(null)}
                className="p-1.5 text-slate-500 hover:text-slate-900 rounded-lg hover:bg-white transition-colors"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            <form
              onSubmit={(e) => { e.preventDefault(); salvarTipo(); }}
              className="p-4 space-y-3 overflow-y-auto"
            >
              <div>
                <label className="block text-[11px] font-bold text-slate-700 uppercase tracking-wide mb-1">Nome do Serviço *</label>
                <input
                  type="text"
                  value={modalTipo.nome}
                  onChange={(e) => setModalTipo({ ...modalTipo, nome: e.target.value })}
                  placeholder="Ex.: Limpeza de Ar em Frost Free, Troca de Placa..."
                  className={inputCls}
                  autoFocus
                />
                {modalTipo.isFixo && (
                  <p className="text-[10px] text-amber-700 mt-1">
                    Tipo padrão da Inovar: renomear não altera OS já registradas, apenas os novos atendimentos.
                  </p>
                )}
              </div>

              <div className="grid grid-cols-3 gap-2">
                <div>
                  <label className="block text-[11px] font-bold text-slate-700 uppercase tracking-wide mb-1">Preço (R$)</label>
                  <input
                    type="number" min="0" step="10"
                    value={modalTipo.preco}
                    onChange={(e) => setModalTipo({ ...modalTipo, preco: e.target.value })}
                    className={inputCls}
                  />
                </div>
                <div>
                  <label className="block text-[11px] font-bold text-slate-700 uppercase tracking-wide mb-1">Tempo</label>
                  <input
                    type="text"
                    value={modalTipo.tempoMedio}
                    onChange={(e) => setModalTipo({ ...modalTipo, tempoMedio: e.target.value })}
                    placeholder="1h30"
                    className={inputCls}
                  />
                </div>
                <div>
                  <label className="block text-[11px] font-bold text-slate-700 uppercase tracking-wide mb-1">Garantia</label>
                  <input
                    type="text"
                    value={modalTipo.garantiaPadrao}
                    onChange={(e) => setModalTipo({ ...modalTipo, garantiaPadrao: e.target.value })}
                    placeholder="90 dias"
                    className={inputCls}
                  />
                </div>
              </div>

              <div>
                <label className="block text-[11px] font-bold text-slate-700 uppercase tracking-wide mb-1">Descrição / Subtítulo</label>
                <input
                  type="text"
                  value={modalTipo.desc}
                  onChange={(e) => setModalTipo({ ...modalTipo, desc: e.target.value })}
                  placeholder="O que este serviço inclui..."
                  className={inputCls}
                />
              </div>

              <div className="grid grid-cols-2 gap-2">
                <div>
                  <label className="block text-[11px] font-bold text-slate-700 uppercase tracking-wide mb-1">Etiqueta (badge)</label>
                  <input
                    type="text"
                    value={modalTipo.badge}
                    onChange={(e) => setModalTipo({ ...modalTipo, badge: e.target.value })}
                    placeholder="Ex.: Mais Pedido"
                    className={inputCls}
                  />
                </div>
                <div className="flex items-end">
                  <p className="text-[10px] text-slate-500 leading-snug">
                    O preço aqui é a sugestão usada em orçamentos, agendamentos e itens rápidos.
                  </p>
                </div>
              </div>

              <div>
                <label className="block text-[11px] font-bold text-slate-700 uppercase tracking-wide mb-1">
                  Procedimento Padrão (um item por linha)
                </label>
                <textarea
                  value={modalTipo.itensTexto}
                  onChange={(e) => setModalTipo({ ...modalTipo, itensTexto: e.target.value })}
                  rows={5}
                  placeholder={'Lavagem de filtros\nMedição de salto térmico\nAplicação de bactericida'}
                  className={inputCls + ' resize-y'}
                />
              </div>

              <div className="flex gap-2 pt-1">
                <button
                  type="button"
                  onClick={() => setModalTipo(null)}
                  className="flex-1 py-2.5 bg-slate-100 hover:bg-slate-200 text-slate-700 border border-slate-300 rounded-xl text-xs font-bold transition-colors"
                >
                  Cancelar
                </button>
                <button
                  type="submit"
                  disabled={!modalTipo.nome.trim()}
                  className="flex-1 py-2.5 bg-inovar-yellow hover:brightness-105 disabled:opacity-40 disabled:cursor-not-allowed text-inovar-navy rounded-xl text-xs font-extrabold flex items-center justify-center gap-1.5 transition-all active:scale-95"
                >
                  <Save className="w-4 h-4" />
                  <span>{modalTipo.key ? 'Salvar' : 'Cadastrar'}</span>
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
};
