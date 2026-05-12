package envconfig

import (
	"testing"
	"time"
)

func TestStringUsesEnvWhenSet(t *testing.T) {
	t.Setenv("MVPVPN_TEST_STRING", "")

	if got := String("MVPVPN_TEST_STRING", "fallback"); got != "" {
		t.Fatalf("String() = %q, want empty env value", got)
	}
	if got := String("MVPVPN_TEST_MISSING", "fallback"); got != "fallback" {
		t.Fatalf("String(missing) = %q, want fallback", got)
	}
}

func TestBoolParsesEnv(t *testing.T) {
	t.Setenv("MVPVPN_TEST_BOOL", "true")

	got, err := Bool("MVPVPN_TEST_BOOL", false)
	if err != nil {
		t.Fatalf("Bool() error = %v", err)
	}
	if !got {
		t.Fatal("Bool() = false, want true")
	}
}

func TestBoolRejectsInvalidEnv(t *testing.T) {
	t.Setenv("MVPVPN_TEST_BOOL_INVALID", "sometimes")

	if _, err := Bool("MVPVPN_TEST_BOOL_INVALID", false); err == nil {
		t.Fatal("Bool() error = nil, want error")
	}
}

func TestIntParsesEnv(t *testing.T) {
	t.Setenv("MVPVPN_TEST_INT", "42")

	got, err := Int("MVPVPN_TEST_INT", 7)
	if err != nil {
		t.Fatalf("Int() error = %v", err)
	}
	if got != 42 {
		t.Fatalf("Int() = %d, want 42", got)
	}
}

func TestUintParsesHexEnv(t *testing.T) {
	t.Setenv("MVPVPN_TEST_UINT", "0x4d56")

	got, err := Uint("MVPVPN_TEST_UINT", 0)
	if err != nil {
		t.Fatalf("Uint() error = %v", err)
	}
	if got != 0x4d56 {
		t.Fatalf("Uint() = %#x, want 0x4d56", got)
	}
}

func TestDurationParsesEnv(t *testing.T) {
	t.Setenv("MVPVPN_TEST_DURATION", "1500ms")

	got, err := Duration("MVPVPN_TEST_DURATION", time.Second)
	if err != nil {
		t.Fatalf("Duration() error = %v", err)
	}
	if got != 1500*time.Millisecond {
		t.Fatalf("Duration() = %s, want 1500ms", got)
	}
}
