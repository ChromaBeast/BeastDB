"use client";
import { useEffect, useState } from "react";
export function useTheme() {
  const [dark, setDark] = useState(false);
  useEffect(() => {
    const media = matchMedia("(prefers-color-scheme: dark)");
    const sync = () => {
      const next = (localStorage.getItem("beastdb-theme") ||
        (media.matches ? "dark" : "light")) === "dark";
      document.documentElement.classList.toggle("dark", next);
      setDark(next);
    };
    sync();
    media.addEventListener("change", sync);
    return () => media.removeEventListener("change", sync);
  }, []);
  const toggle = () => {
    const next = !dark;
    document.documentElement.classList.toggle("dark", next);
    localStorage.setItem("beastdb-theme", next ? "dark" : "light");
    setDark(next);
  };
  return { dark, toggle };
}
