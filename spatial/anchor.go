package spatial

// AnchorKind 标识锚点来源。
type AnchorKind int

const (
	AnchorWorld AnchorKind = iota
	AnchorHead
	AnchorControllerLeft
	AnchorControllerRight
)

// AnchorID 唯一标识一个锚点。
type AnchorID string

// Anchor 是面板可挂载的空间锚点。
type Anchor struct {
	ID   AnchorID
	Kind AnchorKind
	// Base 仅对 AnchorWorld 有意义：世界中的固定姿态。
	Base Pose
}

// PoseSample 是带时间戳的追踪姿态样本。
type PoseSample struct {
	Pose      Pose
	Timestamp int64 // 单调递增的源时间戳
	Seq       uint64
}

// Tracker 保存各追踪源的最新已提交姿态，拒绝乱序与重复样本。
type Tracker struct {
	samples map[AnchorKind]PoseSample
	lastTS  map[AnchorKind]int64
	has     map[AnchorKind]bool
	seq     uint64
}

func NewTracker() *Tracker {
	return &Tracker{
		samples: make(map[AnchorKind]PoseSample),
		lastTS:  make(map[AnchorKind]int64),
		has:     make(map[AnchorKind]bool),
	}
}

// Submit 提交一个姿态样本；时间戳不新于该源已提交样本时返回 false 并丢弃，
// 因此旧帧永远不会覆盖新姿态，重复样本也只生效一次。
func (t *Tracker) Submit(kind AnchorKind, pose Pose, ts int64) bool {
	if t.has[kind] && ts <= t.lastTS[kind] {
		return false
	}
	t.seq++
	t.samples[kind] = PoseSample{Pose: pose, Timestamp: ts, Seq: t.seq}
	t.lastTS[kind] = ts
	t.has[kind] = true
	return true
}

// Latest 返回某源最新已提交姿态。
func (t *Tracker) Latest(kind AnchorKind) (Pose, bool) {
	if !t.has[kind] {
		return Pose{}, false
	}
	return t.samples[kind].Pose, true
}

// Snapshot 返回所有已提交样本的拷贝，供帧合成使用。
func (t *Tracker) Snapshot() map[AnchorKind]PoseSample {
	out := make(map[AnchorKind]PoseSample, len(t.samples))
	for k, v := range t.samples {
		out[k] = v
	}
	return out
}
