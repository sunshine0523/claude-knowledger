import shutil
import subprocess
import sys
from pathlib import Path


def _plugin_src() -> Path:
    if meipass := getattr(sys, "_MEIPASS", None):
        return Path(meipass) / "plugins" / "claude-code-knowledger"
    bundled = Path(__file__).parent / "_bundle" / "claude-code-knowledger"
    if bundled.exists():
        return bundled
    for parent in Path(__file__).parents:
        candidate = parent / "plugins" / "claude-code-knowledger"
        if candidate.exists():
            return candidate
    raise RuntimeError("plugin bundle not found")


def _run(args: list[str]) -> subprocess.CompletedProcess:
    return subprocess.run(args, capture_output=True, text=True)


def install() -> None:
    import typer

    print("Checking Claude Code...")
    if not shutil.which("claude"):
        typer.echo("error: claude not found on PATH. Install Claude Code first.", err=True)
        raise typer.Exit(1)

    if _run(["claude", "plugin", "--help"]).returncode != 0:
        typer.echo("error: Claude Code is too old. Update it and rerun.", err=True)
        raise typer.Exit(1)

    knowledger_bin = str(Path(shutil.which("knowledger") or sys.argv[0]).resolve())

    print("Registering Knowledger MCP server...")
    r = _run(["claude", "mcp", "get", "knowledger"])
    if r.returncode == 0:
        if knowledger_bin in r.stdout and "mcp" in r.stdout:
            print("  MCP server already registered.")
        else:
            typer.echo("error: a conflicting 'knowledger' MCP server already exists.", err=True)
            typer.echo("  Remove it with: claude mcp remove knowledger", err=True)
            raise typer.Exit(1)
    else:
        r = _run(["claude", "mcp", "add", "--scope", "user", "knowledger", "--", knowledger_bin, "mcp"])
        if r.returncode != 0:
            typer.echo(f"error: {r.stderr}", err=True)
            raise typer.Exit(1)

    print("Installing Knowledger Claude Code plugin...")
    marketplace_dir = Path.home() / ".knowledger" / "claude-code" / "marketplace"
    marketplace_dir.mkdir(parents=True, exist_ok=True)
    plugin_dest = marketplace_dir / "claude-code-knowledger"
    if plugin_dest.exists():
        shutil.rmtree(plugin_dest)
    shutil.copytree(_plugin_src(), plugin_dest)

    r = _run(["claude", "plugin", "marketplace", "add", "--scope", "user", str(marketplace_dir)])
    if r.returncode != 0 and "already" not in r.stderr.lower():
        typer.echo(f"error: failed to register marketplace: {r.stderr}", err=True)
        typer.echo("note: MCP server was registered successfully.", err=True)
        raise typer.Exit(1)

    r = _run(["claude", "plugin", "install", "--scope", "user", "claude-code-knowledger@knowledger"])
    if r.returncode != 0 and "already" not in r.stderr.lower():
        typer.echo(f"error: failed to install plugin: {r.stderr}", err=True)
        typer.echo("note: MCP server was registered successfully.", err=True)
        raise typer.Exit(1)

    print("Knowledger is installed for Claude Code.")
    print("\nVerify with:\n  claude mcp get knowledger\n  claude plugin list")
