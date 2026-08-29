package jwxt

import (
	"fmt"
	"net/url"
	"time"

	"backend-go/internal/model"

	"gorm.io/gorm"
)

// QueryTeacherSchedule 查询教师课表 HTML
func (b *BaseSync) QueryTeacherSchedule(semester, skyx, teacherName string) (string, error) {
	teacherURL := b.Auth.BaseURLGL + "/jiaowu/pkgl/zkb/queryzkb_teacher.jsp"
	if _, err := b.Auth.GetText(teacherURL, 30*time.Second); err != nil {
		return "", fmt.Errorf("打开课表页面失败: %w", err)
	}

	form := url.Values{
		"lb":    {"queryzkb_teacher.jsp"},
		"xnxqh": {semester},
		"skyx":  {skyx},
		"kc":    {teacherName},
		"kkyx":  {""},
		"jszc":  {""},
		"zc1":   {""},
		"zc2":   {""},
		"jc1":   {""},
		"jc2":   {""},
	}
	queryURL := b.Auth.BaseURLGL + "/zcbqueryAction.do?method=goQueryZKbByTeacher"
	body, err := b.Auth.PostFormText(queryURL, form, teacherURL, 60*time.Second)
	if err != nil {
		return "", err
	}
	return body, nil
}

// CurrentSemesterCode 按日期推算当前学年学期
func CurrentSemesterCode() string {
	now := time.Now()
	y, m := now.Year(), int(now.Month())
	switch {
	case m >= 8:
		return fmt.Sprintf("%d-%d-1", y, y+1)
	case m >= 2:
		return fmt.Sprintf("%d-%d-2", y-1, y)
	default:
		return fmt.Sprintf("%d-%d-1", y-1, y)
	}
}

// SyncOptions 课表同步选项
type SyncOptions struct {
	Semester    string // 学期（空=当前）
	Skyx        string // 上课院系代码
	TeacherName string // 教师姓名
	CollegeID   int    // 院系 ID（重名教师匹配），0 表示未指定
}

// SyncCourses 同步课程表（指定教师 > 指定院系 > 按院系分批全量）
func (b *BaseSync) SyncCourses(db *gorm.DB, opt SyncOptions) (map[string]interface{}, error) {
	semester := opt.Semester
	if semester == "" {
		semester = CurrentSemesterCode()
	}

	if opt.TeacherName != "" {
		htmlContent, err := b.QueryTeacherSchedule(semester, opt.Skyx, opt.TeacherName)
		if err != nil {
			return nil, err
		}
		schedules, _, err := ParseScheduleHTML(htmlContent)
		if err != nil {
			return nil, err
		}
		stats, err := SaveSchedules(db, schedules, semester, time.Now(), opt.CollegeID)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"success": true, "semester": semester, "stats": stats}, nil
	}

	if opt.Skyx != "" {
		var college model.College
		if err := db.Where("code = ?", opt.Skyx).First(&college).Error; err != nil {
			return nil, fmt.Errorf("未找到院系代码 %s 对应的学院", opt.Skyx)
		}
		htmlContent, err := b.QueryTeacherSchedule(semester, opt.Skyx, "")
		if err != nil {
			return nil, fmt.Errorf("查询院系 %s 课程表失败: %w", opt.Skyx, err)
		}
		schedules, _, err := ParseScheduleHTML(htmlContent)
		if err != nil {
			return nil, err
		}
		stats, err := SaveSchedules(db, schedules, semester, time.Now(), college.ID)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"success": true, "semester": semester, "stats": stats}, nil
	}

	// 按院系分批全量
	var colleges []model.College
	db.Where("status = 1").Find(&colleges)
	if len(colleges) == 0 {
		htmlContent, err := b.QueryTeacherSchedule(semester, "", "")
		if err != nil {
			return nil, err
		}
		schedules, _, err := ParseScheduleHTML(htmlContent)
		if err != nil {
			return nil, err
		}
		stats, err := SaveSchedules(db, schedules, semester, time.Now(), 0)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"success": true, "semester": semester, "stats": stats}, nil
	}

	totals := map[string]int{
		"total_teachers": 0, "total_courses": 0,
		"new_teachers": 0, "updated_teachers": 0, "unchanged_teachers": 0, "unmatched_teachers": 0,
		"colleges_synced": 0, "colleges_failed": 0,
	}
	crawlTime := time.Now()
	for _, college := range colleges {
		htmlContent, err := b.QueryTeacherSchedule(semester, college.Code, "")
		if err != nil {
			totals["colleges_failed"]++
			continue
		}
		schedules, stats, err := ParseScheduleHTML(htmlContent)
		if err != nil {
			totals["colleges_failed"]++
			continue
		}
		if len(schedules) > 0 {
			saveStats, err := SaveSchedules(db, schedules, semester, crawlTime, college.ID)
			if err == nil {
				totals["new_teachers"] += int(saveStats["new_teachers"].(float64))
				totals["updated_teachers"] += int(saveStats["updated_teachers"].(float64))
				totals["unchanged_teachers"] += int(saveStats["unchanged_teachers"].(float64))
				totals["unmatched_teachers"] += int(saveStats["unmatched_teachers"].(float64))
			}
		}
		totals["total_teachers"] += stats.TotalTeachers
		totals["total_courses"] += stats.TotalCourses
		totals["colleges_synced"]++
	}

	return map[string]interface{}{
		"success":  true,
		"semester": semester,
		"stats": map[string]interface{}{
			"semester":           semester,
			"total_teachers":     totals["total_teachers"],
			"total_courses":      totals["total_courses"],
			"new_teachers":       totals["new_teachers"],
			"updated_teachers":   totals["updated_teachers"],
			"unchanged_teachers": totals["unchanged_teachers"],
			"unmatched_teachers": totals["unmatched_teachers"],
			"colleges_synced":    totals["colleges_synced"],
			"colleges_failed":    totals["colleges_failed"],
		},
	}, nil
}
