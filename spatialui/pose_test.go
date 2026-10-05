package spatialui

import "testing"

func sample(ts int64, x float64) PoseSample {
	return PoseSample{
		Pose:      Pose{Position: Vec3{X: x}, Orientation: QuatIdentity()},
		Timestamp: ts,
	}
}

// 乱序与重复输入：旧帧与重复帧不得覆盖新姿态。
func TestPoseTrackerOutOfOrderAndDuplicate(t *testing.T) {
	tr := NewPoseTracker()
	if !tr.Submit(TrackHead, sample(10, 1)) {
		t.Fatal("first sample should be accepted")
	}
	if tr.Submit(TrackHead, sample(5, 99)) {
		t.Fatal("older sample must be rejected")
	}
	if tr.Submit(TrackHead, sample(10, 42)) {
		t.Fatal("duplicate timestamp must be rejected")
	}
	if !tr.Submit(TrackHead, sample(11, 2)) {
		t.Fatal("newer sample should be accepted")
	}
	s := tr.Snapshot()
	if s.Head.Pose.Position.X != 2 || s.Head.Timestamp != 11 {
		t.Fatalf("snapshot = %+v, want latest sample", s.Head)
	}
}

// 各路追踪源互不影响。
func TestPoseTrackerIndependentTracks(t *testing.T) {
	tr := NewPoseTracker()
	tr.Submit(TrackHead, sample(10, 1))
	tr.Submit(TrackHandLeft, sample(3, 7))
	tr.Submit(TrackHead, sample(20, 2))
	s := tr.Snapshot()
	if s.Head.Pose.Position.X != 2 {
		t.Fatalf("head = %v, want 2", s.Head.Pose.Position.X)
	}
	if s.HandLeft.Pose.Position.X != 7 || s.HandLeft.Timestamp != 3 {
		t.Fatalf("hand = %+v, want ts=3 x=7", s.HandLeft)
	}
}

// 端到端：乱序姿态提交后，发布的快照必须对应最新已提交姿态。
func TestEngineOutOfOrderPoseEndToEnd(t *testing.T) {
	e := NewEngine()
	e.AddPanel(Panel{ID: "hud", Anchor: AnchorHead,
		LocalOffset: Pose{Position: Vec3{Z: -1}, Orientation: QuatIdentity()}})
	e.SubmitPose(TrackHead, sample(100, 0))
	e.SubmitPose(TrackHead, sample(50, 9)) // 乱序旧帧，应被拒绝
	s := e.Tick()
	p, _ := placedByID(s, "hud")
	if p.Pose.Position.X != 0 {
		t.Fatalf("panel x = %v, stale pose leaked into snapshot", p.Pose.Position.X)
	}
	if s.Poses.Head.Timestamp != 100 {
		t.Fatalf("snapshot pose ts = %d, want 100", s.Poses.Head.Timestamp)
	}
}
