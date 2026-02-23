# AMLX 二进制歌词格式 v1

**状态**：草案（Draft）
**版本**：1.0
**魔数（Magic）**：`AMLX`
**用途**：
一种 **完全无损**、**高度压缩** 的二进制歌词分发格式，用于大规模歌词存储与分发，
在语义上与结构化 TTML 歌词 **完全等价**。

---

## 1. 设计目标

* **100% 信息保留**

  * 不允许丢失任何歌词、时间、元数据或标志位信息
* **确定性往返转换（Round-trip）**

  ```text
  Binary → Struct → TTML
  TTML   → Struct → Binary
  ```

  两条路径在语义层面必须完全一致
* **极致体积优化**

  * 以降低带宽与存储成本为核心目标
* **向前兼容**

  * 支持未来版本扩展而不破坏现有解码器

---

## 2. 编码约定

### 2.1 整数编码

* 所有可变长度整数使用 **无符号 LEB128（varint）**
* 定长整数使用 **小端序（Little Endian）**

---

### 2.2 时间表示

* 所有时间均使用 **整数毫秒（ms）**
* 二进制格式中 **不出现任何浮点数**

---

### 2.3 字符串编码

* 所有字符串均为 **UTF-8**
* 所有字符串统一存入 **字符串池（String Pool）**
* 通过索引（string_id）引用

---

## 3. 文件整体结构

```text
+-----------------------------+
| Magic        (4 字节)       | "AMLX"
| Version      (1 字节)       | 0x01
| GlobalFlags  (1 字节)       |
| HeaderSize   (varint)       |
+-----------------------------+
| Header Section              |
+-----------------------------+
| String Pool Section         |
+-----------------------------+
| Lyric Data Section          |
+-----------------------------+
```

---

## 4. Header Section（元数据区）

Header Section 用于存储所有 TTML 元数据（Metadata）。

---

### 4.1 结构定义

```text
HeaderSection:
  metadata_count (varint)
  重复 metadata_count 次:
    key_string_id (varint)
    value_count   (varint)
    重复 value_count 次:
      value_string_id (varint)
    error_flag (u8)
```

---

### 4.2 对应 Go 结构

```go
type TTMLMetadata struct {
    Key   string
    Value []string
    Error bool
}
```

| 字段      | 存储方式 |
| ------- | ---- |
| Key     | 字符串池 |
| Value[] | 字符串池 |
| Error   | u8   |

---

## 5. String Pool Section（字符串池）

字符串池存储 **文件中出现的所有唯一字符串**。

---

### 5.1 结构定义

```text
StringPool:
  string_count (varint)
  重复 string_count 次:
    byte_length (varint)
    utf8_bytes  (byte_length)
```

---

### 5.2 必须包含的字符串

以下字段 **必须** 存入字符串池：

* `TTMLMetadata.Key`
* `TTMLMetadata.Value[*]`
* `LyricWord.Word`
* `LyricWord.RomanWord`
* `LyricLine.TranslatedLyric`
* `LyricLine.RomanLyric`

---

### 5.3 规则

* 字符串 **必须去重**
* `string_id` 为 **从 0 开始的索引**

---

## 6. Lyric Data Section（歌词数据区）

```text
LyricData:
  line_count (varint)
  重复 line_count 次:
    LineRecord
```

---

## 7. LineRecord（歌词行）

表示一整行歌词。

---

### 7.1 结构定义

```text
LineRecord:
  line_start_time (varint)
  line_end_time   (varint)
  line_flags      (u8)
  word_count      (varint)

  若 HasTranslatedLyric:
    translated_string_id (varint)

  若 HasRomanLyric:
    roman_string_id (varint)

  重复 word_count 次:
    WordRecord
```

---

### 7.2 行标志位（line_flags）

| 位   | 含义                 |
| --- | ------------------ |
| 0   | IsBG（背景歌词）         |
| 1   | IsDuet（对唱歌词）       |
| 2   | IgnoreSync（忽略同步）   |
| 3   | HasTranslatedLyric |
| 4   | HasRomanLyric      |
| 5–7 | 保留位（必须为 0）         |

---

### 7.3 对应 Go 结构

```go
type LyricLine struct {
    Words           []LyricWord
    TranslatedLyric string
    RomanLyric      string
    IsBG            bool
    IsDuet          bool
    StartTime       float64
    EndTime         float64
    IgnoreSync      bool
}
```

---

## 8. WordRecord（歌词词）

表示一行中的一个词或符号单元。

---

### 8.1 结构定义

```text
WordRecord:
  delta_start_time (varint)
  duration         (varint)
  text_string_id   (varint)
  word_flags       (u8)

  若 HasRomanWord:
    roman_string_id (varint)

  若 HasEmptyBeat:
    empty_beat_ms (varint)
```

---

### 8.2 词标志位（word_flags）

| 位   | 含义            |
| --- | ------------- |
| 0   | Obscene（脏词标记） |
| 1   | HasEmptyBeat  |
| 2   | HasRomanWord  |
| 3   | RomanWarning  |
| 4–7 | 保留位（必须为 0）    |

---

### 8.3 时间语义

```text
word_start_time = line_start_time + Σ(delta_start_time)
word_end_time   = word_start_time + duration
```

---

### 8.4 对应 Go 结构

```go
type LyricWord struct {
    StartTime    float64
    EndTime      float64
    Word         string
    Obscene      bool
    EmptyBeat    float64
    RomanWord    string
    RomanWarning bool
}
```

---

## 9. ID 处理规则

### 9.1 设计原则

* `LyricLine.ID` 与 `LyricWord.ID` **不存储**
* ID 在解码阶段 **按顺序确定性生成**

---

### 9.2 设计理由

* ID 不影响歌词语义
* ID 可可靠重建
* 不存储 ID 可显著减少体积

---

## 10. 合法性校验规则

解码器 **必须拒绝** 以下情况：

* Magic 不等于 `"AMLX"`
* 字符串索引越界
* 时间值非法（负值、溢出、倒退）

解码器 **应当温和拒绝** 以下情况：

* 版本号不支持
* 使用了未定义的保留标志位

---

## 11. 向前兼容策略

* 未识别的标志位 **必须忽略**
* 新字段只能：

  * 添加在现有可选字段之后
  * 或通过新增 Section 实现

---

## 12. 信息完整性清单

| 数据项            | 是否保留 |
| -------------- | ---- |
| 元数据（Metadata）  | ✅    |
| Metadata.Error | ✅    |
| 行开始 / 结束时间     | ✅    |
| 行标志位           | ✅    |
| 翻译歌词           | ✅    |
| 罗马音歌词          | ✅    |
| 词时间信息          | ✅    |
| Obscene 标记     | ✅    |
| EmptyBeat      | ✅    |
| RomanWord      | ✅    |
| RomanWarning   | ✅    |

**不存在任何歌词信息丢失。**

---

## 13. 推荐使用模型

```text
结构化歌词
      ↓
AMLX Binary（站点分发 / CDN）
      ↓
TTML（导出 / 编辑 / 兼容）
```