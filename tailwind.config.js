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
        bgObsidian: '#07090E',
        panelObsidian: '#0F172A',
        panelBorder: '#1E293B',
        panelHover: '#1E293B',
        accentSky: '#38BDF8',
        accentPurple: '#A855F7',
        accentCyan: '#06B6D4',
        accentEmerald: '#10B981',
        accentAmber: '#F59E0B',
        accentRose: '#EF4444',
      },
      fontFamily: {
        mono: ['JetBrains Mono', 'Fira Code', 'Consolas', 'monospace'],
        sans: ['Inter', 'system-ui', 'sans-serif'],
      },
    },
  },
  plugins: [],
}
