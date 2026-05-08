#!/usr/bin/env python3
"""Detect duplication and integrity issues across Claude Code asset scopes.

Outputs one issue per line:  category | severity | scope | asset | detail

Categories: dangling, duplicate, ghost, broken, hook-dupe, perm-dupe,
            plugin-multi-scope, mcp-multi-scope.
"""

from __future__ import annotations

import json
import os
import subprocess
import sys
from pathlib import Path

HOME = Path(os.environ["HOME"])
HOST = HOME / ".claude"
CCPM = HOME / ".ccpm"
AGENTS = HOME / ".agents"


def emit(category: str, severity: str, scope: str, asset: str, detail: str) -> None:
    print(f"{category} | {severity} | {scope} | {asset} | {detail}")


def find_dangling_symlinks(root: Path) -> list[Path]:
    """Symlinks under root whose target does not exist."""
    results: list[Path] = []
    if not root.exists():
        return results
    for path in root.rglob("*"):
        if path.is_symlink() and not path.exists():
            results.append(path)
    return results


def dangling() -> None:
    for root in (HOST / "skills", CCPM / "profiles", CCPM / "share"):
        for link in find_dangling_symlinks(root):
            target = os.readlink(link) if link.is_symlink() else "?"
            emit("dangling", "warn", str(link.parent), link.name,
                 f"target {target} missing")


def real_dirs(scope_root: Path) -> dict[str, Path]:
    """Map of skill name -> path for real (non-symlink) skill dirs in scope."""
    out: dict[str, Path] = {}
    if not scope_root.exists():
        return out
    for entry in scope_root.iterdir():
        if entry.is_dir() and not entry.is_symlink() and entry.name != "_sources":
            out[entry.name] = entry
    return out


def duplicates() -> None:
    """Real-dir duplicates across {agents, share, host}."""
    scopes = {
        "agents": real_dirs(AGENTS / "skills"),
        "share": real_dirs(CCPM / "share" / "skills"),
        "host": real_dirs(HOST / "skills"),
    }
    seen: dict[str, list[str]] = {}
    for scope, names in scopes.items():
        for name in names:
            seen.setdefault(name, []).append(scope)
    for name, dupe_scopes in seen.items():
        if len(dupe_scopes) < 2:
            continue
        # Confirm contents identical with a fast diff
        paths = [scopes[s][name] for s in dupe_scopes]
        try:
            r = subprocess.run(
                ["diff", "-rq", str(paths[0]), str(paths[1])],
                capture_output=True, text=True, timeout=15,
            )
            identical = r.returncode == 0 and not r.stdout.strip()
        except Exception:
            identical = False
        detail = "identical" if identical else "DIVERGED"
        emit("duplicate", "warn", "+".join(dupe_scopes), name, detail)


def ghost_manifest() -> None:
    inst_path = CCPM / "installs.json"
    if not inst_path.is_file():
        return
    try:
        manifest = json.loads(inst_path.read_text())
    except json.JSONDecodeError:
        emit("broken", "error", "ccpm", "installs.json", "invalid JSON")
        return
    profile_dir = CCPM / "profiles"
    live = {p.name for p in profile_dir.iterdir()} if profile_dir.exists() else set()
    referenced = {p for i in manifest.get("installs", []) for p in i.get("profiles", [])}
    ghosts = referenced - live
    if not ghosts:
        return
    for install in manifest.get("installs", []):
        bad = [p for p in install.get("profiles", []) if p in ghosts]
        if bad:
            emit("ghost", "info", "manifest", install.get("id", "?"),
                 f"profiles={bad} not in live={sorted(live)}")


def broken_files() -> None:
    """0-byte files where SKILL.md or symlinks are expected."""
    for root in (HOST / "skills", CCPM / "share" / "skills", AGENTS / "skills"):
        if not root.exists():
            continue
        for path in root.rglob("*"):
            try:
                if path.is_file() and not path.is_symlink() and path.stat().st_size == 0:
                    emit("broken", "warn", str(root), str(path.relative_to(root)),
                         "0-byte file")
            except OSError:
                continue


def hook_duplication() -> None:
    host_settings = HOST / "settings.json"
    if not host_settings.is_file():
        return
    try:
        host = json.loads(host_settings.read_text())
    except json.JSONDecodeError:
        return
    host_hooks = host.get("hooks", {})
    if not host_hooks:
        return
    if not (CCPM / "profiles").exists():
        return
    for prof in (CCPM / "profiles").iterdir():
        s = prof / "settings.json"
        if not s.is_file():
            continue
        try:
            d = json.loads(s.read_text())
        except json.JSONDecodeError:
            continue
        prof_hooks = d.get("hooks", {})
        # Same matchers in profile and host = duplicate cascade
        for kind in prof_hooks:
            if kind in host_hooks:
                emit("hook-dupe", "warn", prof.name, kind,
                     f"profile {kind} duplicates host {kind}")


def permissions_intersection() -> None:
    if not (CCPM / "profiles").exists():
        return
    profiles_perms: dict[str, set[str]] = {}
    for prof in (CCPM / "profiles").iterdir():
        s = prof / "settings.json"
        if not s.is_file():
            continue
        try:
            d = json.loads(s.read_text())
        except json.JSONDecodeError:
            continue
        allow = set(d.get("permissions", {}).get("allow", []))
        if allow:
            profiles_perms[prof.name] = allow
    if len(profiles_perms) < 2:
        return
    # Count entries appearing in 2+ profiles
    from collections import Counter
    counter: Counter[str] = Counter()
    for entries in profiles_perms.values():
        for e in entries:
            counter[e] += 1
    intersection = {e for e, c in counter.items() if c >= 2}
    if intersection:
        emit("perm-dupe", "info", "all-profiles", f"{len(intersection)} entries",
             f"appearing in 2+ profiles — candidate for host promotion")


def plugin_multi_scope() -> None:
    host_settings = HOST / "settings.json"
    host_plugins = set()
    if host_settings.is_file():
        try:
            host_plugins = set(json.loads(host_settings.read_text()).get("enabledPlugins", {}).keys())
        except json.JSONDecodeError:
            pass
    if not (CCPM / "profiles").exists():
        return
    for prof in (CCPM / "profiles").iterdir():
        s = prof / "settings.json"
        if not s.is_file():
            continue
        try:
            d = json.loads(s.read_text())
        except json.JSONDecodeError:
            continue
        prof_plugins = set(d.get("enabledPlugins", {}).keys())
        overlap = host_plugins & prof_plugins
        for p in overlap:
            emit("plugin-multi-scope", "info", f"host+{prof.name}", p,
                 "enabled at both host and profile (cascade dedupes but bytes redundant)")


def mcp_multi_scope() -> None:
    host_mcp_path = HOME / ".claude.json"
    host_mcps = set()
    if host_mcp_path.is_file():
        try:
            host_mcps = set(json.loads(host_mcp_path.read_text()).get("mcpServers", {}).keys())
        except json.JSONDecodeError:
            pass
    if not (CCPM / "profiles").exists():
        return
    for prof in (CCPM / "profiles").iterdir():
        s = prof / ".claude.json"
        if not s.is_file():
            continue
        try:
            d = json.loads(s.read_text())
        except json.JSONDecodeError:
            continue
        overlap = host_mcps & set(d.get("mcpServers", {}).keys())
        for m in overlap:
            emit("mcp-multi-scope", "info", f"host+{prof.name}", m,
                 "MCP defined at both host and profile")


def main() -> int:
    dangling()
    duplicates()
    ghost_manifest()
    broken_files()
    hook_duplication()
    permissions_intersection()
    plugin_multi_scope()
    mcp_multi_scope()
    return 0


if __name__ == "__main__":
    sys.exit(main())
