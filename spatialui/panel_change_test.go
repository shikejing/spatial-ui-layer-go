package spatialui

import "testing"

// 正在被交互的面板被移除时，必须产生可区分的取消事件。
func TestRemoveFocusedPanelCancelsFocus(t *testing.T) {
	e := NewEngine()
	e.focus.SwitchFrames = 1
	e.AddPanel(Panel{ID: "target", Anchor: AnchorWorld, Size: Size{2, 2},
		LocalOffset: Pose{Position: Vec3{Z: -2}, Orientation: QuatIdentity()}})
	e.SubmitPose(TrackHead, sample(1, 0)) // 凝视射线沿 -Z
	s := e.Tick()
	if !s.HasFocus || s.Focus != "target" {
		t.Fatalf("expected focus on target, got %+v", s)
	}

	e.RemovePanel("target")
	s = e.Tick()
	if s.HasFocus {
		t.Fatal("focus must be cleared after removal")
	}
	found := false
	for _, ev := range s.Events {
		if ev.Reason == FocusCancelledRemoved && ev.Previous == "target" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing FocusCancelledRemoved event in %+v", s.Events)
	}
	// 面板确实被移除。
	if _, ok := placedByID(s, "target"); ok {
		t.Fatal("removed panel still placed")
	}
}

// 重新挂载持有焦点的面板：产生 FocusCancelledRemount，面板按新锚点放置。
func TestRemountFocusedPanel(t *testing.T) {
	e := NewEngine()
	e.focus.SwitchFrames = 1
	e.AddPanel(Panel{ID: "p", Anchor: AnchorWorld, Size: Size{2, 2},
		LocalOffset: Pose{Position: Vec3{Z: -2}, Orientation: QuatIdentity()}})
	e.SubmitPose(TrackHead, sample(1, 0))
	e.Tick()

	// 左手柄放到凝视射线之外，重挂到左手柄后面板不再被凝视命中。
	e.SubmitPose(TrackHandLeft, PoseSample{
		Pose:      Pose{Position: Vec3{X: 5, Y: 0, Z: 5}, Orientation: QuatIdentity()},
		Timestamp: 2,
	})
	e.Remount("p", AnchorHand, HandLeft)
	s := e.Tick()
	if s.HasFocus {
		t.Fatal("focus must be cancelled on remount")
	}
	found := false
	for _, ev := range s.Events {
		if ev.Reason == FocusCancelledRemount && ev.Previous == "p" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing FocusCancelledRemount in %+v", s.Events)
	}
	// 重挂后面板跟随左手柄：手柄位姿 * 原局部偏移。
	p, ok := placedByID(s, "p")
	if !ok {
		t.Fatal("remounted panel not placed")
	}
	want := Vec3{X: 5, Y: 0, Z: 3}
	if d := p.Pose.Position.Sub(want).Len(); d > 1e-9 {
		t.Fatalf("remounted panel at %+v, want %+v", p.Pose.Position, want)
	}
}

// 运行期新增面板立即参与布局与命中。
func TestAddPanelAtRuntime(t *testing.T) {
	e := NewEngine()
	e.focus.SwitchFrames = 1
	e.SubmitPose(TrackHead, sample(1, 0))
	e.Tick()
	e.AddPanel(Panel{ID: "late", Anchor: AnchorWorld, Size: Size{1, 1},
		LocalOffset: Pose{Position: Vec3{Z: -1}, Orientation: QuatIdentity()}})
	s := e.Tick()
	if _, ok := placedByID(s, "late"); !ok {
		t.Fatal("runtime-added panel not placed")
	}
	if !s.HasFocus || s.Focus != "late" {
		t.Fatalf("runtime-added panel should be focusable, focus = %q", s.Focus)
	}
}
