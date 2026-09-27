"use client";
import { useEffect, useState } from "react";
export function useTheme() {
  const [dark, setDark] = useState(true);

  useEffect(() => {
    const isDark = document.documentElement.classList.contains("dark");
    setDark(isDark);

    const media = matchMedia("(prefers-color-scheme: dark)");
    const sync = () => {
      const stored = localStorage.getItem("beastdb-theme");
      const next = stored ? stored === "dark" : media.matches;
      document.documentElement.classList.toggle("dark", next);
      setDark(next);
    };

    media.addEventListener("change", sync);
    return () => media.removeEventListener("change", sync);
  }, []);

  const toggle = () => {
    const next = !document.documentElement.classList.contains("dark");
    document.documentElement.classList.toggle("dark", next);
    localStorage.setItem("beastdb-theme", next ? "dark" : "light");
    setDark(next);
  };

  return { dark, toggle };
}
