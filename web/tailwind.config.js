/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    "./src/**/*.{js,jsx,ts,tsx}",
  ],
  darkMode: 'class',
  theme: {
    extend: {
      fontFamily: {
        mono: ['JetBrains Mono', 'ui-monospace', 'SFMono-Regular', 'Menlo', 'Monaco', 'Consolas', 'Liberation Mono', 'Courier New', 'monospace'],
      },
      colors: {
        blue: {
          900: '#0000CD',
          800: '#1D01D4',
          700: '#3B0BD9',
          600: '#5117E2',
          500: '#5E1CE8',
          400: '#7C46EC',
          300: '#966AF0',
          200: '#B596F3',
          100: '#D3C1F7',
          50: '#EFEFF0',
        },
        green: {
          900: '#00961C',
          800: '#00B833',
          700: '#00CA3F',
          600: '#00DF4D',
          500: '#00F770',
          400: '#00F770',
          300: '#00FC8F',
          200: '#83FDB3',
          100: '#BAFED1',
          50: '#E3FFED',
        },
        red: {
          900: '#CA002F',
          800: '#D9173C',
          700: '#E62144',
          600: '#F82D4B',
          500: '#FF354C',
          400: '#FF4F64',
          300: '#F17483',
          200: '#F79CA6',
          100: '#FFCFDA',
          50: '#FFECF1',
        },
      },
    },
  },
  plugins: [],
} 