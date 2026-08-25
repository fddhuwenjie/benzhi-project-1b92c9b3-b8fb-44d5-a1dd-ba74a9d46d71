package dispatch

func flattenCategories(categories map[string][]string) []string {
	var measures []string
	for _, list := range categories {
		measures = append(measures, list...)
	}
	return measures
}
