import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "BeastDB Studio — Console",
  description: "High-Performance Database Engine Management Console",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en" className="dark">
      <body className="antialiased selection:bg-purple-500/30 selection:text-purple-200">
        {children}
      </body>
    </html>
  );
}
