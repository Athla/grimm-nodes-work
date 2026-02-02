package edges

import "github.com/google/uuid"

// Connection and boundary data
type Edge struct {
	Id     string
	Source uuid.UUID
	Target uuid.UUID
	Type   string
	Label  string
}
