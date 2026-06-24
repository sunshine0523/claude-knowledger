from dataclasses import dataclass


@dataclass
class Chunk:
    text: str
    index: int
    total: int = 0


class Splitter:
    def __init__(self, chunk_size: int = 512, overlap: int = 64):
        self.chunk_size = chunk_size
        self.overlap = overlap

    def split(self, text: str) -> list[Chunk]:
        if not text:
            return []
        parts = []
        start = 0
        step = max(self.chunk_size - self.overlap, 1)
        while start < len(text):
            parts.append(text[start:start + self.chunk_size])
            start += step
        total = len(parts)
        return [Chunk(text=t, index=i, total=total) for i, t in enumerate(parts)]


def default_splitter() -> Splitter:
    return Splitter(chunk_size=512, overlap=64)
