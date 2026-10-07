# command-dispatch-go

可复用的命令解析与分发框架（包 `cmdkit`）。定义命令的人只需声明命令与参数的结构和约束，
解析、校验、绑定与分发全部由框架完成。

## 快速示例

```go
root := &cmdkit.Command{Name: "app"}
root.AddCommand(&cmdkit.Command{
    Name:    "serve",
    Aliases: []string{"srv"},
    Flags: []cmdkit.Flag{
        {Name: "port", Shorthand: "p", Default: "8080",
         EnvVars: []string{"APP_PORT"}, ConfigKey: "server.port"},
    },
    Run: func(ctx *cmdkit.Context) error {
        port, src := ctx.Value("port") // 取值可追溯到来源
        fmt.Println("listen on", port, "from", src)
        return nil
    },
})
ctx, err := cmdkit.NewParser().Parse(root, cmdkit.ParseOptions{Args: os.Args[1:]})
```

## 解析语义

### 命令树与子命令匹配

- 命令以树形组织，`AddCommand` 注册子命令，注册与查找均并发安全（读写锁保护）。
- 解析时自根向下逐个消费 token，匹配顺序：**精确名/别名 > 唯一前缀缩写**。
- 前缀命中多个候选时报解析错误，并列出全部候选命令名。
- 无法匹配时报解析错误，并基于编辑距离给出最接近的建议（`did you mean ...?`）。
- 遇到首个 `-` 开头的 token 后进入参数区；叶子命令的剩余 token 按位置参数处理；
  `--` 之后的所有 token 一律视为位置参数。

### 参数来源与优先级

同一参数的取值可来自四个来源，优先级固定且可预期：

```
命令行 (--flag)  >  环境变量 (EnvVars)  >  配置文件 (ConfigKey)  >  内置默认值 (Default)
```

- 命令行支持 `--name value`、`--name=value`、`-s value`、`-s=value` 四种形式。
- 多个环境变量按声明顺序取第一个存在的。
- 每次解析后通过 `ctx.Value(name)` 可同时拿到最终值与 `Source`（`flag`/`env`/`config`/`default`），
  取值可追溯到具体来源。
- 环境变量与配置文件通过 `ParseOptions.Env` / `ParseOptions.Config` 注入；
  `Env` 为 nil 时回落到 `os.Environ()`。解析器本身无状态，多次解析不共享可变状态。

### 校验规则

校验在绑定之前完成，**任一校验失败即整体失败，不会留下部分绑定**（返回的 Context 为 nil，
业务函数 `Run` 不会执行）。支持的约束：

| 约束 | 声明方式 | 失败提示 |
|---|---|---|
| 必填 | `Flag.Required` | 指明缺失的参数名 |
| 枚举 | `Flag.Choices` | 指明出错参数、来源与全部合法值 |
| 互斥组 | `Flag.MutexGroup`（同组最多显式出现一个） | 指明冲突的两个参数 |
| 至少选一组 | `Flag.OneOfGroup`（同组至少显式出现一个） | 列出组内全部参数 |
| 自定义校验 | `Flag.Validator` / `Positional.Validator` | 携带参数名与校验器错误 |
| 位置参数数量 | `Positional.Min` / `Max`（-1 表示不限） | 指明天数与实际个数 |

### 参数继承与覆盖

- 父级命令声明 `Persistent: true` 的参数被所有子孙命令识别。
- 子命令可用同名本地参数覆盖继承参数：非零字段（如 `Default`、`Validator`、`Choices`）覆盖，
  零字段回落到继承值。覆盖关系单向明确（子覆盖父），重复定义不会冲突。

### 错误分类与退出码

错误统一为 `*cmdkit.Error`，按 `Kind` 区分三类：

| Kind | 含义 | 默认退出码 |
|---|---|---|
| `KindParse` | 未知命令/参数、缺值、歧义前缀 | 2 |
| `KindValidation` | 必填缺失、组冲突、校验失败 | 3 |
| `KindBusiness` | `Run` 返回的业务错误 | 1 |

退出码通过 `Parser.ExitCodes` 可配置；相同输入下错误类别与退出码保持稳定。

### 帮助与补全

- `Help(cmd)` 生成与声明的命令树一致的帮助：完整命令路径、别名、子命令列表、
  参数（含继承参数）及其默认值、枚举、环境变量与配置键。`-h`/`--help` 触发 `ErrHelp`。
- `BashCompletion(root, prog)` 与 `ZshCompletion(root, prog)` 生成补全脚本，
  覆盖子命令（含别名）、参数名与枚举取值。

### 并发约定

- `Command` 树的注册（`AddCommand`）与读取（`Children`、查找）由读写锁保护，可并发使用。
- `Parser` 无状态；每次 `Parse` 的全部可变输入都在 `ParseOptions` 中，
  解析结果写入新建的 `Context`，同一进程内并发发起多个解析互不影响、不串扰。
  并发安全性由 `go test -race` 覆盖验证。

## 测试

```sh
go test ./... -race
```

覆盖场景：歧义前缀与候选列表、未知命令建议、来源优先级与可追溯性、
互斥/至少选一组冲突、必填与自定义校验、继承覆盖、并发解析不串扰、
帮助与补全内容一致性、退出码可配置且稳定。
