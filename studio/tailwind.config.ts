import type { Config } from "tailwindcss";

const config: Config = {
  content: [
    "./src/pages/**/*.{js,ts,jsx,tsx,mdx}",
    "./src/components/**/*.{js,ts,jsx,tsx,mdx}",
    "./src/app/**/*.{js,ts,jsx,tsx,mdx}",
  ],
  darkMode: "class",
  theme: {
    extend: {
      colors: {
        background: "#080B11",
        card: "rgba(17, 24, 39, 0.75)",
        border: "rgba(255, 255, 255, 0.08)",
        primary: {
          DEFAULT: "#8B5CF6",
          hover: "#7C3AED",
          glow: "rgba(139, 92, 246, 0.3)",
        },
        accent: {
          cyan: "#06B6D4",
          emerald: "#10B981",
          rose: "#F43F5E",
        },
      },
    },
  },
  plugins: [],
};
export default config;
