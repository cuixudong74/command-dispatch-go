// Package cmdkit 提供可复用的命令解析与分发框架：
// 使用者只需声明命令与参数的结构和约束，解析、校验与绑定由框架完成。
package cmdkit

// Source 表示参数最终取值的来源。
type Source int

const (
	SourceNone    Source = iota // 未取值
	SourceDefault                // 内置默认值
	SourceConfig                 // 配置文件
	SourceEnv                    // 环境变量
	SourceFlag                   // 命令行（优先级最高）
)

func (s Source) String() string {
	switch s {
	case SourceDefault:
		return "default"
	case SourceConfig:
		return "config"
	case SourceEnv:
		return "env"
	case SourceFlag:
		return "flag"
	default:
		return "none"
	}
}

// Validator 是自定义校验函数，入参为最终取值，返回 nil 表示通过。
type Validator func(value string) error

// Flag 声明一个命名参数（--name value 形式）。
type Flag struct {
	Name      string   // 长名，如 "output"
	Shorthand string   // 单字符短名，如 "o"，可为空
	Aliases   []string // 额外别名
	Usage     string   // 帮助说明
	Required  bool     // 是否必填
	Default   string   // 内置默认值
	EnvVars   []string // 关联环境变量（按顺序取第一个存在的）
	ConfigKey string   // 配置文件键
	Choices   []string // 枚举取值约束，为空表示不限制
	Validator Validator

	// MutexGroup 内所有 Flag 互斥：同一组最多出现一个（按最终是否显式赋值判断）。
	MutexGroup string
	// OneOfGroup 内所有 Flag 至少显式出现一个。
	OneOfGroup string

	// Persistent 为 true 时，该参数被所有子孙命令继承。
	Persistent bool
}

// Positional 声明位置参数（可变数量）。
type Positional struct {
	Name      string
	Usage     string
	Min       int // 最少个数
	Max       int // 最多个数，-1 表示不限
	Validator Validator
}

// BoundValue 记录一个参数的最终取值及其来源。
type BoundValue struct {
	Value  string
	Source Source
}

// Context 是业务处理函数的入参，承载解析结果。
type Context struct {
	Command *Command
	values  map[string]BoundValue
	args    []string
}

// Value 返回命名参数的最终取值与来源。
func (c *Context) Value(name string) (string, Source) {
	bv, ok := c.values[name]
	if !ok {
		return "", SourceNone
	}
	return bv.Value, bv.Source
}

// Args 返回绑定后的位置参数。
func (c *Context) Args() []string { return c.args }
