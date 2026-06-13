package importer

import (
	"bufio"
	"context"
	"io"
	"strings"

	"geokeep/internal/ingest"
	"geokeep/internal/model"
)

// owntracksRecParser 解析 OwnTracks Recorder 的 .rec 行式文件。
// 每行格式：`<iso8601>\t*\t{json}\n`，取第 3 段作为 _type=location payload。
// 与 ingest.MapOwnTracksLocation 共用字段映射，保证「上报与导入」字段一致。
type owntracksRecParser struct{}

func (p *owntracksRecParser) Parse(ctx context.Context, r io.Reader, emit func(*model.Point) error) error {
	br := bufio.NewReaderSize(r, 64*1024)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line, err := br.ReadString('\n')
		if len(line) > 0 {
			payload := extractRecPayload(line)
			if payload != "" {
				pt, mapErr := ingest.MapOwnTracksLocation([]byte(payload))
				if mapErr == nil {
					if e := emit(pt); e != nil {
						return e
					}
				}
				// 非 location 类型或解析失败的行：忽略，与「raw_data 全保留」需求一致（已在 raw 字段保留）。
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

func extractRecPayload(line string) string {
	line = strings.TrimRight(line, "\r\n")
	if line == "" || line[0] == '#' {
		return ""
	}
	parts := strings.SplitN(line, "\t", 3)
	if len(parts) == 3 {
		// 第三段以 "* " 之类前缀开头时取 JSON 部分
		body := parts[2]
		if i := strings.Index(body, "{"); i >= 0 {
			return body[i:]
		}
	}
	return ""
}
