package brain

import "testing"

func TestNewValidatesReferencesAndAutonomy(t *testing.T) {
	for _, args := range [][4]string{
		{"", "p1", "a1", string(AutonomyManual)},
		{"b1", "", "a1", string(AutonomyManual)},
		{"b1", "p1", "", string(AutonomyManual)},
		{"b1", "p1", "a1", "INVALID"},
	} {
		if _, err := New(args[0], args[1], args[2], AutonomyMode(args[3])); err == nil {
			t.Fatalf("expected invalid Brain input %q to be rejected", args)
		}
	}
	brain, err := New("b1", "p1", "a1", AutonomySemiAuto)
	if err != nil {
		t.Fatal(err)
	}
	if !brain.Active || brain.Autonomy != AutonomySemiAuto {
		t.Fatalf("unexpected Brain defaults: %+v", brain)
	}
}
