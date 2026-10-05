package spatialui

import (
	"encoding/json"
	"fmt"
)

// SnapshotFormatVersion 是当前序列化格式版本。
// 版本历史：
//
//	v1 — 初版：焦点为裸字符串（空串表示无焦点），面板无 Order 字段，无事件列表。
//	v2 — 当前：焦点为结构化对象，面板含 Order，含事件列表。
const SnapshotFormatVersion = 2

// snapshotDTOv2 是 v2 格式的传输结构，字段与快照一一对应。
type snapshotDTOv2 struct {
	Version int           `json:"version"`
	Frame   uint64        `json:"frame"`
	Poses   PoseSnapshot  `json:"poses"`
	Placed  []PlacedPanel `json:"panels"` // 顺序与 Snapshot.Placed 一致（按 ID 排序）
	Focus   focusDTO      `json:"focus"`
	Events  []FocusEvent  `json:"events"`
}

type focusDTO struct {
	ID  PanelID `json:"id"`
	Has bool    `json:"has"`
}

// snapshotDTOv1 是历史 v1 格式，仅用于读取迁移。
type snapshotDTOv1 struct {
	Frame      uint64          `json:"frame"`
	Poses      PoseSnapshot    `json:"poses"`
	Placed     []placedPanelV1 `json:"panels"`
	FocusPanel PanelID         `json:"focusPanel"` // 空串表示无焦点
}

type panelV1 struct {
	ID          PanelID    `json:"id"`
	Anchor      AnchorKind `json:"anchor"`
	Hand        Handedness `json:"hand"`
	LocalOffset Pose       `json:"localOffset"`
	Size        Size       `json:"size"`
	Group       string     `json:"group"`
}

type placedPanelV1 struct {
	Panel panelV1 `json:"panel"`
	Pose  Pose    `json:"pose"`
}

// MarshalSnapshot 将快照序列化为带版本号的 JSON。
// 输出字段顺序固定、面板按 ID 排序，同一快照的字节输出稳定。
func MarshalSnapshot(s *Snapshot) ([]byte, error) {
	dto := snapshotDTOv2{
		Version: SnapshotFormatVersion,
		Frame:   s.Frame,
		Poses:   s.Poses,
		Placed:  s.Placed,
		Focus:   focusDTO{ID: s.Focus, Has: s.HasFocus},
		Events:  s.Events,
	}
	if dto.Placed == nil {
		dto.Placed = []PlacedPanel{}
	}
	if dto.Events == nil {
		dto.Events = []FocusEvent{}
	}
	return json.Marshal(dto)
}

// UnmarshalSnapshot 读取任意历史版本的字节并迁移到当前版本。
// 缺少 version 字段的数据按 v1 处理。
func UnmarshalSnapshot(data []byte) (*Snapshot, error) {
	var head struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return nil, fmt.Errorf("spatialui: 无法解析快照头: %w", err)
	}
	switch head.Version {
	case 0, 1:
		var v1 snapshotDTOv1
		if err := json.Unmarshal(data, &v1); err != nil {
			return nil, fmt.Errorf("spatialui: 无法解析 v1 快照: %w", err)
		}
		return migrateV1(&v1), nil
	case SnapshotFormatVersion:
		var v2 snapshotDTOv2
		if err := json.Unmarshal(data, &v2); err != nil {
			return nil, fmt.Errorf("spatialui: 无法解析 v2 快照: %w", err)
		}
		return &Snapshot{
			Frame:    v2.Frame,
			Poses:    v2.Poses,
			Placed:   v2.Placed,
			Focus:    v2.Focus.ID,
			HasFocus: v2.Focus.Has,
			Events:   v2.Events,
		}, nil
	default:
		return nil, fmt.Errorf("spatialui: 不支持的快照版本 %d", head.Version)
	}
}

// migrateV1 将 v1 快照升级为等价的当前版本：
// 面板补默认 Order=0（排布次序退化为纯 ID 字典序，与 v1 行为一致），
// 裸字符串焦点转换为结构化焦点，事件列表为空。
func migrateV1(v1 *snapshotDTOv1) *Snapshot {
	placed := make([]PlacedPanel, 0, len(v1.Placed))
	for _, pp := range v1.Placed {
		placed = append(placed, PlacedPanel{
			Panel: Panel{
				ID:          pp.Panel.ID,
				Anchor:      pp.Panel.Anchor,
				Hand:        pp.Panel.Hand,
				LocalOffset: pp.Panel.LocalOffset,
				Size:        pp.Panel.Size,
				Group:       pp.Panel.Group,
				Order:       0,
			},
			Pose: pp.Pose,
		})
	}
	return &Snapshot{
		Frame:    v1.Frame,
		Poses:    v1.Poses,
		Placed:   placed,
		Focus:    v1.FocusPanel,
		HasFocus: v1.FocusPanel != "",
		Events:   nil,
	}
}
