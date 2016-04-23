package utility

// Qualifiable means something can be qualified by some conditions
type Qualifiable interface {
	IsQualified(...interface{}) bool
}

// InStringSlice checks if the `src` string in the `slice`
func InStringSlice(slice []string, src string) bool {
	for _, a := range slice {
		if a == src {
			return true
		}
	}

	return false
}

// GetQualifiedItems gets items which are qualified by qualifier from the item collection
func GetQualifiedItems(src []Qualifiable, params ...interface{}) (int, []Qualifiable) {
	count := 0
	var qualifiedItems []Qualifiable

	for _, item := range src {
		if item.IsQualified(params...) {
			count++
			qualifiedItems = append(qualifiedItems, item)
		}
	}

	return count, qualifiedItems
}
