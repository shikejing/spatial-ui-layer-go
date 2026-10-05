package spatialui

import "math"

// HitResult 是一次射线命中结果。
type HitResult struct {
	PanelID  PanelID
	Distance float64
	Point    Vec3
	Source   RaySource
}

// RaySource 区分射线来源。
type RaySource int

const (
	RayGaze RaySource = iota // 凝视射线
	RayHand                  // 手柄射线
)

// rayRect 计算射线与面板局部空间中矩形的交点。
// 面板局部约定：面板位于 z=0 平面，法线 +Z 朝向观察者，中心在原点。
func rayRect(ray Ray, p PlacedPanel) (float64, bool) {
	inv := p.Pose.Inverse()
	o := inv.TransformPoint(ray.Origin)
	d := inv.TransformDir(ray.Dir)
	if math.Abs(d.Z) < 1e-9 {
		return 0, false // 与面板平面平行
	}
	t := -o.Z / d.Z
	if t <= 1e-6 {
		return 0, false // 在射线背后
	}
	hx := o.X + d.X*t
	hy := o.Y + d.Y*t
	if math.Abs(hx) > p.Panel.Size.Width/2 || math.Abs(hy) > p.Panel.Size.Height/2 {
		return 0, false
	}
	return t, true
}

// Raycast 对面板集合做遮挡感知的命中检测。
// 遮挡语义：沿射线距离最近的面板遮挡其后所有面板，因此最近命中即唯一可见命中。
// 距离相同（数值上极少见）时按 PanelID 字典序裁决，保证结果可复现。
func Raycast(ray Ray, source RaySource, panels []PlacedPanel) (HitResult, bool) {
	best := HitResult{Source: source, Distance: math.Inf(1)}
	found := false
	for _, p := range panels {
		t, ok := rayRect(ray, p)
		if !ok {
			continue
		}
		if t < best.Distance-1e-9 ||
			(math.Abs(t-best.Distance) <= 1e-9 && (!found || p.Panel.ID < best.PanelID)) {
			best = HitResult{
				PanelID:  p.Panel.ID,
				Distance: t,
				Point:    ray.At(t),
				Source:   source,
			}
			found = true
		}
	}
	return best, found
}

// FocusEvent 描述一次焦点变化。
type FocusEvent struct {
	Frame    uint64
	Previous PanelID
	Current  PanelID
	Reason   FocusReason
}

// FocusReason 解释焦点变化原因。
type FocusReason int

const (
	FocusNone FocusReason = iota
	FocusAcquired
	FocusLost
	FocusSwitched
	FocusCancelledRemoved // 面板被移除导致取消
	FocusCancelledRemount // 面板被重新挂载导致取消
)

func (r FocusReason) String() string {
	switch r {
	case FocusAcquired:
		return "acquired"
	case FocusLost:
		return "lost"
	case FocusSwitched:
		return "switched"
	case FocusCancelledRemoved:
		return "cancelled-removed"
	case FocusCancelledRemount:
		return "cancelled-remount"
	default:
		return "none"
	}
}

// FocusManager 维护全局唯一焦点，带防抖滞回：
// 候选面板必须连续命中 SwitchFrames 帧才获得焦点；
// 焦点面板必须连续丢失 SwitchFrames 帧才失去焦点。
// 因此单帧抖动不会造成焦点在面板间反复跳变。
type FocusManager struct {
	focused    PanelID
	hasFocus   bool
	candidate  PanelID
	candFrames int
	missFrames int

	// SwitchFrames 是切换/丢失焦点所需的连续帧数，默认 2。
	SwitchFrames int
}

func NewFocusManager() *FocusManager { return &FocusManager{SwitchFrames: 2} }

// pickTarget 从本帧各射线命中中确定唯一候选：
// 手柄射线优先于凝视射线（主动输入优先），同来源取距离最近者。
func pickTarget(hits []HitResult) (PanelID, bool) {
	var best *HitResult
	for i := range hits {
		h := &hits[i]
		if best == nil ||
			(h.Source == RayHand && best.Source != RayHand) ||
			(h.Source == best.Source && h.Distance < best.Distance) {
			best = h
		}
	}
	if best == nil {
		return "", false
	}
	return best.PanelID, true
}

// Update 输入本帧各射线的命中结果，返回焦点事件（无变化时 Reason 为 FocusNone）。
func (f *FocusManager) Update(frame uint64, hits []HitResult) FocusEvent {
	n := f.SwitchFrames
	if n <= 0 {
		n = 1
	}
	target, ok := pickTarget(hits)

	if ok && f.hasFocus && target == f.focused {
		f.candidate, f.candFrames, f.missFrames = "", 0, 0
		return FocusEvent{Frame: frame, Reason: FocusNone}
	}

	if !ok {
		f.candidate, f.candFrames = "", 0
		if !f.hasFocus {
			return FocusEvent{Frame: frame, Reason: FocusNone}
		}
		f.missFrames++
		if f.missFrames >= n {
			ev := FocusEvent{Frame: frame, Previous: f.focused, Reason: FocusLost}
			f.focused, f.hasFocus, f.missFrames = "", false, 0
			return ev
		}
		return FocusEvent{Frame: frame, Reason: FocusNone}
	}

	// 有候选目标且与当前焦点不同。
	f.missFrames = 0
	if target == f.candidate {
		f.candFrames++
	} else {
		f.candidate, f.candFrames = target, 1
	}
	if f.candFrames < n {
		return FocusEvent{Frame: frame, Reason: FocusNone}
	}
	ev := FocusEvent{Frame: frame, Previous: f.focused, Current: target}
	if f.hasFocus {
		ev.Reason = FocusSwitched
	} else {
		ev.Reason = FocusAcquired
	}
	f.focused, f.hasFocus = target, true
	f.candidate, f.candFrames = "", 0
	return ev
}

// Cancel 因面板移除/重挂而取消其焦点或候选资格。
// 只有被取消的面板确实持有焦点或正是候选时才产生事件，调用方可据此区分。
func (f *FocusManager) Cancel(frame uint64, id PanelID, reason FocusReason) FocusEvent {
	if f.candidate == id {
		f.candidate, f.candFrames = "", 0
	}
	if f.hasFocus && f.focused == id {
		ev := FocusEvent{Frame: frame, Previous: id, Reason: reason}
		f.focused, f.hasFocus, f.missFrames = "", false, 0
		return ev
	}
	return FocusEvent{Frame: frame, Reason: FocusNone}
}

// Focused 返回当前焦点面板。
func (f *FocusManager) Focused() (PanelID, bool) { return f.focused, f.hasFocus }
