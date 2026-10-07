package command

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Context 传递给业务处理函数的上下文。
type Context struct {
	Command *Command
	Bound   map[string]*BoundParam // 按参数名索引
	Args    []string               // 未被消费的剩余参数
}

// Get 按参数名取绑定结果，不存在时返回 nil。
func (c *Context) Get(name string) *BoundParam { return c.Bound[name] }

// Command 命令树节点。注册与查找均可在并发下安全调用。
type Command struct {
	Name    string
	Aliases []string
	Usage   string
	Params  []*Param
	Groups  []*Group
	Run     func(ctx *Context) error

	mu       sync.RWMutex
	parent   *Command
	children map[string]*Command // 主名 -> 子命令
	byAlias  map[string]*Command // 别名 -> 子命令
}

// New 创建命令。
func New(name, usage string, run func(ctx *Context) error) *Command {
	return &Command{
		Name:     name,
		Usage:    usage,
		Run:      run,
		children: make(map[string]*Command),
		byAlias:  make(map[string]*Command),
	}
}

// AddCommand 注册子命令，支持并发调用。
// 同名或同别名的重复注册以最后一次为准（覆盖语义，不视为冲突）。
func (c *Command) AddCommand(subs ...*Command) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, s := range subs {
		s.parent = c
		c.children[s.Name] = s
		for _, a := range s.Aliases {
			c.byAlias[a] = s
		}
	}
}

// Lookup 按名称/别名/唯一前缀查找子命令。
// 前缀存在歧义时报错并列出全部候选；不存在时给出最接近的建议。
func (c *Command) Lookup(token string) (*Command, error) {
	c.mu.RLock()
	exact, okExact := c.children[token]
	alias, okAlias := c.byAlias[token]
	if okExact {
		c.mu.RUnlock()
		return exact, nil
	}
	if okAlias {
		c.mu.RUnlock()
		return alias, nil
	}
	// 前缀匹配：主名与别名都参与，按命令去重。
	seen := map[*Command]bool{}
	var matches []*Command
	collect := func(m map[string]*Command) {
		for name, cmd := range m {
			if strings.HasPrefix(name, token) && !seen[cmd] {
				seen[cmd] = true
				matches = append(matches, cmd)
			}
		}
	}
	collect(c.children)
	collect(c.byAlias)
	if len(matches) == 1 {
		c.mu.RUnlock()
		return matches[0], nil
	}
	if len(matches) > 1 {
		names := make([]string, len(matches))
		for i, m := range matches {
			names[i] = m.Name
		}
		sort.Strings(names)
		c.mu.RUnlock()
		return nil, &Error{
			Kind:   ErrKindParse,
			Msg:    fmt.Sprintf("前缀 %q 存在歧义", token),
			Causes: names,
		}
	}
	// 未命中：收集全部名字用于建议。
	names := make([]string, 0, len(c.children)+len(c.byAlias))
	for n := range c.children {
		names = append(names, n)
	}
	for n := range c.byAlias {
		names = append(names, n)
	}
	c.mu.RUnlock()
	err := &Error{Kind: ErrKindParse, Msg: fmt.Sprintf("未知命令 %q", token)}
	if s := Suggest(token, names); len(s) > 0 {
		err.Causes = append([]string{"你是想要"}, s...)
	}
	return nil, err
}

// Path 返回从根到当前命令的路径名，如 "app remote add"。
func (c *Command) Path() string {
	var parts []string
	for cur := c; cur != nil; cur = cur.parent {
		parts = append([]string{cur.Name}, parts...)
	}
	return strings.Join(parts, " ")
}

// effectiveParams 合并父链参数：父级参数对全部子命令可见，
// 子命令对同名参数的声明整体覆盖父级（默认值、校验规则等随之替换），
// 覆盖关系为"就近优先"，重复定义不视为冲突。
func (c *Command) effectiveParams() []*Param {
	var chain []*Command
	for cur := c; cur != nil; cur = cur.parent {
		chain = append([]*Command{cur}, chain...)
	}
	merged := map[string]*Param{}
	var order []string
	for _, cmd := range chain {
		for _, p := range cmd.Params {
			if _, ok := merged[p.Name]; !ok {
				order = append(order, p.Name)
			}
			merged[p.Name] = p // 子级覆盖父级
		}
	}
	out := make([]*Param, 0, len(order))
	for _, name := range order {
		out = append(out, merged[name])
	}
	return out
}

// effectiveGroups 合并父链参数组，同名组子级覆盖父级。
func (c *Command) effectiveGroups() []*Group {
	var chain []*Command
	for cur := c; cur != nil; cur = cur.parent {
		chain = append([]*Command{cur}, chain...)
	}
	merged := map[string]*Group{}
	var order []string
	for _, cmd := range chain {
		for _, g := range cmd.Groups {
			if _, ok := merged[g.Name]; !ok {
				order = append(order, g.Name)
			}
			merged[g.Name] = g
		}
	}
	out := make([]*Group, 0, len(order))
	for _, name := range order {
		out = append(out, merged[name])
	}
	return out
}

// findParam 在生效参数集中按名或别名查找。
func findParam(params []*Param, token string) *Param {
	for _, p := range params {
		if p.Name == token {
			return p
		}
	}
	for _, p := range params {
		for _, a := range p.Aliases {
			if a == token {
				return p
			}
		}
	}
	return nil
}
