// Package command 提供可复用的命令解析与分发框架。
// 使用方只需声明命令与参数的结构和约束，
// 解析、校验与绑定全部由框架完成。
package command

// ValueType 参数取值类型。
type ValueType int

const (
	String ValueType = iota
	Int
	Float
	Bool
	StringSlice // 逗号分隔或重复出现
)

// Source 标记参数最终取值的来源。
type Source int

const (
	SourceNone    Source = iota // 未取到值
	SourceDefault               // 内置默认值
	SourceConfig                // 配置文件
	SourceEnv                   // 环境变量
	SourceCLI                   // 命令行
)

// String 返回来源的可读名称。
func (s Source) String() string {
	switch s {
	case SourceDefault:
		return "default"
	case SourceConfig:
		return "config"
	case SourceEnv:
		return "env"
	case SourceCLI:
		return "cli"
	default:
		return "none"
	}
}

// GroupMode 参数组约束模式。
type GroupMode int

const (
	GroupMutex      GroupMode = iota // 组内参数互斥，至多一个
	GroupAtLeastOne                  // 组内至少出现一个
)

// ErrorKind 区分错误类别：解析错误 / 校验错误 / 业务处理错误。
type ErrorKind int

const (
	ErrKindParse      ErrorKind = iota // 命令行解析错误（未知命令、缺值等）
	ErrKindValidation                  // 参数校验错误（必填、互斥、自定义校验等）
	ErrKindBusiness                    // 业务处理错误（Run 返回）
)

// ExitCodes 各类错误对应的退出码，可配置。
type ExitCodes struct {
	Parse      int
	Validation int
	Business   int
}

// DefaultExitCodes 默认退出码。
func DefaultExitCodes() ExitCodes {
	return ExitCodes{Parse: 2, Validation: 3, Business: 1}
}
