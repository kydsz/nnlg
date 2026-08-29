package jwxt

import (
	"strings"
	"testing"
)

// buildScheduleHTML 构造最小可解析课表：表头 + 43 列节次行 + 教师行
func buildScheduleHTML(teacherRows []string) string {
	var b strings.Builder
	b.WriteString(`<html><body><table id="kbtable">`)
	b.WriteString(`<tr><td>教师</td><td>星期一</td><td>星期二</td></tr>`)
	b.WriteString(`<tr>`)
	for i := 0; i < 43; i++ {
		b.WriteString(`<td>0102</td>`)
	}
	b.WriteString(`</tr>`)
	for _, row := range teacherRows {
		b.WriteString(`<tr>`)
		b.WriteString(`<td>`)
		b.WriteString(row) // 教师名 &nbsp; 表示空
		b.WriteString(`</td>`)
		for i := 0; i < 43; i++ {
			b.WriteString(`<td>&nbsp;</td>`)
		}
		b.WriteString(`</tr>`)
	}
	b.WriteString(`</table></body></html>`)
	return b.String()
}

func buildScheduleWithCourse(teacher string, courseCells map[int]string) string {
	var b strings.Builder
	b.WriteString(`<html><body><table id="kbtable">`)
	b.WriteString(`<tr><td>教师</td><td>星期一</td></tr>`)
	b.WriteString(`<tr>`)
	for i := 0; i < 43; i++ {
		b.WriteString(`<td>0102</td>`)
	}
	b.WriteString(`</tr>`)
	b.WriteString(`<tr><td>` + teacher + `</td>`)
	for i := 1; i <= 43; i++ {
		if c, ok := courseCells[i]; ok {
			b.WriteString(`<td>` + c + `</td>`)
		} else {
			b.WriteString(`<td>&nbsp;</td>`)
		}
	}
	b.WriteString(`</tr></table></body></html>`)
	return b.String()
}

func TestParseScheduleHTMLErrors(t *testing.T) {
	if _, _, err := ParseScheduleHTML(`<html><body>无表格</body></html>`); err == nil {
		t.Fatal("缺少 kbtable 应报错")
	}
	// 行数不足
	if _, _, err := ParseScheduleHTML(`<table id="kbtable"><tr><td>a</td></tr></table>`); err == nil {
		t.Fatal("行数不足应报错")
	}
	// 表头列数不足
	badHeader := `<table id="kbtable"><tr><td>仅一列</td></tr><tr>` +
		strings.Repeat(`<td>x</td>`, 43) + `</tr><tr><td>张三</td></tr></table>`
	if _, _, err := ParseScheduleHTML(badHeader); err == nil || !strings.Contains(err.Error(), "表头") {
		t.Fatalf("表头结构错误应报错: %v", err)
	}
	// 节次行列数不足
	badSections := `<table id="kbtable"><tr><td>教师</td><td>星期一</td></tr>` +
		`<tr><td>x</td><td>y</td></tr><tr><td>张三</td></tr></table>`
	if _, _, err := ParseScheduleHTML(badSections); err == nil || !strings.Contains(err.Error(), "43") {
		t.Fatalf("节次列数错误应报错: %v", err)
	}
}

func TestParseScheduleHTMLBasicCourse(t *testing.T) {
	cell := `高等数学<br>计科1班(30)<br>(1-12周)<br>逸夫楼101`
	// cell 位置：第 1 天第 1 节 → 行内第 2 个 td（index 1）
	htmlStr := buildScheduleWithCourse("张三", map[int]string{1: cell})
	schedules, stats, err := ParseScheduleHTML(htmlStr)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(schedules) != 1 || schedules[0].TeacherName != "张三" {
		t.Fatalf("教师解析不符: %+v", schedules)
	}
	courses := schedules[0].Courses
	if len(courses) != 1 {
		t.Fatalf("应解析出 1 门课, 实际 %d: %+v", len(courses), courses)
	}
	c := courses[0]
	if c.CourseName != "高等数学" {
		t.Fatalf("课程名不符: %s", c.CourseName)
	}
	if c.ClassInfo != "计科1班(30)" {
		t.Fatalf("班级信息不符: %s", c.ClassInfo)
	}
	if c.WeekPattern != "1-12周" {
		t.Fatalf("周次不符: %s", c.WeekPattern)
	}
	if c.Classroom != "逸夫楼101" {
		t.Fatalf("教室不符: %s", c.Classroom)
	}
	if c.WeekDay != 1 || c.Section != "0102" {
		t.Fatalf("星期/节次不符: day=%d section=%s", c.WeekDay, c.Section)
	}
	if stats.TotalTeachers != 1 || stats.TotalCourses != 1 {
		t.Fatalf("统计不符: %+v", stats)
	}
	if stats.DayDist[1] != 1 || stats.SectionDist["0102"] != 1 {
		t.Fatalf("分布统计不符: %+v", stats)
	}
}

func TestParseScheduleHTMLCellPlacement(t *testing.T) {
	// 第 3 天第 2 节（行内 index = 1 + 2*6 + 1 = 14）→ WeekDay=3, Section="0304"
	cell := `数据结构<br>计科2班(28)<br>机房201`
	idx := 1 + 2*6 + 1
	htmlStr := buildScheduleWithCourse("李四", map[int]string{idx: cell})
	schedules, _, err := ParseScheduleHTML(htmlStr)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	c := schedules[0].Courses[0]
	if c.WeekDay != 3 || c.Section != "0304" {
		t.Fatalf("单元格定位不符: day=%d section=%s", c.WeekDay, c.Section)
	}
	// 无周次行 → WeekPattern 为空，最后一行作为教室
	if c.WeekPattern != "" || c.Classroom != "机房201" {
		t.Fatalf("周次/教室解析不符: %+v", c)
	}
}

func TestParseScheduleHTMLMultiCourseCell(t *testing.T) {
	cell := `高等数学<br>计科1班(30)<br><br>线性代数<br>计科2班(28)`
	htmlStr := buildScheduleWithCourse("王五", map[int]string{1: cell})
	schedules, stats, err := ParseScheduleHTML(htmlStr)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(schedules[0].Courses) != 2 {
		t.Fatalf("同格双课应拆为 2 门, 实际 %+v", schedules[0].Courses)
	}
	if schedules[0].Courses[0].CourseName != "高等数学" ||
		schedules[0].Courses[1].CourseName != "线性代数" {
		t.Fatalf("拆分顺序不符: %+v", schedules[0].Courses)
	}
	if stats.TotalCourses != 2 {
		t.Fatalf("统计不符: %+v", stats)
	}
}

func TestParseScheduleHTMLSkipsEmpty(t *testing.T) {
	// 空名教师行（&nbsp;）应跳过；单行课程块（缺班级行）应丢弃
	htmlStr := buildScheduleHTML([]string{"&nbsp;"})
	schedules, stats, err := ParseScheduleHTML(htmlStr)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(schedules) != 0 || stats.TotalTeachers != 0 {
		t.Fatalf("空教师应跳过: %+v", schedules)
	}

	onlyName := buildScheduleWithCourse("赵六", map[int]string{1: `只有课名`})
	schedules2, _, err := ParseScheduleHTML(onlyName)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	// 教师行保留，但课程块因少于 2 行被丢弃
	if len(schedules2) != 1 || len(schedules2[0].Courses) != 0 {
		t.Fatalf("单行课程块应被丢弃: %+v", schedules2)
	}
}

func TestExtractStudentCount(t *testing.T) {
	cases := []struct {
		in    string
		want  int
		found bool
	}{
		{"一班(30),二班(45)", 45, true},  // 多班级取最后一个
		{"计科(2)班", 2, true},           // 纯数字括号
		{"软工1班(30人)", 0, false},      // 括号内非纯数字不匹配
		{"无括号", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		got, ok := ExtractStudentCount(c.in)
		if ok != c.found || got != c.want {
			t.Fatalf("ExtractStudentCount(%q) = (%d,%v), 期望 (%d,%v)", c.in, got, ok, c.want, c.found)
		}
	}
}

func TestParseCourseBlockTrimsTags(t *testing.T) {
	// 块内含 <b>/<span> 等 HTML 标签应剥离，&nbsp; 转空格
	block := `<b>大学英语</b><br/><span>外语1班&nbsp;(40)</span>`
	c := parseCourseBlock(block, 2, "0304")
	if c == nil {
		t.Fatal("应解析出课程")
	}
	if c.CourseName != "大学英语" {
		t.Fatalf("课程名应剥离标签: %q", c.CourseName)
	}
	if c.ClassInfo != "外语1班 (40)" {
		t.Fatalf("班级信息不符: %q", c.ClassInfo)
	}
	if c.WeekDay != 2 || c.Section != "0304" {
		t.Fatalf("星期/节次不符: %+v", c)
	}
}
