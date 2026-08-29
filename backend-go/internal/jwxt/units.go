package jwxt

import (
	"backend-go/internal/model"

	"gorm.io/gorm"
)

// SyncUnits 同步单位信息（学院 + 教研室）
func (b *BaseSync) SyncUnits(db *gorm.DB) (map[string]interface{}, error) {
	listURL := b.Auth.BaseURLGL + "/ggxx/yxxx_list.jsp?id=01"
	params, err := b.GetPrintParams(listURL, "单位信息")
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

	collegesAdded, roomsAdded, errCount := 0, 0, 0
	collegeMap := map[string]int{}

	// 第一遍：导入学院（编号 3 位）
	for _, row := range rows {
		code := row["编号"]
		name := row["名称"]
		if code == "" || name == "" || len(code) != 3 {
			continue
		}
		var college model.College
		if err := db.Where("code = ?", code).First(&college).Error; err != nil {
			college = model.College{Code: code, Name: name, Status: 1}
			if err := db.Create(&college).Error; err != nil {
				errCount++
				continue
			}
			collegesAdded++
		}
		collegeMap[code] = college.ID
	}

	// 第二遍：导入教研室（编号 6 位，前 3 位为学院编码）
	for _, row := range rows {
		code := row["编号"]
		name := row["名称"]
		if code == "" || name == "" || len(code) != 6 {
			continue
		}
		collegeCode := code[:3]
		collegeID, ok := collegeMap[collegeCode]
		if !ok {
			var college model.College
			if err := db.Where("code = ?", collegeCode).First(&college).Error; err == nil {
				collegeID = college.ID
				collegeMap[collegeCode] = collegeID
			}
		}
		var cnt int64
		db.Model(&model.ResearchRoom{}).Where("code = ?", code).Count(&cnt)
		if cnt == 0 {
			if err := db.Create(&model.ResearchRoom{Code: code, Name: name, CollegeID: collegeID, Status: 1}).Error; err != nil {
				errCount++
				continue
			}
			roomsAdded++
		}
	}

	return map[string]interface{}{
		"success":        true,
		"total":          len(rows),
		"colleges_added": collegesAdded,
		"rooms_added":    roomsAdded,
		"errors":         errCount,
	}, nil
}
