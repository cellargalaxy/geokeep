package importer

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"geokeep/internal/model"
)

// dawarichV2Parser 解析 dawarich v2 archive：
// 期望条目 `points/YYYY-MM.jsonl`（每行一个 Point JSON 对象）。
// 其它子目录（tracks/visits/trips/stats/digests/tags/raw_data_archives）整体跳过。
//
// 输入可能是 .tar.gz / .zip；自动嗅探 magic bytes。
type dawarichV2Parser struct{}

func (p *dawarichV2Parser) Parse(ctx context.Context, r io.Reader, emit func(*model.Point) error) error {
	raw, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if len(raw) >= 4 && raw[0] == 'P' && raw[1] == 'K' {
		return parseV2Zip(ctx, raw, emit)
	}
	if len(raw) >= 2 && raw[0] == 0x1f && raw[1] == 0x8b {
		return parseV2TarGz(ctx, raw, emit)
	}
	return errors.New("dawarich_v2: 仅支持 .tar.gz 或 .zip")
}

func parseV2TarGz(ctx context.Context, raw []byte, emit func(*model.Point) error) error {
	gz, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	matched := 0
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if !isV2PointsEntry(h.Name) {
			continue
		}
		matched++
		if err := parseV2JSONL(ctx, tr, emit); err != nil {
			return err
		}
	}
	if matched == 0 {
		return errors.New("dawarich_v2: 归档内未找到 points/*.jsonl 条目")
	}
	return nil
}

func parseV2Zip(ctx context.Context, raw []byte, emit func(*model.Point) error) error {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return err
	}
	matched := 0
	for _, f := range zr.File {
		if !isV2PointsEntry(f.Name) {
			continue
		}
		matched++
		rc, err := f.Open()
		if err != nil {
			return err
		}
		err = parseV2JSONL(ctx, rc, emit)
		rc.Close()
		if err != nil {
			return err
		}
	}
	if matched == 0 {
		return errors.New("dawarich_v2: 归档内未找到 points/*.jsonl 条目")
	}
	return nil
}

func isV2PointsEntry(name string) bool {
	// 兼容 archive 内可能存在的根目录前缀
	idx := strings.Index(name, "points/")
	if idx < 0 {
		return false
	}
	rest := name[idx:]
	return strings.HasSuffix(rest, ".jsonl")
}

func parseV2JSONL(ctx context.Context, r io.Reader, emit func(*model.Point) error) error {
	br := bufio.NewReaderSize(r, 64*1024)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			line = bytes.TrimRight(line, "\r\n")
			if len(line) > 0 {
				var row dawarichPoint
				if e := json.Unmarshal(line, &row); e == nil {
					if p := row.toModel(); p != nil {
						p.RawData = append([]byte(nil), line...)
						if e2 := emit(p); e2 != nil {
							return e2
						}
					}
				}
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
