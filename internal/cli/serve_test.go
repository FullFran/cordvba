package cli

import (
	"strings"
	"testing"
)

// eye reads sources under licences that in several cases permit personal use
// only, and the store holds an aircraft position history. Exposing that to a
// network has to be a decision somebody made, not a default.
func TestCheckBindRefusesNonLoopbackByDefault(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		addr    string
		public  bool
		wantErr bool
	}{
		{name: "loopback ipv4", addr: "127.0.0.1:8787"},
		{name: "loopback ipv6", addr: "[::1]:8787"},
		{name: "localhost", addr: "localhost:8787"},
		{name: "all interfaces", addr: "0.0.0.0:8787", wantErr: true},
		{name: "all interfaces ipv6", addr: "[::]:8787", wantErr: true},
		{name: "empty host means all interfaces", addr: ":8787", wantErr: true},
		{name: "a real address", addr: "192.168.1.20:8787", wantErr: true},
		{name: "a hostname", addr: "eye.local:8787", wantErr: true},
		{name: "all interfaces with --public", addr: "0.0.0.0:8787", public: true},
		{name: "a real address with --public", addr: "192.168.1.20:8787", public: true},
		{name: "malformed", addr: "not-an-address", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := checkBind(tc.addr, tc.public)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("checkBind(%q, %v) = nil, want a refusal", tc.addr, tc.public)
				}
				// The refusal has to say how to proceed on purpose.
				if !strings.Contains(err.Error(), "--public") && !strings.Contains(err.Error(), "addr") {
					t.Errorf("error = %q, want it to name the way through", err)
				}
				return
			}
			if err != nil {
				t.Errorf("checkBind(%q, %v) = %v", tc.addr, tc.public, err)
			}
		})
	}
}
