// Package mp4 writes H.264 samples into an ISO base media (.mp4) file with
// the index first, so the video can start playing while it downloads.
package mp4

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
)

// Writer collects samples in memory and writes the file on Close.
// Explainer videos are short, so holding the encoded stream is cheap.
type Writer struct {
	w             io.Writer
	width, height int
	fps           int
	sps, pps      []byte
	samples       [][]byte
	keys          []uint32
	closed        bool
}

// New starts a file for H.264 video of the given size and frame rate.
// sps and pps are the raw parameter sets without length prefixes.
func New(w io.Writer, width, height, fps int, sps, pps []byte) *Writer {
	return &Writer{w: w, width: width, height: height, fps: fps, sps: sps, pps: pps}
}

// Add appends one access unit in length-prefixed (AVCC) form.
func (m *Writer) Add(sample []byte, key bool) {
	m.samples = append(m.samples, sample)
	if key {
		m.keys = append(m.keys, uint32(len(m.samples)))
	}
}

// Frames returns the number of samples added so far.
func (m *Writer) Frames() int { return len(m.samples) }

type buf struct{ bytes.Buffer }

func (b *buf) u8(v uint8)   { b.WriteByte(v) }
func (b *buf) u16(v uint16) { binary.Write(&b.Buffer, binary.BigEndian, v) }
func (b *buf) u32(v uint32) { binary.Write(&b.Buffer, binary.BigEndian, v) }
func (b *buf) u64(v uint64) { binary.Write(&b.Buffer, binary.BigEndian, v) }
func (b *buf) zero(n int)   { b.Write(make([]byte, n)) }

func box(typ string, parts ...[]byte) []byte {
	n := 8
	for _, p := range parts {
		n += len(p)
	}
	var b buf
	b.u32(uint32(n))
	b.WriteString(typ)
	for _, p := range parts {
		b.Write(p)
	}
	return b.Bytes()
}

func full(typ string, version uint8, flags uint32, parts ...[]byte) []byte {
	var h buf
	h.u32(uint32(version)<<24 | flags)
	return box(typ, append([][]byte{h.Bytes()}, parts...)...)
}

// Close writes ftyp, moov and mdat.
func (m *Writer) Close() error {
	if m.closed {
		return nil
	}
	m.closed = true
	if len(m.samples) == 0 {
		return errors.New("mp4: no frames")
	}
	if len(m.sps) < 4 || len(m.pps) == 0 {
		return errors.New("mp4: missing SPS or PPS")
	}
	ftyp := box("ftyp", []byte("isom"), []byte{0, 0, 2, 0}, []byte("isomiso2avc1mp41"))

	var total uint64
	for _, s := range m.samples {
		total += uint64(len(s))
	}
	large := total+8 > 0xFFFFFFFF
	mdatHeader := 8
	if large {
		mdatHeader = 16
	}
	// moov size does not depend on the chunk offset value, so build it once
	// with a placeholder, then again with the real offset.
	moov := m.moov(0, large)
	offset := uint64(len(ftyp)+len(moov)) + uint64(mdatHeader)
	moov = m.moov(offset, large)

	if _, err := m.w.Write(ftyp); err != nil {
		return err
	}
	if _, err := m.w.Write(moov); err != nil {
		return err
	}
	var h buf
	if large {
		h.u32(1)
		h.WriteString("mdat")
		h.u64(total + 16)
	} else {
		h.u32(uint32(total + 8))
		h.WriteString("mdat")
	}
	if _, err := m.w.Write(h.Bytes()); err != nil {
		return err
	}
	for _, s := range m.samples {
		if _, err := m.w.Write(s); err != nil {
			return err
		}
	}
	return nil
}

func (m *Writer) moov(chunkOffset uint64, large bool) []byte {
	n := uint32(len(m.samples))
	timescale := uint32(m.fps * 1000)
	durMedia := uint64(n) * 1000
	durMovie := uint64(n) * 1000 / uint64(m.fps) // movie timescale 1000

	var mvhd buf
	mvhd.u32(0)
	mvhd.u32(0)
	mvhd.u32(1000)
	mvhd.u32(uint32(durMovie))
	mvhd.u32(0x00010000) // rate 1.0
	mvhd.u16(0x0100)     // volume
	mvhd.zero(10)
	writeMatrix(&mvhd)
	mvhd.zero(24)
	mvhd.u32(2) // next track id

	var tkhd buf
	tkhd.u32(0)
	tkhd.u32(0)
	tkhd.u32(1) // track id
	tkhd.u32(0)
	tkhd.u32(uint32(durMovie))
	tkhd.zero(8)
	tkhd.u16(0) // layer
	tkhd.u16(0) // group
	tkhd.u16(0) // volume
	tkhd.u16(0)
	writeMatrix(&tkhd)
	tkhd.u32(uint32(m.width) << 16)
	tkhd.u32(uint32(m.height) << 16)

	var mdhd buf
	mdhd.u32(0)
	mdhd.u32(0)
	mdhd.u32(timescale)
	mdhd.u32(uint32(durMedia))
	mdhd.u16(0x55C4) // "und"
	mdhd.u16(0)

	var hdlr buf
	hdlr.u32(0)
	hdlr.WriteString("vide")
	hdlr.zero(12)
	hdlr.WriteString("reel video\x00")

	var vmhd buf
	vmhd.zero(8)

	var dref buf
	dref.u32(1)
	dref.Write(full("url ", 0, 1))

	// sample description
	var avcC buf
	avcC.u8(1)
	avcC.u8(m.sps[1])
	avcC.u8(m.sps[2])
	avcC.u8(m.sps[3])
	avcC.u8(0xFF) // 4-byte lengths
	avcC.u8(0xE1) // one SPS
	avcC.u16(uint16(len(m.sps)))
	avcC.Write(m.sps)
	avcC.u8(1)
	avcC.u16(uint16(len(m.pps)))
	avcC.Write(m.pps)
	if p := m.sps[1]; p == 100 || p == 110 || p == 122 || p == 144 {
		avcC.u8(0xFC | 1) // 4:2:0
		avcC.u8(0xF8)     // 8-bit luma
		avcC.u8(0xF8)     // 8-bit chroma
		avcC.u8(0)
	}
	var colr buf
	colr.WriteString("nclx")
	colr.u16(1) // BT.709 primaries
	colr.u16(1) // BT.709 transfer
	colr.u16(1) // BT.709 matrix
	colr.u8(0)  // limited range
	var pasp buf
	pasp.u32(1)
	pasp.u32(1)

	var avc1 buf
	avc1.zero(6)
	avc1.u16(1) // data reference index
	avc1.zero(16)
	avc1.u16(uint16(m.width))
	avc1.u16(uint16(m.height))
	avc1.u32(0x00480000)
	avc1.u32(0x00480000)
	avc1.u32(0)
	avc1.u16(1)
	name := make([]byte, 32)
	copy(name[1:], "reel")
	name[0] = 4
	avc1.Write(name)
	avc1.u16(0x0018)
	avc1.u16(0xFFFF)
	avc1.Write(box("avcC", avcC.Bytes()))
	avc1.Write(box("colr", colr.Bytes()))
	avc1.Write(box("pasp", pasp.Bytes()))

	var stsd buf
	stsd.u32(1)
	stsd.Write(box("avc1", avc1.Bytes()))

	var stts buf
	stts.u32(1)
	stts.u32(n)
	stts.u32(1000)

	var stss buf
	stss.u32(uint32(len(m.keys)))
	for _, k := range m.keys {
		stss.u32(k)
	}

	var stsc buf
	stsc.u32(1)
	stsc.u32(1)
	stsc.u32(n)
	stsc.u32(1)

	var stsz buf
	stsz.u32(0)
	stsz.u32(n)
	for _, s := range m.samples {
		stsz.u32(uint32(len(s)))
	}

	var stco []byte
	if large {
		var b buf
		b.u32(1)
		b.u64(chunkOffset)
		stco = full("co64", 0, 0, b.Bytes())
	} else {
		var b buf
		b.u32(1)
		b.u32(uint32(chunkOffset))
		stco = full("stco", 0, 0, b.Bytes())
	}

	stbl := box("stbl",
		full("stsd", 0, 0, stsd.Bytes()),
		full("stts", 0, 0, stts.Bytes()),
		full("stss", 0, 0, stss.Bytes()),
		full("stsc", 0, 0, stsc.Bytes()),
		full("stsz", 0, 0, stsz.Bytes()),
		stco,
	)
	minf := box("minf", full("vmhd", 0, 1, vmhd.Bytes()), box("dinf", full("dref", 0, 0, dref.Bytes())), stbl)
	mdia := box("mdia", full("mdhd", 0, 0, mdhd.Bytes()), full("hdlr", 0, 0, hdlr.Bytes()), minf)
	trak := box("trak", full("tkhd", 0, 3, tkhd.Bytes()), mdia)
	return box("moov", full("mvhd", 0, 0, mvhd.Bytes()), trak)
}

func writeMatrix(b *buf) {
	for _, v := range []uint32{0x00010000, 0, 0, 0, 0x00010000, 0, 0, 0, 0x40000000} {
		b.u32(v)
	}
}
