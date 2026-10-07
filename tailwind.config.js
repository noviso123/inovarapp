/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        inovar: {
          navy: '#0B2D4E',
          blue: '#1668B4',
          sky: '#3E9BDE',
          yellow: '#FFC61E',
          iced: '#F2F7FB'
        },
        brand: {
          dark: '#0a101f',
          navy: '#0f172a',
          card: '#1e293b',
          blue: '#1e40af',
          lightBlue: '#0284c7',
          cyan: '#06b6d4',
          accent: '#2563eb'
        }
      }
    },
  },
  plugins: [],
}
