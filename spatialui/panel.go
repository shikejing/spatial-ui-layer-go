package spatialui

// AnchorKind 描述面板挂载的锚点类型。
type AnchorKind int

const (
	AnchorWorld AnchorKind = iota // 世界锚定
	AnchorHead                    // 头部锚定
	AnchorHand                    // 手柄锚定
)

// Handedness 区分左右手柄。
type Handedness int

const (
	HandLeft Handedness = iota
	HandRight
)

// PanelID 是面板的唯一标识。
type PanelID string

// Size 是面板的二维尺寸（米）。
type Size struct {
	Width, Height float64
}

// Panel 描述一块空间界面面板的静态配置。
type Panel struct {
	ID          PanelID
	Anchor      AnchorKind
	Hand        Handedness // 仅 AnchorHand 有效
	LocalOffset Pose       // 相对锚点的偏移
	Size        Size
	Group       string // 所属布局组，同组面板参与统一排布
	Order       int    // 组内排布次序，次序相同按 ID 字典序
}

// PlacedPanel 是面板在某帧的世界空间放置结果。
type PlacedPanel struct {
	Panel Panel
	Pose  Pose // 世界空间位姿
}
