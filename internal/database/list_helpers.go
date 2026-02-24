package database

// IS NULL OR helpers — convert empty Go values to nil so PostgreSQL
// sees NULL and the ($1::type IS NULL OR ...) pattern skips the filter.

func pqIntArray(s []int) any {
	if len(s) == 0 {
		return nil
	}
	return s
}

func pqStringArray(s []string) any {
	if len(s) == 0 {
		return nil
	}
	return s
}

func pqString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
