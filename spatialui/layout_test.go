package spatialui

import (
	"math"
	"testing"
)

func headPose(x, y, z float64) PoseSample {
	return PoseSample{
		Pose:      Pose{Position: Vec3{X: x, Y: y, Z: z}, Orientation: QuatIdentity()},
		Timestamp: 1,
	}
}

func placedByID(s *Snapshot, id PanelID) (PlacedPanel, bool) {
	for _, p := range s.Placed {
		if p.Panel.ID == id {
			return p, true
		}
	}
	return PlacedPanel{}, false
}

// 头部锚定的面板必须随头部位姿稳定移动。
func TestHeadAnchoredPanelFollowsHead(t *testing.T) {
	e := NewEngine()
	e.AddPanel(Panel{ID: "hud", Anchor: AnchorHead, Size: Size{1, 1},
		LocalOffset: Pose{Position: Vec3{Z: -1}, Orientation: QuatIdentity()}})

	e.SubmitPose(TrackHead, headPose(0, 1.6, 0))
	s1 := e.Tick()
	p1, ok := placedByID(s1, "hud")
	if !ok {
		t.Fatal("panel not placed")
	}
	if d := p1.Pose.Position.Sub(Vec3{0, 1.6, -1}).Len(); d > 1e-9 {
		t.Fatalf("frame1 position off by %v", d)
	}

	// 头部移动后面板必须跟随，且偏移保持 -Z 1m。
	e.SubmitPose(TrackHead, PoseSample{
		Pose:      Pose{Position: Vec3{X: 2, Y: 1.6, Z: 3}, Orientation: QuatIdentity()},
		Timestamp: 2,
	})
	s2 := e.Tick()
	p2, _ := placedByID(s2, "hud")
	want := Vec3{2, 1.6, 2}
	if d := p2.Pose.Position.Sub(want).Len(); d > 1e-9 {
		t.Fatalf("frame2 position = %+v, want %+v", p2.Pose.Position, want)
	}
}

// 同一帧内所有面板必须使用同一份姿态快照：
// 头部锚定面板与世界锚定面板的相对关系由同一帧姿态决定。
func TestFrameConsistencyAcrossAnchors(t *testing.T) {
	e := NewEngine()
	e.AddPanel(Panel{ID: "world", Anchor: AnchorWorld,
		LocalOffset: Pose{Position: Vec3{X: 0, Y: 1, Z: -2}, Orientation: QuatIdentity()}})
	e.AddPanel(Panel{ID: "head", Anchor: AnchorHead,
		LocalOffset: Pose{Position: Vec3{Z: -1}, Orientation: QuatIdentity()}})

	e.SubmitPose(TrackHead, PoseSample{
		Pose:      Pose{Position: Vec3{X: 5, Y: 1, Z: 5}, Orientation: QuatIdentity()},
		Timestamp: 1,
	})
	s := e.Tick()
	w, _ := placedByID(s, "world")
	h, _ := placedByID(s, "head")
	rel := h.Pose.Position.Sub(w.Pose.Position)
	want := Vec3{5, 0, 6} // (5,1,4) - (0,1,-2)
	if d := rel.Sub(want).Len(); d > 1e-9 {
		t.Fatalf("relative offset = %+v, want %+v", rel, want)
	}
}

// 分批插入与一次性插入同一批面板，排布结果必须一致。
func TestLayoutDeterministicAcrossInsertionOrders(t *testing.T) {
	panels := []Panel{
		{ID: "a", Group: "g", Order: 2, Size: Size{1, 1}},
		{ID: "b", Group: "g", Order: 0, Size: Size{1, 1}},
		{ID: "c", Group: "g", Order: 1, Size: Size{1, 1}},
		{ID: "d", Group: "g", Order: 0, Size: Size{1, 1}}, // 与 b 同 Order，按 ID 裁决
	}
	layout := GridLayout{CellWidth: 1, CellHeight: 1, Columns: 2}

	build := func(order []int) *Snapshot {
		e := NewEngine()
		e.SetLayout("g", layout, AnchorWorld, HandLeft)
		for _, i := range order {
			e.AddPanel(panels[i])
			e.Tick() // 分批插入，每批后都推进帧
		}
		return e.Tick()
	}

	oneShot := build([]int{0, 1, 2, 3})
	staggered := build([]int{3, 1, 0, 2})
	if len(oneShot.Placed) != len(staggered.Placed) {
		t.Fatal("panel count mismatch")
	}
	for i := range oneShot.Placed {
		a, b := oneShot.Placed[i], staggered.Placed[i]
		if a.Panel.ID != b.Panel.ID || a.Pose != b.Pose {
			t.Fatalf("slot %d differs: %+v vs %+v", i, a, b)
		}
	}
}

// 三种排布策略的基本几何性质。
func TestLayoutStrategies(t *testing.T) {
	panels := []Panel{{ID: "p0"}, {ID: "p1"}, {ID: "p2"}}

	arc := ArcLayout{Radius: 2, AngleStep: math.Pi / 8}
	poses := arc.Arrange(panels)
	for i, p := range poses {
		r := math.Hypot(p.Position.X, p.Position.Z)
		if math.Abs(r-2) > 1e-9 {
			t.Fatalf("arc panel %d radius = %v, want 2", i, r)
		}
	}
	if poses[1].Position.X != 0 || poses[1].Position.Z != -2 {
		t.Fatalf("arc center panel at %+v, want (0,0,-2)", poses[1].Position)
	}

	grid := GridLayout{CellWidth: 0.5, CellHeight: 0.25, Columns: 2}
	poses = grid.Arrange(panels)
	if poses[0].Position.X != -0.25 || poses[1].Position.X != 0.25 {
		t.Fatalf("grid row not centered: %+v %+v", poses[0].Position, poses[1].Position)
	}
	if poses[2].Position.Y != -0.25 {
		t.Fatalf("grid second row y = %v, want -0.25", poses[2].Position.Y)
	}

	stack := StackLayout{Direction: Vec3{Y: -1}, Spacing: 0.3}
	poses = stack.Arrange(panels)
	if poses[2].Position.Y != -0.6 {
		t.Fatalf("stack panel 2 y = %v, want -0.6", poses[2].Position.Y)
	}
}

// 布局重算只应触及受影响的组：修改一组后另一组的缓存排布不得重算。
func TestRelayoutScopedToAffectedGroup(t *testing.T) {
	counting := &countingLayout{}
	e := NewEngine()
	e.SetLayout("g1", counting, AnchorWorld, HandLeft)
	e.SetLayout("g2", counting, AnchorWorld, HandLeft)
	e.AddPanel(Panel{ID: "a", Group: "g1"})
	e.AddPanel(Panel{ID: "b", Group: "g2"})
	e.Tick()
	base := counting.calls

	// 只改 g1，g2 不应重排。
	e.AddPanel(Panel{ID: "c", Group: "g1"})
	e.Tick()
	if got := counting.calls - base; got != 1 {
		t.Fatalf("relayout calls = %d, want 1 (only affected group)", got)
	}
}

type countingLayout struct{ calls int }

func (l *countingLayout) Arrange(panels []Panel) []Pose {
	l.calls++
	out := make([]Pose, len(panels))
	for i := range out {
		out[i] = Pose{Orientation: QuatIdentity()}
	}
	return out
}
