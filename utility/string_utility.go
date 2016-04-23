package utility

import (
	"strings"
)

// GetLastSplit get the last part of splits
func GetLastSplit(src, separator string) string {
	split := strings.Split(src, separator)
	return split[len(split)-1]
}
