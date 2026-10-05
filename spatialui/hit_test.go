package spatialui

import (
	"testing"
)

func mkPlaced(id PanelID, z float64) PlacedPanel {
	return PlacedPanel{
		Panel: Panel{ID: id, Size: Size{1, 1}},
		Pose:  Pose{Position: Vec3{Z: z}, Orientation: QuatIdentity()},
	}
}

// 被更近面板挡住的面板不得被选中。
func TestRaycastOcclusion(t *testing.T) {
	panels := []PlacedPanel{mkPlaced("near", -1), mkPlaced("far", -3)}
	ray := Ray{Origin: Vec3{}, Dir: Vec3{Z: -1}}
	h, ok := Raycast(ray, RayGaze, panels)
	if !ok {
		t.Fatal("expected hit")
	}
	if h.PanelID != "near" {
		t.Fatalf("hit %q, want occluding panel %q", h.PanelID, "near")
	}
	if h.Distance < 0.99 || h.Distance > 1.01 {
		t.Fatalf("distance = %v, want ~1", h.Distance)
	}
}

// 射线偏离面板时不命中；距离相同按 ID 字典序裁决。
func TestRaycastMissAndTieBreak(t *testing.T) {
	panels := []PlacedPanel{mkPlaced("p", -1)}
	ray := Ray{Origin: Vec3{X: 5}, Dir: Vec3{Z: -1}}
	if _, ok := Raycast(ray, RayGaze, panels); ok {
		t.Fatal("expected miss")
	}

	tie := []PlacedPanel{mkPlaced("b", -1), mkPlaced("a", -1)}
	h, ok := Raycast(Ray{Origin: Vec3{}, Dir: Vec3{Z: -1}}, RayGaze, tie)
	if !ok || h.PanelID != "a" {
		t.Fatalf("tie-break hit = %q, want %q", h.PanelID, "a")
	}
}

// 同一时刻最多一个焦点；候选需连续命中才切换，单帧抖动不跳变。
func TestFocusHysteresis(t *testing.T) {
	f := NewFocusManager() // SwitchFrames = 2
	hit := func(id PanelID) []HitResult {
		return []HitResult{{PanelID: id, Distance: 1, Source: RayGaze}}
	}

	if ev := f.Update(1, hit("a")); ev.Reason != FocusNone {
		t.Fatalf("frame1 reason = %v, want none (candidate not yet stable)", ev.Reason)
	}
	if ev := f.Update(2, hit("a")); ev.Reason != FocusAcquired || ev.Current != "a" {
		t.Fatalf("frame2 reason = %v current = %q, want acquired a", ev.Reason, ev.Current)
	}

	// 单帧抖动到 b 又回来：焦点必须保持在 a。
	f.Update(3, hit("b"))
	if ev := f.Update(4, hit("a")); ev.Reason != FocusNone {
		t.Fatalf("jitter caused event %v", ev.Reason)
	}
	if id, _ := f.Focused(); id != "a" {
		t.Fatalf("focus = %q after jitter, want a", id)
	}

	// b 连续两帧命中才切换。
	f.Update(5, hit("b"))
	ev := f.Update(6, hit("b"))
	if ev.Reason != FocusSwitched || ev.Previous != "a" || ev.Current != "b" {
		t.Fatalf("switch event = %+v", ev)
	}

	// 射线移开后需连续丢失才失去焦点。
	f.Update(7, nil)
	if ev := f.Update(8, nil); ev.Reason != FocusLost || ev.Previous != "b" {
		t.Fatalf("lose event = %+v", ev)
	}
	if _, has := f.Focused(); has {
		t.Fatal("focus should be cleared")
	}
}

// 手柄射线优先于凝视射线。
func TestHandRayPriority(t *testing.T) {
	f := NewFocusManager()
	f.SwitchFrames = 1
	hits := []HitResult{
		{PanelID: "gaze-target", Distance: 1, Source: RayGaze},
		{PanelID: "hand-target", Distance: 2, Source: RayHand},
	}
	ev := f.Update(1, hits)
	if ev.Current != "hand-target" {
		t.Fatalf("focus = %q, want hand ray target", ev.Current)
	}
}

// Cancel 只在面板确实持有焦点时产生事件。
func TestFocusCancel(t *testing.T) {
	f := NewFocusManager()
	f.SwitchFrames = 1
	f.Update(1, []HitResult{{PanelID: "a", Distance: 1}})

	if ev := f.Cancel(2, "other", FocusCancelledRemoved); ev.Reason != FocusNone {
		t.Fatalf("cancel of non-focused panel produced %v", ev.Reason)
	}
	ev := f.Cancel(2, "a", FocusCancelledRemoved)
	if ev.Reason != FocusCancelledRemoved || ev.Previous != "a" {
		t.Fatalf("cancel event = %+v", ev)
	}
	if _, has := f.Focused(); has {
		t.Fatal("focus should be cleared after cancel")
	}
}
