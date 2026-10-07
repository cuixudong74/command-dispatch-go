package command

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// buildTree 构造测试用命令树：
// app
//
//	remote (别名 r)
//	  add    参数 --url(必填) --token(继承自 app)
//	  remove (别名 rm)
//	status
//	  show
func buildTree() *Command {
	root := New("app", "测试应用", nil)
	root.Params = []*Param{
		{Name: "token", Usage: "访问令牌", EnvVar: "APP_TOKEN", Default: "root-default"},
		{Name: "verbose", Usage: "详细输出", Type: Bool},
	}
	remote := New("remote", "管理远端", nil)
	remote.Aliases = []string{"r"}
	add := New("add", "添加远端", func(ctx *Context) error { return nil })
	add.Params = []*Param{
		{Name: "url", Usage: "地址", Required: true},
		// 子命令覆盖父级同名参数：默认值与校验规则整体替换。
		{Name: "token", Usage: "覆盖后的令牌", Default: "child-default",
			Validator: func(v string) error {
				if len(v) < 3 {
					return fmt.Errorf("令牌太短")
				}
				return nil
			}},
	}
	remove := New("remove", "删除远端", func(ctx *Context) error { return nil })
	remove.Aliases = []string{"rm"}
	rename := New("rename", "重命名远端", func(ctx *Context) error { return nil })
	remote.AddCommand(add, remove, rename)
	status := New("status", "状态", nil)
	show := New("show", "显示状态", func(ctx *Context) error { return nil })
	status.AddCommand(show)
	root.AddCommand(remote, status)
	return root
}

func TestAmbiguousPrefix(t *testing.T) {
	root := buildTree()
	_, err := root.Parse([]string{"remote", "re"}, nil)
	if err == nil {
		t.Fatal("期望歧义前缀报错")
	}
	var fe *Error
	if !errors.As(err, &fe) || fe.Kind != ErrKindParse {
		t.Fatalf("期望解析错误, 得到 %v", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "歧义") || !strings.Contains(msg, "rename") || !strings.Contains(msg, "remove") {
		t.Fatalf("错误应列出全部候选: %s", msg)
	}
}

func TestUniquePrefixAndAlias(t *testing.T) {
	root := buildTree()
	// 唯一前缀
	res, err := root.Parse([]string{"remo", "a", "--url", "x", "--token", "abc"}, nil)
	if err != nil {
		t.Fatalf("唯一前缀应解析成功: %v", err)
	}
	if res.Command.Name != "add" {
		t.Fatalf("期望 add, 得到 %s", res.Command.Name)
	}
	// 别名
	res, err = root.Parse([]string{"r", "rm"}, nil)
	if err != nil {
		t.Fatalf("别名应解析成功: %v", err)
	}
	if res.Command.Name != "remove" {
		t.Fatalf("期望 remove, 得到 %s", res.Command.Name)
	}
}

func TestUnknownCommandSuggestion(t *testing.T) {
	root := buildTree()
	_, err := root.Parse([]string{"remto"}, nil)
	if err == nil {
		t.Fatal("期望未知命令报错")
	}
	if !strings.Contains(err.Error(), "remote") {
		t.Fatalf("应给出最接近的建议 remote: %s", err)
	}
}

func TestSourcePrecedence(t *testing.T) {
	root := buildTree()
	opts := &ParseOptions{
		Env:    map[string]string{"APP_TOKEN": "from-env"},
		Config: map[string]string{"token": "from-config"},
	}
	// 1. 无 CLI：env 覆盖 config 与 default。
	res, err := root.Parse([]string{"status", "show"}, opts)
	if err != nil {
		t.Fatal(err)
	}
	bp := res.Ctx.Get("token")
	if bp.Value != "from-env" || bp.Source != SourceEnv {
		t.Fatalf("期望 env 来源, 得到 %s(%s)", bp.Value, bp.Source)
	}
	// 2. CLI 覆盖一切。
	res, err = root.Parse([]string{"status", "show", "--token", "from-cli"}, opts)
	if err != nil {
		t.Fatal(err)
	}
	bp = res.Ctx.Get("token")
	if bp.Value != "from-cli" || bp.Source != SourceCLI {
		t.Fatalf("期望 cli 来源, 得到 %s(%s)", bp.Value, bp.Source)
	}
	// 3. 无 env 时 config 生效。
	res, err = root.Parse([]string{"status", "show"},
		&ParseOptions{Env: map[string]string{}, Config: map[string]string{"token": "from-config"}})
	if err != nil {
		t.Fatal(err)
	}
	if bp = res.Ctx.Get("token"); bp.Value != "from-config" || bp.Source != SourceConfig {
		t.Fatalf("期望 config 来源, 得到 %s(%s)", bp.Value, bp.Source)
	}
	// 4. 全部缺省时用内置默认值。
	res, err = root.Parse([]string{"status", "show"},
		&ParseOptions{Env: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	if bp = res.Ctx.Get("token"); bp.Value != "root-default" || bp.Source != SourceDefault {
		t.Fatalf("期望默认值, 得到 %s(%s)", bp.Value, bp.Source)
	}
	// 5. 子命令覆盖处的默认值应为覆盖后的值。
	res, err = root.Parse([]string{"remote", "add", "--url", "u"},
		&ParseOptions{Env: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	if bp = res.Ctx.Get("token"); bp.Value != "child-default" || bp.Source != SourceDefault {
		t.Fatalf("期望覆盖后的默认值, 得到 %s(%s)", bp.Value, bp.Source)
	}
}

func TestInheritanceOverride(t *testing.T) {
	root := buildTree()
	// 在 status show 上，token 仍是父级默认值（未被覆盖）。
	res, err := root.Parse([]string{"status", "show"}, &ParseOptions{Env: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	if bp := res.Ctx.Get("token"); bp.Value != "root-default" {
		t.Fatalf("未覆盖处应保留父级默认值, 得到 %s", bp.Value)
	}
	// 子命令覆盖的校验规则生效：短令牌被拒。
	_, err = root.Parse([]string{"remote", "add", "--url", "u", "--token", "ab"},
		&ParseOptions{Env: map[string]string{}})
	if err == nil || KindOf(err) != ErrKindValidation {
		t.Fatalf("覆盖的校验规则应生效, 得到 %v", err)
	}
	if !strings.Contains(err.Error(), "token") {
		t.Fatalf("错误应关联出错参数: %s", err)
	}
}

func TestGroupConstraints(t *testing.T) {
	cmd := New("mix", "参数组", nil)
	cmd.Params = []*Param{
		{Name: "json", Type: Bool},
		{Name: "yaml", Type: Bool},
		{Name: "out", Usage: "输出"},
		{Name: "fmt", Usage: "格式"},
	}
	cmd.Groups = []*Group{
		{Name: "format", Params: []string{"json", "yaml"}, Mode: GroupMutex},
		{Name: "output", Params: []string{"out", "fmt"}, Mode: GroupAtLeastOne},
	}
	root := New("app", "", nil)
	root.AddCommand(cmd)

	// 互斥冲突。
	_, err := root.Parse([]string{"mix", "--json", "--yaml", "--out", "f"}, nil)
	if err == nil || KindOf(err) != ErrKindValidation {
		t.Fatalf("期望互斥校验错误, 得到 %v", err)
	}
	if !strings.Contains(err.Error(), "互斥") {
		t.Fatalf("提示应说明互斥: %s", err)
	}
	// 至少一个不满足。
	_, err = root.Parse([]string{"mix", "--json"}, nil)
	if err == nil || !strings.Contains(err.Error(), "至少") {
		t.Fatalf("期望至少一个校验错误, 得到 %v", err)
	}
	// 合法组合。
	res, err := root.Parse([]string{"mix", "--json", "--out", "f"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Ctx.Get("json").Bool() {
		t.Fatal("json 应为 true")
	}
}

func TestRequiredAndNoPartialBinding(t *testing.T) {
	root := buildTree()
	// 缺必填参数：Result 必须为 nil（不留部分绑定）。
	res, err := root.Parse([]string{"remote", "add"}, &ParseOptions{Env: map[string]string{}})
	if err == nil || KindOf(err) != ErrKindValidation {
		t.Fatalf("期望必填校验错误, 得到 %v", err)
	}
	if res != nil {
		t.Fatal("校验失败时不得返回部分绑定结果")
	}
	if !strings.Contains(err.Error(), "url") {
		t.Fatalf("错误应指向出错参数 url: %s", err)
	}
}

func TestVariadicAndEnum(t *testing.T) {
	cmd := New("copy", "复制", nil)
	cmd.Params = []*Param{
		{Name: "mode", Enum: []string{"fast", "safe"}, Default: "fast"},
		{Name: "files", Variadic: true, Positional: true},
	}
	root := New("app", "", nil)
	root.AddCommand(cmd)
	res, err := root.Parse([]string{"copy", "a.txt", "b.txt"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Ctx.Get("files").Slice(); len(got) != 2 || got[1] != "b.txt" {
		t.Fatalf("可变参数吸收失败: %v", got)
	}
	// 枚举越界。
	_, err = root.Parse([]string{"copy", "--mode", "slow"}, nil)
	if err == nil || !strings.Contains(err.Error(), "mode") {
		t.Fatalf("枚举校验应关联参数: %v", err)
	}
}

func TestExitCodes(t *testing.T) {
	codes := ExitCodes{Parse: 64, Validation: 65, Business: 70}
	root := buildTree()
	if got := root.Execute([]string{"nope"}, nil, codes); got != 64 {
		t.Fatalf("解析错误退出码应为 64, 得到 %d", got)
	}
	if got := root.Execute([]string{"remote", "add"}, &ParseOptions{Env: map[string]string{}}, codes); got != 65 {
		t.Fatalf("校验错误退出码应为 65, 得到 %d", got)
	}
	biz := New("fail", "", func(ctx *Context) error { return errors.New("boom") })
	root.AddCommand(biz)
	if got := root.Execute([]string{"fail"}, nil, codes); got != 70 {
		t.Fatalf("业务错误退出码应为 70, 得到 %d", got)
	}
	// 相同输入下稳定。
	if root.Execute([]string{"nope"}, nil, codes) != root.Execute([]string{"nope"}, nil, codes) {
		t.Fatal("退出码应稳定")
	}
}

func TestConcurrentRegisterAndParse(t *testing.T) {
	root := buildTree()
	var wg sync.WaitGroup
	// 并发注册。
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			root.AddCommand(New(fmt.Sprintf("extra%d", i), "", nil))
		}(i)
	}
	// 并发解析：每次解析独立，互不串扰。
	errs := make(chan error, 100)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tok := fmt.Sprintf("tok-%d", i)
			res, err := root.Parse([]string{"remote", "add", "--url", "u", "--token", tok}, nil)
			if err != nil {
				errs <- err
				return
			}
			if got := res.Ctx.Get("token").Value; got != tok {
				errs <- fmt.Errorf("串扰: 期望 %s 得到 %s", tok, got)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

func TestHelpConsistency(t *testing.T) {
	root := buildTree()
	h := root.Help()
	for _, want := range []string{"remote", "别名: r", "status", "--token", "默认: root-default", "环境变量: APP_TOKEN"} {
		if !strings.Contains(h, want) {
			t.Fatalf("帮助缺少 %q:\n%s", want, h)
		}
	}
	sub := root.children["remote"].children["add"]
	ha := sub.Help()
	if !strings.Contains(ha, "默认: child-default") {
		t.Fatalf("子命令帮助应展示覆盖后的默认值:\n%s", ha)
	}
}

func TestCompletion(t *testing.T) {
	root := buildTree()
	cmd := New("deploy", "部署", nil)
	cmd.Params = []*Param{{Name: "env", Enum: []string{"dev", "staging", "prod"}}}
	root.AddCommand(cmd)
	for _, shell := range []Shell{Bash, Zsh} {
		s, err := root.CompletionScript(shell)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"remote", "add", "--token", "dev", "staging", "prod"} {
			if !strings.Contains(s, want) {
				t.Fatalf("%s 补全脚本缺少 %q", shell, want)
			}
		}
	}
	if _, err := root.CompletionScript("fish"); err == nil {
		t.Fatal("不支持的 shell 应报错")
	}
}
