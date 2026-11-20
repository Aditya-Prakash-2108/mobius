package roles

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestStoreDefensiveCopy(t *testing.T) {
	dir := t.TempDir()
	writeEngineering(t, dir)
	store := openStore(t, dir)

	role, ok := store.Role("software_engineer")
	if !ok {
		t.Fatal("missing software engineer")
	}
	role.Tools[0] = "mutated"
	role.Name = "mutated"

	again, ok := store.Role("software_engineer")
	if !ok {
		t.Fatal("missing software engineer")
	}
	if again.Tools[0] != "view_file" || again.Name != "Software Engineer" {
		t.Fatalf("stored role changed: %+v", again)
	}

	dept, ok := store.Department("engineering")
	if !ok {
		t.Fatal("missing engineering department")
	}
	dept.Roles[0] = "mutated"
	againDept, _ := store.Department("engineering")
	if againDept.Roles[0] != "software_engineer" {
		t.Fatalf("stored department changed: %+v", againDept)
	}
}

func TestReloadKeepsPreviousCatalog(t *testing.T) {
	dir := t.TempDir()
	writeEngineering(t, dir)
	store := openStore(t, dir)

	writeFile(t, filepath.Join(dir, "software_engineer.yml"), "id: NOT VALID\n")
	err := store.Reload(context.Background())
	if !errors.Is(err, ErrInvalidID) {
		t.Fatalf("reload error = %v", err)
	}
	role, ok := store.Role("software_engineer")
	if !ok || role.Name != "Software Engineer" {
		t.Fatalf("previous role = %+v ok=%v", role, ok)
	}
}

func TestStoreConcurrentReloadAndRead(t *testing.T) {
	dir := t.TempDir()
	writeEngineering(t, dir)
	store := openStore(t, dir)

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	var failed atomic.Bool
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				if err := store.Reload(ctx); err != nil && ctx.Err() == nil {
					failed.Store(true)
					cancel()
					return
				}
			}
		}()
	}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				role, ok := store.Role("software_engineer")
				if !ok || len(role.Tools) == 0 || role.Tools[0] != "view_file" {
					failed.Store(true)
					cancel()
					return
				}
				role.Tools[0] = "mutated"
				dept, ok := store.Department("engineering")
				if !ok || len(dept.Roles) != 2 || dept.Roles[0] != "software_engineer" {
					failed.Store(true)
					cancel()
					return
				}
				dept.Roles[0] = "mutated"
				_ = store.Roles()
				_ = store.Departments()
			}
		}()
	}
	wg.Wait()
	if failed.Load() {
		t.Fatal("concurrent reload or read observed a torn catalog")
	}
}

func openStore(t *testing.T, dir string) *Store {
	t.Helper()
	store, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return store
}
