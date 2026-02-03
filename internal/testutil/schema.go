package testutil

import "github.com/trunk-recorder/tr-engine/internal/database"

// TestSchema returns the schema SQL for testing.
// This uses the same schema as production (from migrations).
func TestSchema() string {
	schema, err := database.Schema()
	if err != nil {
		panic("failed to load schema: " + err.Error())
	}
	return schema
}
