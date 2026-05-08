#!/usr/bin/env python3
"""Per-profile (or host-only) skill description budget audit.

Outputs one line per profile (or host):

    profile=<name> direct=<n> plugin=<n> total=<n> budget=180 status=<ok|OVERFLOW>

If ccpm not present, reports a single host scope.
"""

from __future__ import annotations

import os
from pathlib import Path

HOME = Path(os.environ["HOME"])
HOST = HOME / ".claude"
CCPM = HOME / ".ccpm"

BUDGET = 180  # see budget-math.md


def count_direct(skills_dir: Path) -> int:
    if not skills_dir.exists():
        return 0
    total = 0
    for entry in skills_dir.iterdir():
        if entry.name == "_sources":
            continue
        if entry.is_dir() or entry.is_symlink():
            total += 1
    return total


def count_plugin_skill_md(plugin_root: Path) -> int:
    if not plugin_root.exists():
        return 0
    return sum(1 for _ in plugin_root.rglob("SKILL.md"))


def report(name: str, direct: int, plugin: int) -> None:
    total = direct + plugin
    status = "ok" if total <= BUDGET else f"OVERFLOW ({total - BUDGET} over)"
    print(f"profile={name} direct={direct} plugin={plugin} total={total} budget={BUDGET} status={status}")


def main() -> int:
    if (CCPM / "profiles").exists():
        for prof in (CCPM / "profiles").iterdir():
            if not prof.is_dir():
                continue
            direct = count_direct(prof / "skills")
            plugin = count_plugin_skill_md(prof / "plugins" / "cache")
            report(prof.name, direct, plugin)
    else:
        direct = count_direct(HOST / "skills")
        plugin = count_plugin_skill_md(HOST / "plugins")
        report("host", direct, plugin)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
