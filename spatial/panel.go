package spatial

// PanelID 唯一标识一个面板。
type PanelID string

// Panel 描述一个空间界面面板。
type Panel struct {
	ID      PanelID
	Anchor  AnchorID
	Group   string  // 所属布局组
	Order   int     // 组内排布顺序，稳定排序用
	Size    Vec2    // 面板尺寸（米）
	ZOffset float64 // 沿法线方向的层偏移，用于遮挡排序
}

// PanelState 是面板在某一帧的解算结果。
type PanelState struct {
	Panel Panel
	World Pose // 世界空间姿态
	Local Pose // 组内局部位姿
}
