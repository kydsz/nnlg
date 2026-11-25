package service

import (
	"encoding/json"
	"testing"
	"time"

	"backend-go/internal/model"
)

func TestRound2(t *testing.T) {
	cases := []struct {
		in   float64
		want float64
	}{
		{88.555, 88.56},
		{88.554, 88.55},
		{0, 0},
		{100.0, 100},
		{66.666, 66.67},
	}
	for _, c := range cases {
		if got := round2(c.in); got != c.want {
			t.Fatalf("round2(%v) = %v, 期望 %v", c.in, got, c.want)
		}
	}
}

func TestSemesterRange(t *testing.T) {
	t.Run("第一学期", func(t *testing.T) {
		start, end := semesterRange("2026-2027-1")
		if start == nil || end == nil {
			t.Fatal("第一学期应返回非空日期")
		}
		wantS := time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local)
		wantE := time.Date(2027, 2, 1, 0, 0, 0, 0, time.Local)
		if !start.Equal(wantS) || !end.Equal(wantE) {
			t.Fatalf("第一学期范围 = %v ~ %v, 期望 %v ~ %v", start, end, wantS, wantE)
		}
	})
	t.Run("第二学期", func(t *testing.T) {
		start, end := semesterRange("2025-2026-2")
		wantS := time.Date(2026, 2, 1, 0, 0, 0, 0, time.Local)
		wantE := time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local)
		if !start.Equal(wantS) || !end.Equal(wantE) {
			t.Fatalf("第二学期范围 = %v ~ %v, 期望 %v ~ %v", start, end, wantS, wantE)
		}
	})
	t.Run("非法格式", func(t *testing.T) {
		if start, end := semesterRange("bad"); start != nil || end != nil {
			t.Fatalf("非法学期应返回 nil, 得到 %v ~ %v", start, end)
		}
	})
}

func TestRecordsAvgScore(t *testing.T) {
	scoreCodes := map[string]bool{"score1": true, "score2": true}

	t.Run("空记录返回0", func(t *testing.T) {
		if got := recordsAvgScore(nil, scoreCodes); got != 0 {
			t.Fatalf("空记录平均分应为 0, 得到 %v", got)
		}
	})

	t.Run("仅统计score维度", func(t *testing.T) {
		recs := []model.EvaluationRecord{
			{DimensionValues: json.RawMessage(`{"score1":5,"score2":3,"text1":99}`)},
		}
		if got := recordsAvgScore(recs, scoreCodes); got != 8 {
			t.Fatalf("平均分应为 8, 得到 %v", got)
		}
	})

	t.Run("多条记录取均值", func(t *testing.T) {
		recs := []model.EvaluationRecord{
			{DimensionValues: json.RawMessage(`{"score1":5,"score2":5}`)}, // 10
			{DimensionValues: json.RawMessage(`{"score1":4,"score2":3}`)}, // 7
		}
		// (10+7)/2 = 8.5
		if got := recordsAvgScore(recs, scoreCodes); got != 8.5 {
			t.Fatalf("平均分应为 8.5, 得到 %v", got)
		}
	})

	t.Run("非法JSON跳过", func(t *testing.T) {
		recs := []model.EvaluationRecord{
			{DimensionValues: json.RawMessage(`not-json`)},
			{DimensionValues: json.RawMessage(`{"score1":10}`)},
		}
		if got := recordsAvgScore(recs, scoreCodes); got != 10 {
			t.Fatalf("平均分应为 10, 得到 %v", got)
		}
	})
}

// TestAttendanceRateOf 锁定评教记录批量导出的「出勤率」取值口径：
// 记录里存了 0-100 内的 attendance_rate 就以记录为准（显式 0 也算填过，不被现算覆盖）；
// 缺失或越界（>100、<0）则按「实到 ÷ 应到 × 100」现算并保留一位小数；
// 两边都拿不到时留空，不臆造 0%。
// 背景：历史记录/PC 端/督导端提交的作答里往往没有 attendance_rate 键，
// 只看记录里存的值会让导出整列恒空；旧端存下的越界值则会直接造成 >100% 的导出结果。
func TestAttendanceRateOf(t *testing.T) {
	cases := []struct {
		name       string
		values     map[string]interface{}
		schedCount int
		hasSched   bool
		want       interface{}
	}{
		{
			"记录里有值以记录为准",
			map[string]interface{}{"attendance_rate": 88.5, "actual_count": 40.0, "expected_count": 50.0},
			0, false, 88.5,
		},
		{
			"显式的0不被现算覆盖",
			map[string]interface{}{"attendance_rate": 0.0, "actual_count": 40.0, "expected_count": 50.0},
			0, false, 0.0,
		},
		{
			"存量超100改为按实到÷应到重算",
			map[string]interface{}{"attendance_rate": 120.0, "actual_count": 81.0, "expected_count": 83.0},
			0, false, 97.6,
		},
		{
			"存量负数改为按实到÷应到重算",
			map[string]interface{}{"attendance_rate": -5.0, "actual_count": 40.0, "expected_count": 50.0},
			0, false, 80.0,
		},
		{
			"存量超100且无应到时回退快照人数",
			map[string]interface{}{"attendance_rate": 176.1, "actual_count": 81.0},
			83, true, 97.6,
		},
		{
			"存量超100且无法重算时留空",
			map[string]interface{}{"attendance_rate": 120.0},
			0, false, "",
		},
		{
			"缺记录值按实到÷应到现算",
			map[string]interface{}{"actual_count": 45.0, "expected_count": 50.0},
			0, false, 90.0,
		},
		{
			"现算四舍五入到一位小数",
			map[string]interface{}{"actual_count": 2.0, "expected_count": 3.0},
			0, false, 66.7,
		},
		{
			"无应到时回退课表上课人数",
			map[string]interface{}{"actual_count": 30.0},
			50, true, 60.0,
		},
		{
			"应到为0时回退课表上课人数",
			map[string]interface{}{"actual_count": 30.0, "expected_count": 0.0},
			60, true, 50.0,
		},
		{
			"缺实到时留空",
			map[string]interface{}{"expected_count": 50.0},
			50, true, "",
		},
		{
			"既无应到也无课表人数时留空",
			map[string]interface{}{"actual_count": 30.0},
			0, false, "",
		},
		{
			"应到为0且无课表人数时留空",
			map[string]interface{}{"actual_count": 30.0, "expected_count": 0.0},
			0, false, "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := attendanceRateOf(c.values, c.schedCount, c.hasSched); got != c.want {
				t.Fatalf("attendanceRateOf() = %#v, 期望 %#v", got, c.want)
			}
		})
	}
}

// TestTaskStudentCount 锁定导出「应到人数」取值顺序：
// 任务快照优先（合班总人数），快照缺失才回退课表匹配；
// 合班拆行的多条明细在索引里已合并为「一堂课」（人数累加），同课程多开课次按上课时间定位堂次。
func TestTaskStudentCount(t *testing.T) {
	wdMon, wdWed := int8(1), int8(3)
	idx := &studentCountIndex{
		schedulesByTeacher: map[int][]model.CourseSchedule{
			89: {{Model: model.Model{ID: 15056}, TeacherID: intPtr(89)}},
		},
		mergedBySchedule: map[int][]map[string]interface{}{
			15056: {
				// 周一第1-2节的合班课（2401班46人 + 2402班37人），已合并为 83
				{"course_name": "Python数据分析基础", "class_info": "2401班(46),2402班(37)",
					"student_count": intPtr(83), "week_day": &wdMon, "section": "0102"},
				// 同课程另开课次（周三第5-6节）
				{"course_name": "Python数据分析基础", "class_info": "2403班(40)",
					"student_count": intPtr(40), "week_day": &wdWed, "section": "0506"},
			},
		},
	}

	withSnapshot := model.EvaluationTask{
		TeacherID:        89,
		CourseName:       "Python数据分析基础",
		ScheduleSnapshot: json.RawMessage(`{"class_info":"2401班,2402班(83)","student_count":83}`),
	}
	if got, ok := taskStudentCount(&withSnapshot, idx); !ok || got != 83 {
		t.Fatalf("有快照时应取快照 83, 得到 %d (ok=%v)", got, ok)
	}

	// 无快照回退：按上课时间（周三 14:00 -> 第5-6节）定位到对应堂次，而非取首条 83
	wedClass := model.EvaluationTask{
		TeacherID: 89, CourseName: "Python数据分析基础",
		ClassTime: model.LocalTimePtr(time.Date(2026, 11, 4, 14, 0, 0, 0, time.Local)),
	}
	if got, ok := taskStudentCount(&wedClass, idx); !ok || got != 40 {
		t.Fatalf("应回退到上课时间对应堂次 40, 得到 %d (ok=%v)", got, ok)
	}

	// 无上课时间信息退回首条
	noSnapshot := model.EvaluationTask{TeacherID: 89, CourseName: "Python数据分析基础"}
	if got, ok := taskStudentCount(&noSnapshot, idx); !ok || got != 83 {
		t.Fatalf("无时间信息应回退首条 83, 得到 %d (ok=%v)", got, ok)
	}

	// 快照 student_count 为空时也回退课表
	nullSnapshot := model.EvaluationTask{
		TeacherID:        89,
		CourseName:       "Python数据分析基础",
		ScheduleSnapshot: json.RawMessage(`{"student_count":null}`),
	}
	if got, ok := taskStudentCount(&nullSnapshot, idx); !ok || got != 83 {
		t.Fatalf("快照人数为空时应回退课表 83, 得到 %d (ok=%v)", got, ok)
	}

	// 课表里课程名匹配不上时不再回退首条明细，避免拿到别的课程人数
	unknown := model.EvaluationTask{TeacherID: 89, CourseName: "不存在的课程"}
	if got, ok := taskStudentCount(&unknown, idx); ok {
		t.Fatalf("课程名匹配不上时应返回空, 得到 %d", got)
	}
}

func TestNumOf(t *testing.T) {
	f := 2.5
	cases := []struct {
		name string
		in   interface{}
		want float64
		ok   bool
	}{
		{"float64", float64(3.5), 3.5, true},
		{"int", 3, 3, true},
		{"int64", int64(4), 4, true},
		{"float指针", &f, 2.5, true},
		{"nil", nil, 0, false},
		{"字符串", "abc", 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := numOf(c.in)
			if ok != c.ok || got != c.want {
				t.Fatalf("numOf(%v) = (%v, %v), 期望 (%v, %v)", c.in, got, ok, c.want, c.ok)
			}
		})
	}
}

// TestRecordsAvgScoreZeroScorePolicy 锁定「0 分记录」统计口径（见 recordsAvgScore 注释）：
// 单维度 0 分计入该记录合计；整条记录合计为 0 时整体跳过（不计入分子，也不计入分母）。
func TestRecordsAvgScoreZeroScorePolicy(t *testing.T) {
	scoreCodes := map[string]bool{"score1": true, "score2": true}
	rec := func(raw string) model.EvaluationRecord {
		return model.EvaluationRecord{DimensionValues: json.RawMessage(raw)}
	}

	t.Run("单维度0分计入合计", func(t *testing.T) {
		recs := []model.EvaluationRecord{
			rec(`{"score1":80,"score2":20}`),
			rec(`{"score1":0,"score2":60}`),
		}
		// (100 + 60) / 2 = 80：0 分是有效作答，会拉低该条合计
		if got := recordsAvgScore(recs, scoreCodes); got != 80 {
			t.Fatalf("0 分维度应计入合计（期望 80）, 得到 %v", got)
		}
	})

	t.Run("整条0分跳过", func(t *testing.T) {
		recs := []model.EvaluationRecord{
			rec(`{"score1":0,"score2":0}`),
			rec(`{"score1":90}`),
		}
		if got := recordsAvgScore(recs, scoreCodes); got != 90 {
			t.Fatalf("全 0 分记录应被跳过（期望 90）, 得到 %v", got)
		}
	})

	t.Run("全部0分返回0", func(t *testing.T) {
		recs := []model.EvaluationRecord{rec(`{"score1":0,"score2":0}`)}
		if got := recordsAvgScore(recs, scoreCodes); got != 0 {
			t.Fatalf("全部为 0 分应返回 0, 得到 %v", got)
		}
	})

	t.Run("非评分维度不计入", func(t *testing.T) {
		recs := []model.EvaluationRecord{rec(`{"score1":50,"text1":"很好"}`)}
		if got := recordsAvgScore(recs, scoreCodes); got != 50 {
			t.Fatalf("非评分维度不应计入（期望 50）, 得到 %v", got)
		}
	})
}
