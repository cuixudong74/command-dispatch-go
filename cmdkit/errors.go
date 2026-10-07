package cmdkit

import "fmt"

// Kind 区分错误类别：解析错误、校验错误、业务处理错误。
type Kind int

const (
	KindParse      Kind = iota // 解析错误（未知命令、未知参数、缺值等）
	KindValidation             // 校验错误（必填缺失、互斥冲突、自定义校验失败等）
	KindBusiness               // 业务处理错误（Run 返回的错误）
)

// Error 是框架统一错误类型，携带类别与可配置退出码。
type Error struct {
	Kind     Kind
	ExitCode int
	Msg      string
}

func (e *Error) Error() string { return e.Msg }

// ExitCodes 可配置的退出码集合。
type ExitCodes struct {
	Parse      int
	Validation int
	Business   int
}

// DefaultExitCodes 为默认退出码。
func DefaultExitCodes() ExitCodes {
	return ExitCodes{Parse: 2, Validation: 3, Business: 1}
}

func newError(kind Kind, codes ExitCodes, format string, args ...any) *Error {
	e := &Error{Kind: kind, Msg: fmt.Sprintf(format, args...)}
	switch kind {
	case KindParse:
		e.ExitCode = codes.Parse
	case KindValidation:
		e.ExitCode = codes.Validation
	case KindBusiness:
		e.ExitCode = codes.Business
	}
	return e
}
