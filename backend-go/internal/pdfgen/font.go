// Package pdfgen 基于 gopdf 的中文 PDF 生成（对齐旧端 reportlab/Playwright 导出模板）。
package pdfgen

import (
	"encoding/binary"
	"errors"
	"os"
	"sync"
)

// FontFamily 统一注册到 gopdf 的字体族名
const FontFamily = "zh"

// 中文字体候选路径（对齐旧端 _get_chinese_font_name）：
// Linux 容器（alpine/debian 的 wqy 包）→ Windows 开发机 → macOS。
// 可用环境变量 PDF_FONT_PATH 覆盖。
var fontCandidates = []string{
	"/usr/share/fonts/wqy-zenhei/wqy-zenhei.ttc", // alpine: apk add font-wqy-zenhei (community)
	"/usr/share/fonts/TTF/wqy-zenhei.ttc",
	"/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc",
	"/usr/share/fonts/truetype/wqy/wqy-microhei.ttc",
	"C:/Windows/Fonts/msyh.ttc",
	"C:/Windows/Fonts/msyh.ttf",
	"C:/Windows/Fonts/simhei.ttf",
	"C:/Windows/Fonts/simsun.ttc",
	"/System/Library/Fonts/PingFang.ttc",
	"/Library/Fonts/Arial Unicode.ttf",
}

var (
	fontOnce  sync.Once
	fontBytes []byte
	fontErr   error
)

// ErrNoChineseFont 未找到可用中文字体
var ErrNoChineseFont = errors.New("未找到可用的中文字体（可设置 PDF_FONT_PATH 指定 TTF/TTC 文件）")

// loadChineseFont 查找并缓存中文字体字节（TTC 取第一个 face）。
func loadChineseFont() ([]byte, error) {
	fontOnce.Do(func() {
		paths := fontCandidates
		if p := os.Getenv("PDF_FONT_PATH"); p != "" {
			paths = append([]string{p}, paths...)
		}
		for _, p := range paths {
			data, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			fontBytes, fontErr = extractFirstFont(data), nil
			return
		}
		fontErr = ErrNoChineseFont
	})
	return fontBytes, fontErr
}

// extractFirstFont TTC（TrueType Collection）提取第一个字体为独立 TTF：
// ttcf 头（tag4 + version4 + numFonts4 + offsets[numFonts*4]）指向各 face 的 sfnt
// 表目录，但目录内表 offset 是相对整个 TTC 文件的，不能直接切片——需按 face0
// 目录重建独立 TTF（新头 + 新目录 + 拷贝表数据）。
func extractFirstFont(data []byte) []byte {
	if len(data) < 16 || string(data[:4]) != "ttcf" {
		return data
	}
	off := int(binary.BigEndian.Uint32(data[12:16])) // OffsetTable[0]
	if off+12 > len(data) {
		return data
	}
	numTables := int(binary.BigEndian.Uint16(data[off+4 : off+6]))
	dirStart := off + 12
	if dirStart+numTables*16 > len(data) {
		return data
	}

	type tbl struct {
		tag                      string
		checksum, offset, length uint32
	}
	tables := make([]tbl, 0, numTables)
	for i := 0; i < numTables; i++ {
		e := data[dirStart+i*16 : dirStart+i*16+16]
		t := tbl{
			tag:      string(e[0:4]),
			checksum: binary.BigEndian.Uint32(e[4:8]),
			offset:   binary.BigEndian.Uint32(e[8:12]),
			length:   binary.BigEndian.Uint32(e[12:16]),
		}
		if int(t.offset)+int(t.length) > len(data) {
			return data // 数据越界，放弃提取
		}
		tables = append(tables, t)
	}

	// 重建：头 12B（sfnt version + numTables/searchRange/entrySelector/rangeShift）+ 目录 + 表数据
	out := make([]byte, 12, 12+numTables*16)
	copy(out[0:4], data[off:off+4])
	copy(out[4:12], data[off+4:off+12])

	var payload []byte
	offsets := make([]uint32, numTables)
	for i, t := range tables {
		offsets[i] = uint32(12 + numTables*16 + len(payload))
		payload = append(payload, data[t.offset:t.offset+t.length]...)
		for len(payload)%4 != 0 { // 表数据 4 字节对齐
			payload = append(payload, 0)
		}
	}
	for i, t := range tables {
		entry := make([]byte, 16)
		copy(entry[0:4], t.tag)
		binary.BigEndian.PutUint32(entry[4:8], t.checksum)
		binary.BigEndian.PutUint32(entry[8:12], offsets[i])
		binary.BigEndian.PutUint32(entry[12:16], t.length)
		out = append(out, entry...)
	}
	return append(out, payload...)
}
