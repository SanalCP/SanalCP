/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        brand: {
          50:  '#ecf3ff',
          100: '#dde9ff',
          200: '#c2d6ff',
          300: '#9cb9ff',
          400: '#7592ff',
          500: '#465fff',
          600: '#3641f5',
          700: '#2a31d8',
          800: '#252dae',
          900: '#262e89',
        },
      },
      fontFamily: {
        sans: ['Inter', 'system-ui', '-apple-system', 'Segoe UI', 'Roboto', 'sans-serif'],
        mono: ['JetBrains Mono', 'Menlo', 'Consolas', 'monospace'],
      },
      // Belirsiz (indeterminate) ilerleme çubuğu: süresi bilinmeyen işlemlerde
      // (ör. SSL kurulumu — backend ilerleme bildirmiyor) uydurma bir yüzde
      // göstermek yerine sürekli kayan bir şerit. Bkz. DomainSSLPage.
      keyframes: {
        'ssl-indeterminate': {
          '0%':   { transform: 'translateX(-100%)' },
          '100%': { transform: 'translateX(400%)' },
        },
      },
      animation: {
        'ssl-indeterminate': 'ssl-indeterminate 1.4s ease-in-out infinite',
      },
    },
  },
  plugins: [],
}
