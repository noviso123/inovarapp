import React, { useState } from 'react';
import { TechnicianProfile } from '../types';
import { MODELOS_MENSAGENS, ModeloMensagem, aplicarPlaceholders } from '../services/mensagensWhats';
import { X, MessageCircle, Save, RotateCcw, ChevronRight, CheckCircle2 } from 'lucide-react';

interface MensagensWhatsModalProps {
  profile: TechnicianProfile;
  onClose: () => void;
  onSalvar: (mensagens: Record<string, string>) => void;
  onSalvarAjustes?: (campos: Record<string, unknown>) => void;
}

const EXEMPLO: Record<string, string> = {
  cliente: 'Maria',
  empresa: 'Inovar Refrigeração',
  email: 'maria@email.com',
  senha: '123456',
  numero: '2026-0001',
  os: '#A1B2C3',
  servico: 'Limpeza de Ar',
  data: '10/09/2026',
  hora: '09:00',
  valor: 'R$ 250,00',
  garantia: '90 dias',
  equipamento: 'LG 12.000 BTUs (Sala)',
  data_ultima: '10/03/2026',
  situacao: 'está em atraso há 5 dias',
  meses: '6',
  status: 'CONCLUÍDO',
  app: 'https://inovarapp.vercel.app'
};

// Central de Mensagens do WhatsApp — somente ADMIN. Mensagem por mensagem,
// com placeholders, pré-visualização e padrões restauráveis. Salvo no banco.
export const MensagensWhatsModal: React.FC<MensagensWhatsModalProps> = ({ profile, onClose, onSalvar, onSalvarAjustes }) => {
  const salvas = profile.mensagensWhats || {};
  const [editando, setEditando] = useState<ModeloMensagem | null>(null);
  const [rascunho, setRascunho] = useState('');
  const [salvando, setSalvando] = useState(false);
  const [intervalo, setIntervalo] = useState(String(profile.lembrete_intervalo_dias ?? 7));

  const personalizadas = MODELOS_MENSAGENS.filter((m) => salvas[m.chave] && salvas[m.chave].trim() && salvas[m.chave] !== m.padrao).length;

  const abrirEditor = (m: ModeloMensagem) => {
    setEditando(m);
    setRascunho(salvas[m.chave] || m.padrao);
  };

  const salvarAtual = () => {
    if (!editando) return;
    setSalvando(true);
    const nova = { ...(profile.mensagensWhats || {}) };
    if (rascunho.trim() && rascunho !== editando.padrao) nova[editando.chave] = rascunho;
    else delete nova[editando.chave]; // voltou ao padrão: não guarda cópia
    onSalvar(nova);
    setTimeout(() => {
      setSalvando(false);
      setEditando(null);
    }, 400);
  };

  const restaurarPadrao = () => {
    if (!editando) return;
    const nova = { ...(profile.mensagensWhats || {}) };
    delete nova[editando.chave];
    onSalvar(nova);
    setRascunho(editando.padrao);
  };

  return (
    <div className="fixed inset-0 z-[75] flex items-center justify-center p-2 sm:p-4 bg-black/90 backdrop-blur-sm animate-in fade-in duration-200">
      <div className="bg-slate-900 border border-slate-700 rounded-2xl w-full max-w-lg shadow-2xl text-slate-100 overflow-hidden flex flex-col max-h-[94vh]">
        {/* Header */}
        <div className="p-4 bg-gradient-to-r from-emerald-950 via-slate-900 to-slate-950 border-b border-slate-800 flex items-center justify-between shrink-0">
          <div className="flex items-center gap-2.5">
            <div className="p-2 bg-emerald-500/20 text-emerald-400 rounded-xl border border-emerald-500/30">
              <MessageCircle className="w-5 h-5" />
            </div>
            <div>
              <span className="text-[10px] font-bold text-emerald-400 uppercase tracking-wider block">
                SOMENTE ADMIN • SALVO NO BANCO
              </span>
              <h3 className="text-sm font-bold text-white">Central de Mensagens do WhatsApp</h3>
              <p className="text-[10px] text-slate-400">{personalizadas} mensagem(s) personalizada(s) de {MODELOS_MENSAGENS.length}</p>
            </div>
          </div>
          <button onClick={onClose} className="p-1.5 text-slate-400 hover:text-white rounded-lg hover:bg-slate-800 transition-colors">
            <X className="w-5 h-5" />
          </button>
        </div>

        {!editando ? (
          <div className="p-4 pb-0">
            <div className="p-3 bg-slate-800/70 border border-slate-700 rounded-xl flex items-center justify-between gap-3">
              <span className="text-[11px] text-slate-300 leading-snug">
                ⏰ Lembrete de ciclo vencido: reenviar <b className="text-white">a cada</b>
              </span>
              <div className="flex items-center gap-2 shrink-0">
                <input
                  type="number" min="1" max="180"
                  value={intervalo}
                  onChange={(e) => setIntervalo(e.target.value)}
                  className="w-16 p-1.5 bg-slate-900 border border-slate-600 rounded-lg text-xs text-white text-center"
                />
                <span className="text-[11px] text-slate-400">dias</span>
                <button
                  onClick={() => onSalvarAjustes?.({ lembrete_intervalo_dias: Math.max(1, Number(intervalo) || 7) })}
                  className="px-2.5 py-1.5 bg-emerald-600 hover:bg-emerald-500 text-white rounded-lg text-[10px] font-extrabold"
                >
                  Salvar
                </button>
              </div>
            </div>
          </div>
        ) : null}

        {!editando ? (
          /* Lista de mensagens */
          <div className="overflow-y-auto divide-y divide-slate-800/80">
            {MODELOS_MENSAGENS.map((m) => {
              const personalizada = !!(salvas[m.chave] && salvas[m.chave].trim() && salvas[m.chave] !== m.padrao);
              return (
                <button
                  key={m.chave}
                  onClick={() => abrirEditor(m)}
                  className="w-full text-left px-4 py-3 hover:bg-slate-800/60 transition-colors flex items-center justify-between gap-3"
                >
                  <div className="min-w-0">
                    <div className="flex items-center gap-2 flex-wrap">
                      <span className="text-xs font-bold text-white">{m.titulo}</span>
                      {personalizada && (
                        <span className="px-1.5 py-0.5 rounded bg-amber-500/15 border border-amber-500/40 text-amber-300 text-[9px] font-extrabold">
                          PERSONALIZADA
                        </span>
                      )}
                    </div>
                    <p className="text-[11px] text-slate-400 mt-0.5">{m.quando}</p>
                  </div>
                  <ChevronRight className="w-4 h-4 text-slate-500 shrink-0" />
                </button>
              );
            })}
          </div>
        ) : (
          /* Editor da mensagem */
          <div className="p-4 space-y-3 overflow-y-auto">
            <button onClick={() => setEditando(null)} className="text-xs text-sky-400 hover:text-sky-300 font-semibold">
              ← Todas as mensagens
            </button>
            <div>
              <h4 className="text-sm font-extrabold text-white">{editando.titulo}</h4>
              <p className="text-[11px] text-slate-400">{editando.quando}</p>
            </div>

            <div>
              <span className="text-[10px] font-bold text-slate-400 uppercase tracking-wider block mb-1.5">Variáveis (toque para adicionar):</span>
              <div className="flex flex-wrap gap-1.5">
                {editando.placeholders.map((p) => (
                  <button
                    key={p}
                    onClick={() => setRascunho(rascunho + ' ' + p)}
                    className="px-2 py-1 bg-slate-800 hover:bg-slate-700 border border-slate-600 rounded-lg text-[10px] font-bold text-sky-300 transition-colors"
                  >
                    {p}
                  </button>
                ))}
              </div>
            </div>

            <div>
              <label className="text-[10px] font-bold text-slate-400 uppercase tracking-wider block mb-1.5">Texto da mensagem:</label>
              <textarea
                value={rascunho}
                onChange={(e) => setRascunho(e.target.value)}
                rows={10}
                className="w-full p-3 bg-slate-800/90 border border-slate-700 rounded-xl text-xs text-white font-mono leading-relaxed focus:outline-none focus:border-emerald-500 resize-y"
              />
            </div>

            <div>
              <span className="text-[10px] font-bold text-slate-400 uppercase tracking-wider block mb-1.5">Pré-visualização (exemplo):</span>
              <div className="bg-emerald-950/40 border border-emerald-500/30 rounded-xl p-3 text-[11px] text-slate-200 whitespace-pre-wrap leading-relaxed">
                {aplicarPlaceholders(rascunho, EXEMPLO) || '(vazia)'}
              </div>
            </div>

            <div className="flex gap-2 pt-1">
              <button
                onClick={restaurarPadrao}
                className="flex-1 py-2.5 bg-slate-800 hover:bg-slate-700 text-slate-200 border border-slate-700 rounded-xl text-xs font-bold flex items-center justify-center gap-1.5 transition-colors"
              >
                <RotateCcw className="w-4 h-4" />
                Restaurar padrão
              </button>
              <button
                onClick={salvarAtual}
                disabled={salvando}
                className="flex-1 py-2.5 bg-emerald-600 hover:bg-emerald-500 disabled:opacity-50 text-white rounded-xl text-xs font-extrabold flex items-center justify-center gap-1.5 transition-all active:scale-95"
              >
                {salvando ? <CheckCircle2 className="w-4 h-4" /> : <Save className="w-4 h-4" />}
                <span>Salvar no banco</span>
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
};
