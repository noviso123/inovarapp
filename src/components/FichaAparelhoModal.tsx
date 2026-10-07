import React, { useState, useEffect, useRef } from 'react';
import { Client, Appliance, MaintenanceRecord } from '../types';
import { supabase } from '../services/supabase';
import { X, Wrench, Zap, Wind, Calendar, Check, Save, Plus, Tag, Camera, Loader2, Trash2 } from 'lucide-react';
import { format, parseISO } from 'date-fns';

interface FichaAparelhoModalProps {
  client: Client;
  appliance: Appliance;
  maintenances: MaintenanceRecord[];
  onClose: () => void;
  onSaveAppliance: (updated: Appliance) => void;
  onOpenChecklist: (client: Client, appliance: Appliance) => void;
  onRegistrarHistorico?: (client: Client, appliance: Appliance) => void;
}

const BRANDS = ['LG', 'Gree', 'Midea', 'Samsung', 'Daikin', 'Carrier', 'Elgin', 'Fujitsu', 'Consul', 'Electrolux', 'Springer', 'TCL', 'Outra'];
const CAPACITIES = ['7.000', '9.000', '12.000', '18.000', '24.000', '30.000', '36.000', '48.000', '60.000'];
const TYPES = ['Split Hi-Wall', 'Inverter', 'Cassete', 'Piso Teto', 'Janela', 'Multi Split', 'Portátil'] as const;
const GAS_TYPES = ['R-410A', 'R-32', 'R-22', 'Outro'] as const;

export const FichaAparelhoModal: React.FC<FichaAparelhoModalProps> = ({
  client,
  appliance,
  maintenances,
  onClose,
  onSaveAppliance,
  onOpenChecklist,
  onRegistrarHistorico
}) => {
  const [formData, setFormData] = useState<Appliance>({ ...appliance });
  const [isEditing, setIsEditing] = useState(false);
  const [savedSuccess, setSavedSuccess] = useState(false);

  // Fotos do aparelho (salvas no storage privado, por aparelho)
  const fotoInputRef = useRef<HTMLInputElement>(null);
  const [fotos, setFotos] = useState<{ nome: string; caminho: string; url: string }[]>([]);
  const [enviandoFoto, setEnviandoFoto] = useState(false);
  const [erroFoto, setErroFoto] = useState('');

  const comprimir = (file: File): Promise<string> =>
    new Promise((resolve, reject) => {
      const fr = new FileReader();
      fr.onerror = () => reject(new Error('leitura'));
      fr.onload = () => {
        const bruto = fr.result as string;
        const img = new Image();
        img.onerror = () => resolve(bruto);
        img.onload = () => {
          try {
            const escala = Math.min(1, 1600 / Math.max(img.width, img.height));
            const c = document.createElement('canvas');
            c.width = Math.round(img.width * escala);
            c.height = Math.round(img.height * escala);
            const ctx = c.getContext('2d');
            if (!ctx) return resolve(bruto);
            ctx.drawImage(img, 0, 0, c.width, c.height);
            resolve(c.toDataURL('image/jpeg', 0.82));
          } catch { resolve(bruto); }
        };
        img.src = bruto;
      };
      fr.readAsDataURL(file);
    });

  const carregarFotos = async () => {
    try {
      const { data: sd } = await supabase.auth.getSession();
      const r = await fetch('/api/documentos', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${sd?.session?.access_token || ''}` },
        body: JSON.stringify({ acao: 'fotos-aparelho', aparelhoId: appliance.id })
      });
      const j = await r.json();
      setFotos(j.fotos || []);
    } catch { /* silencioso */ }
  };

  useEffect(() => { carregarFotos(); }, [appliance.id]);

  const anexarFotos = async (files: FileList | null) => {
    if (!files || !files.length) return;
    setEnviandoFoto(true);
    setErroFoto('');
    try {
      const { data: sd } = await supabase.auth.getSession();
      const auth = { 'Content-Type': 'application/json', Authorization: `Bearer ${sd?.session?.access_token || ''}` };
      let ok = 0;
      let falhas = 0;
      for (const file of Array.from(files).slice(0, 6)) {
        try {
          const base64 = await comprimir(file);
          const r = await fetch('/api/documentos', {
            method: 'POST',
            headers: auth,
            body: JSON.stringify({ acao: 'upload', nome: file.name, base64, pasta: 'aparelho', refId: appliance.id })
          });
          if (r.ok) ok++; else falhas++;
        } catch { falhas++; }
      }
      if (ok > 0) {
        await carregarFotos();
        setErroFoto(falhas > 0 ? `${ok} salva(s), ${falhas} falharam. Tente novamente.` : '');
      } else if (falhas > 0) {
        setErroFoto('Não foi possível enviar as fotos. Verifique a conexão e tente de novo.');
      }
    } finally {
      setEnviandoFoto(false);
      if (fotoInputRef.current) fotoInputRef.current.value = '';
    }
  };

  const excluirFotoAparelho = async (foto: { caminho: string }) => {
    if (!confirm('Excluir esta foto?')) return;
    try {
      const { data: sd } = await supabase.auth.getSession();
      await fetch('/api/documentos', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${sd?.session?.access_token || ''}` },
        body: JSON.stringify({ acao: 'excluirfoto', path: foto.caminho })
      });
      await carregarFotos();
    } catch { /* ignore */ }
  };

  const applianceMaintenances = maintenances.filter(m => m.applianceId === appliance.id);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    onSaveAppliance(formData);
    setIsEditing(false);
    setSavedSuccess(true);
    setTimeout(() => setSavedSuccess(false), 2500);
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-950/80 backdrop-blur-xs overflow-y-auto">
      <div className="bg-white text-slate-900 rounded-2xl w-full max-w-lg shadow-2xl border border-slate-200 overflow-hidden my-6">
        {/* Top Header */}
        <div className="bg-gradient-to-r from-slate-900 via-blue-950 to-slate-900 p-4 text-white flex items-center justify-between">
          <div className="flex items-center gap-2.5">
            <div className="p-2 bg-blue-500/20 border border-blue-400/30 rounded-xl text-sky-400">
              <Wrench className="w-5 h-5" />
            </div>
            <div>
              <span className="text-[10px] font-bold text-sky-400 uppercase tracking-wider block">
                ESPECIFICAÇÕES TÉCNICAS • HISTÓRICO DO APARELHO
              </span>
              <h3 className="text-base font-bold text-white">
                Ficha do Aparelho
              </h3>
            </div>
          </div>
          <button
            onClick={onClose}
            className="p-1.5 text-slate-400 hover:text-white rounded-lg hover:bg-slate-800 transition-colors"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Client identification */}
        <div className="bg-slate-50 px-5 py-3 border-b border-slate-200 flex items-center justify-between">
          <div>
            <span className="text-xs text-slate-500">Cliente proprietário:</span>
            <p className="font-bold text-slate-800 text-sm">{client.name}</p>
          </div>
          <div className="text-right">
            <span className="text-xs text-slate-500">Local de Instalação:</span>
            <p className="font-bold text-sky-700 text-sm">{formData.room}</p>
          </div>
        </div>

        {/* Body content */}
        <div className="p-5 max-h-[70vh] overflow-y-auto space-y-5">
          {savedSuccess && (
            <div className="p-3 bg-emerald-50 text-emerald-800 border border-emerald-200 rounded-xl text-xs font-semibold flex items-center gap-2">
              <Check className="w-4 h-4 text-emerald-600" />
              Ficha do aparelho atualizada com sucesso!
            </div>
          )}

          {!isEditing ? (
            /* Display View */
            <div className="space-y-4">
              <div className="grid grid-cols-2 gap-3">
                <div className="p-3 bg-slate-50 rounded-xl border border-slate-200">
                  <span className="text-[11px] font-semibold text-slate-400 block">Marca</span>
                  <span className="text-sm font-bold text-slate-800">{formData.brand}</span>
                </div>
                <div className="p-3 bg-slate-50 rounded-xl border border-slate-200">
                  <span className="text-[11px] font-semibold text-slate-400 block">Capacidade</span>
                  <span className="text-sm font-bold text-sky-700">{formData.capacityBtu} BTUs</span>
                </div>
                <div className="p-3 bg-slate-50 rounded-xl border border-slate-200">
                  <span className="text-[11px] font-semibold text-slate-400 block">Tipo do Aparelho</span>
                  <span className="text-sm font-bold text-slate-800">{formData.type}</span>
                </div>
                <div className="p-3 bg-slate-50 rounded-xl border border-slate-200">
                  <span className="text-[11px] font-semibold text-slate-400 block">Cômodo / Ambiente</span>
                  <span className="text-sm font-bold text-slate-800">{formData.room}</span>
                </div>
                <div className="p-3 bg-slate-50 rounded-xl border border-slate-200">
                  <span className="text-[11px] font-semibold text-slate-400 block">Fluido Refrigerante</span>
                  <span className="text-sm font-bold text-slate-800">{formData.gasType || 'R-410A'}</span>
                </div>
                <div className="p-3 bg-slate-50 rounded-xl border border-slate-200">
                  <span className="text-[11px] font-semibold text-slate-400 block">Tensão Elétrica</span>
                  <span className="text-sm font-bold text-slate-800">{formData.voltage || '220V'}</span>
                </div>
              </div>

              {formData.model && (
                <div className="p-3 bg-slate-50 rounded-xl border border-slate-200">
                  <span className="text-[11px] font-semibold text-slate-400 block">Modelo / Linha</span>
                  <span className="text-sm font-medium text-slate-800">{formData.model}</span>
                </div>
              )}

              {formData.notes && (
                <div className="p-3 bg-slate-50 rounded-xl border border-slate-200">
                  <span className="text-[11px] font-semibold text-slate-400 block">Observações Técnicas</span>
                  <span className="text-xs text-slate-700 whitespace-pre-line">{formData.notes}</span>
                </div>
              )}

              {/* Fotos do aparelho */}
              <div className="pt-2">
                <div className="flex items-center justify-between mb-2 gap-2 flex-wrap">
                  <h4 className="text-xs font-bold text-slate-700 uppercase tracking-wider flex items-center gap-1.5">
                    <Camera className="w-3.5 h-3.5 text-violet-600" />
                    Fotos do Aparelho (salvas automaticamente)
                  </h4>
                  <button
                    type="button"
                    onClick={() => fotoInputRef.current?.click()}
                    className="px-2.5 py-1.5 bg-violet-600 hover:bg-violet-500 text-white rounded-lg text-[10px] font-extrabold flex items-center gap-1 transition-colors active:scale-95"
                  >
                    {enviandoFoto ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Plus className="w-3.5 h-3.5" />}
                    Anexar fotos
                  </button>
                  <input ref={fotoInputRef} type="file" accept="image/*" multiple className="hidden" onChange={(e) => anexarFotos(e.target.files)} />
                </div>
                {erroFoto && (
                  <p className="text-[11px] text-amber-700 bg-amber-50 border border-amber-200 rounded-lg px-2 py-1.5 mb-2">{erroFoto}</p>
                )}
                {fotos.length === 0 && !enviandoFoto ? (
                  <p className="text-xs text-slate-400 italic p-3 bg-slate-50 rounded-xl">
                    Nenhuma foto anexada. Adicione fotos do equipamento (instalação, serial, local) — ficam salvas na ficha.
                  </p>
                ) : (
                  <div className="grid grid-cols-3 gap-2">
                    {fotos.map((f) => (
                      <div key={f.caminho} className="relative group">
                        <img src={f.url} alt="Foto do aparelho" className="w-full h-20 object-cover rounded-lg border border-slate-200" />
                        <button
                          type="button"
                          onClick={() => excluirFotoAparelho(f)}
                          title="Excluir foto"
                          className="absolute top-1 right-1 p-1 bg-white/90 text-red-500 rounded-md shadow"
                        >
                          <Trash2 className="w-3 h-3" />
                        </button>
                      </div>
                    ))}
                  </div>
                )}
              </div>

              {/* Maintenance History */}
              <div className="pt-2">
                <div className="flex items-center justify-between mb-2 gap-2 flex-wrap">
                  <h4 className="text-xs font-bold text-slate-700 uppercase tracking-wider flex items-center gap-1.5">
                    <Calendar className="w-3.5 h-3.5 text-blue-600" />
                    Histórico de Limpezas & Manutenções
                  </h4>
                  {onRegistrarHistorico && (
                    <button
                      type="button"
                      onClick={() => onRegistrarHistorico(client, appliance)}
                      className="px-2.5 py-1.5 bg-violet-600 hover:bg-violet-500 text-white rounded-lg text-[10px] font-extrabold flex items-center gap-1 transition-colors active:scale-95"
                      title="Registrar serviço feito antes de o app ser usado"
                    >
                      <Plus className="w-3.5 h-3.5" />
                      Histórico anterior
                    </button>
                  )}
                </div>

                {applianceMaintenances.length === 0 ? (
                  <p className="text-xs text-slate-400 italic p-3 bg-slate-50 rounded-xl">
                    Nenhuma manutenção registrada neste aparelho ainda. Use "Histórico anterior" para cadastrar serviços já feitos e começar o controle do ciclo.
                  </p>
                ) : (
                  <div className="space-y-2">
                    {applianceMaintenances.map(m => (
                      <div
                        key={m.id}
                        className="p-3 bg-slate-50 rounded-xl border border-slate-200 text-xs flex items-center justify-between"
                      >
                        <div>
                          <p className="font-bold text-slate-800">{m.serviceType}</p>
                          <p className="text-[11px] text-slate-500">
                            Realizado em {m.date ? format(parseISO(m.date), 'dd/MM/yyyy') : '—'} • Retorno {m.returnDate ? format(parseISO(m.returnDate), 'dd/MM/yyyy') : '—'}
                          </p>
                        </div>
                        <span className="font-extrabold text-emerald-700 bg-emerald-50 px-2 py-1 rounded border border-emerald-200">
                          R$ {m.price.toFixed(2)}
                        </span>
                      </div>
                    ))}
                  </div>
                )}
              </div>

              {/* Action Buttons */}
              <div className="flex gap-2 pt-2">
                <button
                  type="button"
                  onClick={() => setIsEditing(true)}
                  className="flex-1 py-2.5 px-4 bg-slate-100 hover:bg-slate-200 text-slate-800 rounded-xl font-semibold text-xs transition-colors"
                >
                  Editar Ficha Técnica
                </button>
                <button
                  type="button"
                  onClick={() => {
                    onClose();
                    onOpenChecklist(client, appliance);
                  }}
                  className="flex-1 py-2.5 px-4 bg-blue-600 hover:bg-blue-500 text-white rounded-xl font-bold text-xs shadow-md shadow-blue-900/20 transition-all flex items-center justify-center gap-1.5"
                >
                  <Wind className="w-4 h-4" />
                  <span>Ordem de Serviço</span>
                </button>
              </div>
            </div>
          ) : (
            /* Edit Form */
            <form onSubmit={handleSubmit} className="space-y-3">
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs font-semibold text-slate-700 mb-1">Marca</label>
                  <select
                    value={formData.brand}
                    onChange={e => setFormData({ ...formData, brand: e.target.value })}
                    className="w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs"
                  >
                    {BRANDS.map(b => (
                      <option key={b} value={b}>{b}</option>
                    ))}
                  </select>
                </div>

                <div>
                  <label className="block text-xs font-semibold text-slate-700 mb-1">Capacidade (BTUs)</label>
                  <select
                    value={formData.capacityBtu}
                    onChange={e => setFormData({ ...formData, capacityBtu: e.target.value })}
                    className="w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs"
                  >
                    {CAPACITIES.map(c => (
                      <option key={c} value={c}>{c} BTUs</option>
                    ))}
                  </select>
                </div>
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs font-semibold text-slate-700 mb-1">Tipo</label>
                  <select
                    value={formData.type}
                    onChange={e => setFormData({ ...formData, type: e.target.value as any })}
                    className="w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs"
                  >
                    {TYPES.map(t => (
                      <option key={t} value={t}>{t}</option>
                    ))}
                  </select>
                </div>

                <div>
                  <label className="block text-xs font-semibold text-slate-700 mb-1">Ambiente / Cômodo</label>
                  <input
                    type="text"
                    required
                    placeholder="Ex: Sala, Quarto Casal..."
                    value={formData.room}
                    onChange={e => setFormData({ ...formData, room: e.target.value })}
                    className="w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs"
                  />
                </div>
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs font-semibold text-slate-700 mb-1">Fluido Refrigerante (Gás)</label>
                  <select
                    value={formData.gasType || 'R-410A'}
                    onChange={e => setFormData({ ...formData, gasType: e.target.value as any })}
                    className="w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs"
                  >
                    {GAS_TYPES.map(g => (
                      <option key={g} value={g}>{g}</option>
                    ))}
                  </select>
                </div>

                <div>
                  <label className="block text-xs font-semibold text-slate-700 mb-1">Tensão</label>
                  <select
                    value={formData.voltage || '220V'}
                    onChange={e => setFormData({ ...formData, voltage: e.target.value as any })}
                    className="w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs"
                  >
                    <option value="220V">220V</option>
                    <option value="110V">110V</option>
                    <option value="Bivolt">Bivolt</option>
                  </select>
                </div>
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-700 mb-1">Modelo / Linha (Opcional)</label>
                <input
                  type="text"
                  placeholder="Ex: Dual Inverter Voice, Eco Garden..."
                  value={formData.model || ''}
                  onChange={e => setFormData({ ...formData, model: e.target.value })}
                  className="w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs"
                />
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-700 mb-1">Observações Técnicas</label>
                <textarea
                  rows={3}
                  placeholder="Acesso ao telhado, tubulação embutida, dreno, etc."
                  value={formData.notes || ''}
                  onChange={e => setFormData({ ...formData, notes: e.target.value })}
                  className="w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs"
                />
              </div>

              <div className="flex gap-2 pt-2">
                <button
                  type="button"
                  onClick={() => setIsEditing(false)}
                  className="flex-1 py-2 px-4 bg-slate-100 hover:bg-slate-200 text-slate-700 rounded-lg text-xs font-semibold"
                >
                  Cancelar
                </button>
                <button
                  type="submit"
                  className="flex-1 py-2 px-4 bg-blue-600 hover:bg-blue-500 text-white rounded-lg text-xs font-bold flex items-center justify-center gap-1.5"
                >
                  <Save className="w-4 h-4" />
                  <span>Salvar Alterações</span>
                </button>
              </div>
            </form>
          )}
        </div>
      </div>
    </div>
  );
};
