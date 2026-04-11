import type { Metadata } from "next";
import { Geist, Geist_Mono } from "next/font/google";
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

// Inline script that runs before paint to set the theme from localStorage,
// preventing FOUC. Must run synchronously in <head>.
const themeInitScript = `
(function() {
  try {
    var stored = localStorage.getItem('theme');
    var theme = stored === 'light' || stored === 'dark' ? stored : 'dark';
    document.documentElement.setAttribute('data-theme', theme);
  } catch (e) {
    document.documentElement.setAttribute('data-theme', 'dark');
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
      <head>
        <script dangerouslySetInnerHTML={{ __html: themeInitScript }} />
      </head>
      <body className="min-h-full flex flex-col bg-bg text-fg">{children}</body>
    </html>
  );
}
