from __future__ import annotations

import sys
from typing import Optional

import typer

from knowledger.core import normalize_scope, SCOPE_GLOBAL, SCOPE_PROJECT, SearchOptions, AddInput, ScopedKBRef
from knowledger.service.service import Service, IndexKnowledgeInput, CreateKnowledgeBaseInput

# Global service state set by create_app callback
_svc: Optional[Service] = None


def _get_svc() -> Service:
    if _svc is None:
        typer.echo("error: service not initialized", err=True)
        raise typer.Exit(1)
    return _svc


def _effective_scope(scope: str) -> str:
    svc = _get_svc()
    if scope:
        return normalize_scope(scope)
    return SCOPE_PROJECT if svc.has_project_scope() else SCOPE_GLOBAL


def create_app() -> typer.Typer:
    app = typer.Typer(add_completion=False)
    kbs_app = typer.Typer(add_completion=False)
    install_app = typer.Typer(add_completion=False)
    app.add_typer(kbs_app, name="kbs")
    app.add_typer(install_app, name="install")

    @app.callback()
    def main_callback(
        config: Optional[str] = typer.Option(None, "--config", help="path to config file"),
        project_root: Optional[str] = typer.Option(None, "--project-root", hidden=True),
    ):
        global _svc
        from knowledger.app.app import build_service, build_default_service, discover_project_root
        root = project_root or discover_project_root()
        if config:
            _svc = build_service(config, root)
        else:
            _svc = build_default_service(root)

    @app.command()
    def serve(
        address: Optional[str] = typer.Option(None, "--address", help="listen address"),
    ):
        """Start the HTTP API server."""
        svc = _get_svc()
        try:
            from knowledger.adapters.web.server import run_server
            run_server(svc, address=address)
        except ImportError:
            typer.echo("web adapter not available", err=True)
            raise typer.Exit(1)

    @app.command()
    def mcp():
        """Start the MCP stdio server."""
        svc = _get_svc()
        try:
            from knowledger.adapters.mcp.server import run_stdio
            run_stdio(svc)
        except ImportError:
            typer.echo("mcp adapter not available", err=True)
            raise typer.Exit(1)

    @app.command()
    def add(
        kb: str = typer.Option(..., "--kb", help="knowledge base id"),
        scope: str = typer.Option("", "--scope", help="scope: project or global"),
        tags: list[str] = typer.Option([], "--tag", help="item tag (repeat for multiple)"),
        title: str = typer.Argument(...),
        content: Optional[str] = typer.Argument(None),
    ):
        """Add a knowledge item."""
        svc = _get_svc()
        effective = _effective_scope(scope)
        body = content if content is not None else sys.stdin.read()
        item, _, _ = svc.add(AddInput(kb_id=kb, scope=effective, title=title, content=body, tags=tags))
        typer.echo(item.id)

    @app.command()
    def search(
        kb: Optional[str] = typer.Option(None, "--kb", help="knowledge base id"),
        scope: str = typer.Option("", "--scope"),
        limit: int = typer.Option(10, "--limit"),
        mode: str = typer.Option("", "--mode", help="search mode"),
        query: str = typer.Argument(...),
    ):
        """Search knowledge."""
        svc = _get_svc()
        refs: list[ScopedKBRef] = []
        if kb:
            effective = _effective_scope(scope)
            refs = [ScopedKBRef(scope=effective, id=kb)]
        result = svc.search(SearchOptions(query=query, limit=limit, kb_ids=refs, search_mode=mode))
        for hit in result.hits:
            typer.echo(f"{hit.score:.3f} [{hit.scope}:{hit.kb_id}] {hit.title}")
            typer.echo(f"  {hit.snippet}")

    @app.command()
    def get(
        kb: str = typer.Option(..., "--kb"),
        scope: str = typer.Option("", "--scope"),
        item_id: str = typer.Argument(...),
    ):
        """Get a knowledge item."""
        svc = _get_svc()
        effective = _effective_scope(scope)
        item = svc.get_knowledge_item(effective, kb, item_id)
        typer.echo(item.content)

    @app.command()
    def delete(
        kb: str = typer.Option(..., "--kb"),
        scope: str = typer.Option("", "--scope"),
        item_id: str = typer.Argument(...),
    ):
        """Delete a knowledge item."""
        svc = _get_svc()
        effective = _effective_scope(scope)
        svc.delete_knowledge_item(effective, kb, item_id)

    @app.command(name="list-items")
    def list_items(
        kb: str = typer.Option(..., "--kb"),
        scope: str = typer.Option("", "--scope"),
        limit: int = typer.Option(0, "--limit"),
        offset: int = typer.Option(0, "--offset"),
    ):
        """List items in a knowledge base."""
        svc = _get_svc()
        effective = _effective_scope(scope)
        items = svc.list_knowledge_items(effective, kb)
        if offset:
            items = items[offset:]
        if limit:
            items = items[:limit]
        for item in items:
            typer.echo(f"{item.id}\t{item.title}")

    @app.command()
    def index(
        kb: Optional[str] = typer.Option(None, "--kb"),
        scope: str = typer.Option("", "--scope"),
        rebuild: bool = typer.Option(False, "--rebuild"),
    ):
        """Run semantic indexing."""
        svc = _get_svc()
        effective = _effective_scope(scope) if not (not kb and not scope) else ""
        inp = IndexKnowledgeInput(
            scope=effective if kb else scope,
            kb_id=kb or "",
            rebuild=rebuild,
        )
        result = svc.index_knowledge(inp)
        for r in result.results:
            typer.echo(f"{r.scope}:{r.kb_id} indexed={r.result.indexed} skipped={r.result.skipped} deleted={r.result.deleted}")
        for w in result.warnings:
            typer.echo(f"warning: {w}", err=True)

    @kbs_app.callback(invoke_without_command=True)
    def kbs_list(ctx: typer.Context):
        """List knowledge bases."""
        if ctx.invoked_subcommand is not None:
            return
        svc = _get_svc()
        summaries = svc.list_knowledge_base_summaries()
        for s in summaries:
            kb = s.record.knowledge_base
            typer.echo(f"{kb.scope}:{kb.id}\t{kb.store_type}\titems={s.item_count}\t{'enabled' if kb.enabled else 'disabled'}")

    @kbs_app.command(name="create")
    def kbs_create(
        id: str = typer.Argument(...),
        type: str = typer.Option(..., "--type", help="store type: text or sqlite"),
        path: str = typer.Option("", "--path"),
        scope: str = typer.Option("", "--scope"),
        name: str = typer.Option("", "--name"),
        tags: list[str] = typer.Option([], "--tag"),
        semantic_enabled: Optional[bool] = typer.Option(None, "--semantic-enabled"),
    ):
        """Create a knowledge base."""
        svc = _get_svc()
        effective = _effective_scope(scope)
        inp = CreateKnowledgeBaseInput(
            scope=effective, id=id, name=name, store_type=type,
            path=path, tags=tags, semantic_enabled=semantic_enabled,
        )
        record = svc.create_knowledge_base(inp)
        kb = record.knowledge_base
        typer.echo(f"created {kb.scope}:{kb.id}")

    @kbs_app.command(name="delete")
    def kbs_delete(
        id: str = typer.Argument(...),
        scope: str = typer.Option("", "--scope"),
    ):
        """Delete a knowledge base."""
        svc = _get_svc()
        effective = _effective_scope(scope)
        svc.delete_knowledge_base(effective, id)

    @install_app.callback(invoke_without_command=True)
    def install_callback(ctx: typer.Context):
        if ctx.invoked_subcommand is None:
            typer.echo("specify a subcommand: claude", err=True)
            raise typer.Exit(1)

    @install_app.command(name="claude")
    def install_claude():
        """Install Claude MCP config."""
        try:
            from knowledger.install.claude import install
            install()
        except ImportError:
            typer.echo("claude installer not available", err=True)
            raise typer.Exit(1)

    return app
