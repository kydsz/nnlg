package jwxt

import (
	"fmt"
	"strings"
	"time"

	"backend-go/internal/model"

	"gorm.io/gorm"
)

// SaveSchedules 保存解析后的课表（版本管理 + 变更比较），对齐旧版 save_schedules_from_parser
func SaveSchedules(db *gorm.DB, schedules []TeacherSchedule, semester string, crawlTime time.Time, collegeID int) (map[string]interface{}, error) {
	// 全局版本快照号
	var maxGlobal int64
	db.Model(&model.CourseScheduleVersion{}).Where("semester = ?", semester).
		Select("COALESCE(MAX(version), 0)").Scan(&maxGlobal)
	globalVersion := int(maxGlobal) + 1

	stats := map[string]interface{}{
		"semester":                semester,
		"version":                 globalVersion,
		"total_teachers":          len(schedules),
		"total_courses":           0,
		"new_teachers":            0,
		"updated_teachers":        0,
		"unchanged_teachers":      0,
		"unmatched_teachers":      0,
		"skipped_empty_teachers":  0,
	}
	inc := func(key string, n int) { stats[key] = stats[key].(int) + n }

	var changeDetails []string

	for _, ts := range schedules {
		// 空课表处理
		if len(ts.Courses) == 0 {
			inc("skipped_empty_teachers", 1)
			resolved := ts.TeacherID
			if resolved != 0 {
				var cur model.CourseSchedule
				err := db.Where("teacher_id = ? AND semester = ? AND is_current = 1", resolved, semester).
					Order("version DESC").First(&cur).Error
				if err == nil {
					db.Model(&cur).Update("crawl_time", crawlTime)
					inc("unchanged_teachers", 1)
				} else {
					version := nextTeacherVersionDB(db, resolved, semester)
					createScheduleWithDetails(db, &ts, semester, resolved, version, crawlTime)
					inc("new_teachers", 1)
					changeDetails = append(changeDetails, fmt.Sprintf("新增教师(无课): %s", ts.TeacherName))
				}
			}
			continue
		}

		// 教师匹配：自带 ID > 姓名（+学院）
		resolvedID := ts.TeacherID
		if resolvedID == 0 {
			q := db.Model(&model.User{}).Where("username = ?", ts.TeacherName)
			if collegeID != 0 {
				var ids []int
				q.Where("college_id = ?", collegeID).Limit(1).Pluck("id", &ids)
				if len(ids) > 0 {
					resolvedID = ids[0]
				}
			}
			if resolvedID == 0 {
				var ids []int
				q.Session(&gorm.Session{}).Limit(1).Pluck("id", &ids)
				if len(ids) > 0 {
					resolvedID = ids[0]
				}
			}
			if resolvedID == 0 {
				inc("unmatched_teachers", 1)
			}
		}

		// 当前版本课表
		var cur *model.CourseSchedule
		{
			q := db.Where("semester = ? AND is_current = 1", semester)
			if resolvedID != 0 {
				q = q.Where("teacher_id = ?", resolvedID)
			} else {
				q = q.Where("teacher_name = ?", ts.TeacherName)
			}
			var c model.CourseSchedule
			if err := q.Order("version DESC").First(&c).Error; err == nil {
				cur = &c
			}
		}

		// 变更比较
		added, removed, modified := compareSchedule(db, cur, ts)
		hasChanges := cur == nil || added > 0 || removed > 0 || modified > 0

		if hasChanges {
			if cur != nil {
				db.Model(cur).Update("is_current", false)
			}
			version := nextTeacherVersionDB(db, resolvedID, semester)
			createScheduleWithDetails(db, &ts, semester, resolvedID, version, crawlTime)
			if cur == nil {
				inc("new_teachers", 1)
				changeDetails = append(changeDetails, fmt.Sprintf("新增教师: %s", ts.TeacherName))
			} else {
				inc("updated_teachers", 1)
				changeDetails = append(changeDetails, fmt.Sprintf("更新教师: %s (+%d, -%d)", ts.TeacherName, added, removed))
			}
		} else {
			inc("unchanged_teachers", 1)
			if cur != nil {
				db.Model(cur).Update("crawl_time", crawlTime)
			}
		}
		inc("total_courses", len(ts.Courses))
	}

	// 版本历史
	summary := ""
	if len(changeDetails) > 20 {
		summary = strings.Join(changeDetails[:20], "\n") + fmt.Sprintf("\n... 等共%d条变更", len(changeDetails))
	} else {
		summary = strings.Join(changeDetails, "\n")
	}
	db.Create(&model.CourseScheduleVersion{
		Semester: semester, Version: globalVersion, CrawlTime: model.LocalTimePtr(crawlTime),
		TeacherCount: stats["total_teachers"].(int), CourseCount: stats["total_courses"].(int),
		ChangeSummary: &summary,
	})

	return stats, nil
}

// nextTeacherVersionDB 教师独立递增版本号
func nextTeacherVersionDB(db *gorm.DB, teacherID int, semester string) int {
	var max int64
	db.Model(&model.CourseSchedule{}).Where("teacher_id = ? AND semester = ?", teacherID, semester).
		Select("COALESCE(MAX(version), 0)").Scan(&max)
	return int(max) + 1
}

func createScheduleWithDetails(db *gorm.DB, ts *TeacherSchedule, semester string, teacherID, version int, crawlTime time.Time) {
	sc := model.CourseSchedule{
		TeacherID: &teacherID, TeacherName: ts.TeacherName,
		Semester: semester, Version: version, IsCurrent: true, CrawlTime: model.LocalTimePtr(crawlTime),
	}
	db.Create(&sc)
	for _, c := range ts.Courses {
		wd := int8(c.WeekDay)
		db.Create(&model.CourseScheduleDetail{
			ScheduleID: sc.ID, CourseName: c.CourseName, ClassInfo: c.ClassInfo,
			WeekPattern: c.WeekPattern, WeekDay: &wd, Section: c.Section,
			Classroom: c.Classroom, RawData: &c.RawData,
		})
	}
}

func compareSchedule(db *gorm.DB, cur *model.CourseSchedule, ts TeacherSchedule) (added, removed, modified int) {
	if cur == nil {
		return len(ts.Courses), 0, 0
	}
	var oldDetails []model.CourseScheduleDetail
	db.Where("schedule_id = ?", cur.ID).Find(&oldDetails)

	key := func(name string, day int8, section, week string) string {
		return fmt.Sprintf("%s|%d|%s|%s", name, day, section, week)
	}
	oldMap := map[string]model.CourseScheduleDetail{}
	for _, d := range oldDetails {
		day := int8(0)
		if d.WeekDay != nil {
			day = *d.WeekDay
		}
		oldMap[key(d.CourseName, day, d.Section, d.WeekPattern)] = d
	}
	newMap := map[string]CourseInfo{}
	for _, c := range ts.Courses {
		newMap[key(c.CourseName, int8(c.WeekDay), c.Section, c.WeekPattern)] = c
	}

	for k := range newMap {
		if _, ok := oldMap[k]; !ok {
			added++
		} else {
			nc := newMap[k]
			oc := oldMap[k]
			if oc.ClassInfo != nc.ClassInfo || oc.Classroom != nc.Classroom {
				modified++
			}
		}
	}
	for k := range oldMap {
		if _, ok := newMap[k]; !ok {
			removed++
		}
	}
	return
}
