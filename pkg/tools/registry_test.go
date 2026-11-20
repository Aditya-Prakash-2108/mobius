package tools

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type stubTool struct {
	name string
}

func (s stubTool) Name() string        { return s.name }
func (s stubTool) Description() string { return "stub" }
func (s stubTool) Schema() ToolSchema  { return ToolSchema{Type: "object"} }
func (s stubTool) Execute(ctx context.Context, jsonArgs string) (string, error) {
	return "", nil
}

func TestNewRegistrySubsetFailClosed(t *testing.T) {
	base := NewDefaultRegistry(t.TempDir())
	if got := len(base.List()); got != 6 {
		t.Fatalf("default registry has %d tools", got)
	}

	cases := []struct {
		name  string
		base  *Registry
		names []string
		want  []string
	}{
		{name: "nil names", base: base, names: nil, want: nil},
		{name: "empty names", base: base, names: []string{}, want: nil},
		{name: "nil base", base: nil, names: []string{"view_file"}, want: nil},
		{name: "unknown skipped", base: base, names: []string{"view_file", "bash", "view_file"}, want: []string{"view_file"}},
		{name: "sorted subset", base: base, names: []string{"view_file", "edit_file"}, want: []string{"edit_file", "view_file"}},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := toolNames(NewRegistrySubset(tt.base, tt.names).List())
			if len(got) != len(tt.want) {
				t.Fatalf("tools = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("tools = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestRegistrySubsetConcurrent(t *testing.T) {
	base := NewRegistry()
	base.Register(stubTool{name: "view_file"})

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	var failed atomic.Bool
	var seq atomic.Int64
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for ctx.Err() == nil {
			name := "extra"
			if seq.Add(1)%2 == 0 {
				name = "view_file"
			}
			base.Register(stubTool{name: name})
			_ = base.List()
		}
	}()

	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				subset := NewRegistrySubset(base, []string{"view_file", "missing"})
				names := toolNames(subset.List())
				if len(names) != 1 || names[0] != "view_file" {
					failed.Store(true)
					cancel()
					return
				}
				if got := len(NewRegistrySubset(base, nil).List()); got != 0 {
					failed.Store(true)
					cancel()
					return
				}
				if got := len(NewRegistrySubset(nil, []string{"view_file"}).List()); got != 0 {
					failed.Store(true)
					cancel()
					return
				}
			}
		}()
	}

	wg.Wait()
	if failed.Load() {
		t.Fatal("subset granted the wrong tools under concurrent registration")
	}
}

func TestSetWorkDirUpdatesSharedTools(t *testing.T) {
	dir := t.TempDir()
	next := t.TempDir()
	base := NewDefaultRegistry(dir)
	subset := NewRegistrySubset(base, []string{"view_file", "run_command"})
	base.SetWorkDir(next)
	view, err := subset.Get("view_file")
	if err != nil {
		t.Fatal(err)
	}
	if view.(*ViewFileTool).WorkingDir != next {
		t.Fatalf("view dir %q", view.(*ViewFileTool).WorkingDir)
	}
	bash, err := base.Get("run_command")
	if err != nil {
		t.Fatal(err)
	}
	if bash.(*BashTool).WorkingDir != next {
		t.Fatalf("bash dir %q", bash.(*BashTool).WorkingDir)
	}
}

func toolNames(list []Tool) []string {
	names := make([]string, len(list))
	for i := range list {
		names[i] = list[i].Name()
	}
	return names
}
