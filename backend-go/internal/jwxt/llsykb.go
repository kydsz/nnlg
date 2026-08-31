package jwxt

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"backend-go/internal/model"

	"golang.org/x/net/html"
	"gorm.io/gorm"
)

// LlsykbRecord llsykb 解析出的单条课程记录
type LlsykbRecord struct {
	Weekday      int
	Section      string
	CourseName   string
	CourseType   string
	Credits      int
	Teacher      string
	Classes      string
	StudentCount int
	WeekPattern  string
	DayType      string
	SectionRange string
	Classroom    string
	Raw          string
}

var sectionMap = map[string]string{
	"第一大节": "0102",
	"第二大节": "0304",
	"第三大节": "0506",
	"第四大节": "0708",
}

// QueryLlsykb 查询教师个人课表（llsykb_kb.jsp）
func (b *BaseSync) QueryLlsykb(semester, teacherID string) (string, error) {
	refererURL := fmt.Sprintf("%s/jiaowu/pkgl/llsykb/llsykb_find_jg0101.jsp?xnxq01id=%s&init=1&isview=1",
		b.Auth.BaseURLGL, url.QueryEscape(semester))
	if _, err := b.Auth.GetText(refererURL, 30*time.Second); err != nil {
		return "", fmt.Errorf("访问 Referer 页面失败: %w", err)
	}

	form := url.Values{
		"type":         {"jg0101"},
		"isview":       {"1"},
		"zc":           {""},
		"yxx":          {""},
		"teacherID":    {teacherID},
		"teacherIDmc":  {""},
		"jg0101id":     {teacherID},
		"jg0101mc":     {""},
		"jszc":         {""},
		"sfFD":         {"1"},
		"sfBZ":         {"1"},
		"xsfl":         {"1"},
		"xnxq01id":     {semester},
	}
	llsykbURL := b.Auth.BaseURLGL + "/jiaowu/pkgl/llsykb/llsykb_kb.jsp"
	return b.Auth.PostFormText(llsykbURL, form, refererURL, 60*time.Second)
}

var courseHeadRe = regexp.MustCompile(`^(.+?)\[(\d+)\]\[(.+?)\]`)
var weekDayTypeRe = regexp.MustCompile(`(\d+(?:-\d+)?(?:,\d+(?:-\d+)?)*)\((全部|单周|双周)\)`)
var sectionRangeRe = regexp.MustCompile(`\[(\d+-\d+节)\]`)
var studentCountRe2 = regexp.MustCompile(`\((\d+)\)`)

// ParseLlsykbHTML 解析 llsykb_kb.jsp 返回的课表 HTML
func ParseLlsykbHTML(content string) []LlsykbRecord {
	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return nil
	}
	table := findTableByID(doc, "kbtable")
	if table == nil {
		return nil
	}

	var results []LlsykbRecord
	rows := collectTags(table, "tr")
	if len(rows) < 2 {
		return nil
	}
	for _, row := range rows[1:] {
		var cells []*html.Node
		for td := firstTag(row.FirstChild, "td"); td != nil; td = nextTag(td, "td") {
			cells = append(cells, td)
		}
		if len(cells) < 2 {
			continue
		}
		sectionText := strings.ReplaceAll(textOf(cells[0]), "\u00a0", "")
		sectionText = strings.TrimSpace(sectionText)
		section, ok := sectionMap[sectionText]
		if !ok {
			continue
		}
		for dayIdx := 0; dayIdx < 7; dayIdx++ {
			cellIdx := dayIdx + 1
			if cellIdx >= len(cells) {
				break
			}
			for _, div := range collectByClass(cells[cellIdx], "kbcontent") {
				divHTML := innerHTML(div)
				if !strings.Contains(divHTML, "老师") {
					continue
				}
				for _, seg := range strings.Split(divHTML, "---") {
					seg = strings.TrimSpace(seg)
					seg = strings.TrimRight(seg, "-")
					seg = strings.TrimSpace(seg)
					if seg == "" || !strings.Contains(seg, "老师") {
						continue
					}
					if rec := parseSingleCourse(seg); rec != nil {
						rec.Weekday = dayIdx + 1
						rec.Section = section
						results = append(results, *rec)
					}
				}
			}
		}
	}
	return results
}

// parseSingleCourse 解析单门课程 HTML 片段
func parseSingleCourse(fragment string) *LlsykbRecord {
	node, err := html.Parse(strings.NewReader(fragment))
	if err != nil {
		return nil
	}
	firstText := firstMeaningfulText(node)
	if firstText == "" {
		return nil
	}
	m := courseHeadRe.FindStringSubmatch(firstText)
	if m == nil {
		return nil
	}
	credits, _ := strconv.Atoi(m[2])
	rec := &LlsykbRecord{
		CourseName: strings.TrimSpace(m[1]),
		Credits:    credits,
		CourseType: strings.TrimSpace(m[3]),
	}

	if f := findFontByTitle(node, "老师"); f != nil {
		rec.Teacher = strings.TrimSpace(textOf(f))
	}
	if f := findFontByTitle(node, "班级"); f != nil {
		for _, span := range collectTags(f, "span") {
			t := attrOf(span, "title")
			if t != "" && t != "选课人数" {
				rec.Classes = t
				break
			}
		}
		for _, span := range collectTags(f, "span") {
			if attrOf(span, "title") == "选课人数" {
				if cm := studentCountRe2.FindStringSubmatch(textOf(span)); cm != nil {
					rec.StudentCount, _ = strconv.Atoi(cm[1])
				}
			}
		}
		text := textOf(f)
		if wm := weekDayTypeRe.FindStringSubmatch(text); wm != nil {
			rec.WeekPattern = wm[1]
			rec.DayType = wm[2]
		}
		if sm := sectionRangeRe.FindStringSubmatch(text); sm != nil {
			rec.SectionRange = sm[1]
		}
	}
	if f := findFontByTitle(node, "上课地点"); f != nil {
		rec.Classroom = strings.TrimSpace(textOf(f))
	}
	raw := strings.TrimSpace(textOf(node))
	raw = strings.Join(strings.Fields(raw), " ")
	if len(raw) > 200 {
		raw = raw[:200]
	}
	rec.Raw = raw
	return rec
}

// collectByClass 递归收集 class 属性包含 cls 的节点
func collectByClass(root *html.Node, cls string) []*html.Node {
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && strings.Contains(attrOf(n, "class"), cls) {
			out = append(out, n)
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		walk(c)
	}
	return out
}

// findFontByTitle 查找 font[title=...] 节点
func findFontByTitle(root *html.Node, title string) *html.Node {
	var find func(*html.Node) *html.Node
	find = func(n *html.Node) *html.Node {
		if n.Type == html.ElementNode && n.Data == "font" && attrOf(n, "title") == title {
			return n
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if found := find(c); found != nil {
				return found
			}
		}
		return nil
	}
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		if found := find(c); found != nil {
			return found
		}
	}
	return nil
}

// firstMeaningfulText 首个非空文本（文本节点或子元素文本）
func firstMeaningfulText(root *html.Node) string {
	var find func(*html.Node) string
	find = func(n *html.Node) string {
		if n.Type == html.TextNode {
			if s := strings.TrimSpace(n.Data); s != "" {
				return s
			}
			return ""
		}
		if n.FirstChild != nil {
			if s := strings.TrimSpace(textOf(n)); s != "" {
				return s
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if s := find(c); s != "" {
				return s
			}
		}
		return ""
	}
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		if s := find(c); s != "" {
			return s
		}
	}
	return ""
}

// SyncByTeacherNos 按工号列表同步课表（llsykb，逐人查询 + 分批入库）
//
// 说明：llsykb_kb.jsp 逐人按工号查询，遍历真实用户表、TeacherID 直接绑定用户，
// 可覆盖无课教师、无姓名匹配歧义、可逐人回推进度（回调 cb）。推荐作为课表同步主入口。
// 按学院批量同步（/sync/llsykb/batch）即经由本函数实现。
func (b *BaseSync) SyncByTeacherNos(db *gorm.DB, semester string, teacherNos []string,
	cb func(no, name string, ok bool, reason string)) map[string]interface{} {

	// 工号 -> 系统用户绑定（不限角色）
	type tinfo struct {
		id       int
		username string
	}
	teacherMap := map[string]tinfo{}
	for _, tid := range teacherNos {
		var u model.User
		if err := db.Where("user_no = ? OR username = ?", tid, tid).First(&u).Error; err == nil {
			teacherMap[tid] = tinfo{id: u.ID, username: u.Username}
		} else {
			teacherMap[tid] = tinfo{username: tid}
		}
	}

	stats := map[string]interface{}{
		"total_teachers":         0,
		"new_teachers":           0,
		"updated_teachers":       0,
		"unchanged_teachers":     0,
		"unmatched_teachers":     0,
		"skipped_empty_teachers": 0,
		"total_courses":          0,
		"version":                nil,
	}
	inc := func(key string, n int) {
		stats[key] = stats[key].(int) + n
	}
	if v, ok := stats["version"]; ok {
		_ = v
	}

	const flushEvery = 50
	var pending []TeacherSchedule
	savedBatches := 0
	allRecords := 0

	flush := func() {
		if len(pending) == 0 {
			return
		}
		batch := pending
		pending = nil
		saveStats, err := SaveSchedules(db, batch, semester, time.Now(), 0)
		if err != nil {
			return
		}
		savedBatches++
		inc("total_teachers", saveStats["total_teachers"].(int))
		inc("new_teachers", saveStats["new_teachers"].(int))
		inc("updated_teachers", saveStats["updated_teachers"].(int))
		inc("unchanged_teachers", saveStats["unchanged_teachers"].(int))
		inc("unmatched_teachers", saveStats["unmatched_teachers"].(int))
		inc("skipped_empty_teachers", saveStats["skipped_empty_teachers"].(int))
		inc("total_courses", saveStats["total_courses"].(int))
		stats["version"] = saveStats["version"]
	}

	for _, tid := range teacherNos {
		info := teacherMap[tid]
		if cb != nil {
			cb(tid, info.username, true, "")
		}
		htmlContent, err := b.QueryLlsykb(semester, tid)
		if err != nil {
			if cb != nil {
				cb(tid, info.username, false, err.Error())
			}
			// 会话可能过期，重新登录一次
			_ = b.Auth.Relogin()
			continue
		}
		records := ParseLlsykbHTML(htmlContent)
		allRecords += len(records)

		ts := TeacherSchedule{TeacherName: info.username, TeacherID: info.id}
		for _, r := range records {
			wp := r.WeekPattern
			if wp != "" && !strings.Contains(wp, "周") {
				wp += "周"
			}
			classes := r.Classes
			if r.StudentCount > 0 && !strings.Contains(classes, fmt.Sprintf("(%d)", r.StudentCount)) {
				classes = fmt.Sprintf("%s(%d)", classes, r.StudentCount)
			}
			var sc *int
			if r.StudentCount > 0 {
				v := r.StudentCount
				sc = &v
			}
			ts.Courses = append(ts.Courses, CourseInfo{
				CourseName: r.CourseName, ClassInfo: classes, WeekPattern: wp,
				WeekDay: r.Weekday, Section: r.Section, Classroom: r.Classroom,
				RawData: r.Raw, StudentCount: sc,
			})
		}
		pending = append(pending, ts)
		if cb != nil {
			cb(tid, info.username, true, "success")
		}
		if len(pending) >= flushEvery {
			flush()
		}
	}
	flush()

	return map[string]interface{}{
		"semester": semester,
		"records":  allRecords,
		"teachers": map[string]interface{}{
			"total":     stats["total_teachers"],
			"new":       stats["new_teachers"],
			"updated":   stats["updated_teachers"],
			"unchanged": stats["unchanged_teachers"],
			"unmatched": stats["unmatched_teachers"],
		},
		"courses": map[string]interface{}{
			"total":    stats["total_courses"],
			"records":  allRecords,
			"semester": semester,
			"version":  stats["version"],
		},
		"stats":         stats,
		"saved_batches": savedBatches,
	}
}
