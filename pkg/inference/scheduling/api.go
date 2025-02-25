package scheduling

import (
	"strings"
)

// trimRequestPathToOpenAIRoot trims a request path to start at the first
// instance of /v1/ to appear in the path.
func trimRequestPathToOpenAIRoot(path string) string {
	index := strings.Index(path, "/v1/")
	if index == -1 {
		return path
	}
	return path[index:]
}
