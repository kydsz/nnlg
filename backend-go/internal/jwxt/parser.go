package jwxt

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// CourseInfo 课程信息
type CourseInfo struct {
	CourseName   string
	ClassInfo    string
	WeekPattern  string
	WeekDay      int
	Section      string
	Classroom    string
	RawData      string
	StudentCount *int // 不入库，仅用于构建 class_info 的人数后缀
}

// TeacherSchedule 教师课程表
type TeacherSchedule struct {
	TeacherName string
	Courses     []CourseInfo
	TeacherID   int // 已知教师 ID 时直接使用（按工号同步场景），0 表示未知
}

// ParseStats 解析统计
type ParseStats struct {
	TotalTeachers int            `json:"total_teachers"`
	TotalCourses  int            `json:"total_courses"`
	DayDist       map[int]int    `json:"day_distribution"`
	SectionDist   map[string]int `json:"section_distribution"`
}

var sections = []string{"0102", "0304", "0506", "0708", "0910", "1112"}

var weekPatternRe = regexp.MustCompile(`\((\d+(?:-\d+)?(?:,\d+(?:-\d+)?)*周)\)`)

// ParseScheduleHTML 解析课程表 HTML（table#kbtable，7 天 × 6 节次）
func ParseScheduleHTML(content string) ([]TeacherSchedule, ParseStats, error) {
	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return nil, ParseStats{}, err
	}
	table := findTableByID(doc, "kbtable")
	if table == nil {
		return nil, ParseStats{}, fmt.Errorf("未找到课程表表格 (id='kbtable')")
	}

	var rows [][]*html.Node
	for _, tr := range collectTags(table, "tr") {
		var cells []*html.Node
		for td := firstTag(tr.FirstChild, "td"); td != nil; td = nextTag(td, "td") {
			cells = append(cells, td)
		}
		rows = append(rows, cells)
	}
	if len(rows) < 3 {
		return nil, ParseStats{}, fmt.Errorf("表格行数不足，无法解析")
	}
	if len(rows[0]) < 2 {
		return nil, ParseStats{}, fmt.Errorf("表头结构不正确")
	}
	if len(rows[1]) < 43 {
		return nil, ParseStats{}, fmt.Errorf("节次列数不正确，期望43列，实际%d列", len(rows[1]))
	}

	var schedules []TeacherSchedule
	for _, cells := range rows[2:] {
		if len(cells) < 2 {
			continue
		}
		teacherName := strings.TrimSpace(textOf(cells[0]))
		if teacherName == "" || teacherName == "&nbsp;" {
			continue
		}
		ts := TeacherSchedule{TeacherName: teacherName}
		for day := 0; day < 7; day++ {
			for sec := 0; sec < 6; sec++ {
				idx := 1 + day*6 + sec
				if idx >= len(cells) {
					continue
				}
				cellHTML := innerHTML(cells[idx])
				if strings.Contains(cellHTML, "&nbsp;") && strings.TrimSpace(textOf(cells[idx])) == "" {
					continue
				}
				ts.Courses = append(ts.Courses, parseCell(cellHTML, day+1, sections[sec])...)
			}
		}
		schedules = append(schedules, ts)
	}

	stats := ParseStats{TotalTeachers: len(schedules), DayDist: map[int]int{}, SectionDist: map[string]int{}}
	for _, s := range schedules {
		for _, c := range s.Courses {
			stats.TotalCourses++
			stats.DayDist[c.WeekDay]++
			stats.SectionDist[c.Section]++
		}
	}
	return schedules, stats, nil
}

// parseCell 解析单元格（可能含多门课程，用 <br><br> 分隔）
func parseCell(cellHTML string, weekDay int, section string) []CourseInfo {
	htmlContent := regexp.MustCompile(`(?i)</?td[^>]*>`).ReplaceAllString(cellHTML, "")
	// html.Render 会把 <br> 规范化为 <br/>，需同时兼容两种写法
	blocks := regexp.MustCompile(`(?i)<br\s*/?>\s*<br\s*/?>`).Split(htmlContent, -1)

	var courses []CourseInfo
	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" || block == "&nbsp;" {
			continue
		}
		if c := parseCourseBlock(block, weekDay, section); c != nil {
			courses = append(courses, *c)
		}
	}
	return courses
}

// parseCourseBlock 解析单个课程块
func parseCourseBlock(block string, weekDay int, section string) *CourseInfo {
	text := regexp.MustCompile(`(?i)<br\s*/?>`).ReplaceAllString(block, "\n")
	text = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(text, "")
	text = strings.ReplaceAll(text, "&nbsp;", " ")

	var lines []string
	for _, line := range strings.Split(text, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) < 2 {
		return nil
	}

	course := &CourseInfo{
		CourseName: lines[0],
		ClassInfo:  lines[1],
		WeekDay:    weekDay,
		Section:    section,
	}
	for _, line := range lines[2:] {
		if m := weekPatternRe.FindStringSubmatch(line); m != nil {
			course.WeekPattern = m[1]
			continue
		}
		if line != "" && !strings.HasPrefix(line, "(") {
			course.Classroom = line
			break
		}
	}
	course.RawData = text
	return course
}

// ExtractStudentCount 从班级信息提取学生人数（取最后一个括号数字）
func ExtractStudentCount(classInfo string) (int, bool) {
	if classInfo == "" {
		return 0, false
	}
	matches := regexp.MustCompile(`\((\d+)\)`).FindAllStringSubmatch(classInfo, -1)
	if len(matches) == 0 {
		return 0, false
	}
	n, err := strconv.Atoi(matches[len(matches)-1][1])
	if err != nil {
		return 0, false
	}
	return n, true
}

// ---------- HTML 遍历辅助 ----------

func findTableByID(n *html.Node, id string) *html.Node {
	if n.Type == html.ElementNode && n.Data == "table" && attrOf(n, "id") == id {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findTableByID(c, id); found != nil {
			return found
		}
	}
	return nil
}

func firstTag(n *html.Node, tag string) *html.Node {
	for c := n; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == tag {
			return c
		}
	}
	return nil
}

// collectTags 按文档序递归收集指定标签（HTML5 解析会插入 tbody 等隐式节点）
func collectTags(root *html.Node, tag string) []*html.Node {
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == tag {
			out = append(out, n)
			return // tr 内部不再嵌套 tr
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	if root.Type == html.ElementNode && root.Data == tag {
		out = append(out, root)
		return out
	}
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		walk(c)
	}
	return out
}

func nextTag(n *html.Node, tag string) *html.Node {
	for c := n.NextSibling; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == tag {
			return c
		}
	}
	return nil
}

func attrOf(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// textOf 提取节点纯文本
func textOf(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(cur *html.Node) {
		if cur.Type == html.TextNode {
			b.WriteString(cur.Data)
		}
		for c := cur.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

// innerHTML 渲染节点内部 HTML
func innerHTML(n *html.Node) string {
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		_ = html.Render(&b, c)
	}
	return b.String()
}
