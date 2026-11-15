package utils

import "regexp"

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// IsUuid reports whether s is formatted like a UUID (8-4-4-4-12 hex
// digits, any version), so callers can tell an ID from a name.
func IsUuid(s string) bool { return uuidPattern.MatchString(s) }
