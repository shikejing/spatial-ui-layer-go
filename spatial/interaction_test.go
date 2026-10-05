package spatial

import "testing"

// 遮挡：近处的面板挡住远处的面板，射线命中最近者。
func TestOcclusionNearestWins(t *testing.T) {
	states := []PanelState{
		{Panel: Panel{ID: "far", Size: Vec2{2, 2}}, World: Pose{Position: Vec3{0, 0, -5}, Orientation: IdentityQuat()}},
		{Panel: Panel{ID: "near", Size: Vec2{1, 1}}, World: Pose{Position: Vec3{0, 0, -2}, Orientation: IdentityQuat()}},
	}
	ray := Ray{Origin: Vec3{0, 0, 0}, Direction: Vec3{0, 0, -1}}
	hit, ok := HitTest(ray, states)
	if !ok || hit != "near" {
		t.Fatalf("expected near panel, got %q", hit)
	}
	// 射线偏离近面板但仍命中远面板时，远面板可被选中。
	ray2 := Ray{Origin: Vec3{0.8, 0, 0}, Direction: Vec3{0, 0, -1}}
	hit2, ok2 := HitTest(ray2, states)
	if !ok2 || hit2 != "far" {
		t.Fatalf("expected far panel, got %q", hit2)
	}
}

// 同一时刻最多一个焦点；焦点切换按滞留规则确定发生。
func TestFocusUniquenessAndSwitch(t *testing.T) {
	e := newEngineWithHeadAnchor()
	e.SetFocusDwell(2)
	e.AddPanel(Panel{ID: "left", Anchor: "room", Group: "g", Order: 0, Size: Vec2{1, 1}})
	e.AddPanel(Panel{ID: "right", Anchor: "room", Group: "g", Order: 1, Size: Vec2{1, 1}})
	e.Commit()
	f := e.Snapshot()
	var leftPos, rightPos Vec3
	for _, st := range f.Panels {
		if st.Panel.ID == "left" {
			leftPos = st.World.Position
		} else {
			rightPos = st.World.Position
		}
	}
	aim := func(target Vec3) Ray {
		return Ray{Origin: Vec3{0, 0, 2}, Direction: Normalize(Sub(target, Vec3{0, 0, 2}))}
	}
	// 第一次命中 left：dwell=1，尚未获得焦点。
	ev := e.CastRay(RayGaze, aim(leftPos))
	if len(ev) != 0 {
		t.Fatalf("focus should require dwell, got %v", ev)
	}
	// 第二次连续命中：获得焦点。
	ev = e.CastRay(RayGaze, aim(leftPos))
	if len(ev) != 1 || ev[0].Type != FocusGained || ev[0].Panel != "left" {
		t.Fatalf("expected FocusGained(left), got %v", ev)
	}
	if got, ok := e.Snapshot().Focused, e.Snapshot().HasFocus; !ok || got != "left" {
		t.Fatalf("snapshot focus mismatch: %v %v", got, ok)
	}
	// 切到 right：先 Lost 后 Gained，同帧有序。
	e.CastRay(RayGaze, aim(rightPos))
	ev = e.CastRay(RayGaze, aim(rightPos))
	if len(ev) != 2 || ev[0].Type != FocusLost || ev[0].Panel != "left" ||
		ev[1].Type != FocusGained || ev[1].Panel != "right" {
		t.Fatalf("expected Lost(left)+Gained(right), got %v", ev)
	}
	// 任意时刻快照里只有一个焦点。
	if e.Snapshot().Focused != "right" {
		t.Fatal("focus not unique/consistent in snapshot")
	}
}

// 抖动：命中在两个面板间交替时，候选不断被重置，焦点不跳变。
func TestJitterDoesNotFlipFocus(t *testing.T) {
	e := newEngineWithHeadAnchor()
	e.SetFocusDwell(3)
	e.AddPanel(Panel{ID: "left", Anchor: "room", Group: "g", Order: 0, Size: Vec2{1, 1}})
	e.AddPanel(Panel{ID: "right", Anchor: "room", Group: "g", Order: 1, Size: Vec2{1, 1}})
	e.Commit()
	f := e.Snapshot()
	var leftPos, rightPos Vec3
	for _, st := range f.Panels {
		if st.Panel.ID == "left" {
			leftPos = st.World.Position
		} else {
			rightPos = st.World.Position
		}
	}
	origin := Vec3{0, 0, 2}
	aim := func(target Vec3) Ray {
		return Ray{Origin: origin, Direction: Normalize(Sub(target, origin))}
	}
	// 让 left 先获得焦点。
	for i := 0; i < 3; i++ {
		e.CastRay(RayGaze, aim(leftPos))
	}
	if e.Snapshot().Focused != "left" {
		t.Fatal("setup: left should be focused")
	}
	// 交替命中 20 次：任何候选都达不到连续 3 帧，焦点必须保持 left。
	for i := 0; i < 20; i++ {
		if i%2 == 0 {
			e.CastRay(RayGaze, aim(rightPos))
		} else {
			e.CastRay(RayGaze, aim(leftPos))
		}
	}
	if got := e.Snapshot().Focused; got != "left" {
		t.Fatalf("jitter flipped focus to %q", got)
	}
}

// 命中判定基于已提交快照：姿态提交但未 Commit 时，命中仍对应旧帧。
func TestHitReflectsCommittedSnapshot(t *testing.T) {
	e := newEngineWithHeadAnchor()
	e.SetFocusDwell(1)
	e.AddPanel(Panel{ID: "p", Anchor: "head", Group: "g", Order: 0, Size: Vec2{1, 1}})
	e.SubmitPose(AnchorHead, Pose{Position: Vec3{0, 0, -3}, Orientation: IdentityQuat()}, 1)
	e.Commit()
	// 命中当前位置中心。
	ray := Ray{Origin: Vec3{0, 0, 0}, Direction: Vec3{0, 0, -1}}
	if ev := e.CastRay(RayGaze, ray); len(ev) != 1 || ev[0].Panel != "p" {
		t.Fatalf("expected hit on p, got %v", ev)
	}
	// 提交新姿态但不 Commit：命中结果必须仍对应旧快照。
	e.SubmitPose(AnchorHead, Pose{Position: Vec3{50, 0, -3}, Orientation: IdentityQuat()}, 2)
	hitFrame := e.Snapshot()
	if hitFrame.Panels[0].World.Position.X != 0 {
		t.Fatal("uncommitted pose leaked into snapshot")
	}
	e.Commit()
	if e.Snapshot().Panels[0].World.Position.X != 50 {
		t.Fatal("committed pose not reflected")
	}
}
