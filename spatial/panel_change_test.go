package spatial

import "testing"

func setupFocusedPanel(t *testing.T) *Engine {
	t.Helper()
	e := newEngineWithHeadAnchor()
	e.SetFocusDwell(1)
	e.AddPanel(Panel{ID: "p", Anchor: "room", Group: "g", Order: 0, Size: Vec2{1, 1}})
	e.Commit()
	pos := e.Snapshot().Panels[0].World.Position
	ray := Ray{Origin: Add(pos, Vec3{0, 0, 2}), Direction: Vec3{0, 0, -1}}
	if ev := e.CastRay(RayGaze, ray); len(ev) != 1 || ev[0].Type != FocusGained {
		t.Fatalf("setup: expected FocusGained, got %v", ev)
	}
	return e
}

// 移除持有焦点的面板：必须产生 FocusCanceled，且快照中不再出现该面板与焦点。
func TestRemoveFocusedPanelCancels(t *testing.T) {
	e := setupFocusedPanel(t)
	events := e.RemovePanel("p")
	if len(events) != 1 || events[0].Type != FocusCanceled || events[0].Panel != "p" {
		t.Fatalf("expected FocusCanceled(p), got %v", events)
	}
	f := e.Snapshot()
	if f.HasFocus {
		t.Fatal("focus leaked after removal")
	}
	if len(f.Panels) != 0 {
		t.Fatal("removed panel still in snapshot")
	}
}

// 移除未持有焦点的面板：无取消事件。
func TestRemoveUnfocusedPanelSilent(t *testing.T) {
	e := newEngineWithHeadAnchor()
	e.AddPanel(Panel{ID: "x", Anchor: "room", Group: "g", Size: Vec2{1, 1}})
	e.Commit()
	if ev := e.RemovePanel("x"); len(ev) != 0 {
		t.Fatalf("unexpected events: %v", ev)
	}
}

// 重新挂载持有焦点的面板：先 FocusCanceled，随后面板出现在新锚点，
// 且可以经正常滞留重新获得焦点（交互状态可解释）。
func TestRemountCancelsThenRefocusable(t *testing.T) {
	e := setupFocusedPanel(t)
	e.AddAnchor(Anchor{ID: "elsewhere", Kind: AnchorWorld, Base: Pose{
		Position:    Vec3{10, 0, 0},
		Orientation: IdentityQuat(),
	}})
	events := e.RemountPanel("p", "elsewhere")
	if len(events) != 1 || events[0].Type != FocusCanceled || events[0].Panel != "p" {
		t.Fatalf("expected FocusCanceled(p), got %v", events)
	}
	f := e.Snapshot()
	if f.HasFocus {
		t.Fatal("focus should be cleared after remount")
	}
	if len(f.Panels) != 1 || f.Panels[0].World.Position.X != 10 {
		t.Fatalf("panel not re-resolved on new anchor: %+v", f.Panels)
	}
	// 重新瞄准新位置：面板可再次获得焦点。
	pos := f.Panels[0].World.Position
	ray := Ray{Origin: Add(pos, Vec3{0, 0, 2}), Direction: Vec3{0, 0, -1}}
	if ev := e.CastRay(RayGaze, ray); len(ev) != 1 || ev[0].Type != FocusGained || ev[0].Panel != "p" {
		t.Fatalf("panel should be refocusable after remount, got %v", ev)
	}
}

// 运行期新增面板立即可参与命中。
func TestAddedPanelInteractable(t *testing.T) {
	e := newEngineWithHeadAnchor()
	e.SetFocusDwell(1)
	e.AddPanel(Panel{ID: "n", Anchor: "room", Group: "g", Size: Vec2{1, 1}})
	e.Commit()
	pos := e.Snapshot().Panels[0].World.Position
	ray := Ray{Origin: Add(pos, Vec3{0, 0, 2}), Direction: Vec3{0, 0, -1}}
	if ev := e.CastRay(RayControllerLeft, ray); len(ev) != 1 || ev[0].Source != RayControllerLeft {
		t.Fatalf("new panel not interactable: %v", ev)
	}
}
