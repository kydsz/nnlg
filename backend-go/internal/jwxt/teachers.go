package jwxt

import (
	"fmt"
	"log"
	"strings"
	"time"

	"backend-go/internal/model"
	"backend-go/pkg/pwd"

	"gorm.io/gorm"
)

// TeacherRecord 教师行记录
type TeacherRecord struct {
	UserNo     string
	Username   string
	Department string
	Status     string
}

// SyncTeachers 同步教师信息
func (b *BaseSync) SyncTeachers(db *gorm.DB, defaultPassword string) (map[string]interface{}, error) {
	listURL := b.Auth.BaseURLGL + "/ggxx/jzg/listJzgxx.jsp?jsd=XXZX-GYXX-LSXX&sttype=2"
	params, err := b.GetPrintParams(listURL, "教职工基础信息")
	if err != nil {
		return nil, err
	}
	data, err := b.DownloadExcel(params, listURL)
	if err != nil {
		return nil, err
	}
	rows, err := readSheetRows(data, 2)
	if err != nil {
		return nil, err
	}

	// 去重解析
	seen := map[string]bool{}
	var teachers []TeacherRecord
	for _, row := range rows {
		userNo := row["教工号"]
		if userNo == "" || seen[userNo] {
			continue
		}
		seen[userNo] = true
		teachers = append(teachers, TeacherRecord{
			UserNo:     userNo,
			Username:   row["姓名"],
			Department: row["教师所属单位"],
			Status:     row["当前状态"],
		})
	}
	if len(teachers) == 0 {
		return map[string]interface{}{"success": true, "total": 0, "new": 0, "updated": 0, "errors": 0}, nil
	}

	defaultHash, err := pwd.Hash(defaultPassword)
	if err != nil {
		return nil, err
	}

	// 学院名称映射
	collegeMap := map[string]int{}
	var colleges []model.College
	db.Where("status = 1").Find(&colleges)
	for _, c := range colleges {
		collegeMap[c.Name] = c.ID
	}

	newCount, updateCount, errCount := 0, 0, 0
	for _, t := range teachers {
		func() {
			defer func() {
				if r := recover(); r != nil {
					errCount++
					log.Printf("[jwxt] 处理教师 %s 异常: %v", t.UserNo, r)
				}
			}()

			var u model.User
			exists := db.Where("user_no = ?", t.UserNo).First(&u).Error == nil
			now := time.Now()
			status := int16(0)
			if t.Status == "在职" {
				status = 1
			}

			if exists {
				updates := map[string]interface{}{
					"username": t.Username, "role": "teacher", "status": status, "update_time": now,
				}
				if err := db.Model(&model.User{}).Where("id = ?", u.ID).Updates(updates).Error; err != nil {
					errCount++
					return
				}
				ensureTeacherRole(db, u.ID)
				if t.Department != "" {
					collegeID, deptErr := resolveCollegeID(db, collegeMap, t.Department)
					if deptErr == nil {
						db.Model(&model.User{}).Where("id = ?", u.ID).Update("college_id", collegeID)
						ensureUserCollege(db, u.ID, collegeID)
					}
				}
				updateCount++
			} else {
				nu := model.User{
					UserNo: t.UserNo, Username: t.Username, Password: defaultHash,
					Role: "teacher", Status: status, MustChangePassword: true,
				}
				if err := db.Create(&nu).Error; err != nil {
					errCount++
					return
				}
				ensureTeacherRole(db, nu.ID)
				if t.Department != "" {
					collegeID, deptErr := resolveCollegeID(db, collegeMap, t.Department)
					if deptErr == nil {
						db.Model(&model.User{}).Where("id = ?", nu.ID).Update("college_id", collegeID)
						ensureUserCollege(db, nu.ID, collegeID)
					}
				}
				newCount++
			}
		}()
	}

	log.Printf("[jwxt] 教师导入完成 | 新增: %d | 更新: %d | 错误: %d", newCount, updateCount, errCount)
	return map[string]interface{}{
		"success": true, "total": len(teachers),
		"new": newCount, "updated": updateCount, "errors": errCount,
	}, nil
}

// ensureTeacherRole 确保用户有 teacher 角色
func ensureTeacherRole(db *gorm.DB, userID int) {
	var cnt int64
	db.Model(&model.UserRole{}).Where("user_id = ? AND role = ?", userID, "teacher").Count(&cnt)
	if cnt == 0 {
		now := time.Now()
		db.Create(&model.UserRole{UserID: userID, Role: "teacher", AssignTime: model.LocalTimePtr(now)})
	}
}

// ensureUserCollege 确保用户-学院关联
func ensureUserCollege(db *gorm.DB, userID, collegeID int) {
	var cnt int64
	db.Model(&model.UserCollege{}).Where("user_id = ? AND college_id = ?", userID, collegeID).Count(&cnt)
	if cnt == 0 {
		now := time.Now()
		db.Create(&model.UserCollege{UserID: userID, CollegeID: collegeID, JoinTime: model.LocalTimePtr(now)})
	}
}

// resolveCollegeID 按部门名称匹配学院（精确 -> 模糊 -> 自动创建 ADMIN_xxx）
func resolveCollegeID(db *gorm.DB, collegeMap map[string]int, department string) (int, error) {
	if department == "" {
		return 0, fmt.Errorf("部门为空")
	}
	if id, ok := collegeMap[department]; ok {
		return id, nil
	}
	// 模糊匹配
	for name, id := range collegeMap {
		if strings.Contains(name, department) || strings.Contains(department, name) {
			return id, nil
		}
	}
	// 数据库再查一次
	var college model.College
	if err := db.Where("name = ?", department).First(&college).Error; err == nil {
		collegeMap[department] = college.ID
		return college.ID, nil
	}
	// 自动创建 ADMIN_001 递增编码
	var maxCode string
	db.Model(&model.College{}).Where("code LIKE ?", "ADMIN_%").Select("code").Order("code DESC").Limit(1).Scan(&maxCode)
	next := 1
	if maxCode != "" {
		var n int
		if _, err := fmt.Sscanf(maxCode, "ADMIN_%d", &n); err == nil {
			next = n + 1
		}
	}
	college = model.College{Code: fmt.Sprintf("ADMIN_%03d", next), Name: department, Status: 1}
	if err := db.Create(&college).Error; err != nil {
		return 0, err
	}
	collegeMap[department] = college.ID
	log.Printf("[jwxt] 自动创建单位: %s", department)
	return college.ID, nil
}
