package cmd

import "testing"

// TestSelectedControlPlane covers the switch that decides which control
// plane `serve` runs. The default must stay legacy: the rebuilt stack does
// not yet serve password auth, /health, /mcp or the assistant
// schema/subgraph endpoints, so defaulting to it would silently drop
// routes that deployments depend on.
func TestSelectedControlPlane(t *testing.T) {
	tests := []struct {
		name    string
		flag    string
		env     string
		want    string
		wantErr bool
	}{
		{name: "unset defaults to legacy", want: controlPlaneLegacy},
		{name: "explicit legacy", flag: "legacy", want: controlPlaneLegacy},
		{name: "explicit v2", flag: "v2", want: controlPlaneV2},
		{name: "case insensitive", flag: "V2", want: controlPlaneV2},
		{name: "surrounding space", flag: "  v2 ", want: controlPlaneV2},
		{name: "env selects v2", env: "v2", want: controlPlaneV2},
		{name: "flag beats env", flag: "legacy", env: "v2", want: controlPlaneLegacy},
		{name: "empty env is legacy", env: "", want: controlPlaneLegacy},

		// A typo must be loud. Falling back to legacy would leave the
		// operator watching the stack they were trying to replace, with
		// nothing in the output to explain why.
		{name: "unknown value errors", flag: "v3", wantErr: true},
		{name: "near miss errors", flag: "leagcy", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			old := controlPlaneFlag
			controlPlaneFlag = tc.flag
			defer func() { controlPlaneFlag = old }()
			t.Setenv("DURAGRAPH_CONTROL_PLANE", tc.env)

			got, err := selectedControlPlane()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want an error for %q, got %q", tc.flag, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
