package spatialui

import (
	"fmt"
	"testing"
)

// 面板总数增长时，单次 Tick 的布局重算只触及受影响的组。
// 对比 n=100 与 n=1000 的 Tick 耗时，增长应远小于 10 倍
// （快照发布本身为 O(n)，重算部分为 O(受影响组)）。
func BenchmarkTickAfterLocalizedChange(b *testing.B) {
	for _, n := range []int{100, 1000} {
		b.Run(fmt.Sprintf("panels=%d", n), func(b *testing.B) {
			e := NewEngine()
			// 面板分布在 n/10 个小组中，每组 10 块。
			for i := 0; i < n; i++ {
				g := fmt.Sprintf("g%04d", i/10)
				e.SetLayout(g, GridLayout{CellWidth: 1, CellHeight: 1, Columns: 5}, AnchorWorld, HandLeft)
				e.AddPanel(Panel{ID: PanelID(fmt.Sprintf("p%04d", i)), Group: g})
			}
			e.Tick()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// 每次只改动一个面板：只有一个 10 块面板的组被重排，
				// 重算耗时与面板总数无关（快照发布本身为 O(n)，属固有成本）。
				e.Resize("p0000", Size{Width: float64(i%3 + 1), Height: 1})
				e.Tick()
			}
		})
	}
}
