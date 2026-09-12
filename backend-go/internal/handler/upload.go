package handler

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"backend-go/internal/config"
	"backend-go/internal/middleware"
	"backend-go/internal/model"
	"backend-go/pkg/response"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// 允许的扩展名
var imageExts = map[string]string{
	".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".png": "image/png",
	".gif": "image/gif", ".webp": "image/webp",
}
var docExts = map[string]string{
	".pdf": "application/pdf", ".zip": "application/zip", ".rar": "application/vnd.rar",
	".doc": "application/msword", ".xls": "application/vnd.ms-excel",
	".ppt": "application/vnd.ms-powerpoint",
	// docx/xlsx/pptx 为 zip 容器，sniff 结果是 zip，改由 validateOfficeContent 校验包内部件
	".docx": "", ".xlsx": "", ".pptx": "",
}

// ole2Magic OLE2 复合文档签名（.doc/.xls/.ppt 同一容器格式）
var ole2Magic = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}

// zipMagic OOXML（.docx/.xlsx/.pptx）zip 容器签名
var zipMagic = []byte{'P', 'K', 0x03, 0x04}

// ooxmlPartDir 各 OOXML 扩展名必须包含的包内部件目录
var ooxmlPartDir = map[string]string{".docx": "word/", ".xlsx": "xl/", ".pptx": "ppt/"}

const (
	maxImageSize = 10 << 20 // 10MB
	maxFileSize  = 20 << 20 // 20MB
)

// Upload 文件上传接口
type Upload struct {
	cfg *config.Config
	db  *gorm.DB
}

func NewUpload(cfg *config.Config, db *gorm.DB) *Upload { return &Upload{cfg: cfg, db: db} }

// validateUpload 校验文件扩展名/大小/MIME（isImage 区分白名单与大小上限）
func validateUpload(fh *multipart.FileHeader, isImage bool) error {
	ext := strings.ToLower(filepath.Ext(fh.Filename))
	var allowed map[string]string
	var maxSize int64
	if isImage {
		allowed, maxSize = imageExts, maxImageSize
	} else {
		allowed, maxSize = docExts, maxFileSize
	}
	mimeType, ok := allowed[ext]
	if !ok {
		return errUnsupportedType(ext)
	}
	if fh.Size > maxSize {
		return errors.New("文件大小超限")
	}
	f, err := fh.Open()
	if err != nil {
		return errors.New("文件读取失败")
	}
	defer f.Close()
	head := make([]byte, 512)
	n, _ := f.Read(head)
	head = head[:n]
	sniffed := http.DetectContentType(head)
	if mimeType != "" && !strings.HasPrefix(sniffed, strings.Split(mimeType, "/")[0]) {
		return errContentMismatch()
	}
	if isImage && !strings.HasPrefix(sniffed, "image/") {
		return errContentMismatch()
	}
	return validateOfficeContent(fh, ext, head)
}

// validateOfficeContent 校验 Office 文件「内容与扩展名一致」：
//   - .doc/.xls/.ppt 必须是 OLE2 复合文档（三者为同一容器，魔数无法再细分）
//   - .docx/.xlsx/.pptx 必须是 zip 容器且包含对应部件目录（word/ xl/ ppt/）
//
// 仅校验扩展名会放行任意改名内容：文件随后由下载接口以附件分发，
// 攻击者可借可信域名投递可执行/网页内容，故必须校验真实容器结构。
func validateOfficeContent(fh *multipart.FileHeader, ext string, head []byte) error {
	switch ext {
	case ".docx", ".xlsx", ".pptx":
		if !bytes.HasPrefix(head, zipMagic) {
			return errContentMismatch()
		}
		partDir := ooxmlPartDir[ext]
		f, err := fh.Open()
		if err != nil {
			return errors.New("文件读取失败")
		}
		defer f.Close()
		zr, err := zip.NewReader(f, fh.Size)
		if err != nil {
			return errContentMismatch()
		}
		for _, zf := range zr.File {
			if strings.HasPrefix(zf.Name, partDir) {
				return nil
			}
		}
		return errContentMismatch()
	case ".doc", ".xls", ".ppt":
		if !bytes.HasPrefix(head, ole2Magic) {
			return errContentMismatch()
		}
	}
	return nil
}

func errContentMismatch() error { return errors.New("文件内容与扩展名不匹配") }

func errUnsupportedType(ext string) error { return errors.New("不支持的文件类型: " + ext) }

// sanitizeName 清理文件名，防路径穿越
func sanitizeName(name string) string {
	base := filepath.Base(name)
	var b strings.Builder
	for _, r := range base {
		if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
			b.WriteByte('_')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// requireUploadAdmin 对齐旧端 require_school_admin（= require_college_admin）
func requireUploadAdmin(c *gin.Context) bool {
	u := middleware.CurrentUser(c)
	if u.HasRole("system_admin") {
		return true
	}
	if u.HasAnyRole("school_admin", "college_admin") {
		if u.CollegeID == nil && len(u.UserColleges) == 0 {
			forbidden(c, "您没有管理的学院")
			return false
		}
		return true
	}
	forbidden(c, "权限不足，需要学院管理员权限")
	return false
}

// Do 上传文件（对齐旧端 POST /upload，管理员）
func (h *Upload) Do(c *gin.Context) {
	if !requireUploadAdmin(c) {
		return
	}
	fh, err := c.FormFile("file")
	if err != nil {
		missingQuery(c, "file")
		return
	}
	fileType := c.DefaultPostForm("file_type", "file")
	isImage := fileType == "image"
	if err := validateUpload(fh, isImage); err != nil {
		badReq(c, err.Error())
		return
	}

	// 保存：uploads/evaluations/temp/{uuid}_{原名}（对齐旧端 save_upload_file）
	name := uuid.NewString() + "_" + sanitizeName(fh.Filename)
	dir := filepath.Join(h.cfg.UploadDir, "evaluations", "temp")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		serverErr(c, "存储目录创建失败")
		return
	}
	dst := filepath.Join(dir, name)
	if err := saveUploaded(fh, dst); err != nil {
		serverErr(c, "文件保存失败")
		return
	}

	rel := "evaluations/temp/" + name
	response.OKMsg(c, "上传成功", gin.H{
		"url": "/api/v1/files/" + rel, "filename": fh.Filename, "size": fh.Size,
	})
}

// dimCodeRe 维度编码白名单（对齐旧端 sanitize_dim_code）
var dimCodeRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// UploadEvaluation 评教文件上传（多文件，对齐旧端 POST /upload/evaluation/{task_id}/{dim_code}）
func (h *Upload) UploadEvaluation(c *gin.Context) {
	taskID, err := parseIDParam(c, "task_id")
	if err != nil {
		invalidParam(c, "path", "task_id", c.Param("task_id"), "Input should be a valid integer, unable to parse string as an integer")
		return
	}
	dimCode := c.Param("dim_code")

	var dim model.EvaluationDimension
	if err := h.db.Where("code = ?", dimCode).First(&dim).Error; err != nil {
		response.Fail(c, http.StatusNotFound, "维度不存在")
		return
	}
	if dim.FieldType != model.FieldImage && dim.FieldType != model.FieldFile {
		badReq(c, "该维度不支持文件上传")
		return
	}
	isImage := dim.FieldType == model.FieldImage

	// 合并维度配置 max_count（field_config 覆盖默认）
	var fc struct {
		MaxCount int `json:"max_count"`
	}
	if len(dim.FieldConfig) > 0 {
		_ = json.Unmarshal(dim.FieldConfig, &fc)
	}
	maxCount := fc.MaxCount
	if maxCount == 0 {
		if isImage {
			maxCount = 9
		} else {
			maxCount = 5
		}
	}

	form, err := c.MultipartForm()
	if err != nil || form == nil || len(form.File["files"]) == 0 {
		missingQuery(c, "files")
		return
	}
	files := form.File["files"]
	if len(files) > maxCount {
		badReq(c, fmt.Sprintf("文件数量超过限制: %d > %d", len(files), maxCount))
		return
	}

	dir := filepath.Join(h.cfg.UploadDir, "evaluations", strconv.Itoa(taskID), dimCode)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		serverErr(c, "存储目录创建失败")
		return
	}

	// 先整体校验再落盘：任一文件校验失败时不留下已写入的半批文件
	for _, fh := range files {
		if err := validateUpload(fh, isImage); err != nil {
			badReq(c, err.Error())
			return
		}
	}

	uploaded := make([]gin.H, 0, len(files))
	written := make([]string, 0, len(files))
	fail := func() {
		// 清理本次请求已写入的文件，避免失败后残留孤儿文件
		for _, p := range written {
			_ = os.Remove(p)
		}
	}
	for _, fh := range files {
		name := uuid.NewString() + "_" + sanitizeName(fh.Filename)
		dst := filepath.Join(dir, name)
		if err := saveUploaded(fh, dst); err != nil {
			fail()
			serverErr(c, "文件保存失败")
			return
		}
		written = append(written, dst)
		rel := "evaluations/" + strconv.Itoa(taskID) + "/" + dimCode + "/" + name
		uploaded = append(uploaded, gin.H{
			"url": "/api/v1/files/" + rel, "filename": fh.Filename, "path": rel,
		})
	}
	response.OKMsg(c, "上传成功", gin.H{"files": uploaded, "count": len(uploaded)})
}

// DeleteEvaluationFile 删除评教上传文件（对齐旧端 DELETE /upload/evaluation/{task_id}/{dim_code}/{filename}）
func (h *Upload) DeleteEvaluationFile(c *gin.Context) {
	taskID, err := parseIDParam(c, "task_id")
	if err != nil {
		invalidParam(c, "path", "task_id", c.Param("task_id"), "Input should be a valid integer, unable to parse string as an integer")
		return
	}
	dimCode := c.Param("dim_code")
	if !dimCodeRe.MatchString(dimCode) {
		badReq(c, "无效的维度编码: "+dimCode)
		return
	}
	filename := sanitizeName(c.Param("filename"))
	target := filepath.Join(h.cfg.UploadDir, "evaluations", strconv.Itoa(taskID), dimCode, filename)

	// 权限：管理员或该任务评教记录提交者
	u := middleware.CurrentUser(c)
	if !u.HasAnyRole("system_admin", "college_admin", "school_admin") {
		var cnt int64
		h.db.Model(&model.EvaluationRecord{}).
			Where("task_id = ? AND evaluator_id = ? AND is_deleted = 0", taskID, u.ID).Count(&cnt)
		if cnt == 0 {
			forbidden(c, "无权删除此文件")
			return
		}
	}
	if _, err := os.Stat(target); err != nil {
		response.Fail(c, http.StatusNotFound, "文件不存在")
		return
	}
	if err := os.Remove(target); err != nil {
		serverErr(c, "删除失败")
		return
	}
	response.OKMsg(c, "删除成功", nil)
}

// saveUploaded 落盘上传文件。任何失败都删除目标文件：
// 半截文件既占用空间，又会被下载接口当作正常附件分发出去。
func saveUploaded(fh *multipart.FileHeader, dst string) (err error) {
	src, err := fh.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := out.Close(); err == nil && cerr != nil {
			err = cerr
		}
		if err != nil {
			_ = os.Remove(dst)
		}
	}()
	_, err = io.Copy(out, src)
	return err
}

// Serve 文件访问（防路径穿越）
func (h *Upload) Serve(c *gin.Context) {
	rel := strings.TrimPrefix(c.Param("filepath"), "/")
	baseAbs, err := filepath.Abs(h.cfg.UploadDir)
	if err != nil {
		serverErr(c, "服务异常")
		return
	}
	target, err := filepath.Abs(filepath.Join(baseAbs, filepath.Clean(rel)))
	if err != nil || !strings.HasPrefix(target, baseAbs+string(filepath.Separator)) {
		forbidden(c, "非法的文件路径")
		return
	}
	if _, err := os.Stat(target); err != nil {
		response.Fail(c, http.StatusNotFound, "文件不存在")
		return
	}
	c.Header("Content-Disposition", "attachment; filename=\""+filepath.Base(target)+"\"")
	c.File(target)
}
