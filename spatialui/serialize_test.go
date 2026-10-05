package spatialui

import (
	"testing"
)

// 序列化往返：结果必须与产生它的那一帧严格对应。
func TestSnapshotRoundTrip(t *testing.T) {
	e := NewEngine()
	e.focus.SwitchFrames = 1
	e.AddPanel(Panel{ID: "a", Anchor: AnchorWorld, Size: Size{1, 1},
		LocalOffset: Pose{Position: Vec3{Z: -2}, Orientation: QuatIdentity()}})
	e.AddPanel(Panel{ID: "b", Anchor: AnchorHead, Size: Size{0.5, 0.5},
		LocalOffset: Pose{Position: Vec3{X: 0.3, Z: -1}, Orientation: QuatIdentity()}})
	e.SubmitPose(TrackHead, sample(7, 1))
	s := e.Tick()

	data, err := MarshalSnapshot(s)
	if err != nil {
		t.Fatal(err)
	}
	back, err := UnmarshalSnapshot(data)
	if err != nil {
		t.Fatal(err)
	}
	if back.Frame != s.Frame || back.HasFocus != s.HasFocus || back.Focus != s.Focus {
		t.Fatalf("frame/focus mismatch: %+v vs %+v", back, s)
	}
	if len(back.Placed) != len(s.Placed) {
		t.Fatalf("placed count %d, want %d", len(back.Placed), len(s.Placed))
	}
	for i := range s.Placed {
		if back.Placed[i] != s.Placed[i] {
			t.Fatalf("panel %d mismatch:\n%+v\n%+v", i, back.Placed[i], s.Placed[i])
		}
	}
	if back.Poses != s.Poses {
		t.Fatal("poses mismatch")
	}
}

// 同一快照序列化两次，字节必须完全一致（顺序稳定）。
func TestMarshalDeterministic(t *testing.T) {
	e := NewEngine()
	e.AddPanel(Panel{ID: "x", Anchor: AnchorWorld})
	e.AddPanel(Panel{ID: "a", Anchor: AnchorWorld})
	s := e.Tick()
	b1, _ := MarshalSnapshot(s)
	b2, _ := MarshalSnapshot(s)
	if string(b1) != string(b2) {
		t.Fatal("marshal not deterministic")
	}
	// 面板按 ID 排序。
	if s.Placed[0].Panel.ID != "a" {
		t.Fatalf("placed order = %q, want sorted by ID", s.Placed[0].Panel.ID)
	}
}

// v1 旧格式（手写夹具）必须能读取并迁移为等价布局与焦点。
func TestUnmarshalV1Migration(t *testing.T) {
	v1 := []byte(`{
	  "frame": 42,
	  "poses": {
	    "Head": {"Pose": {"Position": {"X":1,"Y":2,"Z":3}, "Orientation": {"W":1}}, "Timestamp": 9, "Seq": 0},
	    "HandLeft":  {"Pose": {"Position": {}, "Orientation": {}}, "Timestamp": 0, "Seq": 0},
	    "HandRight": {"Pose": {"Position": {}, "Orientation": {}}, "Timestamp": 0, "Seq": 0}
	  },
	  "panels": [
	    {"panel": {"id": "legacy", "anchor": 0, "hand": 0,
	      "localOffset": {"Position": {"Z": -2}, "Orientation": {"W": 1}},
	      "size": {"Width": 1, "Height": 1}, "group": ""},
	     "pose": {"Position": {"Z": -2}, "Orientation": {"W": 1}}}
	  ],
	  "focusPanel": "legacy"
	}`)
	s, err := UnmarshalSnapshot(v1)
	if err != nil {
		t.Fatal(err)
	}
	if s.Frame != 42 {
		t.Fatalf("frame = %d, want 42", s.Frame)
	}
	if !s.HasFocus || s.Focus != "legacy" {
		t.Fatalf("focus = %q has=%v, want legacy", s.Focus, s.HasFocus)
	}
	if len(s.Placed) != 1 || s.Placed[0].Panel.ID != "legacy" {
		t.Fatalf("placed = %+v", s.Placed)
	}
	if s.Placed[0].Panel.Order != 0 {
		t.Fatal("v1 panels must migrate to Order=0")
	}
	if s.Poses.Head.Pose.Position.X != 1 || s.Poses.Head.Timestamp != 9 {
		t.Fatalf("poses migrated wrong: %+v", s.Poses.Head)
	}

	// 迁移后的快照可再次以当前版本序列化并读回，布局与焦点等价。
	data, err := MarshalSnapshot(s)
	if err != nil {
		t.Fatal(err)
	}
	s2, err := UnmarshalSnapshot(data)
	if err != nil {
		t.Fatal(err)
	}
	if s2.Frame != s.Frame || s2.Focus != s.Focus || s2.HasFocus != s.HasFocus ||
		len(s2.Placed) != 1 || s2.Placed[0] != s.Placed[0] {
		t.Fatal("migrated snapshot not equivalent after v2 round-trip")
	}
}

// 未知版本必须报错而不是静默误读。
func TestUnmarshalUnknownVersion(t *testing.T) {
	if _, err := UnmarshalSnapshot([]byte(`{"version": 99}`)); err == nil {
		t.Fatal("expected error for unknown version")
	}
	if _, err := UnmarshalSnapshot([]byte(`not json`)); err == nil {
		t.Fatal("expected error for invalid json")
	}
}
