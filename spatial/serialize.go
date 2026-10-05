package spatial

import (
	"encoding/json"
	"fmt"
	"sort"
)

// SnapshotVersion 是当前序列化格式版本。
// v1：无 ZOffset / HasFocus 字段，焦点仅由 focused 字符串表达。
// v2：当前版本。读取方同时接受 v1 与 v2，v1 自动迁移为等价 v2。
const SnapshotVersion = 2

// FrameDTO 是 Frame 的可序列化形式，带版本号以支持跨版本读取。
type FrameDTO struct {
	Version  int                `json:"version"`
	Seq      uint64             `json:"seq"`
	Poses    map[string]PoseDTO `json:"poses"`
	Panels   []PanelDTO         `json:"panels"`
	Focused  string             `json:"focused,omitempty"`
	HasFocus bool               `json:"hasFocus"`
}

type PoseDTO struct {
	Pos [3]float64 `json:"pos"`
	Rot [4]float64 `json:"rot"`
	Ts  int64      `json:"ts"`
}

type PanelDTO struct {
	ID      string     `json:"id"`
	Anchor  string     `json:"anchor"`
	Group   string     `json:"group"`
	Order   int        `json:"order"`
	Size    [2]float64 `json:"size"`
	ZOffset float64    `json:"zOffset"`
	World   PoseDTO    `json:"world"`
	Local   PoseDTO    `json:"local"`
}

// frameDTOv1 是 v1 格式：无 ZOffset / HasFocus。
type frameDTOv1 struct {
	Version int                `json:"version"`
	Seq     uint64             `json:"seq"`
	Poses   map[string]PoseDTO `json:"poses"`
	Panels  []panelDTOv1       `json:"panels"`
	Focused string             `json:"focused,omitempty"`
}

type panelDTOv1 struct {
	ID     string     `json:"id"`
	Anchor string     `json:"anchor"`
	Group  string     `json:"group"`
	Order  int        `json:"order"`
	Size   [2]float64 `json:"size"`
	World  PoseDTO    `json:"world"`
	Local  PoseDTO    `json:"local"`
}

func poseToDTO(p Pose, ts int64) PoseDTO {
	return PoseDTO{
		Pos: [3]float64{p.Position.X, p.Position.Y, p.Position.Z},
		Rot: [4]float64{p.Orientation.W, p.Orientation.X, p.Orientation.Y, p.Orientation.Z},
		Ts:  ts,
	}
}

func poseFromDTO(d PoseDTO) Pose {
	return Pose{
		Position:    Vec3{d.Pos[0], d.Pos[1], d.Pos[2]},
		Orientation: Quat{d.Rot[0], d.Rot[1], d.Rot[2], d.Rot[3]}.Normalize(),
	}
}

// MarshalFrame 将帧序列化为稳定的 JSON：
// 面板按 ID 升序、姿态键为固定整数映射，输出与产生它的帧严格对应。
func MarshalFrame(f *Frame) ([]byte, error) {
	dto := FrameDTO{
		Version:  SnapshotVersion,
		Seq:      f.Seq,
		Poses:    make(map[string]PoseDTO, len(f.Poses)),
		Focused:  string(f.Focused),
		HasFocus: f.HasFocus,
	}
	for kind, s := range f.Poses {
		dto.Poses[fmt.Sprintf("%d", kind)] = poseToDTO(s.Pose, s.Timestamp)
	}
	panels := make([]PanelState, len(f.Panels))
	copy(panels, f.Panels)
	sort.Slice(panels, func(i, j int) bool { return panels[i].Panel.ID < panels[j].Panel.ID })
	for _, st := range panels {
		dto.Panels = append(dto.Panels, PanelDTO{
			ID:      string(st.Panel.ID),
			Anchor:  string(st.Panel.Anchor),
			Group:   st.Panel.Group,
			Order:   st.Panel.Order,
			Size:    [2]float64{st.Panel.Size.X, st.Panel.Size.Y},
			ZOffset: st.Panel.ZOffset,
			World:   poseToDTO(st.World, 0),
			Local:   poseToDTO(st.Local, 0),
		})
	}
	return json.Marshal(dto)
}

// UnmarshalFrame 读取任意已知版本的字节并还原为等价 Frame。
func UnmarshalFrame(data []byte) (*Frame, error) {
	var head struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return nil, fmt.Errorf("spatial: bad snapshot: %w", err)
	}
	switch head.Version {
	case SnapshotVersion:
		var dto FrameDTO
		if err := json.Unmarshal(data, &dto); err != nil {
			return nil, fmt.Errorf("spatial: bad v%d snapshot: %w", head.Version, err)
		}
		return frameFromDTO(dto), nil
	case 1:
		var old frameDTOv1
		if err := json.Unmarshal(data, &old); err != nil {
			return nil, fmt.Errorf("spatial: bad v1 snapshot: %w", err)
		}
		return migrateV1(old), nil
	default:
		return nil, fmt.Errorf("spatial: unsupported snapshot version %d", head.Version)
	}
}

func frameFromDTO(dto FrameDTO) *Frame {
	f := &Frame{
		Seq:      dto.Seq,
		Poses:    make(map[AnchorKind]PoseSample, len(dto.Poses)),
		Focused:  PanelID(dto.Focused),
		HasFocus: dto.HasFocus,
	}
	for k, p := range dto.Poses {
		var kind int
		fmt.Sscanf(k, "%d", &kind)
		f.Poses[AnchorKind(kind)] = PoseSample{Pose: poseFromDTO(p), Timestamp: p.Ts}
	}
	for _, pd := range dto.Panels {
		f.Panels = append(f.Panels, PanelState{
			Panel: Panel{
				ID:      PanelID(pd.ID),
				Anchor:  AnchorID(pd.Anchor),
				Group:   pd.Group,
				Order:   pd.Order,
				Size:    Vec2{pd.Size[0], pd.Size[1]},
				ZOffset: pd.ZOffset,
			},
			World: poseFromDTO(pd.World),
			Local: poseFromDTO(pd.Local),
		})
	}
	sort.Slice(f.Panels, func(i, j int) bool { return f.Panels[i].Panel.ID < f.Panels[j].Panel.ID })
	return f
}

// migrateV1 将 v1 快照升级为等价 v2：ZOffset 置 0，
// HasFocus 由 focused 是否为空推导。
func migrateV1(old frameDTOv1) *Frame {
	dto := FrameDTO{
		Version:  SnapshotVersion,
		Seq:      old.Seq,
		Poses:    old.Poses,
		Focused:  old.Focused,
		HasFocus: old.Focused != "",
	}
	for _, p := range old.Panels {
		dto.Panels = append(dto.Panels, PanelDTO{
			ID:     p.ID,
			Anchor: p.Anchor,
			Group:  p.Group,
			Order:  p.Order,
			Size:   p.Size,
			World:  p.World,
			Local:  p.Local,
		})
	}
	return frameFromDTO(dto)
}
