/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["views/**/*.templ", "views/*.html"],
  plugins: [require("daisyui")],
  corePlugins: {
    preflight: true,
  },
  daisyui: {
    themes: [
      {
        light: {
          "primary": "#4a5568",
          "secondary": "#718096",
          "accent": "#48bb78",
          "neutral": "#2d3748",
          "base-100": "#ffffff",
          "base-200": "#f7fafc",
          "base-300": "#edf2f7",
          "info": "#3182ce",
          "success": "#38a169",
          "warning": "#d69e2e",
          "error": "#e53e3e",
        },
      },
      {
        dark: {
          "primary": "#e2e8f0",
          "secondary": "#cbd5e0",
          "accent": "#68d391",
          "neutral": "#4a5568",
          "base-100": "#1a202c",
          "base-200": "#2d3748",
          "base-300": "#4a5568",
          "info": "#63b3ed",
          "success": "#68d391",
          "warning": "#f6ad55",
          "error": "#fc8181",
        },
      },
    ],
    darkTheme: "dark",
  }
};
