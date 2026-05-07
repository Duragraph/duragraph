package cmd

import "os"

// setIfUnset writes value to the named environment variable only when
// the variable is not already set. Used by `dev` to apply embedded-mode
// defaults without trampling explicit operator overrides — an operator
// running `DB_MODE=external duragraph dev` keeps their external DB.
//
// "Unset" here means literally empty (matches Go's os.Getenv contract,
// which returns "" for both unset and explicitly-empty vars). An
// explicitly-empty override (e.g. DB_MODE="") will be replaced — that's
// the same behavior config.Load applies, so the two stay consistent.
func setIfUnset(key, value string) {
	if os.Getenv(key) == "" {
		os.Setenv(key, value)
	}
}
