package command

import (
	"fmt"
	"strconv"
	"strings"
)

// Validator 自定义校验函数，入参为参数最终取值（字符串形式）。
type Validator func(value string) error

// Param 描述一个参数的结构与约束。
type Param struct {
	Name       string   // 参数名（命令行上使用 --name）
	Aliases    []string // 参数别名
	Usage      string   // 帮助说明
	Type       ValueType
	Default    string // 内置默认值（字符串形式）
	EnvVar     string // 关联环境变量名
	Required   bool   // 是否必填（任何来源都未提供时校验失败）
	Variadic   bool   // 位置参数：吸收剩余全部参数
	Positional bool   // 是否位置参数（按声明顺序匹配）
	Enum       []string
	Validator  Validator
}

// Group 参数组约束。
type Group struct {
	Name   string
	Params []string // 组内参数名
	Mode   GroupMode
}

// BoundParam 绑定结果：最终取值与其来源，可追溯。
type BoundParam struct {
	Param  *Param
	Value  string
	Values []string // StringSlice / Variadic 使用
	Source Source
}

// String 返回最终取值。
func (b *BoundParam) String() string {
	if b == nil {
		return ""
	}
	if b.Param != nil && (b.Param.Type == StringSlice || b.Param.Variadic) {
		return strings.Join(b.Values, ",")
	}
	return b.Value
}

// Int 返回 int 取值。
func (b *BoundParam) Int() (int, error) {
	if b == nil {
		return 0, fmt.Errorf("参数未绑定")
	}
	return strconv.Atoi(b.Value)
}

// Float 返回 float64 取值。
func (b *BoundParam) Float() (float64, error) {
	if b == nil {
		return 0, fmt.Errorf("参数未绑定")
	}
	return strconv.ParseFloat(b.Value, 64)
}

// Bool 返回 bool 取值。
func (b *BoundParam) Bool() bool {
	if b == nil {
		return false
	}
	v, _ := strconv.ParseBool(b.Value)
	return v
}

// Slice 返回字符串切片取值。
func (b *BoundParam) Slice() []string {
	if b == nil {
		return nil
	}
	return append([]string(nil), b.Values...)
}
