/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    "../main/resources/themes/08rj/template/**/*.html",
    "../main/resources/themes/08rj/page/**/*.html",
  ],
  theme: {
    extend: {
      colors: {
        primary: {
          50: '#e6f7ff',
          100: '#ccefff',
          200: '#99ddff',
          300: '#66ccff',
          400: '#33bbff',
          500: '#00aaff',
          600: '#0088cc',
          700: '#006699',
          800: '#004466',
          900: '#002233'
        },
        secondary: {
          50: '#f0fdf4',
          100: '#dcfce7',
          200: '#bbf7d0',
          300: '#86efac',
          400: '#4ade80',
          500: '#22c55e',
          600: '#16a34a',
          700: '#15803d',
          800: '#166534',
          900: '#14532d'
        }
      },
      fontFamily: {
        sans: ['-apple-system', 'BlinkMacSystemFont', 'Segoe UI', 'Roboto', 'Helvetica Neue', 'Arial', 'sans-serif'],
      },
    },
  },
  plugins: [],
}