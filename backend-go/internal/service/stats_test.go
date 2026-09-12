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
