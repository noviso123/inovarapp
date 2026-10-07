import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react()],
  build: {
    // Mantém bibliotecas estáveis em arquivos próprios. Assim, uma atualização
    // de tela baixa somente o código alterado, em vez de refazer o download
    // de React, Supabase, ícones e utilitários em cada abertura do PWA.
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes('node_modules')) return undefined;
          if (id.includes('/react/') || id.includes('/react-dom/') || id.includes('/scheduler/') || id.includes('/use-sync-external-store/')) return 'react-core';
          if (id.includes('@supabase')) return 'supabase';
          if (id.includes('date-fns')) return 'dates';
          if (id.includes('lucide-react')) return 'icons';
          if (id.includes('html2canvas')) return 'html-capture';
          if (id.includes('jspdf')) return 'pdf-engine';
          return undefined;
        }
      }
    }
  },
  server: {
    port: 3000,
    host: true
  }
});
