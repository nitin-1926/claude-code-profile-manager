import type { Metadata } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import Script from "next/script";
import "./globals.css";

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

export const metadata: Metadata = {
  title: "ccpm — Claude Code Profile Manager",
  description:
    "Run multiple Claude Code accounts in parallel with full isolation. OAuth + API key. Encrypted vault. 100% local.",
  openGraph: {
    title: "ccpm — Claude Code Profile Manager",
    description:
      "Run multiple Claude Code accounts in parallel with full isolation.",
    url: "https://ccpm.dev",
    siteName: "ccpm",
  },
};

// Inline script that runs before paint: saved preference, else system
// (prefers-color-scheme), else dark. Loaded via next/script beforeInteractive.
const themeInitScript = `
(function() {
  function systemOrDark() {
    try {
      return window.matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark';
    } catch (e) {
      return 'dark';
    }
  }
  try {
    var stored = localStorage.getItem('theme');
    var theme = stored === 'light' || stored === 'dark' ? stored : systemOrDark();
    document.documentElement.setAttribute('data-theme', theme);
  } catch (e) {
    document.documentElement.setAttribute('data-theme', systemOrDark());
  }
})();
`;

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html
      lang="en"
      data-theme="dark"
      suppressHydrationWarning
      className={`${geistSans.variable} ${geistMono.variable} h-full antialiased`}
    >
      <body className="min-h-full flex flex-col bg-bg text-fg">
        <Script
          id="ccpm-theme-init"
          strategy="beforeInteractive"
          dangerouslySetInnerHTML={{ __html: themeInitScript }}
        />
        {children}
      </body>
    </html>
  );
}
