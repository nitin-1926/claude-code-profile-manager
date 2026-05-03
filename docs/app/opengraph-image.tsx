import { ImageResponse } from "next/og";

// Dynamic OpenGraph image rendered by Next at build time. Uses the same
// terracotta accent / dark-first palette as the site so social-card previews
// on Twitter, Slack, GitHub, etc. are recognisable at a glance.

export const runtime = "edge";
export const contentType = "image/png";
export const size = { width: 1200, height: 630 };
export const alt = "ccpm — Claude Code Profile Manager";

export default async function Image() {
  return new ImageResponse(
    (
      <div
        style={{
          width: "100%",
          height: "100%",
          display: "flex",
          flexDirection: "column",
          justifyContent: "space-between",
          backgroundColor: "#0e0e0e",
          color: "#e8e7e3",
          padding: 80,
          fontFamily: "ui-monospace, SFMono-Regular, monospace",
        }}
      >
        <div style={{ display: "flex", alignItems: "center", gap: 20 }}>
          <div
            style={{
              width: 14,
              height: 14,
              borderRadius: 999,
              background: "#c05a3e",
              boxShadow: "0 0 24px rgba(192,90,62,0.8)",
            }}
          />
          <div style={{ fontSize: 36, fontWeight: 600, letterSpacing: -1 }}>
            ccpm
