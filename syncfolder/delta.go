package syncfolder

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// xdelta3Wasm is the streaming xdelta3 encoder the TypeScript SDK ships as
// @limrun/xdelta3-wasm 0.2.0 (upstream xdelta3 plus packages/xdelta3-wasm/native/xd3w.c
// in limrun-inc/typescript-sdk), decoded from the package's dist/wasm-embedded.js.
// Running the same module keeps patches byte-identical across the two SDKs without
// cgo or an xdelta3 binary on the user's machine. sha256:
// bfb81d752492173d2b09ed587e7112f359e2ffe9219d268bf4096ee25e7f2e63.
//
//go:embed xdelta3.wasm
var xdelta3Wasm []byte

// The TypeScript encoder reads the target in 1 MiB chunks and the module turns
// each chunk into one VCDIFF window, so the same chunk size yields the same bytes.
const xdelta3ChunkSize = 1 << 20

// Status values returned by xd3w_step; other values are xdelta3 error codes.
const (
	xd3wNeedInput       = 1
	xd3wHaveOutput      = 2
	xd3wNeedSourceBlock = 3
	xd3wContinue        = 4
)

var (
	xdelta3Once     sync.Once
	xdelta3Runtime  wazero.Runtime
	xdelta3Compiled wazero.CompiledModule
	xdelta3Err      error
)

// xdelta3Module compiles the embedded module once per process; every encode
// then instantiates its own linear memory from the compiled code.
func xdelta3Module() (wazero.Runtime, wazero.CompiledModule, error) {
	xdelta3Once.Do(func() {
		ctx := context.Background()
		// Reserving each instance's full address range up front keeps the encoder's
		// memory growth from copying the linear memory; untouched pages cost nothing.
		runtime := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig().WithMemoryCapacityFromMax(true))
		_, err := runtime.NewHostModuleBuilder("env").
			NewFunctionBuilder().WithFunc(func(int32) {}).Export("emscripten_notify_memory_growth").
			Instantiate(ctx)
		if err == nil {
			xdelta3Compiled, err = runtime.CompileModule(ctx, xdelta3Wasm)
		}
		if err != nil {
			_ = runtime.Close(ctx)
			xdelta3Err = fmt.Errorf("xdelta3: load encoder: %w", err)
			return
		}
		xdelta3Runtime = runtime
	})
	return xdelta3Runtime, xdelta3Compiled, xdelta3Err
}

// xdelta3Encoder drives one xd3w encoder instance inside the module.
type xdelta3Encoder struct {
	module api.Module
	handle uint32
	calls  map[string]api.Function
}

func newXdelta3Encoder(ctx context.Context, sourceSize int64) (*xdelta3Encoder, error) {
	runtime, compiled, err := xdelta3Module()
	if err != nil {
		return nil, err
	}
	module, err := runtime.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithName("").WithStartFunctions())
	if err != nil {
		return nil, fmt.Errorf("xdelta3: instantiate encoder: %w", err)
	}
	e := &xdelta3Encoder{module: module, calls: map[string]api.Function{}}
	for _, name := range []string{"xd3w_new", "xd3w_free", "xd3w_input_buf", "xd3w_input_cap", "xd3w_avail_input", "xd3w_step", "xd3w_output_ptr", "xd3w_output_len", "xd3w_consume_output", "xd3w_source_ptr", "xd3w_source_request_offset", "xd3w_source_request_len", "xd3w_provide_source", "xd3w_close", "xd3w_errmsg"} {
		fn := module.ExportedFunction(name)
		if fn == nil {
			_ = module.Close(ctx)
			return nil, fmt.Errorf("xdelta3: encoder module lacks %s", name)
		}
		e.calls[name] = fn
	}
	if initialize := module.ExportedFunction("_initialize"); initialize != nil {
		if _, err := initialize.Call(ctx); err != nil {
			_ = module.Close(ctx)
			return nil, fmt.Errorf("xdelta3: initialize encoder: %w", err)
		}
	}
	handle, err := e.call(ctx, "xd3w_new", api.EncodeF64(float64(sourceSize)))
	if err != nil {
		_ = module.Close(ctx)
		return nil, err
	}
	if handle == 0 {
		_ = module.Close(ctx)
		return nil, errors.New("xdelta3: failed to allocate encoder")
	}
	e.handle = handle
	return e, nil
}

// call invokes an export whose result, if any, is an i32. The result slot is 64
// bits wide and its upper half is undefined for i32 values, so it is decoded.
func (e *xdelta3Encoder) call(ctx context.Context, name string, params ...uint64) (uint32, error) {
	results, err := e.calls[name].Call(ctx, params...)
	if err != nil {
		return 0, fmt.Errorf("xdelta3: %s: %w", name, err)
	}
	if len(results) == 0 {
		return 0, nil
	}
	return api.DecodeU32(results[0]), nil
}

// sourceRequestOffset is the only export returning a double: byte offsets can
// exceed the 32-bit range of the module.
func (e *xdelta3Encoder) sourceRequestOffset(ctx context.Context) (int64, error) {
	results, err := e.calls["xd3w_source_request_offset"].Call(ctx, uint64(e.handle))
	if err != nil {
		return 0, fmt.Errorf("xdelta3: xd3w_source_request_offset: %w", err)
	}
	return int64(api.DecodeF64(results[0])), nil
}

func (e *xdelta3Encoder) close(ctx context.Context) {
	if e.handle != 0 {
		_, _ = e.call(ctx, "xd3w_free", uint64(e.handle))
	}
	_ = e.module.Close(ctx)
}

func (e *xdelta3Encoder) errorFromStatus(ctx context.Context, status uint32) error {
	ptr, err := e.call(ctx, "xd3w_errmsg", uint64(e.handle))
	if err != nil {
		return err
	}
	message := ""
	if ptr != 0 {
		memory := e.module.Memory()
		for end := ptr; ; end++ {
			b, ok := memory.ReadByte(end)
			if !ok || b == 0 {
				message = string(mustRead(memory, ptr, end-ptr))
				break
			}
		}
	}
	code := int32(status)
	if message == "" {
		return fmt.Errorf("xdelta3 encode failed (code=%d)", code)
	}
	return fmt.Errorf("xdelta3 encode failed: %s (code=%d)", message, code)
}

func mustRead(memory api.Memory, offset, count uint32) []byte {
	data, ok := memory.Read(offset, count)
	if !ok {
		return nil
	}
	return data
}

// drain runs the encoder until it asks for more input, serving source block
// requests from basis and appending every output window through sink.
func (e *xdelta3Encoder) drain(ctx context.Context, basis io.ReaderAt, sink func([]byte) error) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		status, err := e.call(ctx, "xd3w_step", uint64(e.handle))
		if err != nil {
			return err
		}
		switch status {
		case xd3wNeedInput:
			return nil
		case xd3wHaveOutput:
			length, err := e.call(ctx, "xd3w_output_len", uint64(e.handle))
			if err != nil {
				return err
			}
			if length > 0 {
				ptr, err := e.call(ctx, "xd3w_output_ptr", uint64(e.handle))
				if err != nil {
					return err
				}
				data, ok := e.module.Memory().Read(ptr, length)
				if !ok {
					return errors.New("xdelta3: encoder produced output outside its memory")
				}
				if err := sink(data); err != nil {
					return err
				}
			}
			if _, err := e.call(ctx, "xd3w_consume_output", uint64(e.handle)); err != nil {
				return err
			}
		case xd3wNeedSourceBlock:
			ptr, err := e.call(ctx, "xd3w_source_ptr", uint64(e.handle))
			if err != nil {
				return err
			}
			offset, err := e.sourceRequestOffset(ctx)
			if err != nil {
				return err
			}
			length, err := e.call(ctx, "xd3w_source_request_len", uint64(e.handle))
			if err != nil {
				return err
			}
			read := 0
			if length > 0 {
				block, ok := e.module.Memory().Read(ptr, length)
				if !ok {
					return errors.New("xdelta3: encoder requested a source block outside its memory")
				}
				read, err = basis.ReadAt(block, offset)
				if err != nil && err != io.EOF {
					return err
				}
			}
			if _, err := e.call(ctx, "xd3w_provide_source", uint64(e.handle), api.EncodeU32(uint32(read))); err != nil {
				return err
			}
		case xd3wContinue:
		default:
			return e.errorFromStatus(ctx, status)
		}
	}
}

// feed hands one target chunk to the encoder; final flushes the last window.
func (e *xdelta3Encoder) feed(ctx context.Context, basis io.ReaderAt, chunk []byte, final bool, sink func([]byte) error) error {
	inputPtr, err := e.call(ctx, "xd3w_input_buf", uint64(e.handle))
	if err != nil {
		return err
	}
	inputCap, err := e.call(ctx, "xd3w_input_cap")
	if err != nil {
		return err
	}
	if inputPtr == 0 || inputCap == 0 {
		return errors.New("xdelta3: encoder did not provide an input buffer")
	}
	if len(chunk) == 0 {
		if !final {
			return nil
		}
		if _, err := e.call(ctx, "xd3w_avail_input", uint64(e.handle), 0, 1); err != nil {
			return err
		}
		return e.drain(ctx, basis, sink)
	}
	for start := 0; start < len(chunk); start += int(inputCap) {
		end := min(start+int(inputCap), len(chunk))
		if !e.module.Memory().Write(inputPtr, chunk[start:end]) {
			return errors.New("xdelta3: encoder input buffer is outside its memory")
		}
		flush := uint64(0)
		if final && end == len(chunk) {
			flush = 1
		}
		if _, err := e.call(ctx, "xd3w_avail_input", uint64(e.handle), api.EncodeU32(uint32(end-start)), flush); err != nil {
			return err
		}
		if err := e.drain(ctx, basis, sink); err != nil {
			return err
		}
	}
	return nil
}

// errPatchTooLarge stops encoding once the patch cannot beat a full upload.
var errPatchTooLarge = errors.New("xdelta3: patch exceeds the size limit")

// EncodePatch writes a VCDIFF delta of targetPath against basisPath to output and
// returns its size. Encoding stops as soon as the patch exceeds maxBytes; the
// partial file is removed and the result is -1 so callers fall back to a full
// upload cheaply. A negative maxBytes disables the limit.
func EncodePatch(ctx context.Context, basisPath, targetPath, output string, maxBytes int64) (int64, error) {
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
	out, err := os.Create(output)
	if err != nil {
		return 0, err
	}
	keep := false
	defer func() {
		out.Close()
		if !keep {
			os.Remove(output)
		}
	}()
	encoder, err := newXdelta3Encoder(ctx, stat.Size())
	if err != nil {
		return 0, err
	}
	defer encoder.close(ctx)
	written := int64(0)
	sink := func(data []byte) error {
		written += int64(len(data))
		if maxBytes >= 0 && written > maxBytes {
			return errPatchTooLarge
		}
		_, err := out.Write(data)
		return err
	}
	// Each chunk is fed only once the next read shows whether it is the last,
	// because the final window must carry the flush flag. The two buffers swap
	// roles after every read.
	pending := make([]byte, 0, xdelta3ChunkSize)
	next := make([]byte, xdelta3ChunkSize)
	for {
		n, readErr := io.ReadFull(target, next)
		if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
			return 0, readErr
		}
		if n == 0 {
			err = encoder.feed(ctx, basis, pending, true, sink)
			break
		}
		if err = encoder.feed(ctx, basis, pending, false, sink); err != nil {
			break
		}
		pending, next = next[:n], pending[:cap(pending)]
	}
	if errors.Is(err, errPatchTooLarge) {
		return -1, nil
	}
	if err != nil {
		return 0, err
	}
	if status, err := encoder.call(ctx, "xd3w_close", uint64(encoder.handle)); err != nil {
		return 0, err
	} else if status != 0 {
		return 0, encoder.errorFromStatus(ctx, status)
	}
	if err := out.Close(); err != nil {
		return 0, err
	}
	keep = true
	return written, nil
}
