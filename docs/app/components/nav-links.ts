// Shared nav link list consumed by both the desktop Nav and the mobile
// NavMobile drawer. Keeping them in one place stops the two components from
// drifting apart whenever a section is added or removed.
export const navLinks = [
  { href: "/docs", label: "Docs" },
  { href: "/#features", label: "Features" },
  { href: "/#privacy", label: "Privacy" },
] as const;

export type NavLink = (typeof navLinks)[number];
