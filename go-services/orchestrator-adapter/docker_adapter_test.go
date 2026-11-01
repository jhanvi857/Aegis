package orchestrator

import "testing"

func TestDockerTargetName(t *testing.T) {
	tests := []struct {
		name   string
		target string
		want   string
	}{
		{name: "plain service", target: "inventory", want: "inventory"},
		{name: "namespace qualified service", target: "default/inventory", want: "inventory"},
		{name: "trimmed namespace qualified service", target: "  default/inventory  ", want: "inventory"},
		{name: "empty target", target: " ", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dockerTargetName(tt.target); got != tt.want {
				t.Fatalf("dockerTargetName(%q) = %q, want %q", tt.target, got, tt.want)
			}
		})
	}
}
