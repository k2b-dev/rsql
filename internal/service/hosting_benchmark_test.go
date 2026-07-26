package service

import (
	"database/sql"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/k2b-dev/rsql/internal/domain"
	"github.com/k2b-dev/rsql/internal/store/control"
)

func BenchmarkFleetListing(b *testing.B) {
	for _, count := range []int{100, 250, 500} {
		b.Run(fmt.Sprintf("provisioned_%d", count), func(b *testing.B) {
			svc, err := New(b.TempDir(), 0)
			if err != nil {
				b.Fatal(err)
			}
			b.Cleanup(func() { _ = svc.Close() })
			for i := range count {
				name := fmt.Sprintf("tenant_%03d", i)
				if err := svc.registry.Create(name, control.PathFor(svc.dataDir, name)); err != nil {
					b.Fatal(err)
				}
			}

			b.ResetTimer()
			for range b.N {
				if _, err := svc.ListNamespaces(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkActiveNamespaceReads(b *testing.B) {
	for _, count := range []int{25, 50, 100} {
		b.Run(fmt.Sprintf("active_%d", count), func(b *testing.B) {
			svc, err := New(b.TempDir(), 0)
			if err != nil {
				b.Fatal(err)
			}
			b.Cleanup(func() { _ = svc.Close() })
			names := make([]string, count)
			for i := range count {
				names[i] = fmt.Sprintf("tenant_%03d", i)
				if _, err := svc.CreateNamespace(domain.NamespaceDefinition{
					Name:   names[i],
					Config: defaultNamespaceConfig(),
				}); err != nil {
					b.Fatal(err)
				}
				if err := svc.ns.CloseHandle(names[i]); err != nil {
					b.Fatal(err)
				}
			}

			var sequence atomic.Uint64
			var failed atomic.Bool
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					name := names[int(sequence.Add(1))%len(names)]
					if err := svc.WithNamespaceRead(name, func(db *sql.DB) error {
						var value int
						return db.QueryRow(`SELECT 1`).Scan(&value)
					}); err != nil {
						failed.Store(true)
					}
				}
			})
			if failed.Load() {
				b.Fatal("active namespace read failed")
			}
		})
	}
}
