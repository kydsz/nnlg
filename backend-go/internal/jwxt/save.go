package jwxt

import (
	"fmt"
	"strings"
	"time"

	"backend-go/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SaveSchedules 保存解析后的课表（版本管理 + 变更比较），对齐旧版 save_schedules_from_parser。
// 全流程单事务：is_current 切换 / 新版本创建 / 版本历史写入要么全部成功、要么全部回滚，
// 失败时返回真实错误（调用方据此将任务标记为失败），不留孤儿明细或"无当前课表"状态。
func SaveSchedules(db *gorm.DB, schedules []TeacherSchedule, semester string, crawlTime time.Time, collegeID int) (map[string]interface{}, error) {
	var err error
	var stats map[string]interface{}
	txErr := db.Transaction(func(tx *gorm.DB) error {
		stats, err = saveSchedulesTx(tx, schedules, semester, crawlTime, collegeID)
		return err
	})
	if txErr != nil {
		return nil, fmt.Errorf("保存课表失败: %w", txErr)
	}
	return stats, nil
}

// saveSchedulesTx 事务内执行保存逻辑；TxErr 由外层 Transaction 统一回滚。
func saveSchedulesTx(tx *gorm.DB, schedules []TeacherSchedule, semester string, crawlTime time.Time, collegeID int) (map[string]interface{}, error) {
	// 全局版本快照号：事务内锁定该学期的最近版本行（不存在则锁空区间语义由
	// course_schedule_version 唯一索引 uk_sem_version 兜底），并发同步不会取到相同号。
	globalVersion, err := nextGlobalVersion(tx, semester)
	if err != nil {
		return nil, err
	}

	stats := map[string]interface{}{
		"semester": semester, "version": globalVersion,
		"total_teachers": len(schedules), "total_courses": 0,
		"new_teachers": 0, "updated_teachers": 0, "unchanged_teachers": 0,
		"unmatched_teachers": 0, "skipped_empty_teachers": 0,
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
				if err := tx.Where("teacher_id = ? AND semester = ? AND is_current = 1", resolved, semester).
					Order("version DESC").First(&cur).Error; err == nil {
					if err := tx.Model(&cur).Update("crawl_time", crawlTime).Error; err != nil {
						return nil, err
					}
					inc("unchanged_teachers", 1)
				} else {
					version, verr := nextTeacherVersion(tx, resolved, semester)
					if verr != nil {
						return nil, verr
					}
					if err := createScheduleWithDetails(tx, &ts, semester, &resolved, version, crawlTime); err != nil {
						return nil, err
					}
					inc("new_teachers", 1)
					changeDetails = append(changeDetails, fmt.Sprintf("新增教师(无课): %s", ts.TeacherName))
				}
			}
			continue
		}

		// 教师匹配：自带 ID > 姓名（+学院）
		resolvedID := ts.TeacherID
		if resolvedID == 0 {
			q := tx.Model(&model.User{}).Where("username = ?", ts.TeacherName)
			if collegeID != 0 {
				var ids []int
				if err := q.Where("college_id = ?", collegeID).Limit(1).Pluck("id", &ids).Error; err != nil {
					return nil, err
				}
				if len(ids) > 0 {
					resolvedID = ids[0]
				}
			}
			if resolvedID == 0 {
				var ids []int
				if err := q.Session(&gorm.Session{}).Limit(1).Pluck("id", &ids).Error; err != nil {
					return nil, err
				}
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
			q := tx.Where("semester = ? AND is_current = 1", semester)
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
		added, removed, modified, cerr := compareSchedule(tx, cur, ts)
		if cerr != nil {
			return nil, cerr
		}
		hasChanges := cur == nil || added > 0 || removed > 0 || modified > 0

		if hasChanges {
			if cur != nil {
				if err := tx.Model(cur).Update("is_current", false).Error; err != nil {
					return nil, err
				}
			}
			version, verr := nextTeacherVersion(tx, resolvedID, semester)
			if verr != nil {
				return nil, verr
			}
			if err := createScheduleWithDetails(tx, &ts, semester, &resolvedID, version, crawlTime); err != nil {
				return nil, err
			}
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
				if err := tx.Model(cur).Update("crawl_time", crawlTime).Error; err != nil {
					return nil, err
				}
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
	if err := tx.Create(&model.CourseScheduleVersion{
		Semester: semester, Version: globalVersion, CrawlTime: model.LocalTimePtr(crawlTime),
		TeacherCount: stats["total_teachers"].(int), CourseCount: stats["total_courses"].(int),
		ChangeSummary: &summary,
	}).Error; err != nil {
		return nil, err
	}

	return stats, nil
}

// nextGlobalVersion 取学期内下一个全局版本号。
// 在事务内先锁定最近版本历史行，查询与插入受同一事务与唯一索引保护。
func nextGlobalVersion(tx *gorm.DB, semester string) (int, error) {
	var maxGlobal int64
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Model(&model.CourseScheduleVersion{}).
		Where("semester = ?", semester).
		Select("COALESCE(MAX(version), 0)").Scan(&maxGlobal).Error; err != nil {
		return 0, err
	}
	return int(maxGlobal) + 1, nil
}

// nextTeacherVersion 教师课表独立递增版本号（查询与创建同事务）
func nextTeacherVersion(tx *gorm.DB, teacherID int, semester string) (int, error) {
	var max int64
	if err := tx.Model(&model.CourseSchedule{}).Where("teacher_id = ? AND semester = ?", teacherID, semester).
		Select("COALESCE(MAX(version), 0)").Scan(&max).Error; err != nil {
		return 0, err
	}
	return int(max) + 1, nil
}

// createScheduleWithDetails 创建课表主记录与明细，任一步失败整体回滚
func createScheduleWithDetails(tx *gorm.DB, ts *TeacherSchedule, semester string, teacherID *int, version int, crawlTime time.Time) error {
	sc := model.CourseSchedule{
		TeacherID: teacherID, TeacherName: ts.TeacherName,
		Semester: semester, Version: version, IsCurrent: true, CrawlTime: model.LocalTimePtr(crawlTime),
	}
	if err := tx.Create(&sc).Error; err != nil {
		return err
	}
	for _, c := range ts.Courses {
		wd := int8(c.WeekDay)
		if err := tx.Create(&model.CourseScheduleDetail{
			ScheduleID: sc.ID, CourseName: c.CourseName, ClassInfo: c.ClassInfo,
			WeekPattern: c.WeekPattern, WeekDay: &wd, Section: c.Section,
			Classroom: c.Classroom, RawData: &c.RawData,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

func compareSchedule(db *gorm.DB, cur *model.CourseSchedule, ts TeacherSchedule) (added, removed, modified int, err error) {
	if cur == nil {
		return len(ts.Courses), 0, 0, nil
	}
	var oldDetails []model.CourseScheduleDetail
	if err := db.Where("schedule_id = ?", cur.ID).Find(&oldDetails).Error; err != nil {
		return 0, 0, 0, fmt.Errorf("读取课表明细失败: %w", err)
	}

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
