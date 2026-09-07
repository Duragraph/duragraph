package cron

import "encoding/json"

// mustJSON marshals a value that cannot fail to marshal (maps of strings).
func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}

// jsonOrEmpty defaults an absent payload to an empty object, matching the
// runs.input NOT NULL DEFAULT '{}' column.
func jsonOrEmpty(b []byte) []byte {
	if len(b) == 0 {
		return []byte("{}")
	}
	return b
}
