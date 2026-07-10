//go:build windows

package rag

// KnowledgeBase represents a named collection of embedded text chunks.
type KnowledgeBase struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	ModelDir   string `json:"modelDir"`
	LibraryID  string `json:"libraryId,omitempty"`
	ChunkCount int    `json:"chunkCount"`
	CreatedAt  string `json:"createdAt"`
}

// SearchResult is a single hit from a semantic search.
type SearchResult struct {
	ID         string            `json:"id"`
	Content    string            `json:"content"`
	Source     string            `json:"source"`   // document title / file name
	Position   int               `json:"position"` // zero-based chunk index within source
	Metadata   map[string]string `json:"metadata"`
	Similarity float32           `json:"similarity"`
}
