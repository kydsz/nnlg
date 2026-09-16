package service

import (
	"testing"

	"backend-go/internal/model"
)

// TeacherCollegeMap 是「被评教师 → 所属学院」的统一读取口径：
// 只认主学院（user.college_id），教师不存在 / 未绑定学院时该键缺省，
// 调用方按「无学院」处理（前端渲染 "-"）。
func TestTeacherCollegeMap(t *testing.T) {
	db := evaluationTestDB(t)

	college := model.College{Code: "WL", Name: "文理学院", Status: 1}
	if err := db.Create(&college).Error; err != nil {
		t.Fatalf("建学院失败: %v", err)
	}
	withCollege := model.User{
		UserNo: "T001", Username: "有学院", Role: model.RoleTeacher,
		CollegeID: &college.ID, Status: 1,
	}
	withoutCollege := model.User{UserNo: "T002", Username: "无学院", Role: model.RoleTeacher, Status: 1}
	if err := db.Create(&withCollege).Error; err != nil {
		t.Fatalf("建用户失败: %v", err)
	}
	if err := db.Create(&withoutCollege).Error; err != nil {
		t.Fatalf("建用户失败: %v", err)
	}

	got := TeacherCollegeMap(db, []int{withCollege.ID, withoutCollege.ID, 999999})
	if c, ok := got[withCollege.ID]; !ok || c.ID != college.ID || c.Name != "文理学院" {
		t.Fatalf("已绑定学院的教师应解析出 文理学院, 得到 %+v (存在=%v)", c, ok)
	}
	if c, ok := got[withoutCollege.ID]; ok {
		t.Fatalf("未绑定学院的教师不应出现在结果中, 得到 %+v", c)
	}
	if c, ok := got[999999]; ok {
		t.Fatalf("不存在的教师不应出现在结果中, 得到 %+v", c)
	}
	if n := len(TeacherCollegeMap(db, nil)); n != 0 {
		t.Fatalf("空输入应返回空 map, 得到 %d 项", n)
	}
}

// TeacherCollegeDisplay 把「无学院」统一表达为 (nil, nil)，供列表直接把
// college_id / college_name 序列化成 JSON null（前端据此渲染 "-"）。
func TestTeacherCollegeDisplay(t *testing.T) {
	colleges := map[int]model.College{
		7: {Model: model.Model{ID: 3}, Name: "文理学院"},
	}
	id, name := TeacherCollegeDisplay(colleges, 7)
	if id == nil || *id != 3 || name != "文理学院" {
		t.Fatalf("有学院时应给出 id 与名称, 得到 id=%v name=%v", id, name)
	}
	id, name = TeacherCollegeDisplay(colleges, 999)
	if id != nil || name != nil {
		t.Fatalf("无学院时应给出 (nil, nil), 得到 id=%v name=%v", id, name)
	}
}
