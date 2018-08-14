package ansi_test

import (
	"strconv"
	"testing"

	"github.com/jcorbin/anansi/ansi"
	"github.com/stretchr/testify/assert"
)

func TestEscape_AppendWith(t *testing.T) {
	for _, tc := range []struct {
		id     ansi.Escape
		arg    []byte
		expect string
	}{
		{ansi.Escape(0x9F), nil, "\x1b_"},
		{ansi.CUU, nil, "\x1b[A"},
		{ansi.CUU, []byte("5"), "\x1b[5A"},
		{ansi.SM, []byte("42"), "\x1b[42h"},
		{ansi.SGR, []byte("0;1;7"), "\x1b[0;1;7m"},
		{ansi.CSI('M'), []byte("<35;45;6"), "\x1b[<35;45;6M"},
	} {
		t.Run(tc.expect, func(t *testing.T) {
			n := tc.id.Size() + len(tc.arg)
			assert.Equal(t,
				tc.expect,
				string(tc.id.AppendWith(nil, tc.arg...)),
				"from nil")
			assert.Equal(t,
				tc.expect,
				string(tc.id.AppendWith(make([]byte, 0, n-1), tc.arg...)),
				"from just not enough")
			assert.Equal(t,
				tc.expect,
				string(tc.id.AppendWith(make([]byte, 0, n), tc.arg...)),
				"from just enough")

			assert.Equal(t,
				tc.expect,
				string(tc.id.AppendWith(make([]byte, 0, n+1), tc.arg...)),
				"from more than enough")

			prior := "hello"
			b := make([]byte, 0, n+2*len(prior))
			b = append(b, prior...)
			b = tc.id.AppendWith(b, tc.arg...)
			assert.Equal(t, tc.expect, string(b[len(prior):]), "with prior")

		})
	}
}

var seqTestCases = []struct {
	out string
	seq ansi.Seq
}{
	{"\x1b[<t", ansi.CSI('t').With('<')},
	{"\x1b[<=t", ansi.CSI('t').With('<', '=')},
	{"\x1b[<=>t", ansi.CSI('t').With('<', '=', '>')},
	{"\x1b[<=?>t", ansi.CSI('t').With('<', '=', '?', '>')},

	{"\x1b[12t", ansi.CSI('t').WithInts(12)},
	{"\x1b[12;34t", ansi.CSI('t').WithInts(12, 34)},
	{"\x1b[12;34;56t", ansi.CSI('t').WithInts(12, 34, 56)},
	{"\x1b[12;34;56;78t", ansi.CSI('t').WithInts(12, 34, 56, 78)},
	{"\x1b[12;34;56;78;90t", ansi.CSI('t').WithInts(12, 34, 56, 78, 90)},

	{"\x1b[<=?>12;34;56;78;90t", ansi.CSI('t').With('<', '=', '?', '>').WithInts(12, 34, 56, 78, 90)},
}

func TestSeq_String(t *testing.T) {
	for _, tc := range seqTestCases {
		t.Run(strconv.Quote(tc.out), func(t *testing.T) {
			assert.Equal(t, tc.out, tc.seq.String())
		})
	}
}

func BenchmarkSeq_AppendTo(b *testing.B) {
	var p []byte
	for _, tc := range seqTestCases {
		b.Run(strconv.Quote(tc.out), func(b *testing.B) {
			if need := b.N * len(tc.out); cap(p) < need {
				p = make([]byte, 0, need)
			} else {
				p = p[:0]
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				p = tc.seq.AppendTo(p)
			}
		})
	}
}
