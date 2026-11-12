import type { Config } from "tailwindcss";
const config: Config = {
  content: ["./src/**/*.{js,ts,jsx,tsx,mdx}"],
  darkMode: "class",
  theme: {
    container: { center: true, padding: "1.5rem" },
    extend: {
      colors: {
        background: "hsl(var(--background))", foreground: "hsl(var(--foreground))",
        card: { DEFAULT: "hsl(var(--card))", foreground: "hsl(var(--card-foreground))" },
        popover: { DEFAULT: "hsl(var(--popover))", foreground: "hsl(var(--popover-foreground))" },
        primary: { DEFAULT: "hsl(var(--primary))", foreground: "hsl(var(--primary-foreground))" },
        secondary: { DEFAULT: "hsl(var(--secondary))", foreground: "hsl(var(--secondary-foreground))" },
        muted: { DEFAULT: "hsl(var(--muted))", foreground: "hsl(var(--muted-foreground))" },
        accent: { DEFAULT: "hsl(var(--accent))", foreground: "hsl(var(--accent-foreground))" },
        destructive: { DEFAULT: "hsl(var(--destructive))", foreground: "hsl(var(--destructive-foreground))" },
        sidebar: "hsl(var(--sidebar))", code: "hsl(var(--code))",
        warning: "#FBBF24", success: "#2DD4BF", info: "#60A5FA",
        border: "hsl(var(--border))", input: "hsl(var(--input))", ring: "hsl(var(--ring))",
        beast: {
          lime: "#A8F21A",
          "lime-hover": "#97DE10",
          "lime-muted": "rgba(168, 242, 26, 0.12)",
          dark: "#0E120E",
          surface: "#151A15",
          border: "#2A342A",
        },
      },
      borderRadius: { lg: "var(--radius)", md: "calc(var(--radius) - 2px)", sm: "calc(var(--radius) - 4px)" },
    },
  },
  plugins: [],
};
export default config;
