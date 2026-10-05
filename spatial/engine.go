package spatial

import (
	"sort"
	"sync"
)

// Engine 是空间界面层的入口：接收姿态与面板变更，发布自洽帧快照。
//
// 并发模型：写路径（SubmitPose/AddPanel/RemovePanel/RemountPanel/CastRay/Commit）
// 由互斥锁串行化；读路径（Snapshot）走原子指针，永不阻塞写路径。
// 每次 Commit 把当时已提交的全部姿态一次性快照进帧，
// 因此同一帧内所有面板的相对关系必然自洽。
type Engine struct {
	mu      sync.Mutex // 只保护写路径；读路径走 FrameStore 无锁
	tracker *Tracker
	layout  *LayoutEngine
	focus   *FocusManager
	store   *FrameStore

	anchors map[AnchorID]Anchor
	panels  map[PanelID]Panel
	states  map[PanelID]PanelState // 上次提交时各面板的解算缓存

	dirtyKinds map[AnchorKind]bool // 自上次提交后收到新姿态的追踪源
	invalid    map[PanelID]bool    // 需要重新解算世界姿态的面板
	seq        uint64
}

func NewEngine() *Engine {
	return &Engine{
		tracker:    NewTracker(),
		layout:     NewLayoutEngine(),
		focus:      NewFocusManager(2),
		store:      NewFrameStore(),
		anchors:    make(map[AnchorID]Anchor),
		panels:     make(map[PanelID]Panel),
		states:     make(map[PanelID]PanelState),
		dirtyKinds: make(map[AnchorKind]bool),
		invalid:    make(map[PanelID]bool),
	}
}

// SetFocusDwell 设置焦点切换所需的连续命中帧数（抗抖动）。
func (e *Engine) SetFocusDwell(n int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.focus = NewFocusManager(n)
}

func (e *Engine) AddAnchor(a Anchor) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.anchors[a.ID] = a
	// 世界锚点变更会影响挂在它上面的面板。
	for id, p := range e.panels {
		if p.Anchor == a.ID {
			e.invalid[id] = true
		}
	}
}

func (e *Engine) SetLayout(group string, cfg LayoutConfig) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.layout.SetConfig(group, cfg)
}

// SubmitPose 提交追踪姿态；乱序/重复样本被丢弃并返回 false。
// 只记录姿态，不推进帧；帧边界由 Commit 决定。
func (e *Engine) SubmitPose(kind AnchorKind, pose Pose, ts int64) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.tracker.Submit(kind, pose, ts) {
		return false
	}
	e.dirtyKinds[kind] = true
	return true
}

func (e *Engine) AddPanel(p Panel) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.panels[p.ID] = p
	e.layout.MarkDirty(p.Group)
	e.invalid[p.ID] = true
}

// RemovePanel 移除面板；若其持有焦点则产生 FocusCanceled 事件。
// 变更会立即提交一帧，读方不会看到已移除的面板。
func (e *Engine) RemovePanel(id PanelID) []FocusEvent {
	e.mu.Lock()
	defer e.mu.Unlock()
	p, ok := e.panels[id]
	if !ok {
		return nil
	}
	events := e.focus.Cancel(id, e.seq+1, RayGaze)
	delete(e.panels, id)
	delete(e.states, id)
	delete(e.invalid, id)
	e.layout.MarkDirty(p.Group)
	e.commitLocked()
	return events
}

// RemountPanel 将面板挂到另一锚点；若其持有焦点则先以 FocusCanceled
// 显式取消（交互状态可解释），再按新锚点重新解算，可经正常滞留重新获得焦点。
func (e *Engine) RemountPanel(id PanelID, anchor AnchorID) []FocusEvent {
	e.mu.Lock()
	defer e.mu.Unlock()
	p, ok := e.panels[id]
	if !ok {
		return nil
	}
	events := e.focus.Cancel(id, e.seq+1, RayGaze)
	p.Anchor = anchor
	e.panels[id] = p
	e.invalid[id] = true
	e.commitLocked()
	return events
}

// CastRay 用指定来源的射线在最新已提交帧上做命中与焦点推进，
// 随后提交一帧使焦点变化对读方可见。
// 命中结果严格对应某一已提交姿态快照，绝不使用半更新状态。
func (e *Engine) CastRay(src RaySource, ray Ray) []FocusEvent {
	e.mu.Lock()
	defer e.mu.Unlock()
	f := e.store.Latest()
	if f == nil {
		e.commitLocked()
		f = e.store.Latest()
	}
	hit, hasHit := HitTest(ray, f.Panels)
	events := e.focus.Update(hit, hasHit, f.Seq, src)
	if len(events) > 0 {
		e.commitLocked()
	}
	return events
}

// Commit 解算受影响布局、合成并发布新帧，返回帧序号。
// 布局重算只触及 dirty 组与受新姿态影响的锚点上的面板，
// 耗时与受影响范围相关，与面板总数无关。
func (e *Engine) Commit() uint64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.commitLocked()
}

func (e *Engine) commitLocked() uint64 {
	e.seq++

	// 1. 重算受影响的布局组。
	for _, group := range e.layout.takeDirty() {
		var members []Panel
		for _, p := range e.panels {
			if p.Group == group {
				members = append(members, p)
			}
		}
		e.layout.Resolve(group, members)
		for _, p := range members {
			e.invalid[p.ID] = true
		}
	}

	// 2. 找出受新姿态影响的面板：其锚点对应的追踪源有更新。
	for id, p := range e.panels {
		if e.invalid[id] {
			continue
		}
		a, ok := e.anchors[p.Anchor]
		if ok && a.Kind != AnchorWorld && e.dirtyKinds[a.Kind] {
			e.invalid[id] = true
		}
	}

	// 3. 只对失效面板重新解算世界姿态，其余复用缓存。
	for id := range e.invalid {
		p, ok := e.panels[id]
		if !ok {
			continue
		}
		local, _ := e.layout.Local(p.Group, id)
		world := Compose(e.anchorPoseLocked(p.Anchor), local)
		e.states[id] = PanelState{Panel: p, World: world, Local: local}
	}
	for k := range e.invalid {
		delete(e.invalid, k)
	}
	for k := range e.dirtyKinds {
		delete(e.dirtyKinds, k)
	}

	// 4. 合成不可变帧：姿态一次性快照，面板按 ID 排序保证顺序稳定。
	panels := make([]PanelState, 0, len(e.states))
	for _, st := range e.states {
		panels = append(panels, st)
	}
	sort.Slice(panels, func(i, j int) bool { return panels[i].Panel.ID < panels[j].Panel.ID })
	focused, hasFocus := e.focus.Focused()
	e.store.Publish(&Frame{
		Seq:      e.seq,
		Poses:    e.tracker.Snapshot(),
		Panels:   panels,
		Focused:  focused,
		HasFocus: hasFocus,
	})
	return e.seq
}

// anchorPoseLocked 解析锚点当前姿态：世界锚点用固定 Base，
// 追踪锚点用最新已提交姿态（缺失时退回单位姿态）。
func (e *Engine) anchorPoseLocked(id AnchorID) Pose {
	a, ok := e.anchors[id]
	if !ok {
		return Pose{Orientation: IdentityQuat()}
	}
	if a.Kind == AnchorWorld {
		return a.Base
	}
	if pose, ok := e.tracker.Latest(a.Kind); ok {
		return pose
	}
	return Pose{Orientation: IdentityQuat()}
}

// Snapshot 返回最新已发布帧，绝不阻塞写路径。
func (e *Engine) Snapshot() *Frame { return e.store.Latest() }
