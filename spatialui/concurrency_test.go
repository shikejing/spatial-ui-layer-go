package spatialui

import (
	"sync"
	"testing"
)

// 布局更新与姿态推进并发进行时，慢读取方不得阻塞写入，
// 且任一读取方看到的必须是自洽的单个快照：
// 快照内所有面板的位姿必须对应同一份已提交姿态。
func TestConcurrentReadersSeeConsistentSnapshots(t *testing.T) {
	e := NewEngine()
	e.AddPanel(Panel{ID: "head-panel", Anchor: AnchorHead,
		LocalOffset: Pose{Position: Vec3{Z: -1}, Orientation: QuatIdentity()}})
	e.AddPanel(Panel{ID: "world-panel", Anchor: AnchorWorld,
		LocalOffset: Pose{Position: Vec3{X: 1}, Orientation: QuatIdentity()}})

	const writers = 4
	const readers = 8
	const frames = 200

	var wg sync.WaitGroup
	var wwg sync.WaitGroup
	stop := make(chan struct{})

	// 读取方：持续加载快照并校验自洽性。
	for r := 0; r < readers; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				s := e.Snapshot()
				if s == nil {
					continue
				}
				// 自洽性：头部锚定面板位置必须等于快照内头部姿态 + 固定偏移。
				var hp *PlacedPanel
				for i := range s.Placed {
					if s.Placed[i].Panel.ID == "head-panel" {
						hp = &s.Placed[i]
					}
				}
				if hp != nil {
					want := s.Poses.Head.Pose.TransformPoint(Vec3{Z: -1})
					if d := hp.Pose.Position.Sub(want).Len(); d > 1e-9 {
						t.Errorf("inconsistent snapshot: panel off by %v", d)
						return
					}
				}
			}
		}()
	}

	// 写入方：交错推进姿态与帧。
	for w := 0; w < writers; w++ {
		wwg.Add(1)
		go func(id int) {
			defer wwg.Done()
			for i := 0; i < frames; i++ {
				ts := int64(id*frames + i + 1)
				e.SubmitPose(TrackHead, PoseSample{
					Pose:      Pose{Position: Vec3{X: float64(ts)}, Orientation: QuatIdentity()},
					Timestamp: ts,
				})
				e.Tick()
			}
		}(w)
	}

	// 写入方完成后停止读取方。
	wwg.Wait()
	close(stop)
	wg.Wait()
}

// 慢读取方持有旧快照不影响后续发布，旧快照内容不被修改。
func TestSlowReaderKeepsImmutableSnapshot(t *testing.T) {
	e := NewEngine()
	e.AddPanel(Panel{ID: "p", Anchor: AnchorHead,
		LocalOffset: Pose{Position: Vec3{Z: -1}, Orientation: QuatIdentity()}})
	e.SubmitPose(TrackHead, sample(1, 0))
	s1 := e.Tick()
	x1 := s1.Placed[0].Pose.Position.X

	for i := int64(2); i < 100; i++ {
		e.SubmitPose(TrackHead, sample(i, float64(i)))
		e.Tick()
	}
	// 旧快照必须保持原值。
	if s1.Placed[0].Pose.Position.X != x1 {
		t.Fatal("old snapshot mutated by later frames")
	}
	if e.Snapshot().Frame <= s1.Frame {
		t.Fatal("frame did not advance")
	}
}
