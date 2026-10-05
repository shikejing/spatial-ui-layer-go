package spatial

import (
	"math"
	"testing"
)

func almostEq(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func newEngineWithHeadAnchor() *Engine {
	e := NewEngine()
	e.AddAnchor(Anchor{ID: "head", Kind: AnchorHead})
	e.AddAnchor(Anchor{ID: "room", Kind: AnchorWorld, Base: Pose{
		Position:    Vec3{0, 0, 0},
		Orientation: IdentityQuat(),
	}})
	e.SetLayout("g", LayoutConfig{Strategy: StrategyGrid, Spacing: 1.0, Columns: 2})
	return e
}

// 布局跟随：头部姿态更新并提交后，头锚面板的世界姿态必须整体跟随，
// 且同一帧内所有面板使用同一姿态快照。
func TestLayoutFollowsHeadPose(t *testing.T) {
	e := newEngineWithHeadAnchor()
	e.AddPanel(Panel{ID: "a", Anchor: "head", Group: "g", Order: 0, Size: Vec2{0.5, 0.5}})
	e.AddPanel(Panel{ID: "b", Anchor: "head", Group: "g", Order: 1, Size: Vec2{0.5, 0.5}})

	e.SubmitPose(AnchorHead, Pose{Position: Vec3{0, 1.6, 0}, Orientation: IdentityQuat()}, 1)
	e.Commit()
	f1 := e.Snapshot()
	var a1, b1 PanelState
	for _, st := range f1.Panels {
		switch st.Panel.ID {
		case "a":
			a1 = st
		case "b":
			b1 = st
		}
	}
	if !almostEq(a1.World.Position.Y, 1.6) || !almostEq(b1.World.Position.Y, 1.6) {
		t.Fatalf("panels did not follow head pose: a=%v b=%v", a1.World.Position, b1.World.Position)
	}
	rel1 := Sub(b1.World.Position, a1.World.Position)

	// 头部移动后，两个面板必须在同一帧内一起移动（相对关系不变）。
	e.SubmitPose(AnchorHead, Pose{Position: Vec3{1, 1.7, -2}, Orientation: IdentityQuat()}, 2)
	e.Commit()
	f2 := e.Snapshot()
	var a2, b2 PanelState
	for _, st := range f2.Panels {
		switch st.Panel.ID {
		case "a":
			a2 = st
		case "b":
			b2 = st
		}
	}
	// 网格局部偏移为 ±0.5，跟随头部平移 (1,1.7,-2) 后的世界坐标如下。
	if !almostEq(a2.World.Position.X, 0.5) || !almostEq(b2.World.Position.X, 1.5) || !almostEq(b2.World.Position.Z, -2) {
		t.Fatalf("panels did not follow updated pose: a=%v b=%v", a2.World.Position, b2.World.Position)
	}
	rel2 := Sub(b2.World.Position, a2.World.Position)
	if !almostEq(rel1.X, rel2.X) || !almostEq(rel1.Y, rel2.Y) || !almostEq(rel1.Z, rel2.Z) {
		t.Fatalf("relative layout changed across frames: %v -> %v", rel1, rel2)
	}
	// 帧内姿态快照必须是被提交的那一个。
	if f2.Poses[AnchorHead].Pose.Position.X != 1 {
		t.Fatal("frame does not carry the committed pose snapshot")
	}
}

// 世界锚定面板不随头部姿态移动。
func TestWorldAnchorStaysPut(t *testing.T) {
	e := newEngineWithHeadAnchor()
	e.AddPanel(Panel{ID: "w", Anchor: "room", Group: "g", Order: 0, Size: Vec2{1, 1}})
	e.SubmitPose(AnchorHead, Pose{Position: Vec3{0, 0, 0}}, 1)
	e.Commit()
	before := e.Snapshot().Panels[0].World.Position
	e.SubmitPose(AnchorHead, Pose{Position: Vec3{9, 9, 9}}, 2)
	e.Commit()
	after := e.Snapshot().Panels[0].World.Position
	if before != after {
		t.Fatalf("world-anchored panel moved: %v -> %v", before, after)
	}
}

// 分批插入与一次性插入同一批面板，布局结果必须一致。
func TestBatchVsIncrementalInsertion(t *testing.T) {
	panels := []Panel{
		{ID: "p1", Anchor: "room", Group: "g", Order: 2, Size: Vec2{1, 1}},
		{ID: "p2", Anchor: "room", Group: "g", Order: 0, Size: Vec2{1, 1}},
		{ID: "p3", Anchor: "room", Group: "g", Order: 1, Size: Vec2{1, 1}},
	}
	e1 := newEngineWithHeadAnchor()
	for _, p := range panels {
		e1.AddPanel(p)
	}
	e1.Commit()

	e2 := newEngineWithHeadAnchor()
	for _, p := range panels {
		e2.AddPanel(p)
		e2.Commit() // 每次插入都提交，模拟分批到达
	}

	f1, f2 := e1.Snapshot(), e2.Snapshot()
	if len(f1.Panels) != len(f2.Panels) {
		t.Fatal("panel count mismatch")
	}
	for i := range f1.Panels {
		a, b := f1.Panels[i], f2.Panels[i]
		if a.Panel.ID != b.Panel.ID || a.World != b.World || a.Local != b.Local {
			t.Fatalf("panel %d differs: %+v vs %+v", i, a, b)
		}
	}
}

// 三种策略的确定性：相同输入两次解算结果一致。
func TestStrategiesDeterministic(t *testing.T) {
	for _, s := range []Strategy{StrategyArc, StrategyGrid, StrategyStack} {
		le := NewLayoutEngine()
		cfg := LayoutConfig{Strategy: s, Spacing: 0.2, Columns: 3, Radius: 2}
		le.SetConfig("g", cfg)
		ps := []Panel{
			{ID: "x", Group: "g", Order: 1},
			{ID: "y", Group: "g", Order: 0},
			{ID: "z", Group: "g", Order: 2},
		}
		r1 := le.Resolve("g", ps)
		r2 := le.Resolve("g", ps)
		for id, p := range r1 {
			if r2[id] != p {
				t.Fatalf("strategy %d not deterministic for %s", s, id)
			}
		}
		if len(r1) != 3 {
			t.Fatalf("strategy %d lost panels", s)
		}
	}
}

// 尺寸变化触发重排：网格位置依赖面板数量而非尺寸，
// 但集合变化（新增面板）必须整体重排。
func TestRelayoutOnSetChange(t *testing.T) {
	e := newEngineWithHeadAnchor()
	e.AddPanel(Panel{ID: "a", Anchor: "room", Group: "g", Order: 0, Size: Vec2{1, 1}})
	e.Commit()
	solo := e.Snapshot().Panels[0].Local.Position
	e.AddPanel(Panel{ID: "b", Anchor: "room", Group: "g", Order: 1, Size: Vec2{1, 1}})
	e.Commit()
	for _, st := range e.Snapshot().Panels {
		if st.Panel.ID == "a" {
			if st.Local.Position == solo {
				t.Fatal("layout did not re-arrange after set change")
			}
		}
	}
}
