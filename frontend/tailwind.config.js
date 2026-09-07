/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        base: '#0B0B0C',
        surface: '#151517',
        surfaceHover: '#1E1E22',
        surfaceBorder: '#26262B',
        healthy: '#3F8E4F',
        warning: '#D4A017',
        critical: '#AD2831',
        failure: '#AD2831',
        intervention: '#840032',
        execution: '#89023E',
        recovered: '#3F8E4F',
        approvalAmber: '#D4A017',
        approvalOxblood: '#840032',
      },
      fontFamily: {
        mono: ['JetBrains Mono', 'Fira Code', 'Consolas', 'monospace'],
        sans: ['Inter', 'system-ui', 'sans-serif'],
      },
    },
  },
  plugins: [],
}
