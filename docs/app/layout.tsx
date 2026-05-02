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

// Setting metadataBase removes the Next.js build-time warning about relative
// URLs (openGraph.images, twitter.images, etc.) and anchors canonical URLs
// when per-page generateMetadata exports land.
export const metadata: Metadata = {
  metadataBase: new URL("https://ccpm.dev"),
  title: {
    default: "ccpm — Claude Code Profile Manager",
    template: "%s — ccpm",
  },
  description:
    "Run multiple Claude Code accounts in parallel with full isolation. OAuth + API key. Encrypted vault. 100% local.",
  applicationName: "ccpm",
  keywords: [
    "Claude Code",
    "profile manager",
    "Anthropic",
    "CLI",
    "multi-account",
  ],
  openGraph: {
    type: "website",
    title: "ccpm — Claude Code Profile Manager",
    description:
      "Run multiple Claude Code accounts in parallel with full isolation.",
    url: "/",
    siteName: "ccpm",
    images: [
      {
        url: "/opengraph-image",
        width: 1200,
        height: 630,
        alt: "ccpm — Claude Code Profile Manager",
      },
    ],
  },
  twitter: {
    card: "summary_large_image",
    title: "ccpm — Claude Code Profile Manager",
    description:
      "Run multiple Claude Code accounts in parallel with full isolation.",
    images: ["/opengraph-image"],
  },
  robots: {
    index: true,
    follow: true,
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
      // Seed `data-theme="dark"` so the server-rendered HTML matches the
      // dark-first palette before hydration. The beforeInteractive script
      // rewrites the attribute to the user's saved preference (or system
      // preference) before first paint; suppressHydrationWarning mutes the
      // React warning for that legitimate server/client divergence.
      data-theme="dark"
      suppressHydrationWarning
      className={`${geistSans.variable} ${geistMono.variable} h-full antialiased`}
    >
      <body className="min-h-full flex flex-col bg-bg text-fg">
        {/*
          Skip-to-content link. Hidden by default and revealed on focus so
          keyboard users can bypass the sticky nav (WCAG 2.4.1 Bypass Blocks).
          Pages must mark their main region with id="main".
        */}
        <a
          href="#main"
          className="sr-only focus:not-sr-only fixed top-2 left-2 z-[200] rounded-md bg-accent text-accent-fg px-3 py-2 text-sm font-medium shadow-lg focus:outline-none focus-visible:ring-2 focus-visible:ring-accent"
        >
          Skip to content
        </a>
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
