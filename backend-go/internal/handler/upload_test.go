package handler

import (
	"archive/zip"
	"bytes"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fileHeaderOf 构造 multipart.FileHeader（不经 HTTP，直接喂给校验/落盘函数）
func fileHeaderOf(t *testing.T, filename string, content []byte) *multipart.FileHeader {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("构造表单失败: %v", err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatalf("写入表单失败: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("关闭表单失败: %v", err)
	}
	form, err := multipart.NewReader(&buf, w.Boundary()).ReadForm(1 << 20)
	if err != nil {
		t.Fatalf("解析表单失败: %v", err)
	}
	return form.File["file"][0]
}

// zipWith 构造仅含指定条目的 zip 内容
func zipWith(t *testing.T, entries ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range entries {
		fw, err := zw.Create(name)
		if err != nil {
			t.Fatalf("创建 zip 条目失败: %v", err)
		}
		if _, err := fw.Write([]byte("x")); err != nil {
			t.Fatalf("写入 zip 条目失败: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("关闭 zip 失败: %v", err)
	}
	return buf.Bytes()
}

// TestValidateUploadAcceptsMatchingContent 内容与扩展名一致时应通过
func TestValidateUploadAcceptsMatchingContent(t *testing.T) {
	ole2 := append(append([]byte{}, ole2Magic...), bytes.Repeat([]byte{0}, 64)...)
	cases := []struct {
		name     string
		filename string
		content  []byte
		isImage  bool
	}{
		{"docx", "报告.docx", zipWith(t, "[Content_Types].xml", "word/document.xml"), false},
		{"xlsx", "名单.xlsx", zipWith(t, "xl/workbook.xml"), false},
		{"pptx", "课件.pptx", zipWith(t, "ppt/presentation.xml"), false},
		{"doc", "说明.doc", ole2, false},
		{"xls", "表.xls", ole2, false},
		{"pdf", "材料.pdf", []byte("%PDF-1.7\n%%EOF\n"), false},
		{"zip", "打包.zip", zipWith(t, "a.txt"), false},
		{"png", "照片.png", []byte("\x89PNG\r\n\x1a\n" + strings.Repeat("\x00", 32)), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateUpload(fileHeaderOf(t, tc.filename, tc.content), tc.isImage); err != nil {
				t.Fatalf("%s 应通过校验, 实际: %v", tc.filename, err)
			}
		})
	}
}

// TestValidateUploadRejectsContentMismatch Office 改名文件必须被拦截
func TestValidateUploadRejectsContentMismatch(t *testing.T) {
	ole2 := append(append([]byte{}, ole2Magic...), bytes.Repeat([]byte{0}, 64)...)
	cases := []struct {
		name     string
		filename string
		content  []byte
	}{
		{"docx 实为纯文本", "报告.docx", []byte("just plain text content")},
		{"docx 是 zip 但没有 word/", "报告.docx", zipWith(t, "xl/workbook.xml")},
		{"xlsx 是 zip 但没有 xl/", "名单.xlsx", zipWith(t, "word/document.xml")},
		{"pptx 是 zip 但没有 ppt/", "课件.pptx", zipWith(t, "word/document.xml")},
		{"doc 实为 zip", "说明.doc", zipWith(t, "word/document.xml")},
		{"doc 实为纯文本", "说明.doc", []byte("plain")},
		{"xlsx 实为 OLE2 老式容器", "表.xlsx", ole2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateUpload(fileHeaderOf(t, tc.filename, tc.content), false)
			if err == nil || !strings.Contains(err.Error(), "内容与扩展名不匹配") {
				t.Fatalf("%s 应被拦截, 实际: %v", tc.filename, err)
			}
		})
	}
}

// TestValidateUploadRejectsUnsupportedAndOversize 扩展名白名单与大小上限仍然生效
func TestValidateUploadRejectsUnsupportedAndOversize(t *testing.T) {
	if err := validateUpload(fileHeaderOf(t, "木马.exe", []byte("MZ")), false); err == nil ||
		!strings.Contains(err.Error(), "不支持的文件类型") {
		t.Fatalf("exe 应被拒绝, 实际: %v", err)
	}
	fh := fileHeaderOf(t, "大图.png", []byte("\x89PNG\r\n\x1a\n"))
	fh.Size = maxImageSize + 1
	if err := validateUpload(fh, true); err == nil || !strings.Contains(err.Error(), "大小超限") {
		t.Fatalf("超限文件应被拒绝, 实际: %v", err)
	}
}

// TestSaveUploadedCleansUpOnFailure 落盘失败不得留下半截文件
func TestSaveUploadedCleansUpOnFailure(t *testing.T) {
	fh := fileHeaderOf(t, "材料.pdf", []byte("%PDF-1.7\nhello"))

	// 目标目录不存在：os.Create 失败，不应产生任何文件
	dst := filepath.Join(t.TempDir(), "missing-dir", "out.pdf")
	if err := saveUploaded(fh, dst); err == nil {
		t.Fatal("目录不存在时应返回错误")
	}
	if _, err := os.Stat(dst); err == nil {
		t.Fatalf("失败后不应残留文件: %s", dst)
	}

	// 正常落盘：内容完整
	dst2 := filepath.Join(t.TempDir(), "ok.pdf")
	if err := saveUploaded(fh, dst2); err != nil {
		t.Fatalf("正常落盘失败: %v", err)
	}
	got, err := os.ReadFile(dst2)
	if err != nil {
		t.Fatalf("读取落盘文件失败: %v", err)
	}
	if !bytes.Equal(got, []byte("%PDF-1.7\nhello")) {
		t.Fatalf("落盘内容不符: %q", got)
	}
}
