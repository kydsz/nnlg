package jwxt

import (
	"fmt"
	"log"
	"strings"
	"time"

	"backend-go/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SyncTeachingColleges 从教务按「上课院系」课表识别该学院的授课教师，
// 只写 teacher_teaching_college 关系，不改写课表。
//
// 数据来源：queryzkb_teacher.jsp 按 skyx（上课院系代码）整页抓取，页面按教师分组，
// 因此能识别「人事学院在其他学院、但为本次学院授课」的外聘教师。
// 匹配策略：姓名唯一直接采用；同名时优先人事学院=目标学院，其次已有该学院授课关系，
// 仍无法唯一确定则跳过并计入 ambiguous，交由管理员手工补录。
func (b *BaseSync) SyncTeachingColleges(db *gorm.DB, semester string, college model.College) (map[string]interface{}, error) {
	if college.Code == "" {
		return nil, fmt.Errorf("学院 %s 缺少教务院系编码", college.Name)
	}
	if semester == "" {
		semester = CurrentSemesterCode()
	}
	htmlContent, err := b.QueryTeacherSchedule(semester, college.Code, "")
	if err != nil {
		return nil, fmt.Errorf("查询院系 %s 课表失败: %w", college.Name, err)
	}
	schedules, _, err := ParseScheduleHTML(htmlContent)
	if err != nil {
		return nil, fmt.Errorf("解析院系 %s 课表失败: %w", college.Name, err)
	}

	names := make([]string, 0, len(schedules))
	seen := map[string]bool{}
	for _, s := range schedules {
		name := strings.TrimSpace(s.TeacherName)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	if len(names) == 0 {
		return map[string]interface{}{
			"semester": semester, "college_id": college.ID, "college_name": college.Name,
			"teachers": 0, "added": 0, "existing": 0, "unmatched": 0, "ambiguous": 0,
		}, nil
	}

	var users []model.User
	db.Preload("UserRoles").
		Preload("TeachingColleges").
		Where("username IN ? AND status = 1", names).
		Find(&users)
	byName := map[string][]model.User{}
	for i := range users {
		byName[users[i].Username] = append(byName[users[i].Username], users[i])
	}

	added, existing, unmatched, ambiguous, errCount := 0, 0, 0, 0, 0
	for _, name := range names {
		user, ok, amb := pickTeachingCollegeUser(byName[name], college.ID)
		if amb {
			ambiguous++
			continue
		}
		if !ok {
			unmatched++
			continue
		}
		res := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.TeacherTeachingCollege{
			UserID: user.ID, CollegeID: college.ID, Source: "jwxt",
			JoinTime: model.LocalTimePtr(time.Now()),
		})
		if res.Error != nil {
			errCount++
			log.Printf("[jwxt] 写入授课学院失败 user=%d college=%d: %v", user.ID, college.ID, res.Error)
			continue
		}
		if res.RowsAffected > 0 {
			added++
		} else {
			existing++
		}
	}

	return map[string]interface{}{
		"semester": semester, "college_id": college.ID, "college_name": college.Name,
		"teachers": len(names), "added": added, "existing": existing,
		"unmatched": unmatched, "ambiguous": ambiguous, "errors": errCount,
	}, nil
}

// pickTeachingCollegeUser 从同名候选教师中选出唯一目标。
// 返回 (用户, 是否命中, 是否歧义)。
func pickTeachingCollegeUser(candidates []model.User, collegeID int) (model.User, bool, bool) {
	teachers := make([]model.User, 0, len(candidates))
	for _, u := range candidates {
		if u.Role == model.RoleTeacher || u.HasRole(model.RoleTeacher) {
			teachers = append(teachers, u)
		}
	}
	if len(teachers) == 0 {
		return model.User{}, false, false
	}
	if len(teachers) == 1 {
		return teachers[0], true, false
	}
	var mainMatches []model.User
	for _, u := range teachers {
		if u.CollegeID != nil && *u.CollegeID == collegeID {
			mainMatches = append(mainMatches, u)
		}
	}
	if len(mainMatches) == 1 {
		return mainMatches[0], true, false
	}
	if len(mainMatches) > 1 {
		return model.User{}, false, true
	}
	var affiliated []model.User
	for _, u := range teachers {
		for _, tc := range u.TeachingColleges {
			if tc.CollegeID == collegeID {
				affiliated = append(affiliated, u)
				break
			}
		}
	}
	if len(affiliated) == 1 {
		return affiliated[0], true, false
	}
	return model.User{}, false, true
}
