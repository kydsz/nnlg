package jwxt

import (
	"fmt"
	"html"
	"log"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// BaseSync 同步基类：打印参数提取 + Excel 下载
type BaseSync struct {
	Auth *Auth
}

// GetPrintParams 从列表页提取隐藏字段并附加打印控制参数
func (b *BaseSync) GetPrintParams(listURL, tableName string) (url.Values, error) {
	body, err := b.Auth.GetText(listURL, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("获取打印参数页面失败: %w", err)
	}
	params := extractHiddenInputs(body)

	params.Set("r_printfw", "allpages")
	params.Set("dyfs", "xlsserver")
	params.Set("TblFontName", "宋体")
	params.Set("TblFontSize", "12")
	params.Set("FldFontName", "宋体")
	params.Set("FldFontSize", "12")
	params.Set("CellFontName", "宋体")
	params.Set("CellFontSize", "10")
	params.Set("c_tblhead", "true")
	params.Set("TblName2", tableName)
	params.Set("c_bblk", "true")
	params.Set("Bblk", "南宁理工学院")
	params.Set("c_tbldate", "true")
	params.Set("tbldate", time.Now().Format("2006年01月02日"))
	log.Printf("[jwxt] 打印参数(%s): %v", tableName, params)
	return params, nil
}

// DownloadExcel 通过 PublicListPrintServlet 下载 Excel
func (b *BaseSync) DownloadExcel(params url.Values, refererURL string) ([]byte, error) {
	printURL := b.Auth.BaseURLGL + "/PublicListPrintServlet"
	data, err := b.Auth.PostForm(printURL, params, refererURL, 60*time.Second)
	if err != nil {
		log.Printf("[jwxt] 下载 Excel 失败: %v | 响应片段: %s", err, truncate(string(data), 1500))
		return nil, err
	}
	if len(data) < 1000 {
		return nil, fmt.Errorf("下载 Excel 内容过小: %d 字节", len(data))
	}
	log.Printf("[jwxt] 下载 Excel 成功: %d 字节", len(data))
	return data, nil
}

var hiddenInputRe = regexp.MustCompile(`(?is)<input[^>]*type=["']?hidden["']?[^>]*>`)

// extractHiddenInputs 提取页面所有 hidden input 的 name/value（含 HTML 实体反转义；
// 注意页面属性写法为 name = "xxx" 等号两侧可能带空格）
func extractHiddenInputs(page string) url.Values {
	values := url.Values{}
	tagRe := regexp.MustCompile(`(?i)name\s*=\s*["']?([^"'\s>]+)["']?`)
	valRe := regexp.MustCompile(`(?i)value\s*=\s*("([^"]*)"|'([^']*)'|([^\s>]*))`)
	for _, tag := range hiddenInputRe.FindAllString(page, -1) {
		name := ""
		if m := tagRe.FindStringSubmatch(tag); m != nil {
			name = strings.TrimSpace(html.UnescapeString(m[1]))
		}
		if name == "" {
			continue // 与 Python 版一致：无 name 的 input 不提交
		}
		value := ""
		if m := valRe.FindStringSubmatch(tag); m != nil {
			switch {
			case m[2] != "":
				value = m[2]
			case m[3] != "":
				value = m[3]
			default:
				value = m[4]
			}
		}
		values.Set(name, html.UnescapeString(value))
	}
	return values
}
