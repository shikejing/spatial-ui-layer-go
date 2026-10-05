package spatialui

// TrackKind 标识一路追踪源。
type TrackKind int

const (
	TrackHead TrackKind = iota
	TrackHandLeft
	TrackHandRight
	trackKindCount
)

// PoseSample 是带时间戳与序列号的一帧姿态。
type PoseSample struct {
	Pose      Pose
	Timestamp int64 // 单调时钟时间戳，旧帧不得覆盖新帧
	Seq       uint64
}

// PoseSnapshot 是某一时刻全部追踪源的自洽姿态集合。
type PoseSnapshot struct {
	Head      PoseSample
	HandLeft  PoseSample
	HandRight PoseSample
}

// PoseTracker 接收乱序/重复/抖动的姿态输入，只保留每路最新样本。
// 不是并发安全的，调用方（Engine）负责串行化写入。
type PoseTracker struct {
	samples [trackKindCount]PoseSample
	seen    [trackKindCount]bool
}

func NewPoseTracker() *PoseTracker { return &PoseTracker{} }

// Submit 提交一路姿态样本。
// 仅当样本时间戳严格大于已提交时间戳时接受；
// 乱序（更旧）与重复（相同时间戳）样本被拒绝并返回 false，保证幂等。
func (t *PoseTracker) Submit(kind TrackKind, s PoseSample) bool {
	if kind < 0 || kind >= trackKindCount {
		return false
	}
	if t.seen[kind] && s.Timestamp <= t.samples[kind].Timestamp {
		return false
	}
	t.samples[kind] = s
	t.seen[kind] = true
	return true
}

// Snapshot 返回当前全部追踪源的一致快照。
// 快照是值拷贝，读取方看到的永远是同一批已提交样本。
func (t *PoseTracker) Snapshot() PoseSnapshot {
	return PoseSnapshot{
		Head:      t.samples[TrackHead],
		HandLeft:  t.samples[TrackHandLeft],
		HandRight: t.samples[TrackHandRight],
	}
}
