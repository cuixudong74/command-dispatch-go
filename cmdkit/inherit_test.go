package cmdkit

import (
	"fmt"
	"sync"
	"testing"
)

func inheritTree() *Command {
	root := &Command{
		Name: "app",
		Flags: []Flag{
			{Name: "verbose", Shorthand: "v", Persistent: true, Default: "false"},
			{Name: "timeout", Persistent: true, Default: "30",
				Validator: func(v string) error {
					if v == "never" {
						return fmt.Errorf("not allowed")
					}
					return nil
				}},
		},
	}
	parent := &Command{Name: "db"}
	// 子命令覆盖同名继承参数的默认值与校验规则。
	child := &Command{
		Name: "migrate",
		Flags: []Flag{
			{Name: "timeout", Default: "120", Validator: nil}, // 覆盖默认值
		},
	}
	parent.AddCommand(child)
	root.AddCommand(parent)
	return root
}

func TestPersistentFlagInherited(t *testing.T) {
	p := NewParser()
	ctx, err := p.Parse(inheritTree(), ParseOptions{
		Args: []string{"db", "migrate", "--verbose", "true"},
		Env:  map[string]string{},
	})
	if err != nil {
		t.Fatalf("inherited flag not recognized: %v", err)
	}
	if v, s := ctx.Value("verbose"); v != "true" || s != SourceFlag {
		t.Fatalf("got (%q, %s)", v, s)
	}
}

func TestChildOverridesDefault(t *testing.T) {
	p := NewParser()
	ctx, err := p.Parse(inheritTree(), ParseOptions{
		Args: []string{"db", "migrate"},
		Env:  map[string]string{},
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	// 子命令覆盖后的默认值
	if v, s := ctx.Value("timeout"); v != "120" || s != SourceDefault {
		t.Fatalf("got (%q, %s), want (120, default)", v, s)
	}
	// 父级未覆盖，仍是 30
	ctx2, err := p.Parse(inheritTree(), ParseOptions{Args: []string{"db"}, Env: map[string]string{}})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if v, _ := ctx2.Value("timeout"); v != "30" {
		t.Fatalf("parent default got %q, want 30", v)
	}
}

func TestConcurrentParse(t *testing.T) {
	p := NewParser()
	root := &Command{Name: "app"}
	for i := 0; i < 8; i++ {
		name := fmt.Sprintf("cmd%d", i)
		root.AddCommand(&Command{
			Name:  name,
			Flags: []Flag{{Name: "val", Default: "d"}},
			Run:   func(ctx *Context) error { return nil },
		})
	}

	var wg sync.WaitGroup
	errs := make(chan error, 256)
	// 并发注册 + 并发解析交错进行。
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			root.AddCommand(&Command{Name: fmt.Sprintf("late%d", i)})
		}(i)
	}
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			want := fmt.Sprintf("v%d", i)
			ctx, err := p.Parse(root, ParseOptions{
				Args: []string{fmt.Sprintf("cmd%d", i%8), "--val", want},
				Env:  map[string]string{},
			})
			if err != nil {
				errs <- err
				return
			}
			if v, s := ctx.Value("val"); v != want || s != SourceFlag {
				errs <- fmt.Errorf("goroutine %d: got (%q,%s), want (%q,flag)", i, v, s, want)
			}
			if ctx.Command.Name != fmt.Sprintf("cmd%d", i%8) {
				errs <- fmt.Errorf("goroutine %d: wrong command %q", i, ctx.Command.Name)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}
