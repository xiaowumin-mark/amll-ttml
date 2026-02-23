# AMLX Binary Lyric Format v1

**Status**: Draft
**Version**: 1.0
**Magic**: `AMLX`
**Purpose**:
A **lossless**, **compact**, and **binary** lyric distribution format designed for large-scale lyric hosting and delivery, fully equivalent to structured TTML lyrics.

---

## 1. Design Goals

* **100% information preservation**

  * No lyric, timing, metadata, or flag may be lost.
* **Deterministic round-trip**

  ```text
  Binary → Struct → TTML
  TTML   → Struct → Binary
  ```

  must be semantically identical.
* **Extreme size efficiency**

  * Optimized for bandwidth and storage cost.
* **Forward-compatible**

  * Must support future versions without breaking existing decoders.

---

## 2. Encoding Conventions

### 2.1 Integer Encoding

* All variable-length integers use **unsigned LEB128 (varint)**.
* Fixed-width integers are **little-endian**.

### 2.2 Time Representation

* All time values are stored as **integer milliseconds**.
* No floating-point values appear in the binary format.

### 2.3 String Encoding

* All strings are UTF-8 encoded.
* All strings are stored in the **String Pool** and referenced by index.

---

## 3. File Layout

```text
+-----------------------------+
| Magic        (4 bytes)      | "AMLX"
| Version      (1 byte)       | 0x01
| GlobalFlags  (1 byte)       |
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

## 4. Header Section

The Header Section stores all TTML metadata entries.

### 4.1 Structure

```text
HeaderSection:
  metadata_count (varint)
  repeat metadata_count:
    key_string_id (varint)
    value_count   (varint)
    repeat value_count:
      value_string_id (varint)
    error_flag (u8)
```

### 4.2 Mapping to TTMLMetadata

```go
type TTMLMetadata struct {
    Key   string
    Value []string
    Error bool
}
```

| Field   | Storage     |
| ------- | ----------- |
| Key     | string pool |
| Value[] | string pool |
| Error   | u8          |

---

## 5. String Pool Section

The String Pool contains **all unique strings** referenced anywhere in the file.

### 5.1 Structure

```text
StringPool:
  string_count (varint)
  repeat string_count:
    byte_length (varint)
    utf8_bytes  (byte_length)
```

### 5.2 Included Strings (Mandatory)

The following **must** be stored in the string pool:

* `TTMLMetadata.Key`
* `TTMLMetadata.Value[*]`
* `LyricWord.Word`
* `LyricWord.RomanWord`
* `LyricLine.TranslatedLyric`
* `LyricLine.RomanLyric`

### 5.3 Rules

* Strings **must be deduplicated**.
* `string_id` is the **0-based index** in this section.

---

## 6. Lyric Data Section

```text
LyricData:
  line_count (varint)
  repeat line_count:
    LineRecord
```

---

## 7. LineRecord

Represents a single lyric line.

### 7.1 Structure

```text
LineRecord:
  line_start_time (varint)
  line_end_time   (varint)
  line_flags      (u8)
  word_count      (varint)

  if HasTranslatedLyric:
    translated_string_id (varint)

  if HasRomanLyric:
    roman_string_id (varint)

  repeat word_count:
    WordRecord
```

### 7.2 Line Flags

| Bit | Meaning              |
| --- | -------------------- |
| 0   | IsBG                 |
| 1   | IsDuet               |
| 2   | IgnoreSync           |
| 3   | HasTranslatedLyric   |
| 4   | HasRomanLyric        |
| 5–7 | Reserved (must be 0) |

### 7.3 Mapping to LyricLine

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

## 8. WordRecord

Represents a single lyric word or token.

### 8.1 Structure

```text
WordRecord:
  delta_start_time (varint)
  duration         (varint)
  text_string_id   (varint)
  word_flags       (u8)

  if HasRomanWord:
    roman_string_id (varint)

  if HasEmptyBeat:
    empty_beat_ms (varint)
```

### 8.2 Word Flags

| Bit | Meaning              |
| --- | -------------------- |
| 0   | Obscene              |
| 1   | HasEmptyBeat         |
| 2   | HasRomanWord         |
| 3   | RomanWarning         |
| 4–7 | Reserved (must be 0) |

### 8.3 Timing Semantics

```text
word_start_time = line_start_time + Σ(delta_start_time)
word_end_time   = word_start_time + duration
```

### 8.4 Mapping to LyricWord

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

## 9. ID Handling

### 9.1 Design Rule

* `LyricLine.ID` and `LyricWord.ID` **are not stored**.
* IDs are **derived deterministically** from decoding order.

### 9.2 Rationale

* IDs do not affect lyric semantics.
* IDs can be regenerated reliably.
* Omitting IDs significantly reduces file size.

---

## 10. Validation Rules

A decoder **must reject** files if:

* Magic ≠ `"AMLX"`
* String pool index is out of bounds
* Timing values regress (negative or overflow)

A decoder **should gracefully reject** files if:

* Version is unsupported
* Reserved flag bits are set

---

## 11. Forward Compatibility

* Unknown flag bits **must be ignored**.
* New fields may only be added:

  * After existing optional fields
  * Or as new top-level sections

---

## 12. Information Completeness Checklist

| Data             | Preserved |
| ---------------- | --------- |
| Metadata         | ✅         |
| Metadata.Error   | ✅         |
| Line timing      | ✅         |
| Line flags       | ✅         |
| Translated lyric | ✅         |
| Roman lyric      | ✅         |
| Word timing      | ✅         |
| Obscene flag     | ✅         |
| EmptyBeat        | ✅         |
| RomanWord        | ✅         |
| RomanWarning     | ✅         |

**No lyric information is lost.**

---

## 13. Recommended Usage Model

```text
Structured Lyrics
      ↓
AMLX Binary (Distribution / CDN)
      ↓
TTML (Export / Editing / Interop)
```
