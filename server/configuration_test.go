package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRejectionMessageOrDefault(t *testing.T) {
	cases := map[string]struct {
		in   string
		want string
	}{
		"empty returns default":      {"", defaultRejectionMessage},
		"whitespace returns default": {"   ", defaultRejectionMessage},
		"custom is returned as-is":   {"No DMs allowed here", "No DMs allowed here"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := &configuration{RejectionMessage: tc.in}
			assert.Equal(t, tc.want, c.rejectionMessageOrDefault())
		})
	}
}
