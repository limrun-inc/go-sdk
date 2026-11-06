package syncfolder

import (
	"encoding/binary"
	"io"
	"os"
)

// VCDIFF uses big-endian base-128 integers, unlike encoding/binary's varints.
func vcdint(dst []byte, n uint64) []byte {
	var b [10]byte
	i := len(b) - 1
	b[i] = byte(n & 127)
	for n >>= 7; n > 0; n >>= 7 {
		i--
		b[i] = byte(n&127) | 128
	}
	return append(dst, b[i:]...)
}

// EncodePatch writes an RFC 3284 delta that the daemons' xdelta3 decoder accepts.
// Both input files are streamed; the source index has at most one million entries.
// A negative result means the patch exceeded maxBytes and was removed.
func EncodePatch(basisPath, targetPath, output string, maxBytes int64) (int64, error) {
	basis, err := os.Open(basisPath)
	if err != nil {
		return 0, err
	}
	defer basis.Close()
	stat, err := basis.Stat()
	if err != nil {
		return 0, err
	}
	target, err := os.Open(targetPath)
	if err != nil {
		return 0, err
	}
	defer target.Close()
	stride := int64(16)
	if stat.Size()/stride > 1<<20 {
		stride = (stat.Size() + (1 << 20) - 1) / (1 << 20)
	}
	index := make(map[uint64]int64)
	chunk := make([]byte, 4<<20)
	var position int64
	for {
		n, readErr := basis.Read(chunk)
		if n > 0 {
			first := (stride - position%stride) % stride
			for i := first; i+8 <= int64(n); i += stride {
				index[binary.LittleEndian.Uint64(chunk[i:i+8])] = position + i
			}
			position += int64(n)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return 0, readErr
		}
	}
	out, err := os.Create(output)
	if err != nil {
		return 0, err
	}
	success := false
	defer func() {
		out.Close()
		if !success {
			os.Remove(output)
		}
	}()
	written := int64(0)
	write := func(b []byte) error { n, err := out.Write(b); written += int64(n); return err }
	if err = write([]byte{0xd6, 0xc3, 0xc4, 0, 0}); err != nil {
		return 0, err
	}
	compare := make([]byte, len(chunk))
	for {
		n, readErr := io.ReadFull(target, chunk)
		if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
			return 0, readErr
		}
		if n == 0 {
			break
		}
		window := chunk[:n]
		var data, inst, addr []byte
		literal := 0
		add := func(end int) {
			if end > literal {
				inst = append(inst, 1)
				inst = vcdint(inst, uint64(end-literal))
				data = append(data, window[literal:end]...)
			}
		}
		for i := 0; i+8 <= n; {
			offset, ok := index[binary.LittleEndian.Uint64(window[i:i+8])]
			if !ok {
				i++
				continue
			}
			size := n - i
			if remaining := stat.Size() - offset; int64(size) > remaining {
				size = int(remaining)
			}
			got, readErr := basis.ReadAt(compare[:size], offset)
			if readErr != nil && readErr != io.EOF {
				return 0, readErr
			}
			matched := 0
			for matched < got && window[i+matched] == compare[matched] {
				matched++
			}
			if matched < 16 {
				i++
				continue
			}
			add(i)
			inst = append(inst, 19)
			inst = vcdint(inst, uint64(matched))
			addr = vcdint(addr, uint64(offset))
			i += matched
			literal = i
		}
		add(n)
		delta := vcdint(nil, uint64(n))
		delta = append(delta, 0)
		delta = vcdint(delta, uint64(len(data)))
		delta = vcdint(delta, uint64(len(inst)))
		delta = vcdint(delta, uint64(len(addr)))
		delta = append(delta, data...)
		delta = append(delta, inst...)
		delta = append(delta, addr...)
		header := []byte{1}
		header = vcdint(header, uint64(stat.Size()))
		header = vcdint(header, 0)
		header = vcdint(header, uint64(len(delta)))
		if err = write(header); err != nil {
			return 0, err
		}
		if err = write(delta); err != nil {
			return 0, err
		}
		if maxBytes >= 0 && written > maxBytes {
			return -1, nil
		}
	}
	if err = out.Close(); err != nil {
		return 0, err
	}
	success = true
	return written, nil
}
