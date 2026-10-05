package spatial

// RaySource 标识交互射线来源。
type RaySource int

const (
	RayGaze RaySource = iota
	RayControllerLeft
	RayControllerRight
)

// FocusEventType 区分焦点变化类型。
type FocusEventType int

const (
	FocusGained FocusEventType = iota
	FocusLost
	// FocusCanceled 表示持有焦点的面板被移除或重新挂载，交互被显式取消。
	FocusCanceled
)

// FocusEvent 描述一次焦点变化。
type FocusEvent struct {
	Type   FocusEventType
	Panel  PanelID
	Source RaySource
	Frame  uint64
}

// FocusManager 保证同一时刻最多一个面板持有焦点。
// 切换焦点需要候选面板连续 dwellNeed 帧保持命中（滞留计数），
// 因此姿态抖动不会让焦点在两个面板之间反复跳变。
type FocusManager struct {
	focused   PanelID
	hasFocus  bool
	candidate PanelID
	dwell     int
	dwellNeed int
}

func NewFocusManager(dwellNeed int) *FocusManager {
	if dwellNeed < 1 {
		dwellNeed = 1
	}
	return &FocusManager{dwellNeed: dwellNeed}
}

// Update 根据本帧命中结果推进焦点状态机，返回本帧产生的事件。
// 规则（确定性）：
//  1. 命中当前焦点面板：焦点保持，候选清空。
//  2. 命中其他面板：该面板成为候选；候选连续命中 dwellNeed 帧后
//     旧焦点 Lost、新焦点 Gained（同帧有序发出）。
//  3. 未命中任何面板：候选清空；当前焦点保持（焦点是粘性状态，
//     仅被新候选或显式 Cancel 改变）。
func (fm *FocusManager) Update(hit PanelID, hasHit bool, frame uint64, src RaySource) []FocusEvent {
	var events []FocusEvent
	if !hasHit {
		fm.candidate = ""
		fm.dwell = 0
		return events
	}
	if fm.hasFocus && hit == fm.focused {
		fm.candidate = ""
		fm.dwell = 0
		return events
	}
	if hit == fm.candidate {
		fm.dwell++
	} else {
		fm.candidate = hit
		fm.dwell = 1
	}
	if fm.dwell >= fm.dwellNeed {
		if fm.hasFocus {
			events = append(events, FocusEvent{Type: FocusLost, Panel: fm.focused, Source: src, Frame: frame})
		}
		fm.focused = fm.candidate
		fm.hasFocus = true
		events = append(events, FocusEvent{Type: FocusGained, Panel: fm.focused, Source: src, Frame: frame})
		fm.candidate = ""
		fm.dwell = 0
	}
	return events
}

// Cancel 显式取消某面板的焦点（面板被移除/重挂载时调用），
// 返回 FocusCanceled 事件；若该面板未持有焦点则返回 nil。
func (fm *FocusManager) Cancel(id PanelID, frame uint64, src RaySource) []FocusEvent {
	if fm.candidate == id {
		fm.candidate = ""
		fm.dwell = 0
	}
	if !fm.hasFocus || fm.focused != id {
		return nil
	}
	fm.hasFocus = false
	fm.focused = ""
	return []FocusEvent{{Type: FocusCanceled, Panel: id, Source: src, Frame: frame}}
}

func (fm *FocusManager) Focused() (PanelID, bool) { return fm.focused, fm.hasFocus }

// HitTest 在面板集合中做遮挡感知的射线命中：
// 沿射线参数 t 最近的面板获胜，被更近面板挡住的面板不会被选中。
// t 相同时按面板 ID 字典序取小者，保证结果可复现。
func HitTest(ray Ray, states []PanelState) (PanelID, bool) {
	bestT := 0.0
	var best PanelID
	found := false
	for _, st := range states {
		normal := st.World.Orientation.Rotate(Vec3{0, 0, 1})
		t, ok := RayRectIntersect(ray, st.World.Position, normal, Vec2{
			X: st.Panel.Size.X / 2,
			Y: st.Panel.Size.Y / 2,
		})
		if !ok {
			continue
		}
		if !found || t < bestT || (t == bestT && st.Panel.ID < best) {
			found = true
			bestT = t
			best = st.Panel.ID
		}
	}
	return best, found
}
