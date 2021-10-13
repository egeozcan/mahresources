module.exports = {
  mode: 'jit',
  purge: {
    preserveHtmlElements: false,
    content: [
      './templates/**/*.tpl',
      './templates/**/*.html',
    ]
  },
  darkMode: false, // or 'media' or 'class'
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
