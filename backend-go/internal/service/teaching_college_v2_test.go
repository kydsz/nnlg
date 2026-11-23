package service

import (
	"testing"

	"backend-go/internal/model"
)

// 授课学院核心口径：教师归属、任务学院解析、可选学院列表。
func TestTeachingCollegeScopeAndResolve(t *testing.T) {
	db := evaluationTestDB(t)

	xd := model.College{Code: "XD", Name: "信息与电气工程学院", Status: 1}
	tm := model.College{Code: "TM", Name: "土木与工程学院", Status: 1}
	if err := db.Create(&xd).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&tm).Error; err != nil {
		t.Fatal(err)
	}

	teacher := model.User{UserNo: "T900", Username: "陈守学", Role: model.RoleTeacher, CollegeID: &tm.ID, Status: 1}
	if err := db.Create(&teacher).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.TeacherTeachingCollege{
		UserID: teacher.ID, CollegeID: xd.ID, Source: "manual",
	}).Error; err != nil {
		t.Fatal(err)
	}

	xdAdmin := &model.User{Model: model.Model{ID: 9001}, Role: model.RoleCollegeAdmin, CollegeID: &xd.ID}
	tmAdmin := &model.User{Model: model.Model{ID: 9002}, Role: model.RoleCollegeAdmin, CollegeID: &tm.ID}
	wlAdmin := &model.User{Model: model.Model{ID: 9003}, Role: model.RoleCollegeAdmin, CollegeID: intPtr(88)}
	sysAdmin := &model.User{Model: model.Model{ID: 9004}, Role: model.RoleSystemAdmin}

	if !TeacherInScope(db, xdAdmin, &teacher) {
		t.Fatal("信电管理员应可通过授课学院操作该教师")
	}
	if !TeacherInScope(db, tmAdmin, &teacher) {
		t.Fatal("土木管理员应可通过人事学院操作该教师")
	}
	if TeacherInScope(db, wlAdmin, &teacher) {
		t.Fatal("无关学院管理员不应操作该教师")
	}
	if !TeacherInScope(db, sysAdmin, &teacher) {
		t.Fatal("系统管理员应可操作任意教师")
	}

	// 信电管理员：自动解析为授课学院信电
	tc, cc, err := ResolveTaskCollege(db, xdAdmin, &teacher, nil)
	if err != nil || tc == nil || *tc != tm.ID || cc == nil || *cc != xd.ID {
		t.Fatalf("信电管理员自动解析应得 人事=土木/授课=信电, 得到 tc=%v cc=%v err=%v", tc, cc, err)
	}
	// 土木管理员：自动解析为人事学院土木
	_, cc, err = ResolveTaskCollege(db, tmAdmin, &teacher, nil)
	if err != nil || cc == nil || *cc != tm.ID {
		t.Fatalf("土木管理员自动解析应得授课=土木, 得到 %v err=%v", cc, err)
	}
	// 显式指定：信电管理员可选信电，不可选土木
	if _, cc, err = ResolveTaskCollege(db, xdAdmin, &teacher, &xd.ID); err != nil || cc == nil || *cc != xd.ID {
		t.Fatalf("显式信电应成功, 得到 %v err=%v", cc, err)
	}
	if _, _, err = ResolveTaskCollege(db, xdAdmin, &teacher, &tm.ID); err == nil {
		t.Fatal("信电管理员显式指定土木应被拒绝")
	}
	// 系统管理员不指定时优先教师人事学院
	_, cc, err = ResolveTaskCollege(db, sysAdmin, &teacher, nil)
	if err != nil || cc == nil || *cc != tm.ID {
		t.Fatalf("系统管理员默认应取人事学院, 得到 %v err=%v", cc, err)
	}

	selectable := TeacherSelectableColleges(db, &teacher)
	if len(selectable) != 2 {
		t.Fatalf("可选学院应为人事+授课 2 个, 得到 %+v", selectable)
	}
}

// 授课学院维护：系统管理员任意学院；学院管理员仅本学院；目标可跨人事学院。
func TestTeachingCollegeAddRemoveScope(t *testing.T) {
	db := evaluationTestDB(t)

	xd := model.College{Code: "XD", Name: "信息与电气工程学院", Status: 1}
	tm := model.College{Code: "TM", Name: "土木与工程学院", Status: 1}
	wl := model.College{Code: "WL", Name: "文理学院", Status: 1}
	for _, c := range []*model.College{&xd, &tm, &wl} {
		if err := db.Create(c).Error; err != nil {
			t.Fatal(err)
		}
	}
	teacher := model.User{UserNo: "T901", Username: "外聘教师", Role: model.RoleTeacher, CollegeID: &tm.ID, Status: 1}
	if err := db.Create(&teacher).Error; err != nil {
		t.Fatal(err)
	}
	nonTeacher := model.User{UserNo: "A901", Username: "管理员", Role: model.RoleCollegeAdmin, CollegeID: &tm.ID, Status: 1}
	if err := db.Create(&nonTeacher).Error; err != nil {
		t.Fatal(err)
	}

	xdAdmin := &model.User{Model: model.Model{ID: 9101}, Role: model.RoleCollegeAdmin, CollegeID: &xd.ID}
	wlAdmin := &model.User{Model: model.Model{ID: 9102}, Role: model.RoleCollegeAdmin, CollegeID: &wl.ID}
	sysAdmin := &model.User{Model: model.Model{ID: 9103}, Role: model.RoleSystemAdmin}
	svc := NewUser("")

	if err := svc.AddTeachingCollege(db, xdAdmin, teacher.ID, xd.ID); err != nil {
		t.Fatalf("信电管理员应为土木教师添加信电授课关系: %v", err)
	}
	// 幂等
	if err := svc.AddTeachingCollege(db, xdAdmin, teacher.ID, xd.ID); err != nil {
		t.Fatalf("重复添加应幂等成功: %v", err)
	}
	var cnt int64
	db.Model(&model.TeacherTeachingCollege{}).Where("user_id = ? AND college_id = ?", teacher.ID, xd.ID).Count(&cnt)
	if cnt != 1 {
		t.Fatalf("授课关系应只有 1 条, 得到 %d", cnt)
	}

	if err := svc.AddTeachingCollege(db, wlAdmin, teacher.ID, xd.ID); err == nil {
		t.Fatal("文理学院管理员不应能维护信电的授课关系")
	}
	if err := svc.AddTeachingCollege(db, xdAdmin, nonTeacher.ID, xd.ID); err == nil {
		t.Fatal("非教师角色不应有授课学院")
	}
	if err := svc.AddTeachingCollege(db, sysAdmin, teacher.ID, wl.ID); err != nil {
		t.Fatalf("系统管理员应可维护任意学院: %v", err)
	}
	if err := svc.RemoveTeachingCollege(db, xdAdmin, teacher.ID, xd.ID); err != nil {
		t.Fatalf("信电管理员应可移除本学院授课关系: %v", err)
	}
	if err := svc.RemoveTeachingCollege(db, xdAdmin, teacher.ID, wl.ID); err == nil {
		t.Fatal("信电管理员不应能移除文理学院授课关系")
	}
}
