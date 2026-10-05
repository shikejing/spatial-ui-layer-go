package spatialui

// LayoutStrategy 定义一组面板的排布策略，输出相对组锚点的局部位姿。
// 约定：输入面板已按 (Order, ID) 稳定排序，输出与输入一一对应，
// 因此排布结果与面板插入顺序无关。
type LayoutStrategy interface {
	Arrange(panels []Panel) []Pose
}

// ArcLayout 沿水平圆弧排布：面板面向组锚点，居中面板位于锚点正前方（-Z）。
type ArcLayout struct {
	Radius     float64
	AngleStep  float64 // 相邻面板的圆心角（弧度）
	HeightStep float64 // 相邻面板的高度差
}

func (l ArcLayout) Arrange(panels []Panel) []Pose {
	n := len(panels)
	out := make([]Pose, n)
	mid := float64(n-1) / 2
	for i := range panels {
		a := (float64(i) - mid) * l.AngleStep
		pos := Vec3{
			X: l.Radius * sin(a),
			Y: (float64(i) - mid) * l.HeightStep,
			Z: -l.Radius * cos(a),
		}
		out[i] = Pose{
			Position:    pos,
			Orientation: QuatFromAxisAngle(Vec3{Y: 1}, -a),
		}
	}
	return out
}

// GridLayout 按网格排布：从左到右、从上到下填充，水平居中。
type GridLayout struct {
	CellWidth  float64
	CellHeight float64
	Columns    int
}

func (l GridLayout) Arrange(panels []Panel) []Pose {
	n := len(panels)
	out := make([]Pose, n)
	cols := l.Columns
	if cols <= 0 {
		cols = 1
	}
	for i := range panels {
		row := i / cols
		col := i % cols
		// 每行按实际列数居中，保证末行不足一列时仍居中。
		rowStart := row * cols
		rowCount := cols
		if rowStart+rowCount > n {
			rowCount = n - rowStart
		}
		x := (float64(col) - float64(rowCount-1)/2) * l.CellWidth
		y := -float64(row) * l.CellHeight
		out[i] = Pose{Position: Vec3{X: x, Y: y}, Orientation: QuatIdentity()}
	}
	return out
}

// StackLayout 沿给定方向线性堆叠。
type StackLayout struct {
	Direction Vec3
	Spacing   float64
}

func (l StackLayout) Arrange(panels []Panel) []Pose {
	out := make([]Pose, len(panels))
	dir := l.Direction.Norm()
	for i := range panels {
		out[i] = Pose{Position: dir.Scale(l.Spacing * float64(i)), Orientation: QuatIdentity()}
	}
	return out
}
