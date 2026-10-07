import type jsPDF from 'jspdf';
import { BudgetEstimate, TechnicianProfile } from '../types';
import { differenceInCalendarDays } from 'date-fns';
import { imagensMarca } from './brandAssets';
import { StorageService } from './storage';

const money = (value: number) => `R$ ${Number(value || 0).toLocaleString('pt-BR', { minimumFractionDigits: 2 })}`;
const clean = (value: unknown) => String(value ?? '').replace(/[\r\n]+/g, ' ').trim();
const datePt = (value: string) => { const [y, m, d] = String(value || '').slice(0, 10).split('-'); return y && m && d ? `${d}/${m}/${y}` : ''; };

export const BudgetPdfService = {
  async buildBudgetPdf(budget: BudgetEstimate, profile: TechnicianProfile): Promise<jsPDF> {
    // perfil sincronizado (prop) tem prioridade sobre o snapshot local
    const currentProfile = { ...StorageService.getProfile(), ...profile };
    // jsPDF e imagens da marca carregados sob demanda (não pesam mais no bundle)
    const { default: jsPDF } = await import('jspdf');
    const { INOVAR_HEADER, INOVAR_FOOTER_REFERENCE, INOVAR_SIGNATURE_GABRIEL } = await imagensMarca();
    const doc = new jsPDF({ orientation: 'portrait', unit: 'mm', format: 'a4' });
    const navy: [number, number, number] = [9, 30, 78];
    const ink: [number, number, number] = [35, 39, 48];
    const line: [number, number, number] = [130, 135, 145];
    const W = 210; const PAGE_H = 297; const MARGIN = 8;
    doc.addImage(INOVAR_HEADER, 'PNG', 0, 0, W, 50.2, undefined, 'FAST');
    let y = 61;
    doc.setTextColor(...navy); doc.setFont('helvetica', 'bold'); doc.setFontSize(17); doc.text('ORÇAMENTO DE MANUTENÇÃO', W / 2, y, { align: 'center' }); y += 10;
    doc.setTextColor(...ink); doc.setFont('helvetica', 'normal'); doc.setFontSize(8.2);
    const endereco = clean(currentProfile.address) || 'Não informado';
    const cnpj = clean(currentProfile.cnpj) || 'Não informado';
    const numero = clean(budget.numero) || 'Não informado';
    doc.text('Data:', 17, y); doc.line(33, y + 1, 72, y + 1); doc.text(datePt(budget.date), 38, y);
    doc.text('Orçamento nº:', 124, y); doc.line(159, y + 1, 194, y + 1); doc.text(numero, 164, y); y += 8;
    doc.text('Cliente:', 17, y); doc.line(35, y + 1, 101, y + 1); doc.text(clean(budget.clientName), 39, y);
    doc.text('CNPJ:', 124, y); doc.line(140, y + 1, 194, y + 1); doc.text(cnpj, 144, y); y += 8;
    doc.text('Endereço:', 17, y); doc.line(40, y + 1, 194, y + 1); doc.text(endereco, 43, y); y += 6;
    if (clean(budget.equipmentName)) { doc.setFont('helvetica', 'bold'); doc.text('Equipamento:', 17, y); doc.setFont('helvetica', 'normal'); doc.text(clean(budget.equipmentName), 43, y); y += 6; }
    y += 6;

    doc.setFillColor(...navy); doc.roundedRect(16, y, 178, 10, 1.5, 1.5, 'F'); doc.setTextColor(255, 255, 255); doc.setFont('helvetica', 'bold'); doc.setFontSize(8.7); doc.text('ITENS DO ORÇAMENTO', 22, y + 6.5); y += 10;
    const cols = { item: 18, desc: 34, qty: 124, unit: 146, total: 174 };
    const drawItemsHeader = () => {
      doc.setFillColor(13, 35, 83); doc.rect(18, y, 176, 10, 'F'); doc.setTextColor(255, 255, 255); doc.setFontSize(6.8);
      doc.text('ITEM', cols.item, y + 6.5); doc.text('DESCRIÇÃO DO SERVIÇO', cols.desc, y + 6.5); doc.text('QTDE', cols.qty, y + 6.5); doc.text('VALOR UNITÁRIO', cols.unit, y + 6.5); doc.text('SUBTOTAL', cols.total, y + 6.5); y += 10;
    };
    const newItemsPage = () => { doc.addPage(); doc.addImage(INOVAR_HEADER, 'PNG', 0, 0, W, 50.2, undefined, 'FAST'); y = 61; drawItemsHeader(); };
    drawItemsHeader();
    doc.setDrawColor(...line); doc.setTextColor(...ink); doc.setFont('helvetica', 'normal'); doc.setFontSize(7.1);
    budget.items.forEach((item, index) => {
      const lines = doc.splitTextToSize(clean(item.description) || 'Serviço não informado', 82);
      const h = Math.max(15, 7 + lines.length * 4.7);
      // Reserva área para o resumo e evita que itens extensos invadam o rodapé.
      if (y + h > 221) newItemsPage();
      doc.setFillColor(index % 2 ? 253 : 248, index % 2 ? 253 : 248, 248); doc.rect(18, y, 176, h, 'FD');
      doc.setFont('helvetica', 'bold'); doc.text(String(index + 1).padStart(2, '0'), 23, y + 8.5); doc.text(lines, 38, y + 6.5);
      doc.setFont('helvetica', 'normal'); const valueY = y + Math.min(h - 5.5, 8.5); doc.text(String(item.quantity), 129, valueY); doc.text(money(item.unitPrice), 147, valueY); doc.text(money(item.totalPrice), 175, valueY); y += h;
    });
    if (!budget.items.length) { doc.rect(18, y, 176, 17, 'FD'); doc.text('Nenhum item informado', 38, y + 9); y += 17; }
    if (y > 218) newItemsPage();
    const totalY = y; doc.setFillColor(239, 243, 249); doc.roundedRect(132, totalY, 62, 13, 1.5, 1.5, 'FD'); doc.setFont('helvetica', 'bold'); doc.setTextColor(...navy); doc.setFontSize(7.5); doc.text('VALOR TOTAL:', 139, totalY + 8); doc.setFontSize(13); doc.text(money(budget.finalValue), 165, totalY + 8); y += 18;

    if (y + 54 > 252) { doc.addPage(); doc.addImage(INOVAR_HEADER, 'PNG', 0, 0, W, 50.2, undefined, 'FAST'); y = 61; }
    const boxTop = y; const left = 16; const right = 108; const boxW = 86; const boxH = 43;
    const bar = (x: number, title: string) => { doc.setFillColor(...navy); doc.roundedRect(x, boxTop, boxW, 9, 1.5, 1.5, 'F'); doc.setTextColor(255, 255, 255); doc.setFont('helvetica', 'bold'); doc.setFontSize(7.6); doc.text(title, x + 8, boxTop + 6); };
    bar(left, 'CONDIÇÕES'); bar(right, 'RESPONSÁVEL TÉCNICO');
    doc.setFillColor(255, 255, 255); doc.setDrawColor(...line); doc.roundedRect(left, boxTop + 9, boxW, boxH, 1.5, 1.5, 'FD'); doc.roundedRect(right, boxTop + 9, boxW, boxH, 1.5, 1.5, 'FD');
    doc.setTextColor(...ink); doc.setFont('helvetica', 'normal'); doc.setFontSize(6.5);
    const issuedAt = new Date(`${budget.date}T00:00:00`);
    const validUntilAt = new Date(`${budget.validUntil}T00:00:00`);
    let validityDays = Number.isNaN(issuedAt.getTime()) || Number.isNaN(validUntilAt.getTime()) ? 0 : differenceInCalendarDays(validUntilAt, issuedAt);
    if (!Number.isFinite(validityDays) || validityDays < 0) validityDays = 0;
    const conditions = [`• Este orçamento tem validade de ${validityDays} dias, até ${datePt(budget.validUntil)}.`, '• O valor refere-se apenas aos serviços descritos.', '• Materiais, peças e acessórios serão cobrados à parte.', `• Garantia parametrizada: ${clean(budget.warrantyTerms) || 'Não informada'}.`];
    conditions.forEach((t, i) => doc.text(doc.splitTextToSize(t, 75), left + 4, boxTop + 17 + i * 7));
    try {
      const props = doc.getImageProperties(INOVAR_SIGNATURE_GABRIEL);
      const maxW = 58; const maxH = 16; const ratio = props.width / props.height;
      let sigW = maxW; let sigH = sigW / ratio;
      if (sigH > maxH) { sigH = maxH; sigW = sigH * ratio; }
      doc.addImage(INOVAR_SIGNATURE_GABRIEL, props.fileType || 'PNG', right + 43 - sigW / 2, boxTop + 11 + (maxH - sigH) / 2, sigW, sigH);
    } catch { /* a geração continua mesmo se o navegador estiver offline */ }
    doc.setDrawColor(...line); doc.line(right + 12, boxTop + 34, right + 74, boxTop + 34); doc.setTextColor(...ink); doc.setFont('helvetica', 'bold'); doc.setFontSize(7); doc.text('Gabriel Nascimento', right + 43, boxTop + 40, { align: 'center' }); doc.setFont('helvetica', 'normal'); doc.setFontSize(6.5); doc.text('Técnico Responsável', right + 43, boxTop + 45, { align: 'center' });
    const pageCount = doc.getNumberOfPages();
    for (let page = 1; page <= pageCount; page += 1) { doc.setPage(page); doc.addImage(INOVAR_FOOTER_REFERENCE, 'PNG', MARGIN, PAGE_H - MARGIN - 35, W - (MARGIN * 2), 35, undefined, 'FAST'); }
    return doc;
  },
  async generateBudgetPdf(budget: BudgetEstimate, profile: TechnicianProfile): Promise<void> { const doc = await this.buildBudgetPdf(budget, profile); doc.save(`ORCAMENTO_INOVAR_${clean(budget.clientName).replace(/\s+/g, '_')}_${budget.numero || budget.id}.pdf`); }
};
