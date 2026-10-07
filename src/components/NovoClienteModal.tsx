import React, { useState } from 'react';
import { Client, Appliance } from '../types';
import { X, UserPlus, Save, Wrench, Mail, KeyRound, Loader2 } from 'lucide-react';
import { buscarCep, mascaraCep } from '../services/cep';
import { PuxarContato } from './PuxarContato';

interface NovoClienteModalProps {
  onClose: () => void;
  onSave: (client: Client, acesso?: { email: string; senha: string }) => void;
}

const BRANDS = ['LG', 'Gree', 'Midea', 'Samsung', 'Daikin', 'Carrier', 'Elgin', 'Fujitsu', 'Consul', 'Electrolux', 'Springer', 'TCL', 'Outra'];
const CAPACITIES = ['7.000', '9.000', '12.000', '18.000', '24.000', '30.000', '36.000', '48.000', '60.000'];

export const NovoClienteModal: React.FC<NovoClienteModalProps> = ({ onClose, onSave }) => {
  // Client info
  const [name, setName] = useState('');
  const [phone, setPhone] = useState('');
  const [address, setAddress] = useState('');
  const [neighborhood, setNeighborhood] = useState('');
  const [city, setCity] = useState('');
  const [notes, setNotes] = useState('');
  const [cep, setCep] = useState('');
  const [buscandoCep, setBuscandoCep] = useState(false);

  // CEP preenche endereço/bairro/cidade automaticamente — editáveis depois
  const aplicarCep = async (v: string) => {
    const m = mascaraCep(v);
    setCep(m);
    if (m.replace(/\D/g, '').length !== 8) return;
    setBuscandoCep(true);
    const end = await buscarCep(m);
    setBuscandoCep(false);
    if (end) {
      if (end.rua) setAddress(end.rua);
      if (end.bairro) setNeighborhood(end.bairro);
      if (end.cidade) setCity(end.cidade);
    }
  };

  // Acesso do cliente (conta criada automaticamente se o e-mail for preenchido)
  const [email, setEmail] = useState('');
  const [senha, setSenha] = useState('');

  // Initial appliance
  const [brand, setBrand] = useState('LG');
  const [capacityBtu, setCapacityBtu] = useState('12.000');
  const [type, setType] = useState<Appliance['type']>('Split Hi-Wall');
  const [room, setRoom] = useState('Sala');
  const [gasType, setGasType] = useState<Appliance['gasType']>('R-410A');
  const [voltage, setVoltage] = useState<Appliance['voltage']>('220V');

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const clientId = 'c_' + Math.random().toString(36).substring(2, 9);
    const applianceId = 'a_' + Math.random().toString(36).substring(2, 9);

    const firstAppliance: Appliance = {
      id: applianceId,
      clientId: clientId,
      brand,
      capacityBtu,
      type,
      room,
      gasType,
      voltage
    };

    const newClient: Client = {
      id: clientId,
      name,
      phone,
      address,
      neighborhood,
      city,
      notes,
      createdAt: new Date().toISOString().split('T')[0],
      appliances: [firstAppliance]
    };

    onSave(newClient, email.trim() ? { email: email.trim(), senha: senha.trim() || '123456' } : undefined);
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-950/80 backdrop-blur-xs overflow-y-auto">
      <div className="bg-white text-slate-900 rounded-2xl w-full max-w-lg shadow-2xl border border-slate-200 overflow-hidden my-6">
        {/* Header */}
        <div className="bg-sky-50 border-b border-sky-100 p-4 text-slate-900 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <div className="p-2 bg-sky-100 text-sky-700 rounded-xl border border-sky-200">
              <UserPlus className="w-5 h-5" />
            </div>
            <div>
              <h3 className="font-bold text-sm text-slate-900">Cadastrar Novo Cliente</h3>
              <p className="text-[11px] text-slate-600">Insira os dados do cliente e do 1º aparelho</p>
            </div>
          </div>
          <button onClick={onClose} className="p-1.5 text-slate-500 hover:text-slate-900 hover:bg-white rounded-lg">
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Form */}
        <form onSubmit={handleSubmit} className="p-5 max-h-[70vh] overflow-y-auto space-y-4 text-xs">
          {/* Client Details */}
          <div>
            <h4 className="font-bold text-slate-800 uppercase tracking-wider mb-2 text-[11px]">
              <span>Dados do Cliente</span>
            </h4>

            {/* AUTO-PREENCHIMENTO: agenda nativa do celular + contatos do Google */}
            <PuxarContato
              onSelecionar={(c) => {
                if (c.nome) setName(c.nome);
                if (c.telefone) setPhone(c.telefone);
                if (c.email && !email.trim()) setEmail(c.email);
              }}
            />

            <div className="space-y-2.5">
              <div>
                <label className="block text-slate-700 font-semibold mb-1">Nome Completo *</label>
                <input
                  type="text"
                  required
                  placeholder="Ex: Roberto Alcantara"
                  value={name}
                  onChange={e => setName(e.target.value)}
                  className="w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs"
                />
              </div>

              <div>
                <label className="block text-slate-700 font-semibold mb-1">CEP (preenche o endereço automático)</label>
                <div className="relative">
                  <input
                    type="text"
                    placeholder="Ex: 29060-270"
                    inputMode="numeric"
                    value={cep}
                    onChange={e => aplicarCep(e.target.value)}
                    className="w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs pr-9"
                  />
                  {buscandoCep && <Loader2 className="w-4 h-4 text-sky-600 animate-spin absolute right-3 top-2" />}
                </div>
              </div>

              <div className="grid grid-cols-2 gap-2">
                <div>
                  <label className="block text-slate-700 font-semibold mb-1">WhatsApp / Telefone *</label>
                  <input
                    type="tel"
                    required
                    placeholder="11999998888"
                    value={phone}
                    onChange={e => setPhone(e.target.value)}
                    className="w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs"
                  />
                </div>

                <div>
                  <label className="block text-slate-700 font-semibold mb-1">Bairro</label>
                  <input
                    type="text"
                    placeholder="Ex: Moema"
                    value={neighborhood}
                    onChange={e => setNeighborhood(e.target.value)}
                    className="w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs"
                  />
                </div>
              </div>

              <div>
                <label className="block text-slate-700 font-semibold mb-1">Endereço (Rua, Número, Apto)</label>
                <input
                  type="text"
                  placeholder="Ex: Alameda dos Anapurus, 1200 Apt 81"
                  value={address}
                  onChange={e => setAddress(e.target.value)}
                  className="w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs"
                />
              </div>
            </div>
          </div>

          <hr className="border-slate-200" />

          {/* Acesso do cliente ao app */}
          <div>
            <h4 className="font-bold text-emerald-700 uppercase tracking-wider mb-2 text-[11px] flex items-center gap-1.5">
              <Mail className="w-3.5 h-3.5 text-emerald-600" />
              Acesso ao App (opcional — cria a conta do cliente)
            </h4>
            <div className="space-y-2.5">
              <div>
                <label className="block text-slate-700 font-semibold mb-1">E-mail do Cliente</label>
                <input
                  type="email"
                  placeholder="Ex: cliente@email.com"
                  value={email}
                  onChange={e => setEmail(e.target.value)}
                  className="w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs"
                />
                <p className="text-[10px] text-slate-500 mt-1">
                  Com um e-mail válido, a conta é criada na hora e o cliente já entra no app.
                </p>
              </div>

              {email.trim() !== '' && (
                <div>
                  <label className="block text-slate-700 font-semibold mb-1 flex items-center gap-1">
                    <KeyRound className="w-3 h-3" />
                    Senha Inicial
                  </label>
                  <input
                    type="text"
                    placeholder="Padrão: 123456"
                    value={senha}
                    onChange={e => setSenha(e.target.value)}
                    className="w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs"
                  />
                  <p className="text-[10px] text-slate-500 mt-1">
                    Em branco usa <b>123456</b>. O cliente pode trocar a senha no portal dele.
                  </p>
                </div>
              )}
            </div>
          </div>

          <hr className="border-slate-200" />

          {/* First Appliance Details */}
          <div>
            <h4 className="font-bold text-sky-800 uppercase tracking-wider mb-2 text-[11px] flex items-center gap-1.5">
              <Wrench className="w-3.5 h-3.5 text-sky-600" />
              1º Aparelho do Cliente
            </h4>

            <div className="space-y-2.5">
              <div className="grid grid-cols-2 gap-2">
                <div>
                  <label className="block text-slate-700 font-semibold mb-1">Marca</label>
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
                  <label className="block text-slate-700 font-semibold mb-1">Capacidade (BTUs)</label>
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
                  <label className="block text-slate-700 font-semibold mb-1">Tipo</label>
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
                    <option value="Multi Split">Multi Split</option>
                  </select>
                </div>

                <div>
                  <label className="block text-slate-700 font-semibold mb-1">Ambiente / Cômodo *</label>
                  <input
                    type="text"
                    required
                    placeholder="Ex: Sala, Quarto, Consultório"
                    value={room}
                    onChange={e => setRoom(e.target.value)}
                    className="w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs font-semibold"
                  />
                </div>
              </div>

              <div className="grid grid-cols-2 gap-2">
                <div>
                  <label className="block text-slate-700 font-semibold mb-1">Fluido Refrigerante</label>
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
                  <label className="block text-slate-700 font-semibold mb-1">Tensão</label>
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
            </div>
          </div>

          {/* Submit */}
          <div className="pt-2 flex gap-2">
            <button
              type="button"
              onClick={onClose}
              className="flex-1 py-2 px-4 bg-slate-100 hover:bg-slate-200 text-slate-700 rounded-xl font-semibold"
            >
              Cancelar
            </button>
            <button
              type="submit"
              className="flex-1 py-2 px-4 bg-sky-600 hover:bg-sky-500 text-white rounded-xl font-bold shadow-md shadow-sky-900/20 flex items-center justify-center gap-1.5"
            >
              <Save className="w-4 h-4" />
              <span>Salvar Cliente</span>
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
