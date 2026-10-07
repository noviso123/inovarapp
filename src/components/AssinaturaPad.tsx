import React, { useRef } from 'react';

// Assinatura digital em canvas (dedo/mouse) — usada pelo cliente (aprovar
// orçamento) e pelo técnico (salvar a assinatura dele nas Configurações).
export const AssinaturaPad: React.FC<{
  onChange: (dataUrl: string | null) => void;
  valorInicial?: string | null;
  altura?: number;
}> = ({ onChange, valorInicial, altura = 160 }) => {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const drawing = useRef(false);
  const dirty = useRef(false);

  // desenha a assinatura salva — também quando ela chega depois (GET assíncrono)
  React.useEffect(() => {
    if (valorInicial && canvasRef.current) {
      const img = new Image();
      img.onload = () => {
        const canvas = canvasRef.current!;
        const ctx = canvas.getContext('2d')!;
        ctx.clearRect(0, 0, canvas.width, canvas.height);
        // Nunca estica a assinatura já cadastrada: mantém o traço original
        // tanto no celular quanto no computador.
        const scale = Math.min(canvas.width / img.width, canvas.height / img.height);
        const width = img.width * scale;
        const height = img.height * scale;
        ctx.drawImage(img, (canvas.width - width) / 2, (canvas.height - height) / 2, width, height);
      };
      img.src = valorInicial;
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [valorInicial]);

  const pos = (e: any) => {
    const canvas = canvasRef.current!;
    const rect = canvas.getBoundingClientRect();
    const t = e.touches ? e.touches[0] : e;
    return {
      x: ((t.clientX - rect.left) / rect.width) * canvas.width,
      y: ((t.clientY - rect.top) / rect.height) * canvas.height
    };
  };

  const start = (e: any) => {
    e.preventDefault();
    drawing.current = true;
    const ctx = canvasRef.current!.getContext('2d')!;
    const p = pos(e);
    ctx.beginPath();
    ctx.moveTo(p.x, p.y);
  };
  const move = (e: any) => {
    if (!drawing.current) return;
    e.preventDefault();
    const ctx = canvasRef.current!.getContext('2d')!;
    const p = pos(e);
    ctx.lineWidth = 2.5;
    ctx.lineCap = 'round';
    ctx.strokeStyle = '#0B2D4E';
    ctx.lineTo(p.x, p.y);
    ctx.stroke();
    dirty.current = true;
  };
  const end = () => {
    drawing.current = false;
    if (dirty.current && canvasRef.current) onChange(canvasRef.current.toDataURL('image/png'));
  };
  const clear = () => {
    const canvas = canvasRef.current!;
    canvas.getContext('2d')!.clearRect(0, 0, canvas.width, canvas.height);
    dirty.current = false;
    onChange(null);
  };

  return (
    <div>
      <canvas
        ref={canvasRef}
        width={520}
        height={200}
        style={altura ? { height: altura } : undefined}
        className="w-full touch-none bg-white border-2 border-dashed border-slate-300 rounded-xl cursor-crosshair"
        onMouseDown={start}
        onMouseMove={move}
        onMouseUp={end}
        onMouseLeave={end}
        onTouchStart={start}
        onTouchMove={move}
        onTouchEnd={end}
      />
      <div className="flex items-center justify-between mt-1.5 gap-2">
        <span className="text-[10px] text-slate-500">Assine com o dedo, caneta ou mouse</span>
        <button type="button" onClick={clear} className="text-[11px] font-bold text-red-400 hover:text-red-300">
          Limpar
        </button>
      </div>
    </div>
  );
};
