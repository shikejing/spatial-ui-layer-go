package spatial

import (
	"bytes"
	"testing"
)

func buildSerializableFrame(t *testing.T) *Engine {
	t.Helper()
	e := newEngineWithHeadAnchor()
	e.SetFocusDwell(1)
	e.AddPanel(Panel{ID: "a", Anchor: "head", Group: "g", Order: 0, Size: Vec2{0.5, 0.4}, ZOffset: 0.01})
	e.AddPanel(Panel{ID: "b", Anchor: "room", Group: "g", Order: 1, Size: Vec2{1, 1}})
	e.SubmitPose(AnchorHead, Pose{Position: Vec3{0, 1.6, 0}, Orientation: IdentityQuat()}, 7)
	e.Commit()
	// 让 b 获得焦点。
	var pos Vec3
	for _, st := range e.Snapshot().Panels {
		if st.Panel.ID == "b" {
			pos = st.World.Position
		}
	}
	e.CastRay(RayGaze, Ray{Origin: Add(pos, Vec3{0, 0, 2}), Direction: Vec3{0, 0, -1}})
	return e
}

// 序列化往返：还原出的帧与原帧等价。
func TestSerializeRoundTrip(t *testing.T) {
	e := buildSerializableFrame(t)
	f := e.Snapshot()
	data, err := MarshalFrame(f)
	if err != nil {
		t.Fatal(err)
	}
	back, err := UnmarshalFrame(data)
	if err != nil {
		t.Fatal(err)
	}
	if back.Seq != f.Seq || back.HasFocus != f.HasFocus || back.Focused != f.Focused {
		t.Fatalf("header mismatch: %+v vs %+v", back, f)
	}
	if len(back.Panels) != len(f.Panels) || len(back.Poses) != len(f.Poses) {
		t.Fatal("content length mismatch")
	}
	for i := range f.Panels {
		if f.Panels[i] != back.Panels[i] {
			t.Fatalf("panel %d mismatch:\n%+v\n%+v", i, f.Panels[i], back.Panels[i])
		}
	}
	for k, v := range f.Poses {
		got := back.Poses[k]
		// Seq 是进程内单调序号，不参与序列化；只比较姿态与时间戳。
		if got.Pose != v.Pose || got.Timestamp != v.Timestamp {
			t.Fatalf("pose %v mismatch: %+v vs %+v", k, got, v)
		}
	}
}

// 序列化顺序稳定：同一帧写两次字节完全一致。
func TestSerializeDeterministic(t *testing.T) {
	e := buildSerializableFrame(t)
	f := e.Snapshot()
	d1, _ := MarshalFrame(f)
	d2, _ := MarshalFrame(f)
	if !bytes.Equal(d1, d2) {
		t.Fatal("serialization not deterministic")
	}
}

// v1 快照（无 ZOffset / HasFocus）升级后还原出等价布局与焦点。
func TestSerializeV1Compat(t *testing.T) {
	v1 := []byte(`{
	  "version": 1,
	  "seq": 42,
	  "poses": {"1": {"pos": [0, 1.6, 0], "rot": [1, 0, 0, 0], "ts": 7}},
	  "panels": [
	    {"id": "b", "anchor": "room", "group": "g", "order": 1, "size": [1, 1],
	     "world": {"pos": [0.5, 0, -2], "rot": [1, 0, 0, 0]},
	     "local": {"pos": [0.5, 0, 0], "rot": [1, 0, 0, 0]}},
	    {"id": "a", "anchor": "head", "group": "g", "order": 0, "size": [0.5, 0.4],
	     "world": {"pos": [-0.5, 1.6, 0], "rot": [1, 0, 0, 0]},
	     "local": {"pos": [-0.5, 0, 0], "rot": [1, 0, 0, 0]}}
	  ],
	  "focused": "a"
	}`)
	f, err := UnmarshalFrame(v1)
	if err != nil {
		t.Fatal(err)
	}
	if f.Seq != 42 || !f.HasFocus || f.Focused != "a" {
		t.Fatalf("v1 migration lost focus/seq: %+v", f)
	}
	if len(f.Panels) != 2 || f.Panels[0].Panel.ID != "a" {
		t.Fatalf("v1 panels not restored/sorted: %+v", f.Panels)
	}
	if f.Panels[0].Panel.ZOffset != 0 {
		t.Fatal("v1 ZOffset should default to 0")
	}
	if f.Poses[AnchorHead].Timestamp != 7 {
		t.Fatal("v1 pose timestamp lost")
	}
	// 升级后的帧可再次以 v2 写出并被 v2 读取，结果等价。
	data, err := MarshalFrame(f)
	if err != nil {
		t.Fatal(err)
	}
	again, err := UnmarshalFrame(data)
	if err != nil {
		t.Fatal(err)
	}
	if again.Seq != f.Seq || again.Focused != f.Focused || len(again.Panels) != len(f.Panels) {
		t.Fatal("upgraded frame not stable under v2 round-trip")
	}
}

// 未知版本必须报错而不是静默误读。
func TestSerializeUnknownVersion(t *testing.T) {
	if _, err := UnmarshalFrame([]byte(`{"version": 99}`)); err == nil {
		t.Fatal("expected error for unknown version")
	}
}
