package service

import (
	"errors"

	"backend-go/internal/model"

	"gorm.io/gorm"
)

// SaveDraftParams 保存评教草稿参数
type SaveDraftParams struct {
	TaskID          int                    `json:"task_id"`
	DimensionValues map[string]interface{} `json:"dimension_values"`
	IsAnonymous     bool                   `json:"is_anonymous"`
}

// GetMyDraft 查询当前用户在指定任务下的草稿；无草稿返回 (nil, nil)
func (s *Evaluation) GetMyDraft(db *gorm.DB, viewer *model.User, taskID int) (*model.EvaluationDraft, error) {
	var draft model.EvaluationDraft
	err := db.Where("task_id = ? AND evaluator_id = ?", taskID, viewer.ID).First(&draft).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &draft, nil
}

// SaveDraft upsert 保存草稿（同一任务同一评教人仅一条）。
// 草稿允许维度不完整，dimension_values 可为空。
func (s *Evaluation) SaveDraft(db *gorm.DB, viewer *model.User, p SaveDraftParams) (*model.EvaluationDraft, error) {
	var task model.EvaluationTask
	if err := db.Where("id = ? AND is_deleted = 0", p.TaskID).First(&task).Error; err != nil {
		return nil, errors.New("任务不存在")
	}
	if task.Status == model.TaskStatusCancelled {
		return nil, errors.New("任务已取消，不能暂存")
	}

	if p.DimensionValues == nil {
		p.DimensionValues = map[string]interface{}{}
	}
	raw, err := marshalJSON(p.DimensionValues)
	if err != nil {
		return nil, err
	}

	var draft model.EvaluationDraft
	err = db.Where("task_id = ? AND evaluator_id = ?", task.ID, viewer.ID).First(&draft).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		draft = model.EvaluationDraft{
			TaskID:          task.ID,
			EvaluatorID:     viewer.ID,
			EvaluatorName:   viewer.Username,
			DimensionValues: raw,
			IsAnonymous:     p.IsAnonymous,
		}
		if err := db.Create(&draft).Error; err != nil {
			return nil, err
		}
	case err != nil:
		return nil, err
	default:
		draft.DimensionValues = raw
		draft.IsAnonymous = p.IsAnonymous
		if err := db.Save(&draft).Error; err != nil {
			return nil, err
		}
	}
	return &draft, nil
}

// DeleteMyDraft 删除当前用户在指定任务下的草稿（硬删除，无草稿也算成功）
func (s *Evaluation) DeleteMyDraft(db *gorm.DB, viewer *model.User, taskID int) error {
	return db.Where("task_id = ? AND evaluator_id = ?", taskID, viewer.ID).
		Delete(&model.EvaluationDraft{}).Error
}