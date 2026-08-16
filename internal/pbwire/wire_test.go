package pbwire

import (
	"bytes"
	"testing"
)

func TestAppendVarint(t *testing.T) {
	cases := []struct {
		v    uint64
		want []byte
	}{
		{0, []byte{0x00}},
		{1, []byte{0x01}},
		{127, []byte{0x7f}},
		{128, []byte{0x80, 0x01}},
		{300, []byte{0xac, 0x02}},
	}
	for _, c := range cases {
		got := AppendVarint(nil, c.v)
		if !bytes.Equal(got, c.want) {
			t.Errorf("AppendVarint(%d) = %x, want %x", c.v, got, c.want)
		}
	}
}

func TestConnectFrameGolden(t *testing.T) {
	got := ConnectFrame(0x00, []byte("hi"))
	want := []byte{0x00, 0x00, 0x00, 0x00, 0x02, 'h', 'i'}
	if !bytes.Equal(got, want) {
		t.Fatalf("ConnectFrame = %x, want %x", got, want)
	}
	got = ConnectEndStream()
	want = []byte{0x02, 0x00, 0x00, 0x00, 0x02, '{', '}'}
	if !bytes.Equal(got, want) {
		t.Fatalf("ConnectEndStream = %x, want %x", got, want)
	}
}

func TestParseFieldsRoundTrip(t *testing.T) {
	var msg []byte
	msg = AppendString(msg, 1, "hello")
	msg = AppendInt64(msg, 2, 42)
	msg = AppendBool(msg, 3, true)

	fields := ParseFields(msg)
	if len(fields) != 3 {
		t.Fatalf("fields = %d, want 3", len(fields))
	}
	if fields[0].Number != 1 || fields[0].Wire != 2 || string(fields[0].Bytes) != "hello" {
		t.Fatalf("field0 = %+v", fields[0])
	}
	if fields[1].Number != 2 || fields[1].Wire != 0 || fields[1].Varint != 42 {
		t.Fatalf("field1 = %+v", fields[1])
	}
	if fields[2].Number != 3 || fields[2].Wire != 0 || fields[2].Varint != 1 {
		t.Fatalf("field2 = %+v", fields[2])
	}
}

func TestParseFieldsMalformedNoPanic(t *testing.T) {
	cases := [][]byte{
		nil,
		{},
		{0x0a},                                     // 缺 length
		{0x0a, 0xff},                               // length 截断
		{0x0a, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x01}, // 超大 length
		{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, // 10 字节全 continuation
		{0x09, 0x00, 0x00, 0x00},                   // 64-bit 截断
		{0x0d, 0x00, 0x00},                         // 32-bit 截断
		{0x0f},                                     // 非法 wire type
	}
	for _, b := range cases {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("ParseFields(%x) panicked: %v", b, r)
				}
			}()
			_ = ParseFields(b)
		}()
	}
}

func TestReadVarintOverflow(t *testing.T) {
	// 10 字节 varint，第 10 字节 c>1 属溢出编码，应返回 (0,0) 而非回绕
	b := []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x02}
	if v, n := readVarint(b, 0); v != 0 || n != 0 {
		t.Fatalf("readVarint overflow = (%d,%d), want (0,0)", v, n)
	}
	// 合法 10 字节 varint：2^63 = 0x80*9 + 0x01（第 10 字节仅置 bit63）
	b = []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x01}
	if v, n := readVarint(b, 0); v != 1<<63 || n != 10 {
		t.Fatalf("readVarint 2^63 = (%d,%d), want (1<<63,10)", v, n)
	}
	// 最大 uint64 = 0xff*9 + 0x01
	b = []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x01}
	if v, n := readVarint(b, 0); v != ^uint64(0) || n != 10 {
		t.Fatalf("readVarint max = (%d,%d), want (max,10)", v, n)
	}
}

func FuzzParseFields(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0x08, 0x96, 0x01})
	f.Add([]byte{0x12, 0x05, 'h', 'e', 'l', 'l', 'o'})
	f.Add([]byte{0x0a, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x01})
	f.Add([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
	f.Fuzz(func(t *testing.T, data []byte) {
		_ = ParseFields(data)
	})
}
