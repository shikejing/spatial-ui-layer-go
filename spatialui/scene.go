package spatialui

import (
	"sort"
	"sync"
	"sync/atomic"
)

// Snapshot 是某一帧布局与焦点状态的不可变自洽快照。
// 快照一旦发布不再修改，读取方可安全并发使用。
type Snapshot struct {
	Frame    uint64
	Poses    PoseSnapshot
	Placed   []PlacedPanel // 按 PanelID 排序，序列化顺序稳定
	Focus    PanelID
	HasFocus bool
	Events   []FocusEvent // 本帧产生的焦点事件
}

// groupState 缓存一个布局组的排布结果，仅在成员或策略变化时重算。
type groupState struct {
	strategy LayoutStrategy
	anchor   AnchorKind
	hand     Handedness
	members  []Panel // 按 (Order, ID) 排序
	local    []Pose  // 与 members 对应的缓存排布
	dirty    bool
}

// Engine 组合姿态追踪、布局与焦点管理。
// 所有写操作（SubmitPose/Tick/AddPanel/...）由内部互斥锁串行化；
// 读取方通过 Snapshot() 原子加载不可变快照，永不阻塞写入。
type Engine struct {
	mu      sync.Mutex
	tracker *PoseTracker
	panels  map[PanelID]*Panel
	groups  map[string]*groupState
	focus   *FocusManager
	frame   uint64
	pending []FocusEvent // 写操作产生、待下一帧发布的事件

	snap atomic.Pointer[Snapshot]
}

func NewEngine() *Engine {
	e := &Engine{
		tracker: NewPoseTracker(),
		panels:  make(map[PanelID]*Panel),
		groups:  make(map[string]*groupState),
		focus:   NewFocusManager(),
	}
	e.publishLocked(PoseSnapshot{})
	return e
}

// anchorPose 由姿态快照求某锚点的世界位姿。
func anchorPose(kind AnchorKind, hand Handedness, poses PoseSnapshot) Pose {
	switch kind {
	case AnchorHead:
		return poses.Head.Pose
	case AnchorHand:
		if hand == HandLeft {
			return poses.HandLeft.Pose
		}
		return poses.HandRight.Pose
	default:
		return IdentityPose()
	}
}

// SubmitPose 提交一路姿态样本（乱序/重复安全）。
// 样本仅被记录，到下一次 Tick 才进入已发布快照。
func (e *Engine) SubmitPose(kind TrackKind, s PoseSample) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.tracker.Submit(kind, s)
}

// Tick 推进一帧：取同一姿态快照重排受影响的布局组、
// 计算全部面板世界位姿、更新焦点并原子发布新快照。
// 同一帧内所有面板使用同一份姿态快照，相对关系自洽。
func (e *Engine) Tick() *Snapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.frame++
	poses := e.tracker.Snapshot()

	// 仅重算被标记的布局组，耗时与受影响范围相关。
	for _, g := range e.groups {
		if g.dirty {
			g.local = g.strategy.Arrange(g.members)
			g.dirty = false
		}
	}

	placed := make([]PlacedPanel, 0, len(e.panels))
	for _, g := range e.groups {
		ap := anchorPose(g.anchor, g.hand, poses)
		for i, m := range g.members {
			placed = append(placed, PlacedPanel{
				Panel: m,
				Pose:  ap.Mul(g.local[i]).Mul(m.LocalOffset),
			})
		}
	}
	// 未分组面板：直接按自身锚点放置。
	for _, p := range e.panels {
		if p.Group != "" {
			continue
		}
		placed = append(placed, PlacedPanel{
			Panel: *p,
			Pose:  anchorPose(p.Anchor, p.Hand, poses).Mul(p.LocalOffset),
		})
	}
	sort.Slice(placed, func(i, j int) bool { return placed[i].Panel.ID < placed[j].Panel.ID })

	// 凝视射线与手柄射线均取自同一姿态快照。
	var hits []HitResult
	gaze := Ray{Origin: poses.Head.Pose.Position, Dir: poses.Head.Pose.TransformDir(Vec3{Z: -1})}
	if h, ok := Raycast(gaze, RayGaze, placed); ok {
		hits = append(hits, h)
	}
	hand := Ray{Origin: poses.HandRight.Pose.Position, Dir: poses.HandRight.Pose.TransformDir(Vec3{Z: -1})}
	if h, ok := Raycast(hand, RayHand, placed); ok {
		hits = append(hits, h)
	}

	events := e.pending
	e.pending = nil
	if ev := e.focus.Update(e.frame, hits); ev.Reason != FocusNone {
		events = append(events, ev)
	}

	focused, hasFocus := e.focus.Focused()
	s := &Snapshot{
		Frame:    e.frame,
		Poses:    poses,
		Placed:   placed,
		Focus:    focused,
		HasFocus: hasFocus,
		Events:   events,
	}
	e.publishLocked2(s)
	return s
}

func (e *Engine) publishLocked(poses PoseSnapshot) {
	e.publishLocked2(&Snapshot{Frame: e.frame, Poses: poses})
}

func (e *Engine) publishLocked2(s *Snapshot) { e.snap.Store(s) }

// lessPanel 是面板排布的稳定次序：先 Order 后 ID。
// 因此分批插入与一次性插入同一批面板得到相同的排布。
func lessPanel(a, b Panel) bool {
	if a.Order != b.Order {
		return a.Order < b.Order
	}
	return a.ID < b.ID
}

func (e *Engine) groupLocked(name string) *groupState {
	g, ok := e.groups[name]
	if !ok {
		g = &groupState{strategy: StackLayout{Direction: Vec3{X: 1}, Spacing: 1}, anchor: AnchorWorld}
		e.groups[name] = g
	}
	return g
}

// AddPanel 新增面板。若面板属于某布局组，则按稳定次序插入该组并标记重排。
func (e *Engine) AddPanel(p Panel) {
	e.mu.Lock()
	defer e.mu.Unlock()
	cp := p
	e.panels[p.ID] = &cp
	if p.Group != "" {
		g := e.groupLocked(p.Group)
		i := sort.Search(len(g.members), func(i int) bool { return !lessPanel(g.members[i], p) })
		g.members = append(g.members, Panel{})
		copy(g.members[i+1:], g.members[i:])
		g.members[i] = p
		g.dirty = true
	}
}

// RemovePanel 移除面板。若其持有焦点，产生 FocusCancelledRemoved 事件，
// 事件会出现在下一帧快照中，调用方可据此区分"交互被取消"而非静默丢失。
func (e *Engine) RemovePanel(id PanelID) {
	e.mu.Lock()
	defer e.mu.Unlock()
	p, ok := e.panels[id]
	if !ok {
		return
	}
	delete(e.panels, id)
	if p.Group != "" {
		if g, ok := e.groups[p.Group]; ok {
			for i, m := range g.members {
				if m.ID == id {
					g.members = append(g.members[:i], g.members[i+1:]...)
					break
				}
			}
			g.dirty = true
		}
	}
	if ev := e.focus.Cancel(e.frame+1, id, FocusCancelledRemoved); ev.Reason != FocusNone {
		e.pending = append(e.pending, ev)
	}
}

// Remount 将面板重新挂载到新锚点：面板退出原布局组、改为按自身锚点放置。
// 若其持有焦点，产生 FocusCancelledRemount 事件——重挂后焦点状态为"无焦点"，
// 这是确定且可解释的：面板的空间归属已改变，原有交互上下文不再有效。
func (e *Engine) Remount(id PanelID, anchor AnchorKind, hand Handedness) {
	e.mu.Lock()
	defer e.mu.Unlock()
	p, ok := e.panels[id]
	if !ok {
		return
	}
	if p.Group != "" {
		if g, ok := e.groups[p.Group]; ok {
			for i, m := range g.members {
				if m.ID == id {
					g.members = append(g.members[:i], g.members[i+1:]...)
					break
				}
			}
			g.dirty = true
		}
	}
	p.Group = ""
	p.Anchor = anchor
	p.Hand = hand
	if ev := e.focus.Cancel(e.frame+1, id, FocusCancelledRemount); ev.Reason != FocusNone {
		e.pending = append(e.pending, ev)
	}
}

// SetLayout 设置某布局组的排布策略与组锚点，并标记重排。
func (e *Engine) SetLayout(group string, l LayoutStrategy, anchor AnchorKind, hand Handedness) {
	e.mu.Lock()
	defer e.mu.Unlock()
	g := e.groupLocked(group)
	g.strategy = l
	g.anchor = anchor
	g.hand = hand
	g.dirty = true
}

// Resize 更新面板内容尺寸并触发其所在组重排。
func (e *Engine) Resize(id PanelID, size Size) {
	e.mu.Lock()
	defer e.mu.Unlock()
	p, ok := e.panels[id]
	if !ok {
		return
	}
	p.Size = size
	if p.Group != "" {
		if g, ok := e.groups[p.Group]; ok {
			for i, m := range g.members {
				if m.ID == id {
					g.members[i].Size = size
					break
				}
			}
			g.dirty = true
		}
	}
}

// Snapshot 返回最近一次发布的快照（原子加载，不阻塞任何写入）。
func (e *Engine) Snapshot() *Snapshot { return e.snap.Load() }
