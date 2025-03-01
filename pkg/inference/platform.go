package inference

import (
	"runtime"
)

// Supported returns whether or not the current host platform is supported for
// inference operations. This support should expand over time.
func Supported() bool {
	return (runtime.GOOS == "windows" && runtime.GOARCH == "amd64") ||
		(runtime.GOOS == "darwin" && runtime.GOARCH == "arm64")
}
