package spatial

import (
	"sync"
	"testing"
)

// 并发读取：写方持续提交姿态与布局，读方拿到的每一帧都必须自洽——
// 帧内所有头锚面板的世界姿态必须对应同一个已提交姿态快照。
// 用 go test -race 运行以验证无数据竞争。
func TestConcurrentReadersSeeConsistentFrames(t *testing.T) {
	e := newEngineWithHeadAnchor()
	for _, id := range []PanelID{"a", "b", "c"} {
		e.AddPanel(Panel{ID: id, Anchor: "head", Group: "g", Order: int(id[0]), Size: Vec2{1, 1}})
	}
	e.Commit()

	stop := make(chan struct{})
	var wg sync.WaitGroup

	// 写方：持续推进头部姿态。
	wg.Add(1)
	go func() {
		defer wg.Done()
		for ts := int64(1); ts <= 2000; ts++ {
			e.SubmitPose(AnchorHead, Pose{
				Position:    Vec3{float64(ts), 0, 0},
				Orientation: IdentityQuat(),
			}, ts)
			e.Commit()
		}
		close(stop)
	}()

	// 慢读方：逐帧校验自洽性，不阻塞写方。
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				f := e.Snapshot()
				if f == nil || len(f.Panels) == 0 {
					continue
				}
				headY := f.Poses[AnchorHead].Pose.Position
				for _, st := range f.Panels {
					// 面板世界位置 = 头部位置 + 局部位移（本测试布局无旋转）。
					want := Add(headY, st.Local.Position)
					if !almostEq(st.World.Position.X, want.X) ||
						!almostEq(st.World.Position.Y, want.Y) ||
						!almostEq(st.World.Position.Z, want.Z) {
						t.Errorf("frame %d inconsistent: panel %s world=%v local=%v head=%v",
							f.Seq, st.Panel.ID, st.World.Position, st.Local.Position, headY)
						return
					}
				}
			}
		}()
	}
	wg.Wait()
}

// 帧单调递增且只保留最新帧：内存不随运行时间增长。
func TestFrameSeqMonotonic(t *testing.T) {
	e := newEngineWithHeadAnchor()
	e.AddPanel(Panel{ID: "a", Anchor: "room", Group: "g", Size: Vec2{1, 1}})
	var last uint64
	for i := 0; i < 100; i++ {
		seq := e.Commit()
		if seq <= last {
			t.Fatalf("seq not monotonic: %d after %d", seq, last)
		}
		last = seq
	}
	if e.Snapshot().Seq != last {
		t.Fatal("snapshot is not the latest frame")
	}
}
