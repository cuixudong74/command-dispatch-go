package command

import (
	"errors"
	"fmt"
	"strings"
)

// Error 框架错误，携带类别与相关参数信息。
type Error struct {
	Kind   ErrorKind
	Param  string   // 相关参数名（校验错误时填充）
	Msg    string   // 人类可读提示
	Causes []string // 附加信息，如歧义前缀的全部候选、相似命令建议
}

func (e *Error) Error() string {
	var b strings.Builder
	if e.Param != "" {
		fmt.Fprintf(&b, "参数 --%s: ", e.Param)
	}
	b.WriteString(e.Msg)
	if len(e.Causes) > 0 {
		fmt.Fprintf(&b, "（%s）", strings.Join(e.Causes, ", "))
	}
	return b.String()
}

// KindOf 从 error 中提取 ErrorKind，非框架错误视为业务错误。
func KindOf(err error) ErrorKind {
	var fe *Error
	if errors.As(err, &fe) {
		return fe.Kind
	}
	return ErrKindBusiness
}

// ExitCode 按配置把错误映射为退出码；err 为 nil 时返回 0。
// 相同输入下结果稳定（纯函数，无内部状态）。
func ExitCode(err error, codes ExitCodes) int {
	if err == nil {
		return 0
	}
	switch KindOf(err) {
	case ErrKindParse:
		return codes.Parse
	case ErrKindValidation:
		return codes.Validation
	default:
		return codes.Business
	}
}

// 便捷构造函数。
func parseErr(format string, args ...any) *Error {
	return &Error{Kind: ErrKindParse, Msg: fmt.Sprintf(format, args...)}
}

func validErr(param, format string, args ...any) *Error {
	return &Error{Kind: ErrKindValidation, Param: param, Msg: fmt.Sprintf(format, args...)}
}
