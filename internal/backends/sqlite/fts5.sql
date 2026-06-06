CREATE VIRTUAL TABLE IF NOT EXISTS knowledge_items_fts USING fts5(
  title,
  content,
  content='knowledge_items',
  content_rowid='id'
);

CREATE TRIGGER IF NOT EXISTS knowledge_items_ai AFTER INSERT ON knowledge_items BEGIN
  INSERT INTO knowledge_items_fts(rowid, title, content) VALUES (new.id, new.title, new.content);
END;
