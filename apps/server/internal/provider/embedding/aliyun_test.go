package embedding

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	searchdomain "github.com/Actify/echonote/apps/server/internal/domain/search"
)

func TestAliyunEmbeddingContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/services/embeddings/text-embedding/text-embedding" || request.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("path=%q authorization=%q", request.URL.Path, request.Header.Get("Authorization"))
		}
		var body struct {
			Model string `json:"model"`
			Input struct {
				Texts []string `json:"texts"`
			} `json:"input"`
			Parameters struct {
				TextType  string `json:"text_type"`
				Dimension int    `json:"dimension"`
			} `json:"parameters"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Model != AliyunModel || body.Parameters.TextType != "document" || body.Parameters.Dimension != AliyunDimensions || len(body.Input.Texts) != 2 {
			t.Errorf("body=%+v", body)
		}
		first, second := make([]float32, AliyunDimensions), make([]float32, AliyunDimensions)
		first[0], second[1] = 1, 1
		_ = json.NewEncoder(response).Encode(map[string]any{"output": map[string]any{"embeddings": []any{
			map[string]any{"embedding": second, "text_index": 1},
			map[string]any{"embedding": first, "text_index": 0},
		}}})
	}))
	defer server.Close()
	provider, err := newAliyun(server.URL, "secret", server.Client(), true)
	if err != nil {
		t.Fatal(err)
	}
	vectors, err := provider.Embed(context.Background(), []string{"first", "second"}, searchdomain.EmbeddingDocument)
	if err != nil || len(vectors) != 2 || vectors[0][0] != 1 || vectors[1][1] != 1 {
		t.Fatalf("vectors=%v err=%v", len(vectors), err)
	}
}

func TestAliyunEmbeddingRejectsInvalidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte(`{"output":{"embeddings":[{"embedding":[1],"text_index":0}]}}`))
	}))
	defer server.Close()
	provider, err := newAliyun(server.URL, "secret", server.Client(), true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Embed(context.Background(), []string{"text"}, searchdomain.EmbeddingQuery); err == nil {
		t.Fatal("expected invalid dimensions to fail")
	}
}
