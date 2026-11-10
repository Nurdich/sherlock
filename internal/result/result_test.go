package result

import "testing"

func TestStatusString(t *testing.T) {
	cases := []Status{Claimed, Available, Unknown, Illegal, WAF}
	want := []string{"Claimed", "Available", "Unknown", "Illegal", "WAF"}
	for i, status := range cases {
		if status.String() != want[i] {
			t.Fatalf("status %q string = %q", status, status.String())
		}
	}
}

func TestResultString(t *testing.T) {
	plain := Result{Status: Claimed}
	if plain.String() != "Claimed" {
		t.Fatalf("plain = %q", plain.String())
	}
	ctx := "Timeout Error"
	with := Result{Status: Unknown, Context: &ctx}
	if with.String() != "Unknown (Timeout Error)" {
		t.Fatalf("with context = %q", with.String())
	}
}
