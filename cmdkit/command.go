package cmdkit

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Command 表示命令树中的一个节点。
type Command struct {
	Name        string
	Aliases     []string
	Usage       string
	Description string

	Flags       []Flag       // 本命令的参数（含 Persistent 参数）
	Positionals []Positional // 位置参数（仅叶子命令建议声明）

	// Run 为业务处理函数；仅叶子命令需要。
	Run func(ctx *Context) error

	mu       sync.RWMutex
	parent   *Command
	children []*Command
}

// AddCommand 注册子命令，并发安全。
func (c *Command) AddCommand(subs ...*Command) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, sub := range subs {
		sub.parent = c
		c.children = append(c.children, sub)
	}
	return nil
}

// Parent 返回父命令。
func (c *Command) Parent() *Command { return c.parent }

// Children 返回子命令快照。
func (c *Command) Children() []*Command {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]*Command, len(c.children))
	copy(out, c.children)
	return out
}

// names 返回子命令的主名与全部别名。
func (c *Command) names() []string {
	out := make([]string, 0, 1+len(c.Aliases))
	out = append(out, c.Name)
	out = append(out, c.Aliases...)
	return out
}

// findChild 按 精确名/别名 > 唯一前缀 的顺序解析子命令。
// 前缀存在歧义时返回列出全部候选的错误；找不到时返回带建议的错误。
func (c *Command) findChild(token string) (*Command, error) {
	children := c.Children()

	// 1. 精确匹配（主名或别名）。
	for _, ch := range children {
		for _, n := range ch.names() {
			if n == token {
				return ch, nil
			}
		}
	}

	// 2. 前缀匹配：收集所有候选（同一命令只计一次）。
	var matches []*Command
	for _, ch := range children {
		for _, n := range ch.names() {
			if strings.HasPrefix(n, token) {
				matches = append(matches, ch)
				break
			}
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return nil, fmt.Errorf("unknown command %q%s", token, c.suggest(token))
	default:
		cands := make([]string, 0, len(matches))
		for _, m := range matches {
			cands = append(cands, m.Name)
		}
		sort.Strings(cands)
		return nil, fmt.Errorf("ambiguous command %q, candidates: %s", token, strings.Join(cands, ", "))
	}
}

// suggest 对未匹配的命令名给出最接近的建议。
func (c *Command) suggest(token string) string {
	best := ""
	bestDist := 1 << 30
	for _, ch := range c.Children() {
		for _, n := range ch.names() {
			if d := levenshtein(token, n); d < bestDist {
				bestDist, best = d, n
			}
		}
	}
	// 距离过大时不给建议，避免误导。
	if best == "" || bestDist > max(2, len(token)/2) {
		return ""
	}
	return fmt.Sprintf(", did you mean %q?", best)
}

// levenshtein 计算两个字符串的编辑距离。
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 0
			if ra[i-1] != rb[j-1] {
				cost = 1
			}
			cur[j] = min(prev[j]+1, min(cur[j-1]+1, prev[j-1]+cost))
		}
		prev = cur
	}
	return prev[len(rb)]
}
