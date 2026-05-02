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
