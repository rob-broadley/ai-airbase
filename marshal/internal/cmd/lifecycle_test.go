// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd

import "testing"

func TestIsRegistryImage(t *testing.T) {
	tests := []struct {
		name  string
		image string
		want  bool
	}{
		{name: "bare name, local-only", image: "revetment", want: false},
		{name: "bare name with tag", image: "revetment:latest", want: false},
		{name: "simple registry ref", image: "myregistry/img", want: true},
		{name: "full registry ref with tag", image: "ghcr.io/org/img:latest", want: true},
		{name: "localhost registry", image: "localhost/img", want: true},
		{name: "localhost with port", image: "localhost:5000/img", want: true},
		{name: "empty string", image: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRegistryImage(tt.image); got != tt.want {
				t.Fatalf("isRegistryImage(%q) = %t, want %t", tt.image, got, tt.want)
			}
		})
	}
}
