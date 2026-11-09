package syncfolder

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/limrun-inc/go-sdk/internal/proxy"

	"github.com/limrun-inc/go-sdk/internal/direct"
)

type SeedSignatureBlock struct {
	Weak   uint32 `json:"w"`
	Strong string `json:"s"`
}
type SeedSignature struct {
	Version   int                  `json:"version"`
	BlockSize int64                `json:"blockSize"`
	FileSize  int64                `json:"fileSize"`
	SHA256    string               `json:"sha256"`
	Blocks    []SeedSignatureBlock `json:"blocks"`
}
type SeedBlockMatch struct {
	SourcePath   string
	SourceOffset int64
}
type SeedByteRange struct{ Start, End int64 }
type ReconstructSeedOptions struct {
	SignatureURL, SeedURL, Token, OutputPath, ExpectedSHA256 string
	SourcePaths                                              []string
	HTTPClient                                               *http.Client
	OnProgress                                               func(downloadedBytes, totalBytes int64)
}
type ReconstructSeedResult struct {
	DownloadedBytes, TotalDownloadBytes int64
	MatchedBlocks                       int
}

var shaPattern = regexp.MustCompile(`(?i)^[0-9a-f]{64}$`)
var strongPattern = regexp.MustCompile(`(?i)^[0-9a-f]{32}$`)

func WeakRollingChecksum(data []byte) uint32 {
	var a, b uint32
	for i, v := range data {
		a = (a + uint32(v)) & 65535
		b = (b + uint32(len(data)-i)*uint32(v)) & 65535
	}
	return b<<16 | a
}
func strongHash(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:16])
}
func (s SeedSignature) blockLength(i int) int64 {
	n := s.FileSize - int64(i)*s.BlockSize
	if n > s.BlockSize {
		n = s.BlockSize
	}
	return n
}
func FetchSeedSignature(ctx context.Context, signatureURL, token, expectedSHA256 string, client *http.Client) (SeedSignature, error) {
	var s SeedSignature
	c := direct.Client{Token: token, HTTPClient: client}
	res, err := c.Do(ctx, "GET", signatureURL, nil, "", -1)
	if err != nil {
		return s, err
	}
	defer res.Body.Close()
	if err = json.NewDecoder(res.Body).Decode(&s); err != nil {
		return s, err
	}
	if s.Version != 1 || s.BlockSize <= 0 || s.FileSize < 0 || !shaPattern.MatchString(s.SHA256) || !strings.EqualFold(s.SHA256, expectedSHA256) {
		return s, fmt.Errorf("invalid seed signature version, size, or SHA-256")
	}
	expected := s.FileSize / s.BlockSize
	if s.FileSize%s.BlockSize != 0 {
		expected++
	}
	if int64(len(s.Blocks)) != expected {
		return s, fmt.Errorf("invalid seed signature block count: got %d, expected %d", len(s.Blocks), expected)
	}
	for i := range s.Blocks {
		if !strongPattern.MatchString(s.Blocks[i].Strong) {
			return s, fmt.Errorf("invalid seed signature block at index %d", i)
		}
		s.Blocks[i].Strong = strings.ToLower(s.Blocks[i].Strong)
	}
	return s, nil
}
func FindSeedBlockMatches(ctx context.Context, paths []string, s SeedSignature) (map[int]SeedBlockMatch, error) {
	matches := map[int]SeedBlockMatch{}
	type source struct {
		path string
		size int64
	}
	var sources []source
	seen := map[string]bool{}
	for _, p := range paths {
		p, _ = filepath.Abs(p)
		if seen[p] {
			continue
		}
		seen[p] = true
		stat, err := os.Stat(p)
		if err == nil && stat.Mode().IsRegular() {
			sources = append(sources, source{p, stat.Size()})
		}
	}
	if len(s.Blocks) == 0 {
		return matches, nil
	}
	block := make([]byte, s.blockLength(0))
	for _, src := range sources {
		f, err := os.Open(src.path)
		if err != nil {
			return nil, err
		}
		for i, expected := range s.Blocks {
			if ctx.Err() != nil {
				f.Close()
				return nil, ctx.Err()
			}
			if _, ok := matches[i]; ok {
				continue
			}
			n := s.blockLength(i)
			offset := int64(i) * s.BlockSize
			if offset+n > src.size {
				continue
			}
			if _, err = f.ReadAt(block[:n], offset); err != nil {
				f.Close()
				return nil, err
			}
			if strongHash(block[:n]) == expected.Strong {
				matches[i] = SeedBlockMatch{src.path, offset}
			}
		}
		last := len(s.Blocks) - 1
		n := s.blockLength(last)
		if _, ok := matches[last]; !ok && n < s.BlockSize && src.size >= n {
			offset := src.size - n
			if _, err = f.ReadAt(block[:n], offset); err != nil {
				f.Close()
				return nil, err
			}
			if strongHash(block[:n]) == s.Blocks[last].Strong {
				matches[last] = SeedBlockMatch{src.path, offset}
			}
		}
		f.Close()
	}
	var missing int64
	for i := range s.Blocks {
		if _, ok := matches[i]; !ok {
			missing += s.blockLength(i)
		}
	}
	if float64(len(matches))/float64(len(s.Blocks)) >= .8 && missing <= 32*1024*1024 {
		return matches, nil
	}
	candidates := map[uint32][]int{}
	unmatched := 0
	for i := 0; i < int(s.FileSize/s.BlockSize); i++ {
		if _, ok := matches[i]; !ok {
			candidates[s.Blocks[i].Weak] = append(candidates[s.Blocks[i].Weak], i)
			unmatched++
		}
	}
	for _, src := range sources {
		if unmatched == 0 {
			break
		}
		if src.size < s.BlockSize {
			continue
		}
		f, err := os.Open(src.path)
		if err != nil {
			return nil, err
		}
		ring := make([]byte, s.BlockSize)
		if _, err = io.ReadFull(f, ring); err != nil {
			f.Close()
			return nil, err
		}
		checksum := WeakRollingChecksum(ring)
		a, b := checksum&65535, checksum>>16
		var offset int64
		pos := 0
		confirm := func() error {
			indexes := candidates[b<<16|a]
			needed := false
			for _, i := range indexes {
				if _, ok := matches[i]; !ok {
					needed = true
					break
				}
			}
			if !needed {
				return nil
			}
			if _, err = f.ReadAt(block, offset); err != nil {
				return err
			}
			strong := strongHash(block)
			for _, i := range indexes {
				if _, ok := matches[i]; !ok && s.Blocks[i].Strong == strong {
					matches[i] = SeedBlockMatch{src.path, offset}
					unmatched--
				}
			}
			return nil
		}
		if err = confirm(); err != nil {
			f.Close()
			return nil, err
		}
		incoming := make([]byte, 1024*1024)
		for unmatched > 0 && offset+s.BlockSize < src.size {
			if ctx.Err() != nil {
				f.Close()
				return nil, ctx.Err()
			}
			count, readErr := f.Read(incoming)
			for _, v := range incoming[:count] {
				old := ring[pos]
				ring[pos] = v
				pos = (pos + 1) % len(ring)
				a = (a - uint32(old) + uint32(v)) & 65535
				b = (b - uint32(s.BlockSize)*uint32(old) + a) & 65535
				offset++
				if err = confirm(); err != nil {
					f.Close()
					return nil, err
				}
				if unmatched == 0 {
					break
				}
			}
			if readErr != nil {
				if readErr != io.EOF {
					f.Close()
					return nil, readErr
				}
				break
			}
		}
		f.Close()
	}
	return matches, nil
}
func MissingSeedRanges(s SeedSignature, matches map[int]SeedBlockMatch) []SeedByteRange {
	var ranges []SeedByteRange
	start := -1
	for i := 0; i <= len(s.Blocks); i++ {
		_, found := matches[i]
		missing := i < len(s.Blocks) && !found
		if missing && start == -1 {
			start = i
		} else if !missing && start != -1 {
			ranges = append(ranges, SeedByteRange{int64(start) * s.BlockSize, int64(i-1)*s.BlockSize + s.blockLength(i-1) - 1})
			start = -1
		}
	}
	return ranges
}
func ReconstructSeedFromLocalFiles(ctx context.Context, opts ReconstructSeedOptions) (result ReconstructSeedResult, err error) {
	s, err := FetchSeedSignature(ctx, opts.SignatureURL, opts.Token, opts.ExpectedSHA256, opts.HTTPClient)
	if err != nil {
		return result, err
	}
	matches, err := FindSeedBlockMatches(ctx, opts.SourcePaths, s)
	if err != nil {
		return result, err
	}
	result.MatchedBlocks = len(matches)
	ranges := MissingSeedRanges(s, matches)
	for _, r := range ranges {
		result.TotalDownloadBytes += r.End - r.Start + 1
	}
	if opts.OnProgress != nil {
		opts.OnProgress(0, result.TotalDownloadBytes)
	}
	if err = os.MkdirAll(filepath.Dir(opts.OutputPath), 0755); err != nil {
		return result, err
	}
	out, err := os.CreateTemp(filepath.Dir(opts.OutputPath), ".reconstruct-*")
	if err != nil {
		return result, err
	}
	defer func() { out.Close(); os.Remove(out.Name()) }()
	if err = out.Truncate(s.FileSize); err != nil {
		return result, err
	}
	block := make([]byte, s.blockLength(0))
	for i, match := range matches {
		f, e := os.Open(match.SourcePath)
		if e != nil {
			return result, e
		}
		n := s.blockLength(i)
		_, e = f.ReadAt(block[:n], match.SourceOffset)
		f.Close()
		if e != nil {
			return result, e
		}
		if _, err = out.WriteAt(block[:n], int64(i)*s.BlockSize); err != nil {
			return result, err
		}
	}
	client := opts.HTTPClient
	if client == nil {
		client = proxy.Client()
	}
	for _, r := range ranges {
		req, e := http.NewRequestWithContext(ctx, "GET", opts.SeedURL, nil)
		if e != nil {
			return result, e
		}
		req.Header.Set("Authorization", "Bearer "+opts.Token)
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", r.Start, r.End))
		res, e := client.Do(req)
		if e != nil {
			return result, e
		}
		expected := fmt.Sprintf("bytes %d-%d/%d", r.Start, r.End, s.FileSize)
		if res.StatusCode != 206 || res.Header.Get("Content-Range") != expected {
			res.Body.Close()
			return result, fmt.Errorf("invalid seed range response: status %d, Content-Range %q", res.StatusCode, res.Header.Get("Content-Range"))
		}
		offset := r.Start
		buf := make([]byte, 64*1024)
		for {
			n, e := res.Body.Read(buf)
			if offset+int64(n) > r.End+1 {
				res.Body.Close()
				return result, fmt.Errorf("seed range response exceeded the requested length")
			}
			if n > 0 {
				if _, err = out.WriteAt(buf[:n], offset); err != nil {
					res.Body.Close()
					return result, err
				}
				offset += int64(n)
				result.DownloadedBytes += int64(n)
				if opts.OnProgress != nil {
					opts.OnProgress(result.DownloadedBytes, result.TotalDownloadBytes)
				}
			}
			if e != nil {
				if e != io.EOF {
					res.Body.Close()
					return result, e
				}
				break
			}
		}
		res.Body.Close()
		if offset != r.End+1 {
			return result, fmt.Errorf("seed range response was truncated")
		}
	}
	if err = out.Close(); err != nil {
		return result, err
	}
	actual, err := hashFile(out.Name())
	if err != nil {
		return result, err
	}
	if !strings.EqualFold(actual, opts.ExpectedSHA256) {
		return result, fmt.Errorf("reconstructed seed SHA-256 mismatch: got %s, expected %s", actual, opts.ExpectedSHA256)
	}
	return result, os.Rename(out.Name(), opts.OutputPath)
}

type AndroidSyncState struct {
	Roots []struct {
		RootName string `json:"rootName"`
		Files    []struct {
			Path, SHA256 string
			Size         int64
			Mode         uint32
		} `json:"files"`
	} `json:"roots"`
	Seeds []struct {
		SHA256 string  `json:"sha256"`
		Size   int64   `json:"size"`
		Mtime  float64 `json:"mtime"`
	} `json:"seeds"`
}

func BootstrapAndroidBasisCache(ctx context.Context, apkPath, basisDir string, opts Options, progress func(int64, int64)) error {
	c := direct.Client{URL: opts.APIURL, Token: opts.Token, HTTPClient: opts.HTTPClient}
	var state AndroidSyncState
	log := func(msg string) {
		if opts.Log != nil {
			opts.Log("debug", msg)
		}
	}
	if err := c.JSON(ctx, "GET", "/sync/state", nil, &state); err != nil {
		log("Android sync state unavailable: " + err.Error())
		return nil
	}
	remote := filepath.Base(apkPath)
	basis := filepath.Join(basisDir, remote)
	known := map[string]bool{}
	for _, root := range state.Roots {
		for _, f := range root.Files {
			if f.Path == remote {
				known[strings.ToLower(f.SHA256)] = true
			}
		}
	}
	seeds := state.Seeds[:0]
	for _, seed := range state.Seeds {
		if shaPattern.MatchString(seed.SHA256) {
			seeds = append(seeds, seed)
			known[strings.ToLower(seed.SHA256)] = true
		}
	}
	sort.SliceStable(seeds, func(i, j int) bool { return seeds[i].Mtime > seeds[j].Mtime })
	sources := []string{apkPath}
	if _, err := os.Stat(basis); err == nil {
		hash, e := hashFile(basis)
		if e != nil {
			return e
		}
		if known[hash] {
			return nil
		}
		sources = append(sources, basis)
	}
	hash, err := hashFile(apkPath)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(basisDir, 0755); err != nil {
		return err
	}
	if known[hash] {
		in, e := os.Open(apkPath)
		if e != nil {
			return e
		}
		defer in.Close()
		out, e := os.Create(basis)
		if e != nil {
			return e
		}
		_, e = io.Copy(out, in)
		closeErr := out.Close()
		if e != nil {
			return e
		}
		return closeErr
	}
	if len(seeds) == 0 {
		err = os.Remove(basis)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	seed := seeds[0]
	seedURL := strings.TrimRight(opts.APIURL, "/") + "/sync/seeds/" + seed.SHA256
	_, err = ReconstructSeedFromLocalFiles(ctx, ReconstructSeedOptions{SignatureURL: seedURL + ".sig", SeedURL: seedURL, Token: opts.Token, HTTPClient: opts.HTTPClient, SourcePaths: sources, OutputPath: basis, ExpectedSHA256: seed.SHA256, OnProgress: progress})
	if err == nil {
		return nil
	}
	log("basis seed reconstruction unavailable; downloading full seed: " + err.Error())
	if progress != nil {
		progress(0, seed.Size)
	}
	res, err := c.Do(ctx, "GET", seedURL, nil, "", -1)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	out, err := os.Create(basis)
	if err != nil {
		return err
	}
	defer out.Close()
	total := res.ContentLength
	if total < 0 {
		total = 0
	}
	var count int64
	buf := make([]byte, 64*1024)
	for {
		n, e := res.Body.Read(buf)
		if n > 0 {
			if _, err = out.Write(buf[:n]); err != nil {
				return err
			}
			count += int64(n)
			if progress != nil {
				progress(count, total)
			}
		}
		if e == io.EOF {
			return out.Close()
		}
		if e != nil {
			return e
		}
	}
}
