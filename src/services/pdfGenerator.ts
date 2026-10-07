import type jsPDF from 'jspdf';
import { Client, Appliance, MaintenanceRecord, TechnicianProfile } from '../types';
import { addDays, differenceInCalendarDays } from 'date-fns';
import { imagensMarca } from './brandAssets';
import { StorageService } from './storage';

const safe = (v: unknown) => String(v ?? '').replace(/[\r\n]+/g, ' ').trim() || 'Não informado';
const datePt = (v: string) => { const [y,m,d] = String(v || '').slice(0,10).split('-'); return y && m && d ? `${d}/${m}/${y}` : 'Não informado'; };
const money = (v: number) => `R$ ${Number(v || 0).toLocaleString('pt-BR', { minimumFractionDigits: 2 })}`;
const dateValue = (v: string) => { const d = new Date(`${String(v || '').slice(0,10)}T00:00:00`); return Number.isNaN(d.getTime()) ? new Date() : d; };

export const PdfService = {
  async buildServiceOrderPdf(client: Client, appliance: Appliance, record: MaintenanceRecord, profile: TechnicianProfile): Promise<jsPDF> {
    const currentProfile = { ...StorageService.getProfile(), ...profile };
    // jsPDF e imagens da marca carregados sob demanda (não pesam mais no bundle)
    const { default: jsPDF } = await import('jspdf');
    const { INOVAR_HEADER, INOVAR_FOOTER_REFERENCE, INOVAR_SIGNATURE_GABRIEL } = await imagensMarca();
    const doc = new jsPDF({ orientation: 'portrait', unit: 'mm', format: 'a4' });
    const navy: [number,number,number] = [9,30,78]; const ink: [number,number,number] = [35,39,48]; const line: [number,number,number] = [130,135,145]; const W = 210; const PAGE_H = 297; const MARGIN = 8;
    doc.addImage(INOVAR_HEADER, 'PNG', 0, 0, W, 50.2, undefined, 'FAST');
    let y = 61;
    doc.setTextColor(...navy); doc.setFont('helvetica','bold'); doc.setFontSize(16); doc.text('ORDEM DE SERVIÇO E GARANTIA', W/2, y, { align:'center' }); y += 10;
    doc.setTextColor(...ink); doc.setFont('helvetica','normal'); doc.setFontSize(8.2);
    doc.text('Data de conclusão:', 17, y); doc.line(50,y+1,93,y+1); doc.text(datePt(record.completionDate || record.date), 55, y);
    doc.text('OS nº:', 130, y); doc.line(146,y+1,194,y+1); doc.text(safe(record.id).slice(-8).toUpperCase(), 151, y); y += 8;
    doc.text('Cliente:',17,y); doc.line(35,y+1,101,y+1); doc.text(safe(client.name),39,y);
    doc.text('CNPJ:',124,y); doc.line(140,y+1,194,y+1); doc.text(safe(currentProfile.cnpj),144,y); y += 8;
    doc.text('Endereço:',17,y); doc.line(40,y+1,194,y+1); doc.text([client.address,client.neighborhood,client.city].filter(Boolean).join(', ') || 'Não informado',43,y); y += 6;
    const equipamento = [appliance.brand, appliance.model].filter(Boolean).join(' ') || 'Não informado';
    doc.text('Equipamento:',17,y); doc.line(43,y+1,194,y+1); doc.text(`${equipamento} — ${safe(appliance.capacityBtu)} BTUs (${safe(appliance.room)})`,46,y); y += 12;

    doc.setFillColor(...navy); doc.roundedRect(16,y,178,10,1.5,1.5,'F'); doc.setTextColor(255,255,255); doc.setFont('helvetica','bold'); doc.setFontSize(8.7); doc.text('SERVIÇO REALIZADO',22,y+6.5); y += 10;
    doc.setFillColor(255,255,255); doc.setDrawColor(...line); doc.roundedRect(16,y,178,56,1.5,1.5,'FD');
    doc.setTextColor(...ink); doc.setFont('helvetica','bold'); doc.setFontSize(8.5); doc.text(safe(record.serviceType).toUpperCase(),22,y+8); doc.setFont('helvetica','normal'); doc.setFontSize(7.5); doc.text(`Data: ${datePt(record.completionDate || record.date)}`,145,y+8);
    const chk=record.checklist; const left:string[]=[]; const right:string[]=[];
    if (record.serviceType==='Instalação') { left.push(`[${chk?.suporteNivelado !== false?'X':' '}] Fixação e nivelamento`, `[${chk?.vacuoMicrons?'X':' '}] Vácuo: ${safe(chk?.vacuoMicrons)}`, `[${chk?.testeNitrogenio !== false?'X':' '}] Teste de estanqueidade`, `[${chk?.valvulasLiberadas !== false?'X':' '}] Válvulas liberadas`); right.push(`Salto térmico: ${safe(chk?.saltoTermicoDeltaT)}`,`Corrente: ${safe(chk?.correnteAmperes)}`,`Dreno testado`,`Tubulação isolada`); }
    else if (record.serviceType==='Manutenção Corretiva') { left.push(`Diagnóstico: ${safe(chk?.diagnosticoTecnico)}`,`Peças: ${safe(record.partsUsed || chk?.pecasSubstituidas)}`,`Componente: ${safe(chk?.capacitorTestado)}`,`Teste elétrico realizado`); right.push(`Corrente: ${safe(chk?.correnteAmperes)}`,`Pressão: ${safe(chk?.pressaoGasPSI)}`,`Salto térmico: ${safe(chk?.saltoTermicoDeltaT)}`,`Teste operacional realizado`); }
    else if (record.serviceType==='Recarga de Gás') { left.push(`Fluido adicionado: ${safe(chk?.gasAdicionadoGramas)}`,`Teste de vazamento realizado`,`Vácuo prévio realizado`,`Carga por balança`); right.push(`Pressão: ${safe(chk?.pressaoGasPSI)}`,`Salto térmico: ${safe(chk?.saltoTermicoDeltaT)}`,`Corrente: ${safe(chk?.correnteAmperes)}`,`Sistema testado`); }
    else { left.push(`[${chk?.filtrosLavados !== false?'X':' '}] Filtros lavados`, `[${chk?.serpentinaHigienizada !== false?'X':' '}] Serpentina limpa`, `[${chk?.turbinaLimpa !== false?'X':' '}] Turbina limpa`, `[${chk?.condensadoraLavada !== false?'X':' '}] Condensadora lavada`); right.push(`[${chk?.drenoDesobstruido !== false?'X':' '}] Dreno desobstruído`, `[${chk?.aplicacaoBactericida !== false?'X':' '}] Bactericida aplicado`, `Salto térmico: ${safe(chk?.saltoTermicoDeltaT)}`, `Pressão/corrente: ${safe(chk?.pressaoGasPSI)} / ${safe(chk?.correnteAmperes)}`); }
    doc.setFont('helvetica','normal'); doc.setFontSize(7); left.forEach((t,i)=>doc.text(t,22,y+18+i*8)); right.forEach((t,i)=>doc.text(t,110,y+18+i*8)); y += 64;

    doc.setFillColor(239,243,249); doc.setDrawColor(...line); doc.roundedRect(16,y,178,43,1.5,1.5,'FD'); doc.setTextColor(...navy); doc.setFont('helvetica','bold'); doc.setFontSize(8.5); doc.text('VALORES E GARANTIA',22,y+8); doc.setTextColor(...ink); doc.setFont('helvetica','normal'); doc.setFontSize(7.5);
    const labor=record.laborPrice ?? (record.partsPrice ? record.price-record.partsPrice : record.price); const parts=record.partsPrice ?? 0;
    doc.text(`Mão de obra: ${money(labor)}`,22,y+17); doc.text(parts>0?`Peças/materiais: ${money(parts)}${record.partsUsed ? ` (${record.partsUsed})`:''}`:'Materiais/insumos: Inclusos',105,y+17); doc.setFont('helvetica','bold'); doc.text(`Valor total: ${money(record.price)} — ${safe(record.paymentMethod)}`,22,y+26); doc.setFont('helvetica','normal'); doc.text(`PIX: ${safe(currentProfile.pixKey)} (${safe(currentProfile.pixType).toUpperCase()})`,105,y+26);
    const warrantyDays=Number(record.warrantyDays || currentProfile.defaultWarrantyDays || 0); const warrantyDate=addDays(dateValue(record.completionDate || record.date),warrantyDays); doc.setTextColor(16,120,80); doc.text(`Garantia: ${warrantyDays} dias — válida até ${warrantyDate.toLocaleDateString('pt-BR')}`,22,y+35); if (record.returnDate) { doc.setTextColor(80,90,105); doc.setFontSize(7); doc.text(`Próxima revisão: ${datePt(record.returnDate)}`,105,y+35); } y += 52;

    try {
      // A assinatura oficial é carregada junto com os ativos da marca. Não
      // depende mais de uma URL/configuração do aparelho que poderia falhar.
      const props = doc.getImageProperties(INOVAR_SIGNATURE_GABRIEL);
      const maxW = 62; const maxH = 16; const ratio = props.width / props.height;
      let sigW = maxW; let sigH = sigW / ratio;
      if (sigH > maxH) { sigH = maxH; sigW = sigH * ratio; }
      doc.addImage(INOVAR_SIGNATURE_GABRIEL, props.fileType || 'PNG', 55 - sigW / 2, y - 3 + (maxH - sigH) / 2, sigW, sigH);
    } catch { /* a geração continua mesmo se o navegador estiver offline */ }
    doc.setDrawColor(...line); doc.line(25,y+12,85,y+12); doc.line(125,y+12,185,y+12); doc.setTextColor(...ink); doc.setFont('helvetica','normal'); doc.setFontSize(7); doc.text('Assinatura do técnico',55,y+17,{align:'center'}); doc.text('Assinatura do cliente',155,y+17,{align:'center'}); doc.setFontSize(6.5); doc.text('Gabriel Nascimento',55,y+22,{align:'center'}); doc.text(safe(client.name),155,y+22,{align:'center'});
    doc.addImage(INOVAR_FOOTER_REFERENCE, 'PNG', MARGIN, PAGE_H - MARGIN - 35, W - (MARGIN * 2), 35, undefined, 'FAST');
    return doc;
  },
  async generateServiceOrderPdf(client: Client, appliance: Appliance, record: MaintenanceRecord, profile: TechnicianProfile): Promise<void> { const doc = await this.buildServiceOrderPdf(client,appliance,record,profile); doc.save(`OS_INOVAR_${safe(client.name).replace(/\s+/g,'_')}_${record.id}.pdf`); }
};
