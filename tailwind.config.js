module.exports = {
  mode: 'jit',
  content: [
    './templates/**/*.tpl',
    './templates/**/*.html',
    './public/**/*.js',
  ],
  darkMode: 'media', // or 'media' or 'class'
  theme: {
    extend: {},
  },
  variants: {
    extend: {},
  },
  plugins: [
    require('@tailwindcss/forms'),
    require('@tailwindcss/typography'),
  ],
}
