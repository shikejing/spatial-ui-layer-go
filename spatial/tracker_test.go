package spatial

import "testing"

func TestTrackerRejectsOutOfOrderAndDuplicate(t *testing.T) {
	tr := NewTracker()
	p1 := Pose{Position: Vec3{1, 0, 0}, Orientation: IdentityQuat()}
	p2 := Pose{Position: Vec3{2, 0, 0}, Orientation: IdentityQuat()}

	if !tr.Submit(AnchorHead, p1, 100) {
		t.Fatal("first sample should be accepted")
	}
	if tr.Submit(AnchorHead, p2, 100) {
		t.Fatal("duplicate timestamp must be rejected")
	}
	if tr.Submit(AnchorHead, p2, 99) {
		t.Fatal("older timestamp must be rejected")
	}
	got, ok := tr.Latest(AnchorHead)
	if !ok || got.Position.X != 1 {
		t.Fatalf("stale sample overwrote pose: %+v", got)
	}
	if !tr.Submit(AnchorHead, p2, 101) {
		t.Fatal("newer sample should be accepted")
	}
	got, _ = tr.Latest(AnchorHead)
	if got.Position.X != 2 {
		t.Fatalf("new sample not applied: %+v", got)
	}
}

func TestTrackerSourcesAreIndependent(t *testing.T) {
	tr := NewTracker()
	tr.Submit(AnchorHead, Pose{Position: Vec3{1, 0, 0}}, 50)
	// 手柄源时间戳独立计数，不受头显影响。
	if !tr.Submit(AnchorControllerRight, Pose{Position: Vec3{5, 0, 0}}, 10) {
		t.Fatal("independent source timeline should accept its own timestamps")
	}
}
