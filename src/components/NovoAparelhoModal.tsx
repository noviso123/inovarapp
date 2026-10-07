import React, { useState } from 'react';
import { Client, Appliance } from '../types';
import { X, Wrench, Save } from 'lucide-react';

interface NovoAparelhoModalProps {
  client: Client;
  onClose: () => void;
  onSave: (appliance: Appliance) => void;
}

const BRANDS = ['LG', 'Gree', 'Midea', 'Samsung', 'Daikin', 'Carrier', 'Elgin', 'Fujitsu', 'Consul', 'Electrolux', 'Springer', 'TCL', 'Outra'];
const CAPACITIES = ['7.000', '9.000', '12.000', '18.000', '24.000', '30.000', '36.000', '48.000', '60.000'];

export const NovoAparelhoModal: React.FC<NovoAparelhoModalProps> = ({ client, onClose, onSave }) => {
  const [brand, setBrand] = useState('LG');
  const [capacityBtu, setCapacityBtu] = useState('12.000');
  const [type, setType] = useState<Appliance['type']>('Split Hi-Wall');
  const [room, setRoom] = useState('');
  const [gasType, setGasType] = useState<Appliance['gasType']>('R-410A');
  const [voltage, setVoltage] = useState<Appliance['voltage']>('220V');
  const [model, setModel] = useState('');
  const [notes, setNotes] = useState('');

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const newAppliance: Appliance = {
      id: 'a_' + Math.random().toString(36).substring(2, 9),
      clientId: client.id,
      brand,
      capacityBtu,
      type,
      room: room || 'Novo Ambiente',
      gasType,
      voltage,
      model,
      notes
    };
    onSave(newAppliance);
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-950/80 backdrop-blur-xs overflow-y-auto">
      <div className="bg-white text-slate-900 rounded-2xl w-full max-w-md shadow-2xl border border-slate-200 overflow-hidden my-6">
        <div className="bg-slate-900 p-4 text-white flex items-center justify-between">
          <div className="flex items-center gap-2">
            <div className="p-2 bg-sky-500/20 text-sky-400 rounded-xl">
              <Wrench className="w-5 h-5" />
            </div>
            <div>
              <h3 className="font-bold text-sm">Adicionar Aparelho para {client.name}</h3>
              <p className="text-[11px] text-slate-400">Cadastre outro ar-condicionado deste cliente</p>
            </div>
          </div>
          <button onClick={onClose} className="p-1.5 text-slate-400 hover:text-white rounded-lg">
            <X className="w-5 h-5" />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="p-5 space-y-3.5 text-xs">
          <div className="grid grid-cols-2 gap-2">
            <div>
              <label className="block font-semibold text-slate-700 mb-1">Marca</label>
              <select
                value={brand}
                onChange={e => setBrand(e.target.value)}
                className="w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs"
              >
                {BRANDS.map(b => (
                  <option key={b} value={b}>{b}</option>
                ))}
              </select>
            </div>

            <div>
              <label className="block font-semibold text-slate-700 mb-1">Capacidade</label>
              <select
                value={capacityBtu}
                onChange={e => setCapacityBtu(e.target.value)}
                className="w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs font-bold text-sky-700"
              >
                {CAPACITIES.map(c => (
                  <option key={c} value={c}>{c} BTUs</option>
                ))}
              </select>
            </div>
          </div>

          <div className="grid grid-cols-2 gap-2">
            <div>
              <label className="block font-semibold text-slate-700 mb-1">Tipo</label>
              <select
                value={type}
                onChange={e => setType(e.target.value as any)}
                className="w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs"
              >
                <option value="Split Hi-Wall">Split Hi-Wall</option>
                <option value="Inverter">Inverter</option>
                <option value="Cassete">Cassete</option>
                <option value="Piso Teto">Piso Teto</option>
                <option value="Janela">Janela</option>
              </select>
            </div>

            <div>
              <label className="block font-semibold text-slate-700 mb-1">Cômodo / Ambiente *</label>
              <input
                type="text"
                required
                placeholder="Ex: Quarto 2, Sacada, Suíte"
                value={room}
                onChange={e => setRoom(e.target.value)}
                className="w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs font-semibold"
              />
            </div>
          </div>

          <div className="grid grid-cols-2 gap-2">
            <div>
              <label className="block font-semibold text-slate-700 mb-1">Fluido Refrigerante</label>
              <select
                value={gasType}
                onChange={e => setGasType(e.target.value as any)}
                className="w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs"
              >
                <option value="R-410A">R-410A</option>
                <option value="R-32">R-32</option>
                <option value="R-22">R-22</option>
              </select>
            </div>

            <div>
              <label className="block font-semibold text-slate-700 mb-1">Tensão</label>
              <select
                value={voltage}
                onChange={e => setVoltage(e.target.value as any)}
                className="w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs"
              >
                <option value="220V">220V</option>
                <option value="110V">110V</option>
              </select>
            </div>
          </div>

          <div>
            <label className="block font-semibold text-slate-700 mb-1">Observações Técnicas</label>
            <textarea
              rows={2}
              value={notes}
              onChange={e => setNotes(e.target.value)}
              placeholder="Ex: Ponto de energia dedicado no quadro..."
              className="w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs"
            />
          </div>

          <div className="pt-2 flex gap-2">
            <button
              type="button"
              onClick={onClose}
              className="flex-1 py-2 px-4 bg-slate-100 text-slate-700 rounded-xl font-semibold"
            >
              Cancelar
            </button>
            <button
              type="submit"
              className="flex-1 py-2 px-4 bg-sky-600 hover:bg-sky-500 text-white rounded-xl font-bold flex items-center justify-center gap-1.5"
            >
              <Save className="w-4 h-4" />
              <span>Salvar Aparelho</span>
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
