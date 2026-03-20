# cfgbollocks v1

## Text Configuration File Format Specification

### Status

This document defines **cfgbollocks version 1 (v1)**.
It is a **normative**, **implementation-independent** specification.

The key words **MUST**, **MUST NOT**, **REQUIRED**, **SHALL**, **SHALL NOT**, **SHOULD**, **SHOULD NOT**, and **MAY** are to be interpreted as described in RFC 2119.

---

## 1. Design Goals

cfgbollocks v1 is designed to:

* Allow **unescaped, raw values** (including newlines)
* Be **human-readable and inspectable**
* Be **fail-fast and non-ambiguous**
* Preserve **author intent** during read/write cycles
* Make **format rules explicit inside the file itself**

---

## 2. Non-Goals

cfgbollocks v1 does **not** attempt to:

* Be binary-safe beyond UTF-8 text files
* Support inline comments
* Support grammar changes mid-file
* Support multiple format versions in a single file
* Preserve original formatting beyond semantic content

---

## 3. Encoding and BOM Handling

1. Files **MUST** be encoded in UTF-8.
2. A UTF-8 BOM (U+FEFF) **MAY** be present at the beginning of the file.
3. If a BOM is present, it **MUST** be ignored by the parser.
4. Writers **MUST NOT** emit a BOM.

After BOM removal (if any), parsing begins at byte offset 0.

---

## 4. File Structure

A cfgbollocks v1 file consists of:

1. Exactly one **mandatory header record** (`cfgbollocks`)
2. Zero or more **data records**

There **MUST NOT** be:

* Blank lines
* Whitespace
* Any other bytes

before the header record.

---

## 5. Mandatory Header Record

### 5.1 Header Key

The first record **MUST** have the key name:

```
cfgbollocks
```

* The key name is **case-sensitive**
* No leading whitespace is permitted

### 5.2 Header Presence

* If the first record is not `cfgbollocks`, parsing **MUST FAIL**
* If `cfgbollocks` appears later in the file, it **HAS NO SEMANTIC EFFECT**

### 5.3 Header Semantics

The value of `cfgbollocks`:

* Defines the **format version**
* Defines all **grammar tokens** used for the remainder of the file
* Applies to the entire file

---

## 6. Versioning

1. The header value **MUST** declare the version.
2. cfgbollocks v1 files **MUST** declare `v1`.
3. A file **MUST NOT** contain multiple versions.
4. Parsers **MUST FAIL** if the declared version is unsupported.

---

## 7. Grammar Tokens (Defined in Header)

The header **MUST** define the following tokens:

| Token           | Description                          |
| --------------- | ------------------------------------ |
| `DECL`          | Declarator used to define delimiters |
| `ASSIGN`        | Assignment token                     |
| `DEFAULT_DELIM` | Default value delimiter              |

### 7.1 Token Properties

* Tokens are **non-empty UTF-8 strings**
* Tokens **MUST NOT** contain whitespace
* Tokens **MUST NOT** overlap or be prefixes of each other

---

## 8. Record Syntax (v1)

Each data record has the following structure:

```
key <ws> ASSIGN <ws> [DECL DELIM DECL <ws>] DELIM value DELIM
```

Where:

* `<ws>` is zero or more ASCII spaces or tabs
* `DELIM` is either:

  * the `DEFAULT_DELIM`, or
  * a delimiter defined inline using `DECL`

---

## 9. Keys

1. Keys **MUST NOT** contain whitespace.
2. Keys are **case-sensitive**.
3. Keys **MUST NOT** be empty.
4. The key `cfgbollocks` **MUST NOT** be interpreted as data.

---

## 10. Delimiters

### 10.1 Default Delimiter

* Defined in the header
* Used when no inline delimiter is declared

### 10.2 Inline Delimiter Declaration

An inline delimiter is declared as:

```
DECL DELIM DECL
```

Rules:

* `DELIM` **MUST NOT** be empty
* `DELIM` **MUST NOT** contain whitespace
* `DELIM` **MUST NOT** contain `DECL`
* The declared delimiter applies **only to that record**

---

## 11. Values

### 11.1 Value Semantics

1. Values are **raw byte sequences**.
2. No escaping, unescaping, or transformation is performed.
3. Newlines are preserved exactly as present in the file.
4. The value **MAY** be empty.

### 11.2 Value Framing

* The value begins immediately after the opening delimiter.
* The value ends immediately before the closing delimiter.
* The closing delimiter **MUST** match exactly.

### 11.3 Delimiter Collision

* If the delimiter appears inside the value, parsing **MUST FAIL**.

---

## 12. Whitespace Rules

1. Whitespace is allowed:

   * Around `ASSIGN`
   * After inline delimiter declaration
2. Whitespace is **NOT** allowed:

   * Inside tokens
   * Inside delimiters
   * After the closing delimiter

Trailing whitespace after the closing delimiter **MUST cause failure**.

---

## 13. Error Handling

Parsers **MUST FAIL FAST** on:

* Missing or invalid header
* Unsupported version
* Token collisions
* Unterminated values
* Trailing data after delimiters
* Grammar violations

When the header is invalid, **no other errors MUST be reported**.

---

## 14. Writer Requirements

Writers:

1. **MUST** preserve delimiters exactly
2. **MUST NOT** invent new delimiters for existing records
3. **MAY** choose delimiters for newly created records
4. **MUST** emit a valid `cfgbollocks` header

---

## 15. Examples

### 15.1 Minimal Valid File

```
cfgbollocks = |###| ###v1###
```

### 15.2 Record With Default Delimiter

```
path = ###/weird path/with spaces###
```

### 15.3 Record With Inline Delimiter

```
script = |@@@| @@@echo "hello"@@@
```

### 15.4 Invalid (Trailing Whitespace)

```
key = ###value### 
```

---

## 16. Final Notes

cfgbollocks v1 is **text-semantic**:
the structure is interpreted, but **value bytes are sacred**.

---

