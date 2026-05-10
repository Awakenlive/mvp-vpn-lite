package packet

import "testing"

func TestChecksum(t *testing.T) {
	t.Parallel()

	got := Checksum([]byte{0x00, 0x01})
	if got != 0xfffe {
		t.Fatalf("Checksum() = %#04x, want 0xfffe", got)
	}

	got = Checksum([]byte{0x01, 0x02, 0x03})
	if got != 0xfbfd {
		t.Fatalf("Checksum() with odd length = %#04x, want 0xfbfd", got)
	}
}
