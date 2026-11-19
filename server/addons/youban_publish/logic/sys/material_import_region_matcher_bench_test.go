package sys

import (
	"fmt"
	"testing"
)

var materialImportBenchmarkProvince, materialImportBenchmarkCity string

// SmallFixture is illustrative, not a production dictionary snapshot.
// Synthetic4000 adds generated distractors to measure dictionary-size scaling;
// it must not be reported as a production latency/throughput benchmark.
func BenchmarkMaterialImportRegionMatcher(b *testing.B) {
	for _, size := range []int{0, 4000} {
		name := "SmallFixture"
		if size > 0 {
			name = "Synthetic4000"
		}
		b.Run(name, func(b *testing.B) {
			index := materialImportMatcherFixture()
			for i := 0; i < size; i++ {
				row := &legacyCMSRegionOption{Id: int64(7000000 + i), Pid: 330000, Level: 2, Title: fmt.Sprintf("合成城市%04d号", i)}
				index.optionsById[row.Id] = row
				index.citiesByName[row.Title] = []*legacyCMSRegionOption{row}
			}
			// Warm-up is outside both timed variants. No SQL/cache I/O is measured.
			index.regionMatcher()
			for _, input := range []struct{ name, text string }{
				{"ExactHangzhou", "城市：杭州"},
				{"AliasMultiple", "城市：黔南/杭州/宁波"},
				{"ComplexValue", "城市：常驻杭州及宁波"},
				{"OverseasGroup", "国家：新马泰"},
			} {
				b.Run(input.name, func(b *testing.B) {
					b.Run("BaselineLoops", func(b *testing.B) {
						b.ReportAllocs()
						for b.Loop() {
							materialImportBenchmarkProvince, materialImportBenchmarkCity = baselineMaterialImportRegionCodesFromIndex(input.text, index)
						}
					})
					b.Run("Compiled", func(b *testing.B) {
						b.ReportAllocs()
						for b.Loop() {
							materialImportBenchmarkProvince, materialImportBenchmarkCity = materialImportRegionCodesFromIndex(input.text, index)
						}
					})
				})
			}
		})
	}
}
