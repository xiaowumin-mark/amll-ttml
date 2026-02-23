# amll-ttml

[English](README.md)

`amll-ttml` 是一个 Go 库，提供以下能力：

- 将 TTML 歌词解析为结构化数据
- 将结构化歌词导出为 TTML
- `TTML -> AMLX 二进制`
- `AMLX 二进制 -> TTML`

AMLX 二进制格式实现遵循仓库中的 `SPEC.md`。

## 安装

```bash
go get github.com/xiaowumin-mark/amll-ttml
```

## 核心 API

### TTML 解析与导出

- `ParseLyric(ttmlText string) (TTMLLyric, error)`
- `ExportTTMLText(ttmlLyric TTMLLyric, pretty bool) string`

### AMLX 二进制编解码

- `TTMLToBinary(ttmlText string) ([]byte, error)`
- `BinaryToTTML(binaryData []byte, pretty bool) (string, error)`
- `EncodeBinary(ttmlLyric TTMLLyric) ([]byte, error)`
- `DecodeBinary(binaryData []byte) (TTMLLyric, error)`
- 别名：`EncodeAMLX`、`DecodeAMLX`

## 快速示例

```go
package main

import (
	"fmt"
	"os"

	ttml "github.com/xiaowumin-mark/amll-ttml"
)

func main() {
	rawTTML, err := os.ReadFile("input.ttml")
	if err != nil {
		panic(err)
	}

	// TTML -> 二进制
	bin, err := ttml.TTMLToBinary(string(rawTTML))
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile("output.amlx", bin, 0o644); err != nil {
		panic(err)
	}

	// 二进制 -> TTML
	roundTripTTML, err := ttml.BinaryToTTML(bin, false)
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile("roundtrip.ttml", []byte(roundTripTTML), 0o644); err != nil {
		panic(err)
	}

	fmt.Println("done")
}
```

## 兼容策略（旧版 TTML）

为兼容历史 TTML 变体并用于生产环境，编码时包含以下处理：

- 若行时间未覆盖词时间，会自动将行时间归一化为词时间包络。
- 时间值会四舍五入到整数毫秒。
- `empty-beat` 为非法值（非有限数）时，会在编码时忽略该字段。

在保证稳定转换的同时，歌词内容和可支持属性会尽量完整保留。

## 测试

### 常规测试

```bash
go test ./...
```

### 极限批量测试

该测试会：

- 读取 `test/raw-ttml` 下所有 `.ttml` 文件
- 转换为 AMLX 并输出到 `test/binary`
- 再将 AMLX 转回 TTML 输出到 `test/binary-to-ttml`
- 记录每个文件耗时与平均耗时
- 输出日志到：
  - `test/extreme-conversion.log`
  - `test/extreme-conversion.json`

运行方式：

```bash
RUN_EXTREME_TEST=1 go test -run TestExtremeTTMLBinaryPipeline -count=1 ./...
```

PowerShell 示例：

```powershell
$env:RUN_EXTREME_TEST='1'
go test -run TestExtremeTTMLBinaryPipeline -count=1 ./...
```

## 许可证

MIT，见 `LICENSE`。
