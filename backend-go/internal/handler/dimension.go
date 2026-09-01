package handler

import (
	"strconv"

	"backend-go/internal/middleware"
	"backend-go/internal/service"
	"backend-go/pkg/response"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Dimension 评教维度接口
type Dimension struct {
	db  *gorm.DB
	svc *service.Dimension
}

func NewDimension(db *gorm.DB, svc *service.Dimension) *Dimension {
	return &Dimension{db: db, svc: svc}
}

// GroupList 分组列表
func (h *Dimension) GroupList(c *gin.Context) {
	out, err := h.svc.GroupList(h.db, qInt16(c, "status"))
	if err != nil {
		serverErr(c, "查询失败")
		return
	}
	_, size := pageOfDefault(c, 100)
	response.OK(c, pageData(c, out, int64(len(out)), size))
}

// GroupCreate 新建分组
func (h *Dimension) GroupCreate(c *gin.Context) {
	var p service.GroupParams
	if err := c.ShouldBindJSON(&p); err != nil {
		badReq(c, "请求参数错误")
		return
	}
	g, err := h.svc.GroupCreate(h.db, p)
	if err != nil {
		badReq(c, err.Error())
		return
	}
	h.log(c, "create", "dimension_group", &g.ID)
	// 响应字段对齐旧端（无 code）
	invalidateDimensionCache(c)
	response.OKMsg(c, "创建成功", gin.H{
		"id": g.ID, "name": g.Name, "sort_order": g.SortOrder, "status": g.Status,
		"create_time": g.CreateTime,
	})
}

// GroupSort 批量排序
func (h *Dimension) GroupSort(c *gin.Context) {
	var items []service.GroupSortItem
	if err := c.ShouldBindJSON(&items); err != nil {
		badReq(c, "请求参数错误")
		return
	}
	if err := h.svc.GroupSort(h.db, items); err != nil {
		serverErr(c, "排序更新失败")
		return
	}
	invalidateDimensionCache(c)
	response.OKMsg(c, "排序更新成功", nil)
}

// GroupUpdate 更新分组
func (h *Dimension) GroupUpdate(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var p service.GroupParams
	if err := c.ShouldBindJSON(&p); err != nil {
		badReq(c, "请求参数错误")
		return
	}
	g, err := h.svc.GroupUpdate(h.db, id, p)
	if err != nil {
		badReq(c, err.Error())
		return
	}
	h.log(c, "update", "dimension_group", &g.ID)
	// 响应字段对齐旧端（无 code，含 update_time）
	invalidateDimensionCache(c)
	response.OKMsg(c, "更新成功", gin.H{
		"id": g.ID, "name": g.Name, "sort_order": g.SortOrder, "status": g.Status,
		"update_time": g.UpdateTime,
	})
}

// GroupDelete 删除分组
func (h *Dimension) GroupDelete(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := h.svc.GroupDelete(h.db, id); err != nil {
		badReq(c, err.Error())
		return
	}
	h.log(c, "delete", "dimension_group", &id)
	invalidateDimensionCache(c)
	response.OKMsg(c, "删除成功", nil)
}

// DimList 维度列表
func (h *Dimension) DimList(c *gin.Context) {
	page, size := pageOfDefault(c, 100)
	list, total, err := h.svc.DimList(h.db, service.DimParams{
		Keyword:   c.Query("keyword"),
		GroupID:   c.Query("group_id"),
		FieldType: c.Query("field_type"),
		Status:    qInt16(c, "status"),
	}, page, size)
	if err != nil {
		serverErr(c, "查询失败")
		return
	}
	response.OK(c, pageData(c, list, total, size))
}

// DimActive 启用维度列表（评教表单用）
func (h *Dimension) DimActive(c *gin.Context) {
	out, err := h.svc.DimActive(h.db)
	if err != nil {
		serverErr(c, "查询失败")
		return
	}
	response.OK(c, out)
}

// DimSort 批量更新维度排序
func (h *Dimension) DimSort(c *gin.Context) {
	var items []service.GroupSortItem
	if err := c.ShouldBindJSON(&items); err != nil {
		badReq(c, "请求参数错误")
		return
	}
	if err := h.svc.DimSort(h.db, items); err != nil {
		serverErr(c, "排序更新失败")
		return
	}
	invalidateDimensionCache(c)
	response.OKMsg(c, "排序更新成功", nil)
}

// DimGet 维度详情
func (h *Dimension) DimGet(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		// 对齐旧端：path 参数 int 解析失败返回 422
		invalidParam(c, "path", "dim_id", c.Param("id"),
			"Input should be a valid integer, unable to parse string as an integer")
		return
	}
	item, err := h.svc.DimGet(h.db, id)
	if err != nil {
		response.Fail(c, 404, err.Error())
		return
	}
	response.OK(c, item)
}

// DimCreate 新建维度
func (h *Dimension) DimCreate(c *gin.Context) {
	var p service.DimensionParams
	if err := c.ShouldBindJSON(&p); err != nil {
		badReq(c, "请求参数错误")
		return
	}
	d, err := h.svc.DimCreate(h.db, p)
	if err != nil {
		badReq(c, err.Error())
		return
	}
	h.log(c, "create", "evaluation_dimension", &d.ID)
	invalidateDimensionCache(c)
	response.OKMsg(c, "创建成功", service.DimItem(d))
}

// DimUpdate 更新维度
func (h *Dimension) DimUpdate(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var p service.DimensionParams
	if err := c.ShouldBindJSON(&p); err != nil {
		badReq(c, "请求参数错误")
		return
	}
	d, err := h.svc.DimUpdate(h.db, id, p)
	if err != nil {
		badReq(c, err.Error())
		return
	}
	h.log(c, "update", "evaluation_dimension", &d.ID)
	resp := service.DimItem(d)
	delete(resp, "create_time")
	resp["update_time"] = d.UpdateTime
	invalidateDimensionCache(c)
	response.OKMsg(c, "更新成功", resp)
}

// DimDelete 删除维度
func (h *Dimension) DimDelete(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := h.svc.DimDelete(h.db, id); err != nil {
		serverErr(c, "删除失败")
		return
	}
	h.log(c, "delete", "evaluation_dimension", &id)
	invalidateDimensionCache(c)
	response.OKMsg(c, "删除成功", nil)
}

func (h *Dimension) log(c *gin.Context, opType, module string, id *int) {
	u := middleware.CurrentUser(c)
	service.LogRecord(h.db, &u.ID, u.Username, opType, module, id, module, nil)
}
