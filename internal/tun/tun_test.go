package tun

import "testing"

func TestNormalizeDeviceName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
	}{
		{name: "", want: DefaultDeviceName},
		{name: "  ", want: DefaultDeviceName},
		{name: "tun10", want: "tun10"},
		{name: " mvpvpn1 ", want: "mvpvpn1"},
	}

	for _, tt := range tests {
		if got := normalizeDeviceName(tt.name); got != tt.want {
			t.Fatalf("normalizeDeviceName(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}
