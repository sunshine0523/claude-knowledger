import unicodedata


def tokenize_query(query: str) -> list[str]:
    """
    Split a search query into OR-able tokens.

    Separators are whitespace, punctuation, and symbols.
    Word characters include letters, digits, and underscore.
    Result preserves first-occurrence order and is de-duplicated.
    """
    if not query:
        return []

    out = []
    seen = set()
    buf = []

    def flush():
        if not buf:
            return
        tok = "".join(buf)
        buf.clear()
        if tok not in seen:
            seen.add(tok)
            out.append(tok)

    for char in query:
        if char == "_":
            buf.append(char)
            continue

        category = unicodedata.category(char)
        # Zs=space, P*=punctuation, S*=symbol
        if category.startswith(("Z", "P", "S")):
            flush()
            continue

        buf.append(char)

    flush()
    return out
