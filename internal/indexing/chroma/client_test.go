package chroma_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kindbrave/knowledger/internal/indexing/chroma"
)

func TestClientQueryPostsToCollectionEndpoint(t *testing.T) {
	var requestedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client := chroma.New(srv.URL)
	_, err := client.Query(context.Background(), "notes", "core", 3)
	if err != nil {
		t.Fatalf("Query returned error: %v", err)
	}
	if requestedPath != "/api/v1/collections/notes/query" {
		t.Fatalf("unexpected request path %q", requestedPath)
	}
}
