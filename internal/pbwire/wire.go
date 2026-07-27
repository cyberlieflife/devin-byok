package pbwire

import (
	"encoding/binary"
	"math"
)

// 最小 protobuf 编码工具（仅 MVP 需要的 wire types）。

func AppendVarint(b []byte, v uint64) []byte {
	for v >= 0x80 {
		b = append(b, byte(v)|0x80)
		v >>= 7
	}
	return append(b, byte(v))
}

func AppendTag(b []byte, field int, wt int) []byte {
	return AppendVarint(b, uint64(field<<3|wt))
}

func AppendBytes(b []byte, field int, v []byte) []byte {
	b = AppendTag(b, field, 2)
	b = AppendVarint(b, uint64(len(v)))
	return append(b, v...)
}

func AppendString(b []byte, field int, v string) []byte {
	return AppendBytes(b, field, []byte(v))
}

func AppendBool(b []byte, field int, v bool) []byte {
	b = AppendTag(b, field, 0)
	if v {
		return append(b, 1)
	}
	return append(b, 0)
}

func AppendInt32(b []byte, field int, v int32) []byte {
	b = AppendTag(b, field, 0)
	return AppendVarint(b, uint64(uint32(v)))
}

func AppendInt64(b []byte, field int, v int64) []byte {
	b = AppendTag(b, field, 0)
	return AppendVarint(b, uint64(v))
}

func AppendEnum(b []byte, field int, v int32) []byte {
	return AppendInt32(b, field, v)
}

func AppendMessage(b []byte, field int, msg []byte) []byte {
	return AppendBytes(b, field, msg)
}

func AppendFloat32(b []byte, field int, v float32) []byte {
	b = AppendTag(b, field, 5)
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], math.Float32bits(v))
	return append(b, buf[:]...)
}

func AppendFloat64(b []byte, field int, v float64) []byte {
	b = AppendTag(b, field, 1)
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], math.Float64bits(v))
	return append(b, buf[:]...)
}

// Connect 服务端流帧：1 byte flags + 4 byte big-endian length + payload
func ConnectFrame(flags byte, payload []byte) []byte {
	out := make([]byte, 0, 5+len(payload))
	out = append(out, flags)
	var lenbuf [4]byte
	binary.BigEndian.PutUint32(lenbuf[:], uint32(len(payload)))
	out = append(out, lenbuf[:]...)
	out = append(out, payload...)
	return out
}

// ConnectEndStream 发送 end-stream trailer（空 JSON 对象）。
func ConnectEndStream() []byte {
	// flag bit1 = 2 means end stream; payload is JSON trailer {}
	return ConnectFrame(0x02, []byte("{}"))
}


// Field 解析后的 protobuf 字段。
type Field struct {
	Number int
	Wire   int
	Varint uint64
	Bytes  []byte
}

// ParseFields 解析 protobuf message 的顶层字段（长度限定子消息可递归调用）。
func ParseFields(b []byte) []Field {
	var out []Field
	i := 0
	for i < len(b) {
		key, n := readVarint(b, i)
		if n == 0 {
			break
		}
		i += n
		num := int(key >> 3)
		wt := int(key & 7)
		switch wt {
		case 0:
			v, n2 := readVarint(b, i)
			if n2 == 0 {
				return out
			}
			i += n2
			out = append(out, Field{Number: num, Wire: wt, Varint: v})
		case 1:
			if i+8 > len(b) {
				return out
			}
			out = append(out, Field{Number: num, Wire: wt, Bytes: append([]byte(nil), b[i:i+8]...)})
			i += 8
		case 2:
			ln, n2 := readVarint(b, i)
			if n2 == 0 {
				return out
			}
			i += n2
			if i+int(ln) > len(b) {
				return out
			}
			out = append(out, Field{Number: num, Wire: wt, Bytes: append([]byte(nil), b[i:i+int(ln)]...)})
			i += int(ln)
		case 5:
			if i+4 > len(b) {
				return out
			}
			out = append(out, Field{Number: num, Wire: wt, Bytes: append([]byte(nil), b[i:i+4]...)})
			i += 4
		default:
			return out
		}
	}
	return out
}

func readVarint(b []byte, i int) (uint64, int) {
	var x uint64
	var s uint
	for n := 0; n < 10 && i < len(b); n++ {
		c := b[i]
		i++
		if c < 0x80 {
			return x | uint64(c)<<s, n + 1
		}
		x |= uint64(c&0x7f) << s
		s += 7
	}
	return 0, 0
}
