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
    <html lang="en" className="dark scroll-smooth" suppressHydrationWarning>
      <head>
        <script
          dangerouslySetInnerHTML={{
            __html: `(function(){try{var t=localStorage.getItem('beastdb-theme');document.documentElement.classList.toggle('dark',t!=='light')}catch(e){}})()`,
          }}
        />
      </head>
      <body className="bg-zinc-950 text-zinc-100 antialiased min-h-screen selection:bg-beast-lime selection:text-black">{children}</body>
    </html>
  );
}
