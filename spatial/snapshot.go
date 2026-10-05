package spatial

import "sync/atomic"

// Frame 是某一提交帧的不可变快照：姿态、面板解算结果与焦点。
// 帧发布后绝不修改，读方拿到的永远是自洽的单个快照。
type Frame struct {
	Seq      uint64
	Poses    map[AnchorKind]PoseSample
	Panels   []PanelState // 按面板 ID 升序，顺序稳定
	Focused  PanelID
	HasFocus bool
}

// FrameStore 以原子指针发布最新帧：读方无锁，慢读取方不阻塞写方；
// 写方只保留最新帧，内存不随运行时间增长。
type FrameStore struct {
	cur atomic.Pointer[Frame]
}

func NewFrameStore() *FrameStore { return &FrameStore{} }

func (s *FrameStore) Publish(f *Frame) { s.cur.Store(f) }

// Latest 返回最新已发布帧；尚未发布时返回 nil。
func (s *FrameStore) Latest() *Frame { return s.cur.Load() }
