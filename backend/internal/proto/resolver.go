package proto

import (
	"fmt"
	"io"
	"strings"

	"github.com/bufbuild/protocompile"
)

// mapResolver implements protocompile.Resolver by serving files from an in-memory map.
type mapResolver struct {
	files map[string][]byte
}

// FindFileByPath looks up a proto file by its path in the in-memory map.
func (r *mapResolver) FindFileByPath(path string) (protocompile.SearchResult, error) {
	content, ok := r.files[path]
	if !ok {
		return protocompile.SearchResult{}, fmt.Errorf("file %q not found", path)
	}

	return protocompile.SearchResult{
		Source: io.NopCloser(strings.NewReader(string(content))),
	}, nil
}
