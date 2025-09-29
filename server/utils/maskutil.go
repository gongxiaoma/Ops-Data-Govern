package utils

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"github.com/flipped-aurora/gin-vue-admin/server/global"
	"go.uber.org/zap"
	"strings"
)

func MaskAnyStream(dsl string, clusterId string, indexName string, reqId string) (string, string, string, error) {
	// 先尝试 JSON 解析
	if json.Valid([]byte(dsl)) {
		global.GVA_LOG.Warn("DSL是标准的JSON，调用JSON脱敏方法",
			zap.String("reqID", reqId))
		return MaskJSONStream(dsl)
	} else {
		global.GVA_LOG.Warn("DSL不是标准的JSON（阿里云接口返回不完整），现调用非JSON脱敏方法",
			zap.String("reqID", reqId),
			zap.String("集群名", clusterId),
			zap.String("索引名", indexName),
			zap.String("DSL语句", dsl))
		return MaskTruncatedJSON(dsl)
	}
}

// MaskJSONStream 把所有的value（包括string/number/bool/null）替换成字符串 "?"，同时保持对象字段的原始顺序不变（使用流式Token解析）
// @param dsl DSL语句string类型
// @return maskStr
// @return hashStr
// @return error
func MaskJSONStream(dsl string) (string, string, string, error) {
	dec := json.NewDecoder(strings.NewReader(dsl))
	dec.UseNumber() // 保持数字为json.Number，但我们最终仍替换为"?"

	var buf bytes.Buffer
	if err := writeToken(dec, &buf); err != nil {
		return "", "", "", err
	}
	maskedStr := buf.String()

	// 计算 MD5
	hash := md5.Sum([]byte(maskedStr))
	hashStr := hex.EncodeToString(hash[:])
	dslFormat := "json"
	return maskedStr, hashStr, dslFormat, nil
}

// writeToken 读取dec的下一个token处理后文本写入buf，对象/数组结构保持，递归处理内部元素，遇到原子值（string/number/bool/null）写回为 `"?"`。
// @param *json.Decoder
// @return *bytes.Buffer
// @return error
func writeToken(dec *json.Decoder, buf *bytes.Buffer) error {
	t, err := dec.Token()
	if err != nil {
		return err
	}

	switch v := t.(type) {
	case json.Delim:
		// 对象
		if v == '{' {
			buf.WriteByte('{')
			first := true
			for dec.More() {
				if !first {
					buf.WriteByte(',')
				}
				first = false

				// key（JSON object 的 key 一定是 string token）
				keyTok, err := dec.Token()
				if err != nil {
					return err
				}
				key := keyTok.(string)
				// 用 json.Marshal 确保 key 的转义和引号正确
				kb, _ := json.Marshal(key)
				buf.Write(kb)
				buf.WriteByte(':')

				// value 递归处理（如果是原子值会被替换成 "?"）
				if err := writeToken(dec, buf); err != nil {
					return err
				}
			}
			// 读取并写入结束 '}'
			endTok, err := dec.Token()
			if err != nil {
				return err
			}
			buf.WriteByte(byte(endTok.(json.Delim)))
			return nil
		}

		// 数组
		if v == '[' {
			// 读取并保存每个元素的原始 JSON bytes（json.RawMessage）
			var raws [][]byte
			for dec.More() {
				var raw json.RawMessage
				if err := dec.Decode(&raw); err != nil {
					return err
				}
				// 保存原始字节（包含引号/大括号等）
				raws = append(raws, raw)
			}
			// 消费结束的 ']' token
			_, err := dec.Token()
			if err != nil {
				return err
			}

			// 判断是否所有元素都是原子值（字符串/数字/true/false/null），且数组非空
			allPrim := len(raws) > 0
			for _, raw := range raws {
				b := bytes.TrimLeft(raw, " \t\r\n")
				if len(b) == 0 {
					allPrim = false
					break
				}
				switch b[0] {
				case '"': // string
				case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', '-': // number
				case 't', 'f': // true / false
				case 'n': // null
				default:
					// 以 '{' 或 '[' 等开头视为非原子
					allPrim = false
				}
				if !allPrim {
					break
				}
			}

			if allPrim {
				// ✅ 所有元素都是原子 -> 统一替换为单元素数组 ["?"]
				buf.WriteString(`["?"]`)
				return nil
			}

			// 否则逐元素用原来的逻辑处理（保留对象字段顺序）
			buf.WriteByte('[')
			for i, raw := range raws {
				if i > 0 {
					buf.WriteByte(',')
				}
				// 为每个元素新建 decoder 并复用 writeToken 来写入（这样可以保证内部逻辑完全一致）
				d := json.NewDecoder(bytes.NewReader(raw))
				d.UseNumber()
				if err := writeToken(d, buf); err != nil {
					return err
				}
			}
			buf.WriteByte(']')
			return nil
		}

	// 原子类型 -> 统一替换为 "?"
	case string:
		buf.WriteString(`"?"`)
		return nil

	case json.Number:
		buf.WriteString(`"?"`)
		return nil

	case bool:
		buf.WriteString(`"?"`)
		return nil

	case nil:
		buf.WriteString(`"?"`)
		return nil

	default:
		// 兜底：任何其他 token 也替换为 "?"
		buf.WriteString(`"?"`)
		return nil
	}

	return nil
}

// MaskTruncatedJSON 尝试对不完整的JSON做脱敏，只替换value，保留key
// 遇到 : 后面跟的内容视作 value → 统一替换成 "?"；遇到 {、}、[、]、, 原样保留；遇到 key（在 : 前面的字符串）原样保留
// @param dsl DSL语句string类型
// @return maskStr
// @return hashStr
// @return error
func MaskTruncatedJSON(dsl string) (string, string, string, error) {
	var buf bytes.Buffer
	inString := false
	afterColon := false

	for i := 0; i < len(dsl); i++ {
		ch := dsl[i]

		switch ch {
		case '"':
			inString = !inString
			buf.WriteByte(ch)

		case ':':
			buf.WriteByte(ch)
			afterColon = true

		case ',', '{', '}':
			buf.WriteByte(ch)
			if ch == ',' {
				afterColon = false
			}

		case '[':
			buf.WriteByte('[')
			if afterColon {
				// 判断是否是简单数组（全部是原子值）
				j := i + 1
				simple := true
				depth := 1
				for j < len(dsl) && depth > 0 {
					switch dsl[j] {
					case '[':
						depth++
					case ']':
						depth--
					case '{':
						simple = false
						depth++
					}
					if dsl[j] == '}' {
						depth--
					}
					j++
				}
				if simple {
					// 简单数组替换为 [?]
					buf.WriteString("?]")
					i = j - 1
					afterColon = false
				} else {
					// 复杂数组，保留 '['，后续递归处理
					afterColon = true
				}
			}

		case ']':
			buf.WriteByte(']')
		default:
			if afterColon && !inString {
				// value 部分替换成 ?
				if ch != ' ' && ch != '\n' && ch != '\t' {
					buf.WriteString("?")
					// 跳过后续连续的 value 内容（直到遇到分隔符或结构符）
					for i+1 < len(dsl) && !strings.ContainsRune(",{}[] \n\t", rune(dsl[i+1])) {
						i++
					}
					afterColon = false
					continue
				} else {
					buf.WriteByte(ch)
				}
			} else {
				buf.WriteByte(ch)
			}
		}
	}

	masked := buf.String()

	// 计算 MD5
	hash := md5.Sum([]byte(masked))
	hashStr := hex.EncodeToString(hash[:])
	dslFormat := "non-json"
	return masked, hashStr, dslFormat, nil
}
