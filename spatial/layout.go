package spatial

import (
	"math"
	"sort"
)

// Strategy 是布局排布策略。
type Strategy int

const (
	StrategyArc Strategy = iota
	StrategyGrid
	StrategyStack
)

// LayoutConfig 描述一个布局组的排布规则。
type LayoutConfig struct {
	Strategy Strategy
	Spacing  float64 // 面板间距（Grid/Stack 为米，Arc 为弧度）
	Columns  int     // Grid 专用：列数，<=0 时取 ceil(sqrt(n))
	Radius   float64 // Arc 专用：弧线半径
	Anchor   AnchorID
}

// LayoutEngine 负责按组重排面板。重算以组为粒度：
// 单次 Commit 的耗时只与被标记 dirty 的组相关，与面板总数无关。
type LayoutEngine struct {
	configs map[string]LayoutConfig
	locals  map[string]map[PanelID]Pose // group -> panel -> local pose
	dirty   map[string]bool
}

func NewLayoutEngine() *LayoutEngine {
	return &LayoutEngine{
		configs: make(map[string]LayoutConfig),
		locals:  make(map[string]map[PanelID]Pose),
		dirty:   make(map[string]bool),
	}
}

func (le *LayoutEngine) SetConfig(group string, cfg LayoutConfig) {
	le.configs[group] = cfg
	le.dirty[group] = true
}

func (le *LayoutEngine) MarkDirty(group string) { le.dirty[group] = true }

// DirtyGroups 返回并清空待重算的组集合。
func (le *LayoutEngine) takeDirty() []string {
	out := make([]string, 0, len(le.dirty))
	for g := range le.dirty {
		out = append(out, g)
	}
	sort.Strings(out) // 固定处理顺序，保证可复现
	for _, g := range out {
		delete(le.dirty, g)
	}
	return out
}

// Resolve 计算指定组的局部位姿并缓存。
// 面板先按 (Order, ID) 稳定排序，因此同一批面板无论分批还是
// 一次性插入，最终排布完全一致。
func (le *LayoutEngine) Resolve(group string, panels []Panel) map[PanelID]Pose {
	cfg := le.configs[group]
	sorted := make([]Panel, len(panels))
	copy(sorted, panels)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Order != sorted[j].Order {
			return sorted[i].Order < sorted[j].Order
		}
		return sorted[i].ID < sorted[j].ID
	})
	out := make(map[PanelID]Pose, len(sorted))
	switch cfg.Strategy {
	case StrategyGrid:
		resolveGrid(cfg, sorted, out)
	case StrategyStack:
		resolveStack(cfg, sorted, out)
	default: // StrategyArc
		resolveArc(cfg, sorted, out)
	}
	le.locals[group] = out
	return out
}

// resolveArc 沿水平圆弧排布，面板面向弧心（+Z 朝内）。
func resolveArc(cfg LayoutConfig, panels []Panel, out map[PanelID]Pose) {
	n := len(panels)
	r := cfg.Radius
	if r <= 0 {
		r = 2
	}
	for i, p := range panels {
		// 以中间面板为 0 弧度，向两侧对称展开。
		ang := (float64(i) - float64(n-1)/2) * cfg.Spacing
		pos := Vec3{X: r * math.Sin(ang), Y: 0, Z: -r * math.Cos(ang)}
		rot := QuatFromAxisAngle(Vec3{0, 1, 0}, -ang)
		local := Pose{Position: pos, Orientation: rot}
		local.Position.Z += p.ZOffset
		out[p.ID] = local
	}
}

// resolveGrid 按行优先网格排布，以网格中心为原点。
func resolveGrid(cfg LayoutConfig, panels []Panel, out map[PanelID]Pose) {
	n := len(panels)
	cols := cfg.Columns
	if cols <= 0 {
		cols = int(math.Ceil(math.Sqrt(float64(n))))
	}
	if cols < 1 {
		cols = 1
	}
	rows := (n + cols - 1) / cols
	for i, p := range panels {
		r, c := i/cols, i%cols
		// 最后一行居中。
		rowCount := cols
		if r == rows-1 {
			rowCount = n - r*cols
		}
		x := (float64(c) - float64(rowCount-1)/2) * cfg.Spacing
		y := -(float64(r) - float64(rows-1)/2) * cfg.Spacing
		out[p.ID] = Pose{Position: Vec3{X: x, Y: y, Z: p.ZOffset}, Orientation: IdentityQuat()}
	}
}

// resolveStack 沿 -Z 方向纵深堆叠，索引越小越靠前。
func resolveStack(cfg LayoutConfig, panels []Panel, out map[PanelID]Pose) {
	for i, p := range panels {
		out[p.ID] = Pose{
			Position:    Vec3{Z: -float64(i)*cfg.Spacing + p.ZOffset},
			Orientation: IdentityQuat(),
		}
	}
}

// Local 返回已解算的局部位姿。
func (le *LayoutEngine) Local(group string, id PanelID) (Pose, bool) {
	m, ok := le.locals[group]
	if !ok {
		return Pose{}, false
	}
	p, ok := m[id]
	return p, ok
}
