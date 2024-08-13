/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./internal/ui/**/*.templ"],
  theme: {
    extend: {
      zIndex: {
        toast: 100,
      },
    },
  },
  plugins: [],
};
