SCOPE_GLOBAL = "global"
SCOPE_PROJECT = "project"


def normalize_scope(s: str) -> str:
    normalized = s.strip().lower()
    if normalized in ("", SCOPE_GLOBAL):
        return SCOPE_GLOBAL
    if normalized == SCOPE_PROJECT:
        return SCOPE_PROJECT
    raise ValueError(f"unknown scope {s!r} (expected {SCOPE_GLOBAL!r} or {SCOPE_PROJECT!r})")
