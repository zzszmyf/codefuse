package index

import (
	"bytes"
	"encoding/gob"
	"os"
	"path/filepath"
	"testing"
	
	"github.com/yifanmeng/codefuse/pkg/types"
)

func BenchmarkPhases(b *testing.B) {
	dir := "/Users/yifanmeng/Project/dubbo/.codefuse"
	data, err := os.ReadFile(filepath.Join(dir, "graph.gob"))
	if err != nil {
		b.Skip("no index found")
	}

	b.Run("DiskRead", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			os.ReadFile(filepath.Join(dir, "graph.gob"))
		}
	})

	b.Run("GobDecode", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var tg types.Graph
			gob.NewDecoder(bytes.NewReader(data)).Decode(&tg)
		}
	})

	b.Run("BuildIndexes", func(b *testing.B) {
		var tg types.Graph
		gob.NewDecoder(bytes.NewReader(data)).Decode(&tg)
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			tg.BuildIndexes()
		}
	})

	b.Run("ReadLine", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			ReadLine("/Users/yifanmeng/Project/dubbo/dubbo-registry/dubbo-registry-api/src/main/java/org/apache/dubbo/registry/RegistryService.java", 29)
		}
	})
}
