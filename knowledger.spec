# -*- mode: python ; coding: utf-8 -*-
from PyInstaller.utils.hooks import collect_all

chromadb_datas, chromadb_binaries, chromadb_hiddenimports = collect_all("chromadb")

a = Analysis(
    ["knowledger/main.py"],
    pathex=["."],
    binaries=chromadb_binaries,
    datas=[
        ("plugins/claude-code-knowledger", "plugins/claude-code-knowledger"),
        *chromadb_datas,
    ],
    hiddenimports=[
        "knowledger.adapters.mcp.server",
        "knowledger.adapters.web.server",
        "knowledger.install.claude",
        *chromadb_hiddenimports,
    ],
    hookspath=[],
    runtime_hooks=[],
    excludes=[],
    noarchive=False,
)
pyz = PYZ(a.pure)
exe = EXE(
    pyz,
    a.scripts,
    a.binaries,
    a.datas,
    name="knowledger",
    debug=False,
    strip=False,
    upx=True,
    console=True,
    target_arch=None,
)
